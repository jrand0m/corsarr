package plugin

import (
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestPutReplacesAndRemoveDrops(t *testing.T) {
	r := NewRegistry()
	if err := r.Put(reg("site", KindIndexer, APIVersion)); err != nil {
		t.Fatal(err)
	}
	next := reg("site", KindIndexer, APIVersion)
	next.Manifest.Version = "2"
	if err := r.Put(next); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Get("site"); got.Manifest.Version != "2" {
		t.Errorf("not replaced: %+v", got.Manifest)
	}
	if r.Put(reg("bad", KindIndexer, APIVersion+1)) == nil || r.Put(Registration{}) == nil {
		t.Error("invalid registration accepted")
	}
	r.Remove("site")
	if _, ok := r.Get("site"); ok {
		t.Error("not removed")
	}
}

func TestNewerThan(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rels := []domain.Release{{ID: "a", PublishedAt: t0}, {ID: "b", PublishedAt: t0.Add(time.Hour)}}
	fresh, cur := NewerThan(rels, "")
	if len(fresh) != 2 || cur != t0.Add(time.Hour).Format(time.RFC3339Nano)+"|b" {
		t.Fatalf("%v %q", fresh, cur)
	}
	if fresh, again := NewerThan(rels, cur); len(fresh) != 0 || again != cur {
		t.Errorf("replay handed %v, cursor %q", fresh, again)
	}
	if fresh, c := NewerThan(nil, cur); fresh != nil || c != cur {
		t.Error("empty feed moved the cursor")
	}
}
