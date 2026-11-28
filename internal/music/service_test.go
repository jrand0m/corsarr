package music

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

type fakeMeta struct {
	mu       sync.Mutex
	rec      plugin.ArtistRecord
	releases map[string][]plugin.ReleaseRecord
	err      error
}

func (f *fakeMeta) Test(context.Context) error { return nil }

func (f *fakeMeta) SearchArtists(context.Context, string) ([]plugin.ArtistRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.rec
	r.Albums = nil
	return []plugin.ArtistRecord{r, {MBID: "other", Name: "Other"}}, f.err
}

func (f *fakeMeta) Artist(_ context.Context, mbid string) (plugin.ArtistRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return plugin.ArtistRecord{}, f.err
	}
	if mbid != f.rec.MBID {
		return plugin.ArtistRecord{}, plugin.ErrNotFound
	}
	return f.rec, nil
}

func (f *fakeMeta) Releases(_ context.Context, mbid string) ([]plugin.ReleaseRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.releases[mbid], f.err
}

type recorder struct {
	mu  sync.Mutex
	evs []domain.Event
}

func (r *recorder) Publish(ev domain.Event) (domain.Event, error) {
	r.mu.Lock()
	r.evs = append(r.evs, ev)
	r.mu.Unlock()
	return ev, nil
}

func (r *recorder) of(t domain.EventType) []domain.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.Event
	for _, ev := range r.evs {
		if ev.Type == t {
			out = append(out, ev)
		}
	}
	return out
}

type fixture struct {
	svc  *Service
	meta *fakeMeta
	pub  *recorder
	st   *store.Store
	root string
	mode domain.KindMode
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f := &fixture{st: st, pub: &recorder{}, root: t.TempDir(), mode: domain.ModeActive, meta: &fakeMeta{
		rec: plugin.ArtistRecord{MBID: "a1", Name: "Example Artist", Albums: []plugin.AlbumRecord{
			{MBID: "g1", Title: "First Album", PrimaryType: "Album", FirstRelease: date(2020, 1)},
			{MBID: "g2", Title: "Live Set", PrimaryType: "Album", SecondaryTypes: []string{"Live"}, FirstRelease: date(2021, 1)},
			{MBID: "g3", Title: "Next", PrimaryType: "Album", FirstRelease: date(2030, 1)}}},
		releases: map[string][]plugin.ReleaseRecord{"g1": {rel("r1", "official", "CD", date(2020, 1), 3)}}}}
	f.svc = New(Deps{Repo: st.Music(), Files: st.Library(), Pub: f.pub, Metadata: func() (plugin.MusicMetadata, bool) { return f.meta, true },
		RootFolders: func() []string { return []string{f.root} }, Modes: func(domain.MediaKind) domain.KindMode { return f.mode },
		Now: func() time.Time { return now }})
	return f
}

func (f *fixture) add(t *testing.T) domain.Artist {
	t.Helper()
	ar, created, err := f.svc.Add(context.Background(), AddRequest{MBID: "a1", ProfileKey: "lossless", SearchNow: true})
	if err != nil || !created {
		t.Fatalf("add: %v %v", created, err)
	}
	f.svc.Stop() // background edition fetches
	return ar
}

func TestAddStoresDiscographyMonitorsInProfileAndFetchesEditions(t *testing.T) {
	f := newFixture(t)
	ar := f.add(t)
	if ar.Path != f.root+"/Example Artist" || ar.MetadataProfile != "standard" || ar.MonitorNew != domain.MonitorNewItemsAll {
		t.Fatalf("%+v", ar)
	}
	d, err := f.svc.Detail(context.Background(), ar.ID)
	if err != nil || len(d.Albums) != 3 {
		t.Fatalf("%v %v", d, err)
	}
	first, live := d.Albums[0], d.Albums[1]
	if !first.Monitored || live.Monitored || live.InProfile || first.TrackCount != 3 || first.Status != StatusMissing {
		t.Fatalf("first %+v live %+v", first, live)
	}
	if d.Artist.Stats.AlbumCount != 2 || d.Artist.Stats.TotalAlbums != 3 || d.Artist.Stats.NextRelease == nil {
		t.Fatalf("stats %+v", d.Artist.Stats)
	}
	if ev := f.pub.of(domain.EvItemAdded); len(ev) != 1 || !ev[0].Data.(domain.ItemEventData).SearchNow {
		t.Fatalf("item.added %+v", ev)
	}
	if _, created, err := f.svc.Add(context.Background(), AddRequest{MBID: "a1"}); created || err != nil {
		t.Fatal("adding twice")
	}
	// released monitored albums were covered by the search on add; the future one fires later
	must(t, f.svc.CheckReleased(context.Background()))
	if len(f.pub.of(domain.EvItemAvailable)) != 0 {
		t.Fatal("nothing new is released yet")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestAddValidatesInput(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, req := range []AddRequest{{}, {MBID: "a1", Monitor: "x"}, {MBID: "a1", MonitorNew: "x"}, {MBID: "a1", ProfileKey: "x"},
		{MBID: "a1", MetadataProfile: "x"}, {MBID: "a1", RootFolder: "/elsewhere"}} {
		var fe FieldError
		if _, created, err := f.svc.Add(ctx, req); !errors.As(err, &fe) || created {
			t.Errorf("%+v: %v", req, err)
		}
	}
	if _, created, err := f.svc.Add(ctx, AddRequest{MBID: "missing"}); !errors.Is(err, plugin.ErrNotFound) || created {
		t.Fatalf("unknown artist: %v", err)
	}
	f.svc.d.Metadata = func() (plugin.MusicMetadata, bool) { return nil, false }
	if _, created, err := f.svc.Add(ctx, AddRequest{MBID: "a1"}); !errors.Is(err, ErrNoMetadata) || created {
		t.Fatal(err)
	}
	if _, err := f.svc.Lookup(ctx, "x"); !errors.Is(err, ErrNoMetadata) {
		t.Fatal(err)
	}
}

func TestLookupMarksLibraryArtists(t *testing.T) {
	f := newFixture(t)
	ar := f.add(t)
	items, err := f.svc.Lookup(context.Background(), "example")
	if err != nil || len(items) != 2 || items[0].LibraryID != ar.ID || items[1].LibraryID != 0 {
		t.Fatalf("%+v %v", items, err)
	}
	items, _ = f.svc.Lookup(context.Background(), "mbid:a1")
	if len(items) != 1 || len(items[0].Albums) != 0 {
		t.Fatalf("%+v", items)
	}
	if items, err := f.svc.Lookup(context.Background(), "mbid:nope"); err != nil || len(items) != 0 {
		t.Fatal("unknown id is an empty result")
	}
}

func TestRefreshAddsNewReleaseGroupsAndPublishesRelease(t *testing.T) {
	f := newFixture(t)
	ar := f.add(t)
	f.meta.mu.Lock()
	f.meta.rec.Albums = append(f.meta.rec.Albums, plugin.AlbumRecord{MBID: "g4", Title: "Fresh", PrimaryType: "Album", FirstRelease: date(2026, 5)})
	f.meta.mu.Unlock()
	res, err := f.svc.Refresh(context.Background(), ar.ID)
	f.svc.Stop()
	if err != nil || res.Added != 1 || len(f.pub.of(domain.EvItemUpdated)) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	must(t, f.svc.CheckReleased(context.Background()))
	av := f.pub.of(domain.EvItemAvailable)
	if len(av) != 1 || av[0].Items[0].Title != "Example Artist - Fresh" {
		t.Fatalf("item.available %+v", av)
	}
	// refreshed just now: the job skips it
	f.meta.err = errors.New("down")
	must(t, f.svc.RefreshAll(context.Background()))
	if _, err := f.svc.Refresh(context.Background(), ar.ID); err == nil {
		t.Fatal("refresh with the provider down fails")
	}
}

func TestMonitorPinsAndDeleteKeepsFilesUnlessAsked(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ar := f.add(t)
	albums, _ := f.svc.Albums(ctx, ar.ID)
	live := albums[1]
	f.meta.releases["g2"] = []plugin.ReleaseRecord{rel("r2", "official", "CD", nil, 2)}
	if n, err := f.svc.SetAlbumsMonitored(ctx, []int64{live.ID}, true); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	f.svc.Stop()
	page, err := f.svc.AlbumPage(ctx, live.ID)
	if err != nil || page.Album.PinReason != domain.PinOwner || len(page.Tracks) != 2 || page.Artist.Name != "Example Artist" {
		t.Fatalf("%+v %v", page, err)
	}
	tracks, _ := f.svc.Tracks(ctx, live.ID)
	if got, err := f.svc.Track(ctx, tracks[0].ID); err != nil || got.AlbumID != live.ID {
		t.Fatalf("track %+v %v", got, err)
	}
	if _, err := f.svc.Track(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatal("unknown track")
	}
	file, err := f.st.Library().SaveFile(ctx, domain.LibraryFile{Item: tracks[0].Ref(), Path: "/x/1.flac", Size: 10, Quality: "flac", AddedAt: now})
	must(t, err)
	must(t, f.svc.SetTrackFile(ctx, tracks[0].ID, file.ID))
	if page, _ = f.svc.AlbumPage(ctx, live.ID); page.Tracks[0].File == nil || page.Album.Status != StatusPartial {
		t.Fatalf("%+v", page)
	}
	f.mode = domain.ModeShadow
	if err := f.svc.Delete(ctx, ar.ID, true); err == nil {
		t.Fatal("shadow mode must not delete files")
	}
	f.mode = domain.ModeActive
	must(t, f.svc.Delete(ctx, ar.ID, true))
	del := f.pub.of(domain.EvItemDeleted)
	if len(del) != 1 || len(del[0].Items) != 2 || del[0].Items[1].Ref != tracks[0].Ref() {
		t.Fatalf("%+v", del)
	}
	if _, err := f.svc.Artist(ctx, ar.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted")
	}
	if it := f.svc.AlbumItem(ctx, live.ID); it.Title != "" {
		t.Fatal("gone album has no title")
	}
}

func TestUpdateChangesProfilesAndPublishes(t *testing.T) {
	f := newFixture(t)
	ar := f.add(t)
	p := "standardsingles"
	got, err := f.svc.Update(context.Background(), ar.ID, Patch{MetadataProfile: &p})
	if err != nil || got.MetadataProfile != p || len(f.pub.of(domain.EvItemUpdated)) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := f.svc.Update(context.Background(), ar.ID, Patch{MetadataProfile: &p}); err != nil || len(f.pub.of(domain.EvItemUpdated)) != 1 {
		t.Fatal("no change publishes nothing")
	}
}

type mux struct{ *http.ServeMux }

func (m mux) Handle(p string, h http.HandlerFunc) { m.ServeMux.Handle(p, h) }

func TestHTTPEndpoints(t *testing.T) {
	f := newFixture(t)
	m := mux{http.NewServeMux()}
	f.svc.Mount(m)
	srv := httptest.NewServer(m)
	defer srv.Close()
	call := func(method, path, body string) (int, string) {
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := call("GET", "/api/v1/artists/lookup?term=example", ""); code != 200 || !strings.Contains(body, `"libraryId":0`) {
		t.Fatalf("lookup %d %s", code, body)
	}
	if code, _ := call("GET", "/api/v1/artists/lookup", ""); code != 400 {
		t.Fatal("term required")
	}
	if code, body := call("POST", "/api/v1/artists", `{"mbid":"a1"}`); code != 201 {
		t.Fatalf("add %d %s", code, body)
	}
	f.svc.Stop()
	for path, want := range map[string]int{"/api/v1/artists": 200, "/api/v1/artists/1": 200, "/api/v1/artists/99": 404,
		"/api/v1/albums/1": 200, "/api/v1/artists/x": 400, "/api/v1/music/profiles": 200} {
		if code, body := call("GET", path, ""); code != want {
			t.Errorf("%s: %d %s", path, code, body)
		}
	}
	if code, _ := call("PUT", "/api/v1/albums/monitor", `{"ids":[2],"monitored":true}`); code != 200 {
		t.Fatal("monitor")
	}
	if code, _ := call("PUT", "/api/v1/albums/monitor", `{"ids":[]}`); code != 400 {
		t.Fatal("ids required")
	}
	if code, _ := call("PUT", "/api/v1/artists/1", `{"profileKey":"nope"}`); code != 400 {
		t.Fatal("bad profile")
	}
	if code, _ := call("POST", "/api/v1/artists/1/refresh", ""); code != 200 {
		t.Fatal("refresh")
	}
	f.svc.Stop()
	f.mode = domain.ModeDisabled
	if code, _ := call("GET", "/api/v1/artists", ""); code != 409 {
		t.Fatal("disabled kind answers 409")
	}
	f.mode = domain.ModeActive
	if code, _ := call("DELETE", "/api/v1/artists/1", ""); code != 204 {
		t.Fatal("delete")
	}
	f.svc.d.Metadata = func() (plugin.MusicMetadata, bool) { return nil, false }
	if code, _ := call("POST", "/api/v1/artists", `{"mbid":"a1"}`); code != 503 {
		t.Fatal("no metadata")
	}
	f.svc.d.Metadata = func() (plugin.MusicMetadata, bool) { return f.meta, true }
	f.meta.err = errors.New("down")
	if code, _ := call("GET", "/api/v1/artists/lookup?term=x", ""); code != 502 {
		t.Fatal("upstream")
	}
}

var _ api.Mounter = mux{}
