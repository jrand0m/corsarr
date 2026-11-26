package store

import (
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestLegacyIDsAreStablePerSpaceAndSurviveReopen(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	r := s.LegacyIDs()
	a, err := r.ID(ctx, "tag", "kids")
	must(t, err)
	b, _ := r.ID(ctx, "tag", "anime")
	again, _ := r.ID(ctx, "tag", "kids")
	other, _ := r.ID(ctx, "profile", "hd1080p")
	if a != 1 || b != 2 || again != 1 || other != 1 {
		t.Fatalf("ids: %d %d %d %d", a, b, again, other)
	}
	if k, ok, err := r.Key(ctx, "tag", 2); err != nil || !ok || k != "anime" {
		t.Fatalf("key: %q %v %v", k, ok, err)
	}
	if _, ok, err := r.Key(ctx, "tag", 9); err != nil || ok {
		t.Fatalf("unknown id: %v %v", ok, err)
	}
	must(t, s.Close())
	s = open(t, dir)
	if keys, err := s.LegacyIDs().Keys(ctx, "tag"); err != nil || len(keys) != 2 || keys[0] != "kids" || keys[1] != "anime" {
		t.Fatalf("after reopen: %v %v", keys, err)
	}
}

func TestGrabsByIndexerCountsGrabsAndFailures(t *testing.T) {
	s := open(t, t.TempDir())
	r := s.Queue()
	g, d := newGrab(1, "aaa")
	g.Release.Indexers = []domain.IndexerRef{{Key: "one"}, {Key: "two"}}
	_, _, err := r.CreateGrab(ctx, g, d)
	must(t, err)
	g, d = newGrab(2, "bbb")
	g.Release.Indexers = []domain.IndexerRef{{Key: "one"}}
	_, sd, err := r.CreateGrab(ctx, g, d)
	must(t, err)
	sd.State = domain.StateFailed
	must(t, r.Save(ctx, sd))
	got, err := r.GrabsByIndexer(ctx)
	must(t, err)
	if got["one"] != (IndexerGrabs{Grabs: 2, Failed: 1}) || got["two"] != (IndexerGrabs{Grabs: 1}) {
		t.Fatalf("%+v", got)
	}
}
