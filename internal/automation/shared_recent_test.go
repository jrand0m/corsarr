package automation

import (
	"context"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestRecentInMemoryKeepsTheNewestReleases(t *testing.T) {
	ctx := context.Background()
	r := NewRecent(nil, nil)
	if err := r.Put(ctx); err != nil {
		t.Fatal(err)
	}
	var rels []domain.Release
	for i := 0; i < RecentMax+2; i++ {
		rels = append(rels, domain.Release{ID: string(rune('a'+i%26)) + string(rune('A'+i/26))})
	}
	if err := r.Put(ctx, rels...); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get(ctx, rels[0].ID); ok {
		t.Fatal("the oldest release is dropped beyond the bound")
	}
	if got, ok := r.Get(ctx, rels[len(rels)-1].ID); !ok || got.ID != rels[len(rels)-1].ID {
		t.Fatal("the newest release is kept")
	}
	_ = r.Put(ctx, rels[len(rels)-1]) // re-put keeps one entry
	if _, ok := r.Get(ctx, "missing"); ok {
		t.Fatal("unknown id")
	}
}
