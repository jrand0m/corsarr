package automation

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/parse"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/queue"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/testkit"
)

var ctx = context.Background()

type fakeMovies struct{ m map[int64]domain.Movie }

func (f *fakeMovies) Get(_ context.Context, id int64) (domain.Movie, error) {
	m, ok := f.m[id]
	if !ok {
		return m, movies.ErrNotFound
	}
	return m, nil
}
func (f *fakeMovies) Each(_ context.Context, fn func(domain.Movie) error) error {
	for _, m := range f.m {
		if err := fn(m); err != nil {
			return err
		}
	}
	return nil
}

type fakeQueue struct {
	mu      sync.Mutex
	grabs   []queue.GrabRequest
	queued  []string
	err     error
	grabbed map[string]bool
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
func (q *fakeQueue) GrabbedBefore(_ context.Context, _ domain.ItemRef, r domain.Release) (bool, error) {
	return q.grabbed[r.Title], nil
}
func (q *fakeQueue) QueuedQualities(context.Context, domain.ItemRef) ([]string, error) {
	return q.queued, nil
}

type fakeIndexer struct {
	rels     []domain.Release
	feed     []domain.Release
	cursor   string
	err      error
	searches int
	seen     []string // cursors passed to Feed
}

func (f *fakeIndexer) Test(context.Context) error                { return nil }
func (f *fakeIndexer) Caps(context.Context) (plugin.Caps, error) { return plugin.Caps{}, nil }
func (f *fakeIndexer) Search(context.Context, domain.SearchRequest) ([]domain.Release, error) {
	f.searches++
	return f.rels, f.err
}
func (f *fakeIndexer) Feed(_ context.Context, _ domain.MediaKind, c string) ([]domain.Release, string, error) {
	f.seen = append(f.seen, c)
	return f.feed, f.cursor, f.err
}
func (f *fakeIndexer) Resolve(context.Context, domain.Release) (plugin.Payload, error) {
	return plugin.Payload{}, nil
}

type env struct {
	svc *Service
	st  *store.Store
	mv  *fakeMovies
	q   *fakeQueue
	ix  *fakeIndexer
	rec *testkit.Recorder
}

func day(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

var film = domain.Movie{ID: 1, TMDBID: 10, Title: "Example Movie", Year: 2021, Monitored: true, MinAvailability: domain.AvailReleased,
	ProfileKey: "hd1080p", Digital: day(2021, 6, 1), RuntimeMin: 110}

func rel(title string, size int64, seeders int) domain.Release {
	return domain.Release{ID: "id-" + title, Title: title, Protocol: domain.ProtoTorrent, Size: size, Seeders: seeders,
		GUID: "g-" + title, Indexers: []domain.IndexerRef{{Key: "ix"}}, PublishedAt: time.Now().Add(-time.Hour)}
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{st: testkit.Store(t), mv: &fakeMovies{m: map[int64]domain.Movie{1: film}}, q: &fakeQueue{grabbed: map[string]bool{}},
		ix: &fakeIndexer{}, rec: &testkit.Recorder{}}
	e.svc = New(Deps{Movies: e.mv, Files: e.st.Library(), Queue: e.q, Blocklist: e.st.Blocklist(), Cursors: e.st.FeedCursors(), Searches: e.st.Searches(), Recent: e.st.RecentReleases(),
		Indexers: func() []Indexer { return []Indexer{{Key: "ix", Impl: e.ix}} }, Pub: e.rec,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err := e.svc.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	return e
}

const (
	good   = "Example.Movie.2021.1080p.WEB-DL.x264-GRP"
	better = "Example.Movie.2021.1080p.BluRay.x264-GRP"
	other  = "Other.Film.2021.1080p.WEB-DL.x264-GRP"
)

func TestWantedReason(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	low := &decide.CurrentFile{Quality: "webdl-1080p"}
	top := &decide.CurrentFile{Quality: "bluray-1080p"}
	off := film
	off.Monitored = false
	future := film
	future.Digital = day(2030, 1, 1)
	cases := map[string]struct {
		m    domain.Movie
		cur  *decide.CurrentFile
		want Reason
	}{
		"missing":       {film, nil, ReasonMissing},
		"below cutoff":  {film, low, ReasonCutoffUnmet},
		"at cutoff":     {film, top, ReasonNotWanted},
		"unmonitored":   {off, nil, ReasonNotWanted},
		"not available": {future, nil, ReasonNotWanted},
		"outside":       {film, &decide.CurrentFile{Quality: "sdtv"}, ReasonCutoffUnmet},
	}
	for name, c := range cases {
		if got := WantedReason(c.m, c.cur, now); got != c.want {
			t.Errorf("%s: %q", name, got)
		}
	}
}

func TestSearchOnAddGrabsBestAcceptedRelease(t *testing.T) {
	e := newEnv(t)
	e.ix.rels = []domain.Release{rel(good, 4e9, 10), rel(better, 6e9, 10), rel(other, 4e9, 10), rel(good, 4e9, 10)}
	e.svc.OnEvent(ctx, domain.Event{Type: domain.EvItemAdded, Items: []domain.EventItem{film.EventItem()}, Data: domain.ItemEventData{SearchNow: true}})
	e.svc.Wait()
	if len(e.q.grabs) != 1 || e.q.grabs[0].Release.Title != better || e.q.grabs[0].Origin != domain.OriginAutomatic {
		t.Fatalf("grabs %+v", e.q.grabs)
	}
	sc := e.rec.Of(domain.EvSearchCompleted)
	if len(sc) != 1 {
		t.Fatal("search.completed")
	}
	data := sc[0].Data.(domain.SearchEventData)
	if data.Releases != 3 || data.Accepted != 2 || data.Grabbed != better {
		t.Fatalf("%+v", data)
	}
	// no searchNow: index refresh only
	e.svc.OnEvent(ctx, domain.Event{Type: domain.EvItemAdded, Items: []domain.EventItem{film.EventItem()}, Data: domain.ItemEventData{}})
	e.svc.Wait()
	e.svc.OnEvent(ctx, domain.Event{Type: domain.EvItemAvailable, Items: []domain.EventItem{film.EventItem()}})
	e.svc.Wait()
	if e.ix.searches != 2 {
		t.Fatalf("searches %d", e.ix.searches)
	}
}

func TestDecisionLookups(t *testing.T) {
	e := newEnv(t)
	_, _ = e.svc.AddBlock(ctx, domain.BlocklistEntry{Item: film.Ref(), Release: domain.ReleaseIdentity{Title: good}, Source: "manual"}, film.EventItem())
	if d := e.svc.decideOne(ctx, film, rel(good, 4e9, 5)); d.Accepted || d.Reasons[0].RuleID != "blocklist" {
		t.Fatalf("%+v", d)
	}
	e.q.grabbed[better] = true
	if d := e.svc.decideOne(ctx, film, rel(better, 8e9, 5)); d.Accepted {
		t.Fatal("grabbed before")
	}
	e.q.queued = []string{"Bluray-1080p"}
	ok, _ := e.svc.queuedEqualOrBetter(ctx, film, mustProfile(t), rel("Example.Movie.2021.1080p.WEB-DL-X", 1, 1))
	e.q.queued = []string{"WEBDL-1080p"}
	notBetter, _ := e.svc.queuedEqualOrBetter(ctx, film, mustProfile(t), rel(better, 1, 1))
	e.q.queued = []string{"weird"}
	unknown, _ := e.svc.queuedEqualOrBetter(ctx, film, mustProfile(t), rel(better, 1, 1))
	if !ok || notBetter || !unknown {
		t.Fatalf("queued lookups %v %v %v", ok, notBetter, unknown)
	}
}

func mustProfile(t *testing.T) decide.QualityProfile {
	p, ok := movies.ProfileByKey("hd1080p")
	if !ok {
		t.Fatal("profile")
	}
	return p
}

type rows struct {
	recs []any
	errs []string
}

func (r *rows) Record(v any) error              { r.recs = append(r.recs, v); return nil }
func (r *rows) Error(src string, _ error) error { r.errs = append(r.errs, src); return nil }

func TestInteractiveSearchStreamsEveryDecision(t *testing.T) {
	e := newEnv(t)
	e.ix.rels = []domain.Release{rel(good, 4e9, 10), rel(other, 4e9, 10)}
	var out rows
	sum, err := e.svc.Interactive(ctx, 1, &out)
	if err != nil || len(out.recs) != 2 || sum.Accepted != 1 || len(e.q.grabs) != 0 {
		t.Fatalf("%+v %v", sum, err)
	}
	e.ix.err = errors.New("indexer down")
	out = rows{}
	sum, _ = e.svc.Interactive(ctx, 1, &out)
	if sum.Failed != 1 || out.errs[0] != "ix" {
		t.Fatalf("%+v", sum)
	}
	if _, err := e.svc.Interactive(ctx, 9, &out); !errors.Is(err, movies.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestManualGrabOverrides(t *testing.T) {
	e := newEnv(t)
	tooSmall := rel("Example.Movie.2021.1080p.WEB-DL.x264-TINY", 1e6, 10)
	notMovie := rel(other, 4e9, 10)
	exe := rel("Example.Movie.2021.1080p.WEB-DL.x264-BAD", 4e9, 10)
	exe.FileList = []string{"movie.mkv", "movie.exe"}
	e.ix.rels = []domain.Release{rel(good, 4e9, 10), tooSmall, notMovie, exe}
	if _, err := e.svc.Interactive(ctx, 1, &rows{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.GrabRelease(ctx, 1, "nope", false); !errors.Is(err, ErrUnknownRelease) {
		t.Fatal(err)
	}
	if _, _, err := e.svc.GrabRelease(ctx, 1, exe.ID, true); !errors.Is(err, ErrRefused) {
		t.Fatalf("never: %v", err)
	}
	if _, _, err := e.svc.GrabRelease(ctx, 1, tooSmall.ID, false); !errors.Is(err, ErrNeedsConfirm) {
		t.Fatalf("confirm: %v", err)
	}
	if _, _, err := e.svc.GrabRelease(ctx, 1, tooSmall.ID, true); err != nil || !e.q.grabs[0].Override {
		t.Fatalf("confirmed: %v", err)
	}
	if _, _, err := e.svc.GrabRelease(ctx, 1, rel(good, 0, 0).ID, false); err != nil || e.q.grabs[1].Override {
		t.Fatalf("accepted: %v", err)
	}
	if _, _, err := e.svc.GrabRelease(ctx, 9, "x", false); !errors.Is(err, movies.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestRSSSyncMatchesWantedAndAdvancesCursorAfterBatch(t *testing.T) {
	e := newEnv(t)
	e.ix.feed, e.ix.cursor = []domain.Release{rel(other, 4e9, 10), rel(good, 4e9, 10), rel(better, 6e9, 10)}, "c1"
	if err := e.svc.SyncRSS(ctx); err != nil {
		t.Fatal(err)
	}
	if len(e.q.grabs) != 1 || e.q.grabs[0].Release.Title != better || e.q.grabs[0].Origin != domain.OriginRSS {
		t.Fatalf("%+v", e.q.grabs)
	}
	if c, _ := e.st.FeedCursors().Get(ctx, "ix", domain.KindMovie); c != "c1" {
		t.Fatalf("cursor %q", c)
	}
	_ = e.svc.SyncRSS(ctx)
	if e.ix.seen[1] != "c1" {
		t.Fatalf("second sync reads from the cursor: %v", e.ix.seen)
	}
	e.ix.err, e.ix.cursor = errors.New("down"), "c2"
	if err := e.svc.SyncRSS(ctx); err == nil {
		t.Fatal("error reported")
	}
	if c, _ := e.st.FeedCursors().Get(ctx, "ix", domain.KindMovie); c != "c1" {
		t.Fatal("failed batch keeps the cursor")
	}
	e.ix.err, e.q.err = nil, errors.New("downloader gone")
	if err := e.svc.SyncRSS(ctx); err != nil {
		t.Fatal("grab failures are logged, the batch still counts")
	}
}

func TestFailureFollowUpBlocklistsThenResearches(t *testing.T) {
	e := newEnv(t)
	e.ix.rels = []domain.Release{rel(good, 4e9, 10)}
	items := []domain.EventItem{film.EventItem()}
	id := domain.ReleaseIdentity{Indexer: "ix", GUID: "g-" + good, Title: good}
	e.svc.OnFailure(ctx, domain.Event{Type: domain.EvPayloadRejected, Items: items, Data: domain.PayloadEventData{Release: id, Files: []string{"x.scr"}}})
	e.svc.Wait()
	if ok, _ := e.st.Blocklist().Has(ctx, film.Ref(), id); !ok {
		t.Fatal("blocklisted")
	}
	if e.ix.searches != 0 || len(e.rec.Of(domain.EvBlocklistAdded)) != 1 {
		t.Fatalf("payload.rejected blocklists only; the search waits for the removal: %d", e.ix.searches)
	}
	// the safety removal lands, then the re-search runs
	e.svc.OnFailure(ctx, domain.Event{Type: domain.EvDownloadRemoved, Items: items, Data: domain.DownloadEventData{Cause: "unsafe_payload", Release: id}})
	e.svc.Wait()
	if e.ix.searches != 1 || len(e.q.grabs) != 0 || len(e.rec.Of(domain.EvBlocklistAdded)) != 1 {
		t.Fatalf("re-search ran and rejected the blocklisted release: %d %d", e.ix.searches, len(e.q.grabs))
	}
	cases := []struct {
		ev     domain.Event
		search bool
	}{
		{domain.Event{Type: domain.EvDownloadFailed, Data: domain.DownloadEventData{Release: domain.ReleaseIdentity{Title: "a"}}}, true},
		{domain.Event{Type: domain.EvDownloadFailed, Data: domain.DownloadEventData{Held: true, Release: domain.ReleaseIdentity{Title: "held"}}}, false},
		{domain.Event{Type: domain.EvGrabFailed, Data: domain.GrabEventData{Release: rel("b", 1, 1), Downloader: "qb"}}, true},
		{domain.Event{Type: domain.EvGrabFailed, Data: domain.GrabEventData{Release: rel("c", 1, 1)}}, false},
		{domain.Event{Type: domain.EvDownloadRemoved, Data: domain.DownloadEventData{Cause: "user", Blocklist: true, Release: domain.ReleaseIdentity{Title: "d"}}}, false},
		{domain.Event{Type: domain.EvDownloadRemoved, Data: domain.DownloadEventData{Cause: "user", Research: true}}, true},
		{domain.Event{Type: domain.EvDownloadRemoved, Data: domain.DownloadEventData{Cause: "unsafe_payload"}}, true},
		{domain.Event{Type: domain.EvDownloadState, Data: domain.DownloadEventData{}}, false},
		{domain.Event{Type: domain.EvGrabSent, Data: "junk"}, false},
	}
	for i, c := range cases {
		before := e.ix.searches
		c.ev.Items = items
		e.svc.OnFailure(ctx, c.ev)
		e.svc.Wait()
		if (e.ix.searches > before) != c.search {
			t.Errorf("case %d: search %v", i, c.search)
		}
	}
	list, _, _ := e.st.Blocklist().List(ctx, 0, 50)
	if len(list) != 4 {
		t.Fatalf("blocklist %+v", list)
	}
}

func TestIndexMatchPageAndRemove(t *testing.T) {
	e := newEnv(t)
	second := film
	second.ID, second.Title, second.FileID = 2, "Example Movie", 0
	e.mv.m[2] = second
	e.svc.refresh(ctx, 2)
	if e.svc.Index().Len() != 2 {
		t.Fatal("two wanted")
	}
	page, next := e.svc.Index().Page(ReasonMissing, 0, 1)
	if len(page) != 1 || next != 1 {
		t.Fatalf("%v %d", page, next)
	}
	page, next = e.svc.Index().Page(ReasonMissing, next, 1)
	if len(page) != 1 || next != 0 {
		t.Fatalf("%v %d", page, next)
	}
	delete(e.mv.m, 2)
	e.svc.refresh(ctx, 2)
	if _, ok := e.svc.Index().Get(2); ok {
		t.Fatal("deleted movie leaves the index")
	}
	if got := e.svc.Index().Match(parse.ParseRelease(good)); len(got) != 1 {
		t.Fatal("match")
	}
	f, _ := e.st.Library().SaveFile(ctx, domain.LibraryFile{Item: film.Ref(), Path: "/m/x.mkv", Quality: "bluray-1080p"})
	withFile := film
	withFile.FileID = f.ID
	e.mv.m[1] = withFile
	e.svc.refresh(ctx, 1)
	if e.svc.Index().Len() != 0 {
		t.Fatal("cutoff met: not wanted")
	}
	if !strings.Contains(first([]string{"a"}), "a") || first(nil) != "" {
		t.Fatal("first")
	}
}

func modeIs(m domain.KindMode) domain.Modes {
	return func(domain.MediaKind) domain.KindMode { return m }
}

func TestShadowModeRecordsWouldBeGrabs(t *testing.T) {
	e := newEnv(t)
	e.svc.d.Modes = modeIs(domain.ModeShadow)
	e.ix.rels = []domain.Release{rel(good, 4e9, 10), rel(better, 6e9, 10)}
	sum, err := e.svc.Search(ctx, 1, domain.OriginAutomatic)
	if err != nil || len(e.q.grabs) != 0 || sum.Grabbed != "" || sum.Shadowed != better {
		t.Fatalf("shadow search must not grab: %+v %v %+v", sum, err, e.q.grabs)
	}
	sh := e.rec.Of(domain.EvDecisionShadowed)
	if len(sh) != 1 || sh[0].Data.(domain.GrabEventData).Release.Title != better || sh[0].Actor != "automation" {
		t.Fatalf("decision.shadowed %+v", sh)
	}
	if e.rec.Of(domain.EvSearchCompleted)[0].Data.(domain.SearchEventData).Grabbed != "" {
		t.Fatal("search.completed names no grab")
	}
	// a manual grab is shadowed too (no exception)
	if _, _, err := e.svc.GrabRelease(ctx, 1, rel(good, 4e9, 10).ID, true); !errors.Is(err, ErrShadowed) || len(e.q.grabs) != 0 {
		t.Fatalf("manual grab in shadow: %v", err)
	}
	if sh := e.rec.Of(domain.EvDecisionShadowed); len(sh) != 2 || sh[1].Actor != "user" {
		t.Fatalf("manual shadow %+v", sh)
	}
	// RSS decides but does not grab
	e.ix.feed = []domain.Release{rel(better, 6e9, 10)}
	if err := e.svc.SyncRSS(ctx); err != nil || len(e.q.grabs) != 0 || len(e.rec.Of(domain.EvDecisionShadowed)) != 3 {
		t.Fatalf("rss in shadow: %v %d", err, len(e.q.grabs))
	}
}

func TestDisabledModeDoesNothing(t *testing.T) {
	e := newEnv(t)
	e.svc.d.Modes = modeIs(domain.ModeDisabled)
	e.ix.rels = []domain.Release{rel(better, 6e9, 10)}
	e.ix.feed = e.ix.rels
	var me domain.ModeError
	if _, err := e.svc.Search(ctx, 1, domain.OriginAutomatic); !errors.As(err, &me) || me.Mode != domain.ModeDisabled {
		t.Fatalf("search: %v", err)
	}
	if _, err := e.svc.Interactive(ctx, 1, nopEmitter{}); !errors.As(err, &me) {
		t.Fatalf("interactive: %v", err)
	}
	if err := e.svc.SyncRSS(ctx); err != nil {
		t.Fatal(err)
	}
	e.svc.OnEvent(ctx, domain.Event{Type: domain.EvItemAdded, Items: []domain.EventItem{film.EventItem()}, Data: domain.ItemEventData{SearchNow: true}})
	e.svc.Wait()
	e.svc.OnFailure(ctx, domain.Event{Type: domain.EvDownloadFailed, Items: []domain.EventItem{film.EventItem()},
		Data: domain.DownloadEventData{Release: domain.ReleaseIdentity{Title: "x"}}})
	e.svc.Wait()
	if e.ix.searches != 0 || len(e.ix.seen) != 0 || len(e.q.grabs) != 0 || len(e.rec.Of(domain.EvDecisionShadowed)) != 0 {
		t.Fatalf("disabled kind touched indexers: searches %d feeds %d", e.ix.searches, len(e.ix.seen))
	}
}
