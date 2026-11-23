package store

import (
	"context"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestSearchStateAndQueryLog(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	r := s.Searches()
	ref := domain.ItemRef{Kind: domain.KindMovie, ID: 7}
	if st, err := r.State(ctx, ref); err != nil || st.Item != ref || st.LastSearched != nil {
		t.Fatalf("never searched: %+v %v", st, err)
	}
	now := time.UnixMilli(1_700_000_000_000).UTC()
	next := now.Add(time.Hour)
	res := domain.SearchEventData{Origin: domain.OriginAutomatic, Releases: 3, Grabbed: "X"}
	for range 2 { // upsert
		if err := r.SaveState(ctx, domain.SearchState{Item: ref, LastSearched: &now, NextEligible: &next, Misses: 2, LastResult: &res}); err != nil {
			t.Fatal(err)
		}
	}
	st, _ := r.State(ctx, ref)
	if !st.LastSearched.Equal(now) || !st.NextEligible.Equal(next) || st.Misses != 2 || st.LastResult.Grabbed != "X" {
		t.Fatalf("%+v", st)
	}
	_ = r.SaveState(ctx, domain.SearchState{Item: domain.ItemRef{Kind: domain.KindTV, ID: 7}})
	all, err := r.States(ctx, domain.KindMovie)
	if err != nil || len(all) != 1 || all[7].Misses != 2 {
		t.Fatalf("%+v %v", all, err)
	}
	for i := range 3 {
		_ = r.RecordQuery(ctx, "ix", now.Add(time.Duration(i)*time.Minute))
	}
	_ = r.RecordQuery(ctx, "other", now)
	n, oldest, _ := r.Queries(ctx, "ix", now.Add(time.Minute))
	if n != 2 || !oldest.Equal(now.Add(time.Minute)) {
		t.Fatalf("%d %v", n, oldest)
	}
	_ = r.PruneQueries(ctx, now.Add(2*time.Minute))
	if n, _, _ := r.Queries(ctx, "ix", now.Add(-time.Hour)); n != 1 {
		t.Fatalf("pruned: %d", n)
	}
	if n, oldest, _ := r.Queries(ctx, "none", now); n != 0 || !oldest.IsZero() {
		t.Fatal("empty window")
	}
}
