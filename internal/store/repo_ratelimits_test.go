package store

import (
	"context"
	"testing"
)

func TestRateLimitsRoundTripAndSurviveReopen(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	s := open(t, dir)
	r := s.RateLimits()
	if err := r.SaveRateLimit(ctx, "api.example.org", []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveRateLimit(ctx, "api.example.org", []byte(`{"a":2}`)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	got, err := open(t, dir).RateLimits().LoadRateLimits(ctx)
	if err != nil || len(got) != 1 || string(got["api.example.org"]) != `{"a":2}` {
		t.Fatalf("%v %q", err, got)
	}
}
