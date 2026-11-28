package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/events"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/importer"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/rootfolders"
	"github.com/jrand0m/corsarr/internal/rules"
	"github.com/jrand0m/corsarr/internal/scheduler"
	"github.com/jrand0m/corsarr/internal/testkit"
)

// everything implements every extension point the pipeline uses.
type everything struct{}

func (everything) Test(context.Context) error                { return nil }
func (everything) Caps(context.Context) (plugin.Caps, error) { return plugin.Caps{}, nil }
func (everything) Search(context.Context, domain.SearchRequest) ([]domain.Release, error) {
	return nil, nil
}
func (everything) Feed(context.Context, domain.MediaKind, string) ([]domain.Release, string, error) {
	return nil, "", nil
}
func (everything) Resolve(context.Context, domain.Release) (plugin.Payload, error) {
	return plugin.Payload{}, nil
}
func (everything) Protocols() []domain.Protocol                               { return []domain.Protocol{domain.ProtoTorrent} }
func (everything) Add(context.Context, plugin.AddRequest) (string, error)     { return "", nil }
func (everything) List(context.Context, []string) ([]plugin.Snapshot, error)  { return nil, nil }
func (everything) Remove(context.Context, string, bool) error                 { return nil }
func (everything) Pause(context.Context, string) error                        { return nil }
func (everything) Resume(context.Context, string) error                       { return nil }
func (everything) SetPriority(context.Context, string, plugin.Priority) error { return nil }
func (everything) EnsureCategory(context.Context, string, string) error       { return nil }
func (everything) SearchMovies(context.Context, string, int) ([]plugin.MovieRecord, error) {
	return nil, nil
}
func (everything) Movie(context.Context, map[string]string) (plugin.MovieRecord, error) {
	return plugin.MovieRecord{}, nil
}
func (everything) RefreshPaths(context.Context, []string) error { return nil }
func (everything) Schemes() []string                            { return nil }
func (everything) Send(context.Context, domain.Event, plugin.Message) error {
	return nil
}

type httpFor struct{}

func (httpFor) For(domain.IntegrationRef) plugin.HTTPDoer { return http.DefaultClient }

func registry() *plugin.Registry {
	reg := plugin.NewRegistry()
	for _, k := range []plugin.Kind{plugin.KindIndexer, plugin.KindDownloader, plugin.KindMetadata, plugin.KindMediaServer, plugin.KindNotifier} {
		reg.MustRegister(plugin.Registration{Manifest: plugin.Manifest{ID: "x" + string(k), Kind: k, APIVersion: plugin.APIVersion},
			New: func(plugin.Instance) (any, error) { return everything{}, nil }})
	}
	return reg
}

func testConfig(root string) *config.Config {
	c := &config.Config{Instances: map[string]map[domain.ObjectKey]config.Instance{},
		RootFolders: []config.RootFolder{{Path: root, Kind: domain.KindMovie}, {Path: "/tv", Kind: domain.KindTV}}}
	for _, k := range []plugin.Kind{plugin.KindIndexer, plugin.KindDownloader, plugin.KindMetadata, plugin.KindMediaServer, plugin.KindNotifier} {
		c.Instances[k.Section()] = map[domain.ObjectKey]config.Instance{"a": {Section: k.Section(), Key: "a", Type: "x" + string(k),
			Enabled: true, Settings: plugin.Settings{"on": []any{"import"}}}}
	}
	return c
}

func TestAppWiresModulesJobsAndAdapters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := testkit.Store(t)
	bus := events.New()
	defer bus.Close()
	reg := health.New(bus, nil, nil)
	jobs := scheduler.New(scheduler.Options{})
	cfg := testConfig(t.TempDir())
	a, err := New(ctx, Deps{Config: func() *config.Config { return cfg }, Store: st, Bus: bus, Health: reg, Jobs: jobs, HTTP: httpFor{},
		Plugins: registry(), UIClients: func() int { return 0 }, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	jobs.Start(ctx)
	defer func() { _ = jobs.Stop(context.Background()) }()
	if err := a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if len(jobs.List()) != 12 || len(a.Mounts()) != 15 { // movie, TV and music pipelines, indexer settings, migration, compatibility API, MCP, profiles
		t.Fatalf("jobs %d mounts %d", len(jobs.List()), len(a.Mounts()))
	}
	p := plugins{a}
	if _, ok := p.Indexer("a"); !ok || len(p.Downloaders()) != 1 || len(a.indexers()) != 1 {
		t.Fatal("indexers and downloaders")
	}
	if _, ok := a.metadata(); !ok || len(a.mediaServers()) != 1 || a.connections()[0].Toggles[0] != "import" {
		t.Fatal("metadata, media servers, connections")
	}
	if roots := a.movieRoots(); len(roots) != 1 || len(a.roots()) != 2 {
		t.Fatalf("roots %v", roots)
	}
	if h, _ := reg.Get(rootfolders.Ref("/tv")); h.LastErrorClass != health.ClassPath {
		t.Fatalf("missing root folder reported at start: %+v", h)
	}
	if roots := a.tvRoots(); len(roots) != 1 || roots[0] != "/tv" {
		t.Fatalf("tv roots %v", roots)
	}
	if _, ok := a.seriesMetadata(); ok {
		t.Fatal("the test plugin serves no TV metadata")
	}
	ser, _, _ := st.TV().AddSeries(ctx, domain.Series{TMDBID: 1, Title: "Show", RootFolder: "/tv", Path: "/tv/Show", ProfileKey: "any",
		Type: domain.SeriesStandard, MonitorNew: domain.MonitorNewAll, Tags: []string{}}, nil, []domain.Episode{{Season: 1, Number: 2}})
	tvTitles := a.itemTitles(ctx, []domain.ItemRef{{Kind: domain.KindTV, ID: 1}, ser.Ref(), {Kind: domain.KindTV, ID: 9, Variant: domain.VariantSeries}})
	if tvTitles[0].Title != "Show S01E02" || tvTitles[1].Title != "Show" || tvTitles[2].Title != "" {
		t.Fatalf("tv item titles %+v", tvTitles)
	}
	cfg.RootFolders = append(cfg.RootFolders, config.RootFolder{Path: "/music", Kind: domain.KindMusic})
	if roots := a.musicRoots(); len(roots) != 1 || roots[0] != "/music" {
		t.Fatalf("music roots %v", roots)
	}
	if _, ok := a.musicMetadata(); ok {
		t.Fatal("the test plugin serves no music metadata")
	}
	ar, _, _ := st.Music().AddArtist(ctx, domain.Artist{MBID: "m", Name: "Band", RootFolder: "/music", Path: "/music/Band",
		ProfileKey: "standard", MetadataProfile: "standard", MonitorNew: domain.MonitorNewItemsAll, Tags: []string{}}, []domain.Album{{MBID: "g", Title: "Record"}})
	albums, _ := st.Music().Albums(ctx, ar.ID)
	musicTitles := a.itemTitles(ctx, []domain.ItemRef{albums[0].Ref(), ar.Ref(), {Kind: domain.KindMusic, ID: 9, Variant: domain.VariantArtist}})
	if musicTitles[0].Title != "Band - Record" || musicTitles[1].Title != "Band" || musicTitles[2].Title != "" {
		t.Fatalf("music item titles %+v", musicTitles)
	}
	// the start-up self-test runs concurrently and may report ok first; a
	// later auth failure must stick and hide the downloader
	testkit.Eventually(t, 2*time.Second, "action-needed downloaders are skipped", func() bool {
		reg.Fail(domain.IntegrationRef{Type: "downloader", Key: "a"}, health.ClassAuth, "bad password")
		return len(p.Downloaders()) == 0
	})
	// the downloader overview still lists it, with its health
	if cl := a.downloadClients(); len(cl) != 1 || cl[0].Health == nil || cl[0].Type != "xdownloader" {
		t.Fatalf("download clients %+v", cl)
	}
	if err := a.testDownloader(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	m, _, err := st.Movies().Add(ctx, domain.Movie{TMDBID: 1, Title: "Film", MinAvailability: domain.AvailReleased, ProfileKey: "any", RootFolder: "/m", Path: "/m/f"})
	if err != nil {
		t.Fatal(err)
	}
	titles := a.itemTitles(ctx, []domain.ItemRef{m.Ref(), {Kind: domain.KindMovie, ID: 99}})
	if titles[0].Title != "Film" || titles[1].Title != "" {
		t.Fatalf("%+v", titles)
	}
	// events reach the modules: an item.available for an unknown movie is a no-op search
	_, _ = bus.Publish(domain.Event{Type: domain.EvItemDeleted, Items: []domain.EventItem{m.EventItem()}, Data: domain.ItemEventData{}})
	_, _ = bus.Publish(domain.Event{Type: domain.EvDownloadCompleted, Data: domain.DownloadEventData{DownloadID: 42}})
	time.Sleep(50 * time.Millisecond)
	if m, _ := a.Rules.Mode(importer.RuleRecyclePurge); m != rules.ModeDryRun {
		t.Fatalf("recycle purge ships dry-run: %s", m)
	}
	a.OnConfig(&config.Config{RuleModes: map[string]string{importer.RuleRecyclePurge: "live", "ghost": "off"}})
	if _, ok := a.metadata(); ok {
		t.Fatal("config change drops instances")
	}
	if m, _ := a.Rules.Mode(importer.RuleRecyclePurge); m != rules.ModeLive {
		t.Fatalf("automation.rules.recyclepurge.mode switches live: %s", m)
	}
	a.Close()
}

func TestNewFailsOnDuplicateJobs(t *testing.T) {
	st := testkit.Store(t)
	bus := events.New()
	defer bus.Close()
	jobs := scheduler.New(scheduler.Options{})
	_ = jobs.Register(context.Background(), scheduler.Job{Name: "queue.poll", Interval: time.Hour, Run: func(context.Context) error { return nil }})
	_, err := New(context.Background(), Deps{Config: func() *config.Config { return &config.Config{} }, Store: st, Bus: bus,
		Health: health.New(nil, nil, nil), Jobs: jobs, HTTP: httpFor{}, Plugins: registry(), Log: slog.Default()})
	if err == nil {
		t.Fatal("duplicate job registration must fail")
	}
}

func TestModeChanges(t *testing.T) {
	shadow := modesOf(&config.Config{MediaKinds: map[domain.MediaKind]domain.KindMode{domain.KindMovie: domain.ModeShadow}})
	if shadow[domain.KindMovie] != domain.ModeShadow || shadow[domain.KindTV] != domain.ModeActive {
		t.Fatalf("modesOf normalises: %v", shadow)
	}
	got := ModeChanges(shadow, modesOf(&config.Config{}))
	if len(got) != 1 || got[0] != (domain.ModeEventData{Kind: domain.KindMovie, From: domain.ModeShadow, To: domain.ModeActive}) {
		t.Fatalf("%+v", got)
	}
	if len(ModeChanges(shadow, shadow)) != 0 {
		t.Fatal("no change, no event")
	}
}

func TestActivationImportsWhatWaitedInShadow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := testkit.Store(t)
	bus := events.New()
	defer bus.Close()
	rec := bus.Subscribe(16, func(t domain.EventType) bool { return t == domain.EvMediaKindMode })
	defer rec.Close()
	var cur atomic.Pointer[config.Config]
	cfg := testConfig(t.TempDir())
	cfg.MediaKinds = map[domain.MediaKind]domain.KindMode{domain.KindMovie: domain.ModeShadow}
	cur.Store(cfg)
	jobs := scheduler.New(scheduler.Options{})
	a, err := New(ctx, Deps{Config: cur.Load, Store: st, Bus: bus, Health: health.New(bus, nil, nil), Jobs: jobs, HTTP: httpFor{},
		Plugins: registry(), UIClients: func() int { return 0 }, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	ref := domain.ItemRef{Kind: domain.KindMovie, ID: 1}
	_, d, err := st.Queue().CreateGrab(ctx, domain.Grab{Kind: domain.KindMovie, ItemRefs: []domain.ItemRef{ref}, Downloader: "a", DownloadID: "h"},
		domain.Download{Downloader: "a", DownloadID: "h", Kind: domain.KindMovie, State: domain.StateCompleted,
			Links: []domain.DownloadLink{{Item: ref, Status: domain.LinkPending}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.Importer.Import(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.Queue.Get(ctx, d.ID); got.State != domain.StateCompleted {
		t.Fatalf("shadow: no import, got %s", got.State)
	}
	active := testConfig(t.TempDir())
	cur.Store(active)
	a.OnConfig(active)
	ev, err := rec.Next(ctx)
	if err != nil || ev.Data.(domain.ModeEventData).To != domain.ModeActive {
		t.Fatalf("mode_changed %v %+v", err, ev)
	}
	testkit.Eventually(t, 2*time.Second, "the waiting download is imported", func() bool {
		got, _ := a.Queue.Get(ctx, d.ID)
		return got.State != domain.StateCompleted
	})
}
