package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestManagedIndexersRoundTripWithSealedSecrets(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	r := s.Indexers()
	t0 := time.UnixMilli(1_700_000_000_000).UTC()
	m := ManagedIndexer{Key: "tpb", PluginID: "thepiratebay", Name: "TPB", Enabled: true,
		Settings: map[string]any{"priority": 10.0}, Secrets: map[string]string{"cookie": "sid=abc"}}
	if err := r.Save(ctx, m, t0); err != nil {
		t.Fatal(err)
	}
	var raw string
	_ = s.R().QueryRowContext(ctx, "SELECT secrets FROM indexer_instances").Scan(&raw)
	if strings.Contains(raw, "sid=abc") {
		t.Fatal("secret stored in clear")
	}
	m.Enabled, m.Settings = false, nil
	if err := r.Save(ctx, m, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, ok, err := r.Get(ctx, "tpb")
	if err != nil || !ok || got.Enabled || got.Secrets["cookie"] != "sid=abc" || !got.CreatedAt.Equal(t0) || !got.UpdatedAt.Equal(t0.Add(time.Hour)) {
		t.Fatalf("%+v %v", got, err)
	}
	list, err := r.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
	if ok, _ := r.Delete(ctx, "tpb"); !ok {
		t.Error("not deleted")
	}
	if ok, _ := r.Delete(ctx, "tpb"); ok {
		t.Error("deleted twice")
	}
	if _, ok, err := r.Get(ctx, "tpb"); ok || err != nil {
		t.Error("still there", err)
	}
}
