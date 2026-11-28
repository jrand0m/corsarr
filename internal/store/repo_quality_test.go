package store

import (
	"context"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestQualityRepo_SaveListDeleteAndUse(t *testing.T) {
	ctx := context.Background()
	s := open(t, t.TempDir())
	q := s.Quality()
	t0 := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	if err := q.Save(ctx, TableProfiles, QualityObject{Key: "ps5", Name: "PS5", Doc: []byte(`{"key":"ps5"}`)}, t0); err != nil {
		t.Fatal(err)
	}
	if err := q.Save(ctx, TableProfiles, QualityObject{Key: "ps5", Name: "PS5 High", Doc: []byte(`{"key":"ps5","upgrades":true}`)}, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	list, err := q.List(ctx, TableProfiles)
	if err != nil || len(list) != 1 {
		t.Fatalf("list %v %v", list, err)
	}
	if o := list[0]; o.Name != "PS5 High" || !o.CreatedAt.Equal(t0) || !o.UpdatedAt.Equal(t0.Add(time.Hour)) || string(o.Doc) != `{"key":"ps5","upgrades":true}` {
		t.Fatalf("object %+v", o)
	}
	if f, _ := q.List(ctx, TableFormats); len(f) != 0 {
		t.Fatal("formats table is separate")
	}
	if _, _, err := s.Movies().Add(ctx, domain.Movie{TMDBID: 1, Title: "M", ProfileKey: "ps5", MinAvailability: domain.AvailReleased}); err != nil {
		t.Fatal(err)
	}
	if n, err := q.ProfileUse(ctx, "ps5"); err != nil || n != 1 {
		t.Fatalf("use %d %v", n, err)
	}
	if ok, err := q.Delete(ctx, TableProfiles, "ps5"); !ok || err != nil {
		t.Fatalf("delete %v %v", ok, err)
	}
	if ok, _ := q.Delete(ctx, TableProfiles, "ps5"); ok {
		t.Fatal("second delete")
	}
}
