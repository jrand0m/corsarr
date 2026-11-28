package wasm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
	"github.com/jrand0m/corsarr/internal/wasm/wasmtest"
	"github.com/jrand0m/corsarr/plugins/example-downloader/fakeapi"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

func TestMain(m *testing.M) {
	code := m.Run()
	wasmtest.Cleanup()
	os.Exit(code)
}

type events struct {
	mu  sync.Mutex
	got []domain.Event
}

func (e *events) Publish(ev domain.Event) (domain.Event, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.got = append(e.got, ev)
	return ev, nil
}

type logBuf struct {
	mu sync.Mutex
	sb strings.Builder
}

func (l *logBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sb.Write(p)
}

func (l *logBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sb.String()
}

func engine(t *testing.T, opts ...func(*Options)) *Engine {
	t.Helper()
	o := Options{CacheDir: wasmtest.CacheDir(), Compiler: true,
		Resolve: func(_ context.Context, host string) ([]netip.Addr, error) {
			if strings.HasPrefix(host, "private.") {
				return []netip.Addr{netip.MustParseAddr("10.0.0.7")}, nil
			}
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		}}
	for _, fn := range opts {
		fn(&o)
	}
	e, err := NewEngine(o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close(context.Background()) })
	return e
}

func load(t *testing.T, e *Engine, module, sidecar []byte) *Module {
	t.Helper()
	m, err := e.Load(context.Background(), module, sidecar)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close(context.Background()) })
	return m
}

func exampleClient(t *testing.T, e *Engine) (*Downloader, *fakeapi.Server) {
	t.Helper()
	m := load(t, e, wasmtest.Example(t), nil)
	api := fakeapi.New("k3y")
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	p, err := m.NewPool(Binding{Key: "ex", Settings: map[string]any{"url": srv.URL, "apiKey": "k3y"}, HTTP: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	return kinds[abi.KindDownloader].Adapter(NewHandle(p)).(*Downloader), api
}

// The example module passes the same downloader suite as built-ins.
func TestExampleDownloaderConformance(t *testing.T) {
	e := engine(t)
	d, api := exampleClient(t, e)
	conformance.Downloader(t, d, plugin.AddRequest{Payload: plugin.Payload{Kind: "magnet", URL: "magnet:?xt=urn:btih:abc", Name: "Some.Movie"},
		Category: "movies", SavePath: "/downloads/movies"})
	if api.Categories()["movies"] != "/downloads/movies" || len(api.Deleted) != 1 {
		t.Errorf("client state: %v %v", api.Categories(), api.Deleted)
	}
	for _, ok := range []bool{
		is[plugin.FileLister](d), is[plugin.PriorityLeveler](d), is[plugin.GlobalPauser](d), is[plugin.SpeedLimiter](d),
		is[plugin.Rechecker](d), is[plugin.Reannouncer](d), is[plugin.Categoriser](d), is[plugin.ClientStats](d), is[plugin.TrackerLister](d),
	} {
		if !ok {
			t.Error("example module lacks an optional capability")
		}
	}
	if d.KeepsDataOnRemove() || len(d.Protocols()) != 1 {
		t.Error("flags / protocols")
	}
}

func is[T any](d any) bool { _, ok := plugin.As[T](d); return ok }

func TestExampleDetails(t *testing.T) {
	e := engine(t)
	d, _ := exampleClient(t, e)
	ctx := context.Background()
	id, err := d.Add(ctx, plugin.AddRequest{Payload: plugin.Payload{Kind: "torrent", Content: []byte("d4:info"), Name: "X"}, Category: "tv"})
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := d.List(ctx, nil)
	if err != nil || len(snaps) != 1 || snaps[0].State != domain.StateDownloading || snaps[0].LastActivityAt == nil {
		t.Fatalf("%+v %v", snaps, err)
	}
	if err := d.Pause(ctx, "nope"); !errors.Is(err, plugin.ErrNotFound) {
		t.Errorf("missing download: %v", err)
	}
	if err := d.SetSpeedLimits(ctx, plugin.SpeedLimits{DownKiB: 100}); err != nil {
		t.Fatal(err)
	}
	if st, err := d.Stats(ctx); err != nil || st.Limits.DownKiB != 100 || st.FreeSpace != 1<<30 {
		t.Errorf("%+v %v", st, err)
	}
	if fs, err := d.Files(ctx, id); err != nil || len(fs) != 1 {
		t.Errorf("%v %v", fs, err)
	}
	if tr, err := d.Trackers(ctx, id); err != nil || tr[0].Status != "working" {
		t.Errorf("%v %v", tr, err)
	}
	if on, err := d.AltSpeed(ctx); err != nil || on {
		t.Errorf("%v %v", on, err)
	}
}

func TestExampleAuthAndAction(t *testing.T) {
	e := engine(t)
	m := load(t, e, wasmtest.Example(t), nil)
	srv := httptest.NewServer(fakeapi.New("right").Handler())
	defer srv.Close()
	b := Binding{Key: "a", Settings: map[string]any{"url": srv.URL, "apiKey": "wrong"}, HTTP: http.DefaultClient}
	p, _ := m.NewPool(b)
	defer p.Drain(context.Background())
	if err := NewHandle(p).Test(context.Background()); plugin.ClassOf(err) != plugin.ClassAuth {
		t.Errorf("wrong key: %v", err)
	}
	res, err := m.Action(context.Background(), b, "test")
	if err != nil || res.Outcome != abi.OutcomeFail || !strings.Contains(res.Text, "401") {
		t.Errorf("%+v %v", res, err)
	}
	b.Settings["apiKey"] = "right"
	if res, err := m.Action(context.Background(), b, "test"); err != nil || res.Outcome != abi.OutcomeOK {
		t.Errorf("%+v %v", res, err)
	}
	if _, err := m.Action(context.Background(), b, "nope"); !errors.Is(err, plugin.ErrUnsupported) {
		t.Errorf("undeclared action: %v", err)
	}
	b.Settings["url"] = "ftp://x"
	if _, err := m.Action(context.Background(), b, "test"); err == nil || !strings.Contains(err.Error(), "url:") {
		t.Errorf("invalid values: %v", err)
	}
}

func TestPrivateAddressNeedsSetting(t *testing.T) {
	e := engine(t)
	m := load(t, e, wasmtest.Example(t), nil)
	// the configured address is on loopback: allowed because the setting names it
	srv := httptest.NewServer(fakeapi.New("").Handler())
	defer srv.Close()
	p, _ := m.NewPool(Binding{Key: "a", Settings: map[string]any{"url": srv.URL}, HTTP: http.DefaultClient})
	defer p.Drain(context.Background())
	if err := NewHandle(p).Test(context.Background()); err != nil {
		t.Fatalf("configured LAN host refused: %v", err)
	}
}
