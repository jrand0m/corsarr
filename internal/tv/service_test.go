package tv

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

// fakeMeta serves one series whose episodes a test may change between refreshes.
type fakeMeta struct {
	mu   sync.Mutex
	rec  plugin.SeriesRecord
	err  error
	gets int
}

func (f *fakeMeta) Test(context.Context) error { return nil }

func (f *fakeMeta) SearchSeries(context.Context, string) ([]plugin.SeriesRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.rec
	r.Episodes = nil
	return []plugin.SeriesRecord{r, {IDs: map[string]string{"tmdb": "1000"}, Title: "Other"}}, f.err
}

func (f *fakeMeta) Series(_ context.Context, ids map[string]string) (plugin.SeriesRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	if f.err != nil {
		return plugin.SeriesRecord{}, f.err
	}
	if ids["tmdb"] != "77" && ids["tvdb"] != "321" {
		return plugin.SeriesRecord{}, plugin.ErrNotFound
	}
	return f.rec, nil
}

func (f *fakeMeta) set(fn func(*plugin.SeriesRecord)) { f.mu.Lock(); fn(&f.rec); f.mu.Unlock() }

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

func record() plugin.SeriesRecord {
	return plugin.SeriesRecord{IDs: map[string]string{"tmdb": "77", "tvdb": "321"}, Title: "Example Show", Year: 2023,
		Status: "continuing", RuntimeMin: 30, Episodes: []plugin.EpisodeRecord{
			{Season: 0, Number: 1, Title: "Special", AirAt: at(-500)},
			{Season: 1, Number: 1, Title: "Pilot", AirAt: at(-400)},
			{Season: 1, Number: 2, Title: "Two", AirAt: at(-393)},
			{Season: 2, Number: 1, Title: "Return", AirAt: at(-10)},
			{Season: 2, Number: 2, Title: "Soon", AirAt: at(5)},
			{Season: 2, Number: 3, Title: "TBA"},
		}}
}

type harness struct {
	svc   *Service
	meta  *fakeMeta
	pub   *recorder
	st    *store.Store
	root  string
	now   time.Time
	mode  domain.KindMode
	clock sync.Mutex
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	st, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := &harness{meta: &fakeMeta{rec: record()}, pub: &recorder{}, st: st, root: t.TempDir(), now: day0, mode: domain.ModeActive}
	h.svc = New(Deps{Repo: st.TV(), Files: st.Library(), Pub: h.pub,
		Metadata:    func() (plugin.SeriesMetadata, bool) { return h.meta, true },
		RootFolders: func() []string { return []string{h.root} },
		Modes:       func(domain.MediaKind) domain.KindMode { return h.mode },
		Now:         func() time.Time { h.clock.Lock(); defer h.clock.Unlock(); return h.now }})
	return h
}

func (h *harness) add(t *testing.T, mode string) domain.Series {
	t.Helper()
	s, created, err := h.svc.Add(context.Background(), AddRequest{TMDBID: 77, Monitor: mode, SearchNow: true})
	if err != nil || !created {
		t.Fatalf("add: %v %v", created, err)
	}
	return s
}

func TestAddAppliesModeAndPublishesOnce(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	s := h.add(t, "latestSeason")
	if s.Path != h.root+"/Example Show (2023)" || s.ProfileKey != DefaultProfileKey || !s.SeasonFolders || s.MonitorNew != domain.MonitorNewAll {
		t.Fatalf("%+v", s)
	}
	d, err := h.svc.Detail(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	var mon []string
	for _, e := range d.Episodes {
		if e.Monitored {
			mon = append(mon, e.Code()+"/"+e.Status)
		}
	}
	if !eq(mon, []string{"S02E01/missing", "S02E02/notAired", "S02E03/notAired"}) {
		t.Fatalf("monitored %v", mon)
	}
	if st := d.Series.Stats; st.SeasonCount != 2 || st.TotalEpisodeCount != 6 || st.EpisodeCount != 1 || st.NextAiring == nil {
		t.Fatalf("stats %+v", st)
	}
	added := h.pub.of(domain.EvItemAdded)
	if len(added) != 1 || !added[0].Data.(domain.ItemEventData).SearchNow || added[0].Items[0].Ref != s.Ref() {
		t.Fatalf("item.added %+v", added)
	}
	if again, created, err := h.svc.Add(ctx, AddRequest{TMDBID: 77}); err != nil || created || again.ID != s.ID {
		t.Fatal("second add must return the existing series")
	}
	if len(h.pub.of(domain.EvItemAvailable)) != 0 {
		t.Fatal("episodes aired before the add are covered by the search on add, not item.available")
	}
}

func TestAddValidation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	for _, c := range []struct {
		req   AddRequest
		field string
	}{
		{AddRequest{}, "tmdbId"},
		{AddRequest{TMDBID: 77, Monitor: "most"}, "monitor"},
		{AddRequest{TMDBID: 77, MonitorNew: "x"}, "monitorNew"},
		{AddRequest{TMDBID: 77, SeriesType: "x"}, "seriesType"},
		{AddRequest{TMDBID: 77, ProfileKey: "x"}, "profileKey"},
		{AddRequest{TMDBID: 77, RootFolder: "/elsewhere"}, "rootFolder"},
	} {
		var fe FieldError
		if _, created, err := h.svc.Add(ctx, c.req); !errors.As(err, &fe) || fe.Field != c.field || created {
			t.Errorf("%+v: %v", c.req, err)
		}
	}
	h.svc.d.Metadata = func() (plugin.SeriesMetadata, bool) { return nil, false }
	if _, created, err := h.svc.Add(ctx, AddRequest{TMDBID: 77}); !errors.Is(err, ErrNoMetadata) || created {
		t.Fatal(err)
	}
	if _, err := h.svc.Lookup(ctx, "x"); !errors.Is(err, ErrNoMetadata) {
		t.Fatal(err)
	}
}

func TestLookupMarksLibrarySeries(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	s := h.add(t, "all")
	res, err := h.svc.Lookup(ctx, "example")
	if err != nil || len(res) != 2 || res[0].LibraryID != s.ID || res[1].LibraryID != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	byID, err := h.svc.Lookup(ctx, "tvdb:321")
	if err != nil || len(byID) != 1 || len(byID[0].Episodes) != 0 {
		t.Fatalf("by id %+v %v", byID, err)
	}
	if none, err := h.svc.Lookup(ctx, "imdb:tt404"); err != nil || len(none) != 0 {
		t.Fatalf("unknown id %+v %v", none, err)
	}
}

func TestUpdateAndToggles(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	s := h.add(t, "none")
	off, prof, newNone, folders, typ, tags, mode := false, "uhd", "none", false, "daily", []string{"kids"}, "all"
	if _, err := h.svc.Update(ctx, s.ID, Patch{ProfileKey: &prof}); err == nil {
		t.Fatal("unknown profile accepted")
	}
	prof = "ultrahd"
	got, err := h.svc.Update(ctx, s.ID, Patch{Monitored: &off, ProfileKey: &prof, MonitorNew: &newNone, SeasonFolders: &folders,
		SeriesType: &typ, Tags: &tags, Monitor: &mode})
	if err != nil || got.Monitored || got.ProfileKey != "ultrahd" || got.MonitorNew != domain.MonitorNewNone || got.SeasonFolders ||
		got.Type != domain.SeriesDaily || got.Tags[0] != "kids" {
		t.Fatalf("%+v %v", got, err)
	}
	upd := h.pub.of(domain.EvItemUpdated)
	if len(upd) != 1 || len(upd[0].Data.(domain.ItemEventData).Changed) != 7 {
		t.Fatalf("item.updated %+v", upd)
	}
	if _, err := h.svc.Update(ctx, s.ID, Patch{}); err != nil || len(h.pub.of(domain.EvItemUpdated)) != 1 {
		t.Fatal("an empty patch changes nothing")
	}
	d, _ := h.svc.Detail(ctx, s.ID)
	if n := countMonitored(d); n != 5 {
		t.Fatalf("mode all monitors every regular episode: %d", n)
	}
	if err := h.svc.SetSeasonMonitored(ctx, s.ID, 1, false); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.SetSeasonMonitored(ctx, s.ID, 9, false); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	d, _ = h.svc.Detail(ctx, s.ID)
	if countMonitored(d) != 3 || d.Seasons[1].Monitored {
		t.Fatalf("season 1 off: %+v", d.Seasons)
	}
	n, err := h.svc.SetEpisodesMonitored(ctx, []int64{d.Episodes[1].ID, d.Episodes[0].ID}, true)
	if err != nil || n != 2 {
		t.Fatalf("episode toggles %d %v", n, err)
	}
	if _, err := h.svc.SetEpisodesMonitored(ctx, []int64{999}, true); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	for _, bad := range []Patch{{MonitorNew: ptr("x")}, {SeriesType: ptr("x")}, {Monitor: ptr("x")}} {
		if _, err := h.svc.Update(ctx, s.ID, bad); err == nil {
			t.Errorf("bad patch %+v accepted", bad)
		}
	}
	if _, err := h.svc.Update(ctx, 999, Patch{}); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func ptr[T any](v T) *T { return &v }

func countMonitored(d Detail) int {
	n := 0
	for _, e := range d.Episodes {
		if e.Monitored {
			n++
		}
	}
	return n
}

func TestRefreshAddsNewEpisodesAndKeepsFiles(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	s := h.add(t, "all")
	d, _ := h.svc.Detail(ctx, s.ID)
	f, err := h.st.Library().SaveFile(ctx, domain.LibraryFile{Item: d.Episodes[1].Ref(), Path: "/tv/a.mkv", Size: 100, Quality: "webdl-1080p", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.svc.SetEpisodeFile(ctx, []int64{d.Episodes[1].ID}, f.ID); err != nil {
		t.Fatal(err)
	}
	h.meta.set(func(r *plugin.SeriesRecord) {
		r.Title = "Example Show!"
		r.Episodes = append(r.Episodes[2:], plugin.EpisodeRecord{Season: 2, Number: 4, Title: "New"}, plugin.EpisodeRecord{Season: 3, Number: 1})
	})
	res, err := h.svc.Refresh(ctx, s.ID)
	if err != nil || res.Added != 2 || res.Removed != 1 || res.Orphaned != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	again, _ := h.svc.Refresh(ctx, s.ID)
	if again.Added != 0 || again.Removed != 0 || again.Orphaned != 0 {
		t.Fatalf("refresh must be idempotent: %+v", again)
	}
	d, _ = h.svc.Detail(ctx, s.ID)
	byCode := map[string]EpisodeView{}
	for _, e := range d.Episodes {
		byCode[e.Code()] = e
	}
	if e := byCode["S01E01"]; e.InMetadata || e.File == nil || e.File.Quality != "WEBDL-1080p" || e.Status != StatusDownloaded {
		t.Fatalf("kept file episode %+v", e)
	}
	if !byCode["S02E04"].Monitored || !byCode["S03E01"].Monitored || d.Series.Title != "Example Show!" {
		t.Fatalf("new episodes follow the rules: %+v", byCode)
	}
	if d.Series.Stats.SizeOnDisk != 100 || d.Series.Stats.EpisodeFileCount != 1 {
		t.Fatalf("stats %+v", d.Series.Stats)
	}
}

func TestRefreshAllSkipsFreshAndSurvivesFailures(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.add(t, "all")
	before := h.meta.gets
	if err := h.svc.RefreshAll(ctx); err != nil || h.meta.gets != before {
		t.Fatalf("fresh series refreshed: %v %d", err, h.meta.gets-before)
	}
	h.clock.Lock()
	h.now = h.now.Add(25 * time.Hour)
	h.clock.Unlock()
	h.meta.err = errors.New("down")
	if err := h.svc.RefreshAll(ctx); err == nil {
		t.Fatal("provider failure must surface for the job's back-off")
	}
	h.meta.err = nil
	if err := h.svc.RefreshAll(ctx); err != nil || h.meta.gets != before+2 {
		t.Fatalf("stale series refreshed: %v %d", err, h.meta.gets-before)
	}
}

func TestDeleteRespectsModes(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	s := h.add(t, "all")
	h.mode = domain.ModeShadow
	var me domain.ModeError
	if err := h.svc.Delete(ctx, s.ID, true); !errors.As(err, &me) {
		t.Fatalf("shadow mode must refuse deleting files: %v", err)
	}
	h.mode = domain.ModeActive
	d, _ := h.svc.Detail(ctx, s.ID)
	_ = h.svc.SetEpisodeFile(ctx, []int64{d.Episodes[1].ID}, 5)
	if err := h.svc.Delete(ctx, s.ID, true); err != nil {
		t.Fatal(err)
	}
	del := h.pub.of(domain.EvItemDeleted)
	if len(del) != 1 || len(del[0].Items) != 2 || !del[0].Data.(domain.ItemEventData).FilesDeleted {
		t.Fatalf("item.deleted names the series and its episodes with files: %+v", del)
	}
	if err := h.svc.Delete(ctx, s.ID, false); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestAvailabilityTimerPublishesOncePerEpisode(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.svc.Start(ctx)
	defer h.svc.Stop()
	h.add(t, "all")
	h.clock.Lock()
	h.now = day0.AddDate(0, 0, 6)
	h.clock.Unlock()
	h.svc.Kick()
	h.svc.Kick()
	av := h.pub.of(domain.EvItemAvailable)
	if len(av) != 1 || av[0].Items[0].Title != "Example Show S02E02" || av[0].Data.(domain.ItemEventData).Reason != "aired" {
		t.Fatalf("item.available %+v", av)
	}
	cal, err := h.svc.Upcoming(ctx, day0, day0.AddDate(0, 0, 10))
	if err != nil || len(cal) != 1 || cal[0].Series.Title != "Example Show" {
		t.Fatalf("calendar %+v %v", cal, err)
	}
	if it := h.svc.EpisodeItem(ctx, 999); it.Title != "" || it.Ref.ID != 999 {
		t.Fatalf("unknown episode item %+v", it)
	}
}

func TestAddMonitorsTheChosenSeasonsOrReportsTheMetadataError(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.meta.err = errors.New("metadata down")
	if _, created, err := h.svc.Add(ctx, AddRequest{TMDBID: 77}); err == nil || err.Error() != "metadata down" || created {
		t.Fatalf("%v %v", created, err)
	}
	h.meta.err = nil
	s, created, err := h.svc.Add(ctx, AddRequest{TMDBID: 77, Monitor: "none", Seasons: []int{1}})
	if err != nil || !created {
		t.Fatal(err)
	}
	d, _ := h.svc.Detail(ctx, s.ID)
	var mon []string
	for _, e := range d.Episodes {
		if e.Monitored {
			mon = append(mon, e.Code())
		}
	}
	if !eq(mon, []string{"S01E01", "S01E02"}) {
		t.Fatalf("monitored %v", mon)
	}
}
