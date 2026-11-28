package tvauto

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/queue"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/tv"
)

var (
	ctx  = context.Background()
	day0 = time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
)

func at(days int) *time.Time { t := day0.AddDate(0, 0, days); return &t }

type meta struct{ rec plugin.SeriesRecord }

func (m meta) Test(context.Context) error { return nil }
func (m meta) SearchSeries(context.Context, string) ([]plugin.SeriesRecord, error) {
	return []plugin.SeriesRecord{m.rec}, nil
}
func (m meta) Series(context.Context, map[string]string) (plugin.SeriesRecord, error) {
	return m.rec, nil
}

// show: season 1 fully aired (4 episodes), season 2 with one aired and one future episode.
func show() plugin.SeriesRecord {
	r := plugin.SeriesRecord{IDs: map[string]string{"tmdb": "77", "tvdb": "321"}, Title: "Example Show", Year: 2023, RuntimeMin: 30}
	for n := 1; n <= 4; n++ {
		r.Episodes = append(r.Episodes, plugin.EpisodeRecord{Season: 1, Number: n, AirAt: at(-300 + 7*n)})
	}
	r.Episodes = append(r.Episodes, plugin.EpisodeRecord{Season: 2, Number: 1, AirAt: at(-3)}, plugin.EpisodeRecord{Season: 2, Number: 2, AirAt: at(4)})
	return r
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

// QueuedQualities: an episode in a sent grab is queued at its quality.
func (q *fakeQueue) QueuedQualities(_ context.Context, item domain.ItemRef) ([]string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, g := range q.grabs {
		for _, it := range g.Items {
			if it.Ref == item {
				return []string{g.Decision.Quality}, nil
			}
		}
	}
	return nil, nil
}

func (q *fakeQueue) titles() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []string
	for _, g := range q.grabs {
		out = append(out, g.Release.Title)
	}
	return out
}

// fakeIndexer answers by request: season requests get packs, episode
// requests single episodes; titles listed in feed come from RSS.
type fakeIndexer struct {
	mu       sync.Mutex
	bySeason map[int][]string
	byEp     map[string][]string
	feed     []string
	feeds    int
	cursor   string
	reqs     []domain.SearchRequest
	err      error
}

func release(title string) domain.Release {
	return domain.Release{ID: title, Title: title, Protocol: domain.ProtoTorrent, Size: 900_000_000, Seeders: 30,
		PublishedAt: day0.AddDate(0, 0, -1), Indexers: []domain.IndexerRef{{Key: "ix"}}, InfoHash: title}
}

func (f *fakeIndexer) Test(context.Context) error                { return nil }
func (f *fakeIndexer) Caps(context.Context) (plugin.Caps, error) { return plugin.Caps{}, nil }
func (f *fakeIndexer) Search(_ context.Context, r domain.SearchRequest) ([]domain.Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, r)
	var titles []string
	if len(r.Episodes) == 0 {
		titles = f.bySeason[r.Season]
	} else {
		titles = f.byEp[r.Text]
	}
	out := make([]domain.Release, len(titles))
	for i, t := range titles {
		out[i] = release(t)
	}
	return out, f.err
}

func (f *fakeIndexer) Feed(context.Context, domain.MediaKind, string) ([]domain.Release, string, error) {
	f.feeds++
	out := make([]domain.Release, len(f.feed))
	for i, t := range f.feed {
		out[i] = release(t)
	}
	return out, f.cursor, f.err
}

func (f *fakeIndexer) Resolve(context.Context, domain.Release) (plugin.Payload, error) {
	return plugin.Payload{}, nil
}

func (f *fakeIndexer) texts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, r := range f.reqs {
		out = append(out, r.Text)
	}
	return out
}

type bus struct {
	mu  sync.Mutex
	evs []domain.Event
}

func (b *bus) Publish(ev domain.Event) (domain.Event, error) {
	b.mu.Lock()
	b.evs = append(b.evs, ev)
	b.mu.Unlock()
	return ev, nil
}

func (b *bus) has(t domain.EventType) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ev := range b.evs {
		if ev.Type == t {
			return true
		}
	}
	return false
}

type cursors struct{ m map[string]string }

func (c *cursors) Get(_ context.Context, ix domain.ObjectKey, k domain.MediaKind) (string, error) {
	return c.m[string(ix)+string(k)], nil
}
func (c *cursors) Set(_ context.Context, ix domain.ObjectKey, k domain.MediaKind, v string) error {
	c.m[string(ix)+string(k)] = v
	return nil
}

type env struct {
	tv   *tv.Service
	svc  *Service
	q    *fakeQueue
	ix   *fakeIndexer
	pub  *bus
	st   *store.Store
	mode domain.KindMode
	cur  *cursors
}

func newEnv(t *testing.T) *env {
	t.Helper()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	root := t.TempDir()
	e := &env{q: &fakeQueue{}, ix: &fakeIndexer{bySeason: map[int][]string{}, byEp: map[string][]string{}}, pub: &bus{}, st: st,
		mode: domain.ModeActive, cur: &cursors{m: map[string]string{}}}
	modes := domain.Modes(func(domain.MediaKind) domain.KindMode { return e.mode })
	now := func() time.Time { return day0 }
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e.tv = tv.New(tv.Deps{Repo: st.TV(), Files: st.Library(), Pub: e.pub, Modes: modes, Now: now, Log: log,
		Metadata:    func() (plugin.SeriesMetadata, bool) { return meta{show()}, true },
		RootFolders: func() []string { return []string{root} }})
	e.svc = New(Deps{TV: e.tv, Files: st.Library(), Queue: e.q, Blocklist: st.Blocklist(), Cursors: e.cur,
		Indexers: func() []automation.Indexer { return []automation.Indexer{{Key: "ix", Impl: e.ix}} },
		Pub:      e.pub, Modes: modes, Searches: st.Searches(), Now: now, Log: log})
	return e
}

// add adds the show and feeds item.added to the automation like the bus does.
func (e *env) add(t *testing.T, searchNow bool) domain.Series {
	t.Helper()
	s, _, err := e.tv.Add(ctx, tv.AddRequest{TMDBID: 77, Monitor: "all", SearchNow: searchNow})
	if err != nil {
		t.Fatal(err)
	}
	e.svc.OnEvent(ctx, domain.Event{Type: domain.EvItemAdded, Items: []domain.EventItem{s.EventItem()},
		Data: domain.ItemEventData{SearchNow: searchNow}})
	return s
}

func TestWantedIndexAndRebuild(t *testing.T) {
	e := newEnv(t)
	e.add(t, false)
	if n := e.svc.Index().Len(); n != 5 {
		t.Fatalf("aired monitored episodes are wanted: %d", n)
	}
	fresh := New(Deps{TV: e.tv, Files: e.st.Library(), Now: func() time.Time { return day0 }})
	if err := fresh.Rebuild(ctx); err != nil || fresh.Index().Len() != 5 {
		t.Fatalf("rebuild %v %d", err, fresh.Index().Len())
	}
	page, next := e.svc.Index().Page(automation.ReasonMissing, 0, 2)
	if len(page) != 2 || next != page[1].Episode.ID || page[0].Series.Title != "Example Show" {
		t.Fatalf("page %+v next %d", page, next)
	}
}

func TestSearchOnAddPrefersSeasonPackAndLinksEveryEpisode(t *testing.T) {
	e := newEnv(t)
	e.ix.bySeason[1] = []string{"Example.Show.S01.1080p.WEB-DL.x264-GRP"}
	e.ix.byEp["Example Show S02E01"] = []string{"Example.Show.S02E01.1080p.WEB-DL.x264-GRP"}
	e.add(t, true)
	if got := e.q.titles(); len(got) != 2 || got[0] != "Example.Show.S01.1080p.WEB-DL.x264-GRP" {
		t.Fatalf("grabs %v (searches %v)", got, e.ix.texts())
	}
	if pack := e.q.grabs[0]; pack.Kind != domain.KindTV || len(pack.Items) != 4 || pack.Items[0].Series != "Example Show" {
		t.Fatalf("one pack grab linked to all four episodes: %+v", pack.Items)
	}
	for _, txt := range e.ix.texts() {
		if strings.HasPrefix(txt, "Example Show S01E") {
			t.Fatalf("a grabbed pack must not be followed by episode searches: %v", e.ix.texts())
		}
	}
}

func TestSeasonFallsBackToEpisodes(t *testing.T) {
	e := newEnv(t)
	e.add(t, false)
	e.ix.byEp["Example Show S01E02"] = []string{"Example.Show.S01E02.1080p.HDTV.x264-GRP"}
	sum, err := e.svc.SearchSeason(ctx, 1, 1, domain.OriginAutomatic)
	if err != nil || sum.Searches != 5 || sum.Grabbed != "Example.Show.S01E02.1080p.HDTV.x264-GRP" {
		t.Fatalf("%+v %v", sum, err)
	}
}

func TestEpisodeSearchAndInteractiveAndManualGrab(t *testing.T) {
	e := newEnv(t)
	s := e.add(t, false)
	title := "Example.Show.S02E01.1080p.WEB-DL.x264-GRP"
	e.ix.byEp["Example Show S02E01"] = []string{title, "Other.Show.S02E01.1080p.WEB-DL.x264-GRP"}
	eps, _ := e.tv.Episodes(ctx, s.ID)
	s2e1 := eps[4]
	var rec emitter
	sum, err := e.svc.InteractiveEpisode(ctx, s2e1.ID, &rec)
	if err != nil || sum.Releases != 2 || sum.Accepted != 1 || len(e.q.grabs) != 0 {
		t.Fatalf("interactive %+v %v", sum, err)
	}
	if a := sum.Automation; a == nil || a.Pick != title || a.Outcomes[title].Status != automation.OutcomeGrab || len(sum.Rejections) != 1 {
		t.Fatalf("automation verdict %+v %+v", a, sum.Rejections)
	}
	if _, _, err := e.svc.GrabRelease(ctx, s.ID, "Other.Show.S02E01.1080p.WEB-DL.x264-GRP", false); !errors.Is(err, automation.ErrNeedsConfirm) {
		t.Fatalf("another show needs confirmation: %v", err)
	}
	if _, _, err := e.svc.GrabRelease(ctx, s.ID, "nope", false); !errors.Is(err, automation.ErrUnknownRelease) {
		t.Fatal(err)
	}
	g, d, err := e.svc.GrabRelease(ctx, s.ID, title, false)
	if err != nil || g.ID != 1 || d.Decision.ItemRefs[0] != s2e1.Ref() {
		t.Fatalf("manual grab %+v %v", d, err)
	}
	// queued now: an automatic search does not grab it again
	if sum, _ := e.svc.SearchEpisode(ctx, s2e1.ID, domain.OriginAutomatic); sum.Grabbed != "" {
		t.Fatalf("grabbed twice: %+v", sum)
	}
	if st, err := e.st.Searches().State(ctx, s2e1.Ref()); err != nil || st.LastResult == nil || len(st.LastResult.Rejections) == 0 {
		t.Fatalf("the episode keeps its last search and why releases were rejected: %+v %v", st, err)
	}
	if _, err := e.svc.InteractiveSeason(ctx, s.ID, 9, &rec); !errors.Is(err, tv.ErrNotFound) {
		t.Fatal(err)
	}
	if sum, err := e.svc.InteractiveSeason(ctx, s.ID, 2, &rec); err != nil || sum.Indexers != 1 {
		t.Fatal(err)
	}
}

func TestShadowAndDisabledModes(t *testing.T) {
	e := newEnv(t)
	e.mode = domain.ModeShadow
	e.ix.bySeason[1] = []string{"Example.Show.S01.1080p.WEB-DL.x264-GRP"}
	e.add(t, true)
	if len(e.q.grabs) != 0 || !e.pub.has(domain.EvDecisionShadowed) {
		t.Fatalf("shadow mode records instead of grabbing: %v", e.q.titles())
	}
	e.mode = domain.ModeDisabled
	var me domain.ModeError
	for _, err := range []error{
		func() error { _, err := e.svc.SearchSeries(ctx, 1, domain.OriginAutomatic); return err }(),
		func() error { _, err := e.svc.SearchEpisode(ctx, 1, domain.OriginAutomatic); return err }(),
		func() error { _, err := e.svc.SearchSeason(ctx, 1, 1, domain.OriginAutomatic); return err }(),
		func() error { _, err := e.svc.InteractiveEpisode(ctx, 1, automation.NopEmitter); return err }(),
		func() error { _, err := e.svc.InteractiveSeason(ctx, 1, 1, automation.NopEmitter); return err }(),
	} {
		if !errors.As(err, &me) {
			t.Errorf("disabled kind must refuse: %v", err)
		}
	}
	if err := e.svc.SyncRSS(ctx); err != nil || e.ix.feeds != 0 {
		t.Fatalf("disabled kind reads no feeds: %v", err)
	}
}

func TestRSSGrabsLargestCoverFirstAndAdvancesCursor(t *testing.T) {
	e := newEnv(t)
	e.add(t, false)
	e.ix.feed = []string{"Example.Show.S01E01.1080p.WEB-DL.x264-GRP", "Example.Show.S01.1080p.WEB-DL.x264-GRP",
		"Unknown.Show.S01E01.1080p.WEB-DL.x264-GRP", "Example.Show.S02E02.1080p.WEB-DL.x264-GRP"}
	e.ix.cursor = "c2"
	if err := e.svc.SyncRSS(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.q.titles(); len(got) != 1 || got[0] != "Example.Show.S01.1080p.WEB-DL.x264-GRP" {
		t.Fatalf("the pack wins, the overlapping episode and unaired one are skipped: %v", got)
	}
	if e.cur.m["ixtv"] != "c2" {
		t.Fatalf("cursor %v", e.cur.m)
	}
	e.ix.err = errors.New("down")
	if err := e.svc.SyncRSS(ctx); err == nil || e.cur.m["ixtv"] != "c2" {
		t.Fatal("a failed feed keeps its cursor")
	}
}

func TestAiredEpisodeAndFailureResearch(t *testing.T) {
	e := newEnv(t)
	s := e.add(t, false)
	eps, _ := e.tv.Episodes(ctx, s.ID)
	e.ix.byEp["Example Show S02E01"] = []string{"Example.Show.S02E01.1080p.WEB-DL.x264-GRP"}
	e.svc.OnEvent(ctx, domain.Event{Type: domain.EvItemAvailable, Items: []domain.EventItem{eps[4].EventItem("Example Show")}})
	if got := e.q.titles(); len(got) != 1 {
		t.Fatalf("search on air: %v", got)
	}
	e.ix.bySeason[1] = []string{"Example.Show.S01.1080p.WEBRip.x264-GRP"}
	var items []domain.EventItem
	for _, ep := range eps[:2] {
		items = append(items, ep.EventItem("Example Show"))
	}
	e.svc.OnFailure(ctx, domain.Event{Type: domain.EvDownloadFailed, Items: items, Data: domain.DownloadEventData{}})
	if got := e.q.titles(); len(got) != 2 || got[1] != "Example.Show.S01.1080p.WEBRip.x264-GRP" {
		t.Fatalf("two failed episodes of one season are searched as the season: %v", got)
	}
	before := len(e.ix.reqs)
	e.svc.OnFailure(ctx, domain.Event{Type: domain.EvDownloadState, Items: items})
	if len(e.ix.reqs) != before {
		t.Fatal("a non-failure event searches nothing")
	}
	e.svc.OnEvent(ctx, domain.Event{Type: domain.EvItemDeleted, Items: []domain.EventItem{s.EventItem()}})
	if err := e.tv.Delete(ctx, s.ID, false); err != nil {
		t.Fatal(err)
	}
	e.svc.OnEvent(ctx, domain.Event{Type: domain.EvItemDeleted, Items: []domain.EventItem{s.EventItem(), {Ref: domain.ItemRef{Kind: domain.KindMovie, ID: 1}}}})
	if e.svc.Index().Len() != 0 {
		t.Fatal("deleted series leave the index")
	}
}

func TestPlan(t *testing.T) {
	var eps []domain.Episode
	for n := 1; n <= 10; n++ {
		eps = append(eps, domain.Episode{ID: int64(n), Season: 1, Number: n, AirAt: at(-100), Monitored: true})
	}
	eps = append(eps, domain.Episode{ID: 11, Season: 2, Number: 1, AirAt: at(-10), Monitored: true},
		domain.Episode{ID: 12, Season: 2, Number: 2, AirAt: at(10), Monitored: true},
		domain.Episode{ID: 13, Season: 0, Number: 1, AirAt: at(-10)})
	wanted := func(ids ...int64) map[int64]bool {
		m := map[int64]bool{}
		for _, id := range ids {
			m[id] = true
		}
		return m
	}
	steps := Plan(domain.Series{}, eps, wanted(1, 2, 3, 4, 5, 6, 7, 8, 11, 13), day0, DefaultPackThreshold)
	if len(steps) != 3 || steps[0].Season != 0 || steps[0].Pack || !steps[1].Pack || len(steps[1].Episodes) != 8 || steps[2].Pack {
		t.Fatalf("8 of 10 missing → pack; unfinished season → episodes: %+v", steps)
	}
	if steps := Plan(domain.Series{}, eps, wanted(1, 2), day0, DefaultPackThreshold); steps[0].Pack {
		t.Fatalf("2 of 10 missing → episodes: %+v", steps)
	}
}

func TestSummaryMerge(t *testing.T) {
	var s Summary
	s.add(automation.Summary{Indexers: 1, Grabbed: "a"})
	var o Summary
	o.add(automation.Summary{Indexers: 2, Shadowed: "b"})
	o.add(automation.Summary{Indexers: 1})
	s.merge(o)
	if s.Searches != 3 || s.Indexers != 4 || s.Grabbed != "a" || s.Shadowed != "b" {
		t.Fatalf("%+v", s)
	}
}

func TestWantedReason(t *testing.T) {
	ser := domain.Series{Monitored: true, ProfileKey: tv.DefaultProfileKey}
	ep := domain.Episode{Monitored: true, AirAt: at(-1)}
	if WantedReason(ser, ep, nil, day0) != automation.ReasonMissing {
		t.Fatal("missing")
	}
	if r := WantedReason(ser, ep, cur("hdtv-720p"), day0); r != automation.ReasonCutoffUnmet {
		t.Fatalf("cutoff %q", r)
	}
	if r := WantedReason(ser, ep, cur("bluray-1080p"), day0); r != automation.ReasonNotWanted {
		t.Fatalf("met %q", r)
	}
	ser.Monitored = false
	if WantedReason(ser, ep, nil, day0) != automation.ReasonNotWanted {
		t.Fatal("unmonitored series")
	}
}

func cur(q string) *decide.CurrentFile {
	return &decide.CurrentFile{Quality: decide.QualityID(q), Revision: 1}
}

type emitter struct{ records, errors int }

func (e *emitter) Record(any) error          { e.records++; return nil }
func (e *emitter) Error(string, error) error { e.errors++; return nil }

func TestSearchWeighsAReleaseAgainstTheEpisodesFile(t *testing.T) {
	e := newEnv(t)
	s := e.add(t, false)
	eps, _ := e.tv.Episodes(ctx, s.ID)
	s2e1 := eps[4]
	f, err := e.st.Library().SaveFile(ctx, domain.LibraryFile{Item: s2e1.Ref(), Path: "/tv/s2e1.mkv", Quality: "bluray-1080p", Revision: 1, AddedAt: day0})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.tv.SetEpisodeFile(ctx, []int64{s2e1.ID}, f.ID); err != nil {
		t.Fatal(err)
	}
	e.ix.byEp["Example Show S02E01"] = []string{"Example.Show.S02E01.1080p.WEB-DL.x264-GRP"}
	var rec emitter
	sum, err := e.svc.InteractiveEpisode(ctx, s2e1.ID, &rec)
	if err != nil || sum.Releases != 1 || sum.Accepted != 0 {
		t.Fatalf("a worse release than the file on disk is no upgrade: %+v %v", sum, err)
	}
}
