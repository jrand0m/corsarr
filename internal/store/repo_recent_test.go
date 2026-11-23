package store

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestRecentReleasesAreBoundedSealedAndExpire(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	r := s.RecentReleases()
	now := time.UnixMilli(1_700_000_000_000).UTC()
	old := []domain.Release{{ID: "old", Title: "Old", DownloadURL: "magnet:?old"}}
	if err := r.PutMany(ctx, old, now.Add(-2*time.Hour), now.Add(-time.Hour*3), 10); err != nil {
		t.Fatal(err)
	}
	var batch []domain.Release
	for i := range 4 {
		batch = append(batch, domain.Release{ID: "r" + strconv.Itoa(i), Title: "T", DownloadURL: "magnet:?" + strconv.Itoa(i)})
	}
	if err := r.PutMany(ctx, batch, now, now.Add(-time.Hour), 3); err != nil { // "old" expires, one more is over the cap
		t.Fatal(err)
	}
	if _, ok, _ := r.Get(ctx, "old", time.Time{}); ok {
		t.Fatal("expired release kept")
	}
	var kept int
	for i := range 4 {
		if rel, ok, err := r.Get(ctx, "r"+strconv.Itoa(i), now); err == nil && ok {
			kept++
			if rel.DownloadURL != "magnet:?"+strconv.Itoa(i) {
				t.Fatalf("link round-trips sealed: %q", rel.DownloadURL)
			}
		}
	}
	if kept != 3 {
		t.Fatalf("bounded to 3: %d", kept)
	}
	if _, ok, _ := r.Get(ctx, "r0", now.Add(time.Second)); ok {
		t.Fatal("since filters stale rows")
	}
	var body string
	_ = s.R().QueryRowContext(ctx, "SELECT download_url_sealed FROM recent_releases LIMIT 1").Scan(&body)
	if body == "" || body[:7] == "magnet:" {
		t.Fatalf("stored in clear: %q", body)
	}
}
