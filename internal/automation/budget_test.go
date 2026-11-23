package automation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/testkit"
)

func TestNextEligibleBacksOffPerMiss(t *testing.T) {
	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for misses, want := range map[int]time.Duration{0: RetryBase, 1: RetryBase, 2: 2 * RetryBase, 3: 4 * RetryBase, 50: RetryMax} {
		if got := NextEligible(at, misses).Sub(at); got != want {
			t.Errorf("misses %d: %v", misses, got)
		}
	}
}

func TestDueOrdersNeverSearchedThenOldest(t *testing.T) {
	now := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	ts := func(h int) *time.Time { v := now.Add(time.Duration(h) * time.Hour); return &v }
	states := map[int64]domain.SearchState{
		1: {LastSearched: ts(-10), NextEligible: ts(-1)},
		2: {LastSearched: ts(-20), NextEligible: ts(-2)},
		3: {LastSearched: ts(-1), NextEligible: ts(5)}, // not yet eligible
	}
	got := Due([]int64{3, 1, 4, 2, 5}, states, now, 3)
	if len(got) != 3 || got[0] != 4 || got[1] != 5 || got[2] != 2 {
		t.Fatalf("%v", got)
	}
}

func TestParseLimit(t *testing.T) {
	if l := ParseLimit(5, "hour"); l.Per != time.Hour || l.Queries != 5 {
		t.Fatal(l)
	}
	if l := ParseLimit(5, ""); l.Per != 24*time.Hour {
		t.Fatal(l)
	}
}

func TestIndexerQueryLimitSkipsTheIndexer(t *testing.T) {
	e := newEnv(t)
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	e.svc.d.Now = func() time.Time { return now }
	e.svc.d.Indexers = func() []Indexer { return []Indexer{{Key: "ix", Impl: e.ix, Limit: Limit{Queries: 2, Per: time.Hour}}} }
	for range 2 {
		if _, err := e.svc.Search(ctx, film.ID, domain.OriginAutomatic); err != nil {
			t.Fatal(err)
		}
	}
	sum, _ := e.svc.Search(ctx, film.ID, domain.OriginAutomatic)
	if e.ix.searches != 2 || sum.Skipped != 1 {
		t.Fatalf("searches %d skipped %d", e.ix.searches, sum.Skipped)
	}
	var r rows
	_, _ = e.svc.Interactive(ctx, film.ID, &r)
	if len(r.errs) != 1 || r.errs[0] != "ix" {
		t.Fatalf("interactive search reports the skipped indexer: %v", r.errs)
	}
	var le LimitError
	if err := e.svc.allow(ctx, Indexer{Key: "ix", Limit: Limit{Queries: 2, Per: time.Hour}}); !errors.As(err, &le) ||
		!le.Resets.Equal(now.Add(time.Hour)) || !strings.Contains(err.Error(), "resets at") {
		t.Fatalf("%v", err)
	}
	now = now.Add(time.Hour + time.Second) // the window frees up
	_, _ = e.svc.Search(ctx, film.ID, domain.OriginAutomatic)
	if e.ix.searches != 3 {
		t.Fatalf("searches after the window: %d", e.ix.searches)
	}
}

func TestSearchRecordsStateAndWantedShowsIt(t *testing.T) {
	e := newEnv(t)
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	e.svc.d.Now = func() time.Time { return now }
	_, _ = e.svc.Search(ctx, film.ID, domain.OriginAutomatic) // nothing found
	_, _ = e.svc.Search(ctx, film.ID, domain.OriginAutomatic)
	st, _ := e.st.Searches().State(ctx, film.Ref())
	if st.Misses != 2 || !st.LastSearched.Equal(now) || !st.NextEligible.Equal(now.Add(2*RetryBase)) || st.LastResult == nil {
		t.Fatalf("%+v", st)
	}
	srv := testkit.Serve(t, func(m testkit.Mux) { e.svc.Mount(m) })
	if code, body := testkit.Do(t, srv, "GET", "/api/v1/wanted/missing", ""); code != 200 ||
		!strings.Contains(body, `"lastSearched":"2024-01-01T12:00:00Z"`) || !strings.Contains(body, `"nextEligible":`) {
		t.Fatal(code, body)
	}
	e.ix.rels = []domain.Release{rel(good, 4e9, 10)}
	_, _ = e.svc.Search(ctx, film.ID, domain.OriginAutomatic)
	if st, _ := e.st.Searches().State(ctx, film.Ref()); st.Misses != 0 || st.LastResult.Grabbed != good {
		t.Fatalf("a grab resets the misses: %+v", st)
	}
}

func TestBacklogQueuesDueItemsWithinBudget(t *testing.T) {
	e := newEnv(t)
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	e.svc.d.Now = func() time.Time { return now }
	_ = e.st.Searches().RecordQuery(ctx, "ix", now.Add(-2*QueryLogKeep))
	if err := e.svc.Backlog(ctx); err != nil {
		t.Fatal(err)
	}
	e.svc.Wait()
	if e.ix.searches != 1 {
		t.Fatalf("due movie searched once: %d", e.ix.searches)
	}
	if n, _, _ := e.st.Searches().Queries(ctx, "ix", now.Add(-3*QueryLogKeep)); n != 1 {
		t.Fatalf("old query log pruned, the new query kept: %d", n)
	}
	_ = e.svc.Backlog(ctx) // just searched: not eligible yet
	e.svc.Wait()
	if e.ix.searches != 1 {
		t.Fatalf("searched before it was eligible: %d", e.ix.searches)
	}
	e.svc.d.Modes = modeIs(domain.ModeDisabled)
	if err := e.svc.Backlog(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSearchQueueRunsOnceForAWaitingItem(t *testing.T) {
	q := newSearchQueue()
	release, started := make(chan struct{}), make(chan struct{}, 4)
	var mu sync.Mutex
	var ran []int64
	run := func(_ context.Context, id int64) {
		started <- struct{}{}
		if id == 1 {
			<-release
		}
		mu.Lock()
		ran = append(ran, id)
		mu.Unlock()
	}
	q.add(ctx, 1, run) // running
	<-started
	q.add(ctx, 2, run) // waiting
	q.add(ctx, 2, run) // already waiting: dropped
	q.add(ctx, 1, run) // running, so queued again
	close(release)
	q.wait()
	if len(ran) != 3 || ran[0] != 1 || ran[1] != 2 || ran[2] != 1 {
		t.Fatalf("%v", ran)
	}
}

func TestManualGrabSurvivesARestartAndExpires(t *testing.T) {
	e := newEnv(t)
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	e.svc.d.Now = func() time.Time { return now }
	r := rel(good, 4e9, 10)
	r.DownloadURL = "magnet:?xt=urn:btih:secret"
	e.ix.rels = []domain.Release{r}
	_, _ = e.svc.Interactive(ctx, film.ID, &rows{})
	// a new service on the same store stands in for a restart
	d := e.svc.d
	restarted := New(d)
	if _, _, err := restarted.GrabRelease(ctx, film.ID, r.ID, false); err != nil {
		t.Fatal(err)
	}
	if len(e.q.grabs) != 1 || e.q.grabs[0].Release.DownloadURL != r.DownloadURL {
		t.Fatalf("grabbed with the stored link: %+v", e.q.grabs)
	}
	now = now.Add(RecentTTL + time.Minute)
	if _, _, err := restarted.GrabRelease(ctx, film.ID, r.ID, false); !errors.Is(err, ErrUnknownRelease) ||
		!strings.Contains(err.Error(), "search again") {
		t.Fatalf("expired: %v", err)
	}
	srv := testkit.Serve(t, func(m testkit.Mux) { restarted.Mount(m) })
	if code, body := testkit.Do(t, srv, "POST", "/api/v1/grabs", `{"movieId":1,"releaseId":"gone"}`); code != 404 ||
		!strings.Contains(body, "release_expired") {
		t.Fatal(code, body)
	}
}
