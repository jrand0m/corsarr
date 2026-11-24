package musicauto

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/music"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/queue"
	"github.com/jrand0m/corsarr/internal/store"
)

var (
	ctx = context.Background()
	now = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
)

func date(y int) *time.Time { t := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC); return &t }

type meta struct{}

func (meta) Test(context.Context) error { return nil }
func (meta) SearchArtists(context.Context, string) ([]plugin.ArtistRecord, error) {
	return nil, nil
}
func (meta) Artist(context.Context, string) (plugin.ArtistRecord, error) {
	return plugin.ArtistRecord{MBID: "a1", Name: "Example Artist", Albums: []plugin.AlbumRecord{
		{MBID: "g1", Title: "First Album", PrimaryType: "Album", FirstRelease: date(2020)},
		{MBID: "g2", Title: "Second Album", PrimaryType: "Album", FirstRelease: date(2022)},
		{MBID: "g3", Title: "Future Album", PrimaryType: "Album", FirstRelease: date(2030)}}}, nil
}
func (meta) Releases(context.Context, string) ([]plugin.ReleaseRecord, error) {
	return []plugin.ReleaseRecord{{MBID: "r", Status: "official", Media: []plugin.MediumRecord{{Position: 1,
		Tracks: []plugin.TrackRecord{{Position: 1, Title: "A", LengthSec: 1200}, {Position: 2, Title: "B", LengthSec: 1200}}}}}}, nil
}

type fakeQueue struct {
	mu    sync.Mutex
	grabs []queue.GrabRequest
	err   error
}

func (q *fakeQueue) Grab(_ context.Context, r queue.GrabRequest) (domain.Grab, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return domain.Grab{}, q.err
	}
	q.grabs = append(q.grabs, r)
	return domain.Grab{ID: int64(len(q.grabs)), Release: r.Release}, nil
}

func (q *fakeQueue) GrabbedBefore(context.Context, domain.ItemRef, domain.Release) (bool, error) {
	return false, nil
}

func (q *fakeQueue) QueuedQualities(_ context.Context, item domain.ItemRef) ([]string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, g := range q.grabs {
		if g.Items[0].Ref == item {
			return []string{g.Decision.Quality}, nil
		}
	}
	return nil, nil
}

func (q *fakeQueue) count() int { q.mu.Lock(); defer q.mu.Unlock(); return len(q.grabs) }

// indexer answers every album search with that album in FLAC and MP3.
type indexer struct {
	mu   sync.Mutex
	reqs []domain.SearchRequest
	err  error
}

func (ix *indexer) Test(context.Context) error { return nil }
func (ix *indexer) Caps(context.Context) (plugin.Caps, error) {
	return plugin.Caps{Modes: []domain.SearchMode{domain.SearchMusic}}, nil
}
func (ix *indexer) Search(_ context.Context, req domain.SearchRequest) ([]domain.Release, error) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.reqs = append(ix.reqs, req)
	if ix.err != nil {
		return nil, ix.err
	}
	base := req.IDs["artist"] + " - " + req.IDs["album"]
	return []domain.Release{rel(base + " [FLAC]"), rel(base + " [MP3 320kbps]")}, nil
}
func (ix *indexer) Feed(context.Context, domain.MediaKind, string) ([]domain.Release, string, error) {
	return nil, "", nil
}
func (ix *indexer) Resolve(context.Context, domain.Release) (plugin.Payload, error) {
	return plugin.Payload{}, nil
}

func rel(title string) domain.Release {
	return domain.Release{ID: title, Title: title, Protocol: domain.ProtoTorrent, Size: 200e6, Seeders: 30,
		Indexers: []domain.IndexerRef{{Key: "ix"}}, InfoHash: title}
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

func (r *recorder) count(t domain.EventType) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, ev := range r.evs {
		if ev.Type == t {
			n++
		}
	}
	return n
}

type fixture struct {
	svc   *Service
	mus   *music.Service
	q     *fakeQueue
	ix    *indexer
	pub   *recorder
	st    *store.Store
	mode  domain.KindMode
	artID int64
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	root := t.TempDir()
	f := &fixture{st: st, q: &fakeQueue{}, ix: &indexer{}, pub: &recorder{}, mode: domain.ModeActive}
	modes := domain.Modes(func(domain.MediaKind) domain.KindMode { return f.mode })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	f.mus = music.New(music.Deps{Repo: st.Music(), Files: st.Library(), Pub: f.pub, Metadata: func() (plugin.MusicMetadata, bool) { return meta{}, true },
		RootFolders: func() []string { return []string{root} }, Modes: modes, Now: func() time.Time { return now }, Log: log})
	ar, _, err := f.mus.Add(ctx, music.AddRequest{MBID: "a1", ProfileKey: "lossless"})
	if err != nil {
		t.Fatal(err)
	}
	f.mus.Stop()
	f.artID = ar.ID
	f.svc = New(Deps{Music: f.mus, Queue: f.q, Blocklist: st.Blocklist(), Pub: f.pub, Modes: modes, Now: func() time.Time { return now },
		Indexers: func() []automation.Indexer { return []automation.Indexer{{Key: "ix", Impl: f.ix}} }, Log: log})
	if err := f.svc.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestIndexHoldsReleasedMonitoredAlbumsWithMissingTracks(t *testing.T) {
	f := newFixture(t)
	if f.svc.Index().Len() != 2 {
		t.Fatalf("two released albums are wanted, got %d", f.svc.Index().Len())
	}
	page, next := f.svc.Index().Page(automation.ReasonMissing, 0, 1)
	if len(page) != 1 || next != page[0].Album.ID || page[0].Artist.Name != "Example Artist" {
		t.Fatalf("%+v %d", page, next)
	}
	ar, _ := f.mus.Artist(ctx, f.artID)
	al := domain.Album{Monitored: true, TrackCount: 2, ReleaseDate: date(2020)}
	if WantedReason(ar, al, []domain.LibraryFile{{Quality: "flac"}, {Quality: "flac"}}, now) != automation.ReasonNotWanted ||
		WantedReason(ar, al, []domain.LibraryFile{{Quality: "flac"}}, now) != automation.ReasonMissing ||
		WantedReason(ar, al, []domain.LibraryFile{{Quality: "wav"}, {Quality: "mp3-320"}}, now) != automation.ReasonCutoffUnmet {
		t.Fatal("wanted reasons")
	}
}

func TestSearchArtistGrabsTheBestReleaseOfEachWantedAlbum(t *testing.T) {
	f := newFixture(t)
	sum, err := f.svc.SearchArtist(ctx, f.artID, domain.OriginAutomatic)
	if err != nil || sum.Searches != 2 || f.q.count() != 2 {
		t.Fatalf("%+v %v", sum, err)
	}
	g := f.q.grabs[0]
	if g.Kind != domain.KindMusic || !strings.HasSuffix(g.Release.Title, "[FLAC]") || g.Items[0].Title != "Example Artist - First Album" {
		t.Fatalf("%+v", g)
	}
	req := f.ix.reqs[0]
	if req.Mode != domain.SearchMusic || req.IDs["album"] != "First Album" {
		t.Fatalf("%+v", req)
	}
	// queued at FLAC: a second search finds nothing better
	sum, _ = f.svc.SearchAlbum(ctx, g.Items[0].Ref.ID, domain.OriginAutomatic)
	if sum.Grabbed != "" || f.q.count() != 2 {
		t.Fatalf("%+v", sum)
	}
	if _, err := f.svc.SearchArtist(ctx, 999, domain.OriginAutomatic); err == nil {
		t.Fatal("unknown artist")
	}
}

func TestEventsDriveSearches(t *testing.T) {
	f := newFixture(t)
	ar, _ := f.mus.Artist(ctx, f.artID)
	f.svc.OnEvent(ctx, domain.Event{Type: domain.EvItemAdded, Items: []domain.EventItem{ar.EventItem()}, Data: domain.ItemEventData{}})
	if f.q.count() != 0 {
		t.Fatal("added without searchNow does not search")
	}
	f.svc.OnEvent(ctx, domain.Event{Type: domain.EvItemAdded, Items: []domain.EventItem{ar.EventItem()}, Data: domain.ItemEventData{SearchNow: true}})
	if f.q.count() != 2 {
		t.Fatalf("search on add: %d", f.q.count())
	}
	albums, _ := f.mus.Albums(ctx, f.artID)
	item := albums[0].EventItem(ar.Name)
	f.svc.OnFailure(ctx, domain.Event{Type: domain.EvDownloadFailed, Items: []domain.EventItem{item},
		Data: domain.DownloadEventData{Reason: "stalled"}})
	// the album is searched again; its grab is still queued, so nothing new is grabbed
	if f.searches() != 3 || f.q.count() != 2 {
		t.Fatalf("failure re-searches: %d %d", f.searches(), f.q.count())
	}
	f.svc.OnEvent(ctx, domain.Event{Type: domain.EvItemAvailable, Items: []domain.EventItem{item}})
	if f.searches() != 4 {
		t.Fatalf("release searches: %d", f.searches())
	}
	tracks, _ := f.mus.Tracks(ctx, albums[0].ID)
	f.svc.OnEvent(ctx, domain.Event{Type: domain.EvFileDeleted, Items: []domain.EventItem{{Ref: tracks[0].Ref()}, {Ref: domain.ItemRef{Kind: domain.KindTV, ID: 1}}}})
	f.svc.OnEvent(ctx, domain.Event{Type: domain.EvItemDeleted, Items: []domain.EventItem{{Ref: domain.ItemRef{Kind: domain.KindMusic, ID: 404, Variant: domain.VariantArtist}}}})
	f.mode = domain.ModeDisabled
	f.svc.OnEvent(ctx, domain.Event{Type: domain.EvItemAvailable, Items: []domain.EventItem{item}})
	if f.searches() != 4 {
		t.Fatal("a disabled kind does not search")
	}
}

func TestShadowAndDisabledModes(t *testing.T) {
	f := newFixture(t)
	f.mode = domain.ModeShadow
	albums, _ := f.mus.Albums(ctx, f.artID)
	sum, err := f.svc.SearchAlbum(ctx, albums[0].ID, domain.OriginAutomatic)
	if err != nil || sum.Shadowed == "" || f.q.count() != 0 || f.pub.count(domain.EvDecisionShadowed) != 1 {
		t.Fatalf("%+v %v", sum, err)
	}
	f.mode = domain.ModeDisabled
	var me domain.ModeError
	if _, err := f.svc.SearchAlbum(ctx, albums[0].ID, domain.OriginAutomatic); !errors.As(err, &me) {
		t.Fatal(err)
	}
	if _, err := f.svc.Interactive(ctx, albums[0].ID, automation.NopEmitter); !errors.As(err, &me) {
		t.Fatal(err)
	}
}

func TestInteractiveSearchAndManualGrab(t *testing.T) {
	f := newFixture(t)
	albums, _ := f.mus.Albums(ctx, f.artID)
	id := albums[0].ID
	f.q.err = errors.New("no downloader")
	if sum, _ := f.svc.SearchAlbum(ctx, id, domain.OriginAutomatic); sum.Grabbed != "" {
		t.Fatal("a failed grab is not a grab")
	}
	f.q.err = nil
	sum, err := f.svc.Interactive(ctx, id, automation.NopEmitter)
	if err != nil || sum.Releases != 2 || sum.Accepted != 1 {
		t.Fatalf("%+v %v", sum, err)
	}
	mp3 := "Example Artist - First Album [MP3 320kbps]"
	if _, _, err := f.svc.GrabRelease(ctx, id, mp3, false); !errors.Is(err, automation.ErrNeedsConfirm) {
		t.Fatalf("a rejected quality needs confirmation: %v", err)
	}
	if g, d, err := f.svc.GrabRelease(ctx, id, mp3, true); err != nil || g.ID == 0 || d.Decision.Accepted {
		t.Fatalf("%+v %v", g, err)
	}
	if _, _, err := f.svc.GrabRelease(ctx, id, "nope", false); !errors.Is(err, automation.ErrUnknownRelease) {
		t.Fatal(err)
	}
	f.ix.err = errors.New("down")
	if sum, _ := f.svc.Interactive(ctx, id, automation.NopEmitter); sum.Failed != 1 {
		t.Fatalf("%+v", sum)
	}
}

func (f *fixture) searches() int { f.ix.mu.Lock(); defer f.ix.mu.Unlock(); return len(f.ix.reqs) }

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
	if code, body := call("GET", "/api/v1/wanted/missing/albums", ""); code != 200 || !strings.Contains(body, "First Album") {
		t.Fatalf("%d %s", code, body)
	}
	if code, body := call("GET", "/api/v1/albums/1/releases", ""); code != 200 || !strings.Contains(body, `"accepted":1`) {
		t.Fatalf("%d %s", code, body)
	}
	if code, _ := call("GET", "/api/v1/albums/99/releases", ""); code != 404 {
		t.Fatal("unknown album")
	}
	cases := []struct {
		body string
		code int
	}{{`{"releaseId":"nope"}`, 404}, {`{"releaseId":"Example Artist - First Album [MP3 320kbps]"}`, 409},
		{`{"releaseId":"Example Artist - First Album [FLAC]"}`, 201}}
	for _, c := range cases {
		if code, body := call("POST", "/api/v1/albums/1/grabs", c.body); code != c.code {
			t.Errorf("%s: %d %s", c.body, code, body)
		}
	}
	if code, _ := call("POST", "/api/v1/albums/2/search", ""); code != 200 {
		t.Fatal("album search")
	}
	if code, _ := call("POST", "/api/v1/artists/1/search", ""); code != 200 {
		t.Fatal("artist search")
	}
	if code, _ := call("POST", "/api/v1/albums/99/search", ""); code != 404 {
		t.Fatal("unknown album search")
	}
	f.mode = domain.ModeShadow
	if code, _ := call("POST", "/api/v1/albums/1/grabs", `{"releaseId":"Example Artist - First Album [FLAC]","confirm":true}`); code != 409 {
		t.Fatal("shadow grab")
	}
}
