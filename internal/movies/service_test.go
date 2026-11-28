package movies

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/testkit"
)

var ctx = context.Background()

type fakeMeta struct {
	recs  map[string]plugin.MovieRecord
	err   error
	calls int // Movie calls
}

func (f *fakeMeta) Test(context.Context) error { return nil }

func (f *fakeMeta) SearchMovies(_ context.Context, q string, _ int) ([]plugin.MovieRecord, error) {
	var out []plugin.MovieRecord
	for _, r := range f.recs {
		if strings.Contains(strings.ToLower(r.Title), strings.ToLower(q)) {
			out = append(out, r)
		}
	}
	return out, f.err
}

func (f *fakeMeta) Movie(_ context.Context, ids map[string]string) (plugin.MovieRecord, error) {
	f.calls++
	if f.err != nil {
		return plugin.MovieRecord{}, f.err
	}
	for _, r := range f.recs {
		if r.IDs["tmdb"] == ids["tmdb"] || (ids["imdb"] != "" && r.IDs["imdb"] == ids["imdb"]) {
			return r, nil
		}
	}
	return plugin.MovieRecord{}, fmt.Errorf("x: %w", plugin.ErrNotFound)
}

type env struct {
	svc  *Service
	st   *store.Store
	rec  *testkit.Recorder
	meta *fakeMeta
	root string
	now  time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{st: testkit.Store(t), rec: &testkit.Recorder{}, root: t.TempDir(), now: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)}
	e.meta = &fakeMeta{recs: map[string]plugin.MovieRecord{
		"603": {IDs: map[string]string{"tmdb": "603", "imdb": "tt0133093"}, Title: "The Matrix", Year: 1999, RuntimeMin: 136, Digital: day(1999, 9, 1),
			PosterURL: "https://img.example/603.jpg", FanartURL: "https://img.example/603b.jpg"},
		"700": {IDs: map[string]string{"tmdb": "700"}, Title: "Future Film", Year: 2024, InCinemas: day(2024, 6, 10)},
	}}
	e.svc = New(Deps{Repo: e.st.Movies(), Pub: e.rec, RootFolders: func() []string { return []string{e.root} },
		Metadata: func() (plugin.MovieMetadata, bool) { return e.meta, e.meta != nil }, Cache: e.st.Metadata(), Now: func() time.Time { return e.now }})
	return e
}

func TestAddPublishesItemAddedOnceAndPlansFolder(t *testing.T) {
	e := newEnv(t)
	m, created, err := e.svc.Add(ctx, AddRequest{TMDBID: 603, SearchNow: true})
	if err != nil || !created || m.Path != e.root+"/The Matrix (1999)" || m.ProfileKey != DefaultProfileKey || !m.Monitored || m.IMDBID != "tt0133093" {
		t.Fatalf("%+v %v", m, err)
	}
	again, created, err := e.svc.Add(ctx, AddRequest{TMDBID: 603})
	if err != nil || created || again.ID != m.ID {
		t.Fatal("idempotent add")
	}
	added := e.rec.Of(domain.EvItemAdded)
	if len(added) != 1 || !added[0].Data.(domain.ItemEventData).SearchNow || added[0].Items[0].Ref != m.Ref() {
		t.Fatalf("%+v", added)
	}
	// Already available at add: no item.available later.
	if next, _ := e.svc.CheckAvailability(ctx); !next.IsZero() || len(e.rec.Of(domain.EvItemAvailable)) != 0 {
		t.Fatal("available at add must not publish item.available")
	}
}

func TestAddValidation(t *testing.T) {
	e := newEnv(t)
	no := false
	for name, req := range map[string]AddRequest{
		"tmdbId":          {},
		"profileKey":      {TMDBID: 603, ProfileKey: "nope"},
		"rootFolder":      {TMDBID: 603, RootFolder: "/elsewhere"},
		"minAvailability": {TMDBID: 603, MinAvailability: "soon"},
	} {
		var fe FieldError
		if _, created, err := e.svc.Add(ctx, req); !errors.As(err, &fe) || fe.Field != name || created {
			t.Errorf("%s: %v", name, err)
		}
	}
	e.svc.d.RootFolders = func() []string { return []string{e.root + "/missing"} }
	if _, _, err := e.svc.Add(ctx, AddRequest{TMDBID: 603, Monitored: &no}); err == nil {
		t.Error("inaccessible root folder")
	}
	e.svc.d.RootFolders = func() []string { return []string{e.root} }
	if _, created, err := e.svc.Add(ctx, AddRequest{TMDBID: 999}); !errors.Is(err, plugin.ErrNotFound) || created {
		t.Errorf("unknown tmdb id: %v", err)
	}
	e.meta = nil
	if _, _, err := e.svc.Add(ctx, AddRequest{TMDBID: 603}); !errors.Is(err, ErrNoMetadata) {
		t.Errorf("no metadata: %v", err)
	}
	if _, err := e.svc.Lookup(ctx, "x"); !errors.Is(err, ErrNoMetadata) {
		t.Errorf("lookup without metadata: %v", err)
	}
}

func TestLookup(t *testing.T) {
	e := newEnv(t)
	if recs, err := e.svc.Lookup(ctx, "matrix"); err != nil || len(recs) != 1 {
		t.Fatalf("%v %v", recs, err)
	}
	if recs, err := e.svc.Lookup(ctx, "imdb:tt0133093"); err != nil || len(recs) != 1 || recs[0].Year != 1999 {
		t.Fatalf("%v %v", recs, err)
	}
	if recs, err := e.svc.Lookup(ctx, "tmdb:1"); err != nil || len(recs) != 0 {
		t.Fatalf("not found is empty: %v %v", recs, err)
	}
}

func TestAvailabilityTimerPublishesOnceAtTheDate(t *testing.T) {
	e := newEnv(t)
	m, _, err := e.svc.Add(ctx, AddRequest{TMDBID: 700, MinAvailability: "inCinemas"})
	if err != nil {
		t.Fatal(err)
	}
	next, err := e.svc.CheckAvailability(ctx)
	if err != nil || !next.Equal(*day(2024, 6, 10)) || len(e.rec.Of(domain.EvItemAvailable)) != 0 {
		t.Fatalf("next %v %v", next, err)
	}
	e.now = time.Date(2024, 6, 10, 0, 0, 1, 0, time.UTC)
	if _, err := e.svc.CheckAvailability(ctx); err != nil {
		t.Fatal(err)
	}
	_, _ = e.svc.CheckAvailability(ctx)
	av := e.rec.Of(domain.EvItemAvailable)
	if len(av) != 1 || av[0].Items[0].Ref != m.Ref() || av[0].Data.(domain.ItemEventData).Reason != "min_availability" {
		t.Fatalf("exactly one item.available: %+v", av)
	}
}

func TestTimerFiresFromStart(t *testing.T) {
	e := newEnv(t)
	e.now = time.Now()
	soon := time.Now().Add(50 * time.Millisecond)
	e.meta.recs["800"] = plugin.MovieRecord{IDs: map[string]string{"tmdb": "800"}, Title: "Soon", Year: 2030, InCinemas: &soon}
	c, cancel := context.WithCancel(ctx)
	defer cancel()
	e.svc.Start(c)
	defer e.svc.Stop()
	e.svc.d.Now = time.Now
	if _, _, err := e.svc.Add(ctx, AddRequest{TMDBID: 800, MinAvailability: "inCinemas"}); err != nil {
		t.Fatal(err)
	}
	testkit.Eventually(t, 3*time.Second, "item.available from the timer", func() bool { return len(e.rec.Of(domain.EvItemAvailable)) == 1 })
	cancel()
	e.svc.Kick() // a stopped context arms nothing
}

func TestUpdateDeleteSetFile(t *testing.T) {
	e := newEnv(t)
	m, _, _ := e.svc.Add(ctx, AddRequest{TMDBID: 603})
	off, prof, avail := false, "any", "announced"
	got, err := e.svc.Update(ctx, m.ID, Patch{Monitored: &off, ProfileKey: &prof, MinAvailability: &avail})
	if err != nil || got.Monitored || got.ProfileKey != "any" || got.MinAvailability != domain.AvailAnnounced {
		t.Fatalf("%+v %v", got, err)
	}
	up := e.rec.Of(domain.EvItemUpdated)
	if len(up) != 1 || len(up[0].Data.(domain.ItemEventData).Changed) != 3 {
		t.Fatalf("%+v", up)
	}
	if _, err := e.svc.Update(ctx, m.ID, Patch{Monitored: &off}); err != nil || len(e.rec.Of(domain.EvItemUpdated)) != 1 {
		t.Fatal("no change, no event")
	}
	bad, soon := "zzz", "soon"
	var fe FieldError
	if _, err := e.svc.Update(ctx, m.ID, Patch{ProfileKey: &bad}); !errors.As(err, &fe) {
		t.Fatal(err)
	}
	if _, err := e.svc.Update(ctx, m.ID, Patch{MinAvailability: &soon}); !errors.As(err, &fe) {
		t.Fatal(err)
	}
	if _, err := e.svc.Update(ctx, 99, Patch{}); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := e.svc.SetFile(ctx, m.ID, 5); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.Get(ctx, m.ID); got.FileID != 5 {
		t.Fatal("file id")
	}
	n := 0
	_ = e.svc.Each(ctx, func(domain.Movie) error { n++; return nil })
	if err := e.svc.Delete(ctx, m.ID, true); err != nil || n != 1 {
		t.Fatal(err)
	}
	del := e.rec.Of(domain.EvItemDeleted)
	if len(del) != 1 || !del[0].Data.(domain.ItemEventData).FilesDeleted {
		t.Fatalf("%+v", del)
	}
	if err := e.svc.Delete(ctx, m.ID, false); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
