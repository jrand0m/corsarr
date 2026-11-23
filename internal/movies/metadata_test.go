package movies

import (
	"errors"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

func TestMergeNamesChangedFields(t *testing.T) {
	m := domain.Movie{ID: 1, Title: "Old", Year: 1999, Monitored: true, ProfileKey: "p", Path: "/m/Old (1999)", Digital: day(1999, 9, 1)}
	next, changed := Merge(m, plugin.MovieRecord{IDs: map[string]string{"tmdb": "1"}, Title: "New", Year: 1999, Digital: day(1999, 9, 1)})
	if next.Title != "New" || next.Path != m.Path || !next.Monitored || next.ProfileKey != "p" || len(changed) != 1 || changed[0] != "title" {
		t.Fatalf("%+v %v", next, changed)
	}
	_, changed = Merge(m, plugin.MovieRecord{Title: "Old", Year: 1999, Digital: day(2000, 1, 1), AltTitles: []string{"A"}})
	if len(changed) != 2 || changed[0] != "altTitles" || changed[1] != "releaseDates" {
		t.Fatal(changed)
	}
	if _, changed := Merge(m, plugin.MovieRecord{Title: "Old", Year: 1999, Digital: day(1999, 9, 1)}); len(changed) != 0 {
		t.Fatal(changed)
	}
}

func TestNextRefreshIsStaggered(t *testing.T) {
	at := time.Unix(0, 0)
	if NextRefresh(at, 24).Sub(at) != RefreshEvery || NextRefresh(at, 5).Sub(at) != RefreshEvery+5*time.Hour {
		t.Fatal("stagger")
	}
}

func TestLookupsServeTheCacheAndSurviveAProviderOutage(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.Lookup(ctx, "tmdb:603"); err != nil || e.meta.calls != 1 {
		t.Fatal(err, e.meta.calls)
	}
	if recs, err := e.svc.Lookup(ctx, "tmdb:603"); err != nil || len(recs) != 1 || e.meta.calls != 1 {
		t.Fatalf("a fresh record comes from the cache: %v %d", err, e.meta.calls)
	}
	e.meta.err = errors.New("tmdb down")
	e.now = e.now.Add(RefreshEvery + 24*time.Hour) // stale: the provider is asked and fails
	if recs, err := e.svc.Lookup(ctx, "tmdb:603"); err != nil || len(recs) != 1 || recs[0].Title != "The Matrix" {
		t.Fatalf("stale cache while down: %v %v", recs, err)
	}
	if recs, err := e.svc.Lookup(ctx, "matr"); err != nil || len(recs) != 1 {
		t.Fatalf("title search from the cache while down: %v %v", recs, err)
	}
	if _, err := e.svc.Lookup(ctx, "nothing cached"); err == nil {
		t.Fatal("an uncached search reports the outage")
	}
	if m, created, err := e.svc.Add(ctx, AddRequest{TMDBID: 603}); err != nil || !created || m.Title != "The Matrix" {
		t.Fatalf("add from the cache while down: %v", err)
	}
	if _, _, err := e.svc.Add(ctx, AddRequest{TMDBID: 700}); err == nil {
		t.Fatal("never fetched: the outage shows")
	}
	e.meta = nil
	if recs, err := e.svc.Lookup(ctx, "tmdb:603"); err != nil || len(recs) != 1 {
		t.Fatalf("no usable provider: cache answers: %v", err)
	}
}

func TestRefreshUpdatesChangedMoviesWithinTheBatch(t *testing.T) {
	e := newEnv(t)
	m, _, _ := e.svc.Add(ctx, AddRequest{TMDBID: 603})
	f, _, _ := e.svc.Add(ctx, AddRequest{TMDBID: 700})
	calls := e.meta.calls
	if err := e.svc.Refresh(ctx); err != nil || e.meta.calls != calls {
		t.Fatalf("fresh records are not refetched: %v %d", err, e.meta.calls-calls)
	}
	rec := e.meta.recs["603"]
	rec.Title, rec.AltTitles = "The Matrix (Remastered)", []string{"Matrix"}
	e.meta.recs["603"] = rec
	delete(e.meta.recs, "700") // gone at the provider: left alone
	e.now = e.now.Add(RefreshEvery + 24*time.Hour)
	if err := e.svc.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(ctx, m.ID)
	if got.Title != "The Matrix (Remastered)" || got.Path != m.Path {
		t.Fatalf("%+v", got)
	}
	if still, err := e.svc.Get(ctx, f.ID); err != nil || still.Title != "Future Film" {
		t.Fatal("a movie gone at the provider stays", err)
	}
	up := e.rec.Of(domain.EvItemUpdated)
	if len(up) != 1 || up[0].Data.(domain.ItemEventData).Changed[0] != "title" || up[0].Data.(domain.ItemEventData).Origin != "metadata" {
		t.Fatalf("%+v", up)
	}
	e.now = e.now.Add(2 * (RefreshEvery + 24*time.Hour))
	e.meta.err = errors.New("tmdb down")
	if err := e.svc.Refresh(ctx); err == nil {
		t.Fatal("a provider failure ends the run")
	}
	e.meta = nil
	if err := e.svc.Refresh(ctx); err != nil {
		t.Fatal("no provider: nothing to do")
	}
}
