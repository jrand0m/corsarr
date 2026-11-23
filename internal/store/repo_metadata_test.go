package store

import (
	"context"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestMetadataCache(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	r := s.Metadata()
	now := time.UnixMilli(1_700_000_000_000).UTC()
	put := func(id, title string, next time.Time) {
		if err := r.Put(ctx, MetadataEntry{Source: "tmdb", Kind: domain.KindMovie, ExternalID: id, Title: title, Record: `{"title":"` + title + `"}`,
			FetchedAt: now, NextRefresh: next}); err != nil {
			t.Fatal(err)
		}
	}
	put("1", "The Matrix", now.Add(time.Hour))
	put("1", "The Matrix", now.Add(-time.Hour)) // upsert
	put("2", "100%_Film", now.Add(time.Hour))
	e, ok, err := r.Get(ctx, "tmdb", domain.KindMovie, "1")
	if err != nil || !ok || e.Title != "The Matrix" || !e.NextRefresh.Equal(now.Add(-time.Hour)) || !e.FetchedAt.Equal(now) {
		t.Fatalf("%+v %v %v", e, ok, err)
	}
	if _, ok, _ := r.Get(ctx, "tmdb", domain.KindTV, "1"); ok {
		t.Fatal("kind is part of the key")
	}
	if hits, _ := r.Search(ctx, domain.KindMovie, "MATRIX", 10); len(hits) != 1 || hits[0].ExternalID != "1" {
		t.Fatalf("%+v", hits)
	}
	if hits, _ := r.Search(ctx, domain.KindMovie, "0%_", 10); len(hits) != 1 || hits[0].ExternalID != "2" {
		t.Fatalf("wildcards are literal: %+v", hits)
	}
}
