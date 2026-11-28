package plugintest_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin"
	"github.com/jrand0m/corsarr/sdk/go/plugintest"
)

func TestCallDecodesResultsAndErrors(t *testing.T) {
	h := plugintest.New(plugintest.Host{})
	plugin.Op("t.echo", func(r map[string]string) (map[string]string, error) { return r, nil })
	plugin.Handle("t.nothing", func([]byte) (any, error) { return nil, nil })
	plugin.Op("t.fail", func(plugin.Empty) (plugin.Empty, error) {
		return plugin.Empty{}, plugin.Errorf(abi.ClassAuth, "no")
	})

	var echo map[string]string
	if err := h.Call("t.echo", map[string]string{"a": "b"}, &echo); err != nil || echo["a"] != "b" {
		t.Fatalf("echo: %v %v", echo, err)
	}
	if err := h.Call("t.echo", map[string]string{"a": "b"}, nil); err != nil {
		t.Errorf("discarded result: %v", err)
	}
	keep := map[string]string{"kept": "yes"}
	if err := h.Call("t.nothing", plugin.Empty{}, &keep); err != nil || keep["kept"] != "yes" {
		t.Errorf("empty result must leave resp alone: %v %v", keep, err)
	}
	if err := h.Call("t.fail", plugin.Empty{}, nil); plugin.ClassOf(err) != abi.ClassAuth {
		t.Errorf("module error: %v", err)
	}
	if err := h.Call("t.echo", func() {}, nil); err == nil {
		t.Error("an unencodable request is an error")
	}
	var wrong int
	if err := h.Call("t.echo", map[string]string{"a": "b"}, &wrong); err == nil {
		t.Error("a result that does not fit resp is an error")
	}
}

func TestCapabilitiesListsRegisteredOperations(t *testing.T) {
	h := plugintest.New(plugintest.Host{})
	plugin.Op("t.listed", func(plugin.Empty) (plugin.Empty, error) { return plugin.Empty{}, nil })
	c, err := h.Capabilities()
	if err != nil || c.ABI != abi.Version {
		t.Fatalf("%+v %v", c, err)
	}
	found := false
	for _, op := range c.Ops {
		found = found || op == "t.listed"
	}
	if !found {
		t.Errorf("t.listed missing from %v", c.Ops)
	}
}

func TestLogsAndEventsAreCaptured(t *testing.T) {
	h := plugintest.New(plugintest.Host{})
	plugin.Info("one", "k", "v")
	plugin.Warn("two")
	if err := plugin.Emit("download.progress", map[string]any{"id": "x"}); err != nil {
		t.Fatal(err)
	}
	logs := h.Logs()
	if len(logs) != 2 || logs[0].Message != "one" || logs[0].Attrs["k"] != "v" || logs[1].Level != "warn" {
		t.Errorf("logs: %+v", logs)
	}
	logs[0].Message = "changed"
	if h.Logs()[0].Message != "one" {
		t.Error("Logs must return a copy")
	}
	ev := h.Events()
	if len(ev) != 1 || ev[0].Type != "download.progress" || ev[0].Data["id"] != "x" {
		t.Errorf("events: %+v", ev)
	}
}

func TestSettingsSecretsAndClock(t *testing.T) {
	at := time.Date(2031, 5, 6, 7, 8, 9, 0, time.UTC)
	plugintest.New(plugintest.Host{
		Settings: map[string]any{"url": "http://box"},
		Secrets:  map[string]string{"token": "s3"},
		Now:      func() time.Time { return at },
	})
	if v := plugin.SettingString("url"); v != "http://box" {
		t.Errorf("setting: %q", v)
	}
	if v, err := plugin.Secret("token"); err != nil || v != "s3" {
		t.Errorf("secret: %q %v", v, err)
	}
	_, err := plugin.Secret("other")
	var e *abi.Error
	if !errors.As(err, &e) || e.Class != abi.ClassDenied || e.Message != "secret other is not declared" {
		t.Errorf("undeclared secret: %v", err)
	}
	if !plugin.Now().Equal(at) {
		t.Errorf("clock: %v", plugin.Now())
	}
	start := time.Now()
	if err := plugin.Sleep(time.Hour); err != nil || time.Since(start) > time.Minute {
		t.Errorf("sleep returns at once: %v", err)
	}
}

func TestDefaultClockIsTheRealOne(t *testing.T) {
	plugintest.New(plugintest.Host{})
	if d := time.Since(plugin.Now()); d < 0 || d > time.Minute {
		t.Errorf("clock is %v off", d)
	}
}

func TestHTTPGoesThroughTheClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Echo", r.Header.Get("X-In")+r.Method)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("hi"))
	}))
	defer srv.Close()

	for name, hosts := range map[string][]string{"any host": nil, "declared host": {"127.0.0.1"}} {
		plugintest.New(plugintest.Host{Hosts: hosts})
		resp, err := plugin.Do(plugin.Request{Method: "PUT", URL: srv.URL, Headers: map[string][]string{"X-In": {"a"}}})
		if err != nil || resp.Status != http.StatusAccepted || string(resp.Body) != "hi" || http.Header(resp.Headers).Get("X-Echo") != "aPUT" {
			t.Errorf("%s: %+v %v", name, resp, err)
		}
	}
	plugintest.New(plugintest.Host{Client: srv.Client()})
	if resp, err := plugin.Do(plugin.Request{Method: "GET", URL: srv.URL}); err != nil || resp.Status != http.StatusAccepted {
		t.Errorf("own client: %+v %v", resp, err)
	}
}

func TestHTTPHostAllowList(t *testing.T) {
	plugintest.New(plugintest.Host{Hosts: []string{"*.example.org", "exact.test"}, Client: refusing()})
	for url, class := range map[string]string{
		"http://a.example.org/x":   abi.ClassUnreachable, // allowed; the client then fails
		"http://exact.test/":       abi.ClassUnreachable,
		"http://example.org/":      abi.ClassDenied,
		"http://other.test/":       abi.ClassDenied,
		"http://exact.test.evil/":  abi.ClassDenied,
		"http://b.a.example.org/x": abi.ClassUnreachable,
	} {
		if _, err := plugin.Do(plugin.Request{Method: "GET", URL: url}); plugin.ClassOf(err) != class {
			t.Errorf("%s: got %v, want %s", url, err, class)
		}
	}
	plugintest.New(plugintest.Host{Hosts: []string{"exact.test"}, Client: refusing()})
	if _, err := plugin.Do(plugin.Request{Method: "GET", URL: "http://other.test/"}); plugin.ClassOf(err) != abi.ClassDenied {
		t.Errorf("a single declared host still limits: %v", err)
	}
	_, err := plugin.Do(plugin.Request{Method: "GET", URL: "http://other.test/"})
	if !containsMsg(err, "host other.test is not declared") {
		t.Errorf("denial names the host: %v", err)
	}
}

func TestHTTPBadRequestsAreServerErrors(t *testing.T) {
	plugintest.New(plugintest.Host{})
	for name, req := range map[string]plugin.Request{
		"unparsable url": {Method: "GET", URL: "http://[::1"},
		"invalid method": {Method: "BAD METHOD", URL: "http://127.0.0.1/"},
	} {
		if _, err := plugin.Do(req); plugin.ClassOf(err) != abi.ClassServerError {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func refusing() *http.Client {
	return &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("refused")
	})}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func containsMsg(err error, msg string) bool {
	var e *abi.Error
	return errors.As(err, &e) && e.Message == msg
}
