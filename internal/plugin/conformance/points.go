package conformance

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return c
}

func must(t *testing.T, what string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

// ── indexer ──────────────────────────────────────────────────────────────────

// Indexer checks caps, search, feed paging and resolve against a fixture-backed indexer.
func Indexer(t *testing.T, ix plugin.Indexer, req domain.SearchRequest) {
	t.Helper()
	must(t, "Test", ix.Test(ctx(t)))
	indexerCaps(t, ix)
	rels := indexerSearch(t, ix, req)
	indexerFeed(t, ix, req.Kind)
	p, err := ix.Resolve(ctx(t), rels[0])
	must(t, "Resolve", err)
	if p.Kind == "" || (p.URL == "" && len(p.Content) == 0) {
		t.Errorf("empty payload %+v", p)
	}
}

func indexerCaps(t *testing.T, ix plugin.Indexer) {
	t.Helper()
	caps, err := ix.Caps(ctx(t))
	must(t, "Caps", err)
	if len(caps.Modes) == 0 {
		t.Error("caps declare no search modes")
	}
}

func indexerSearch(t *testing.T, ix plugin.Indexer, req domain.SearchRequest) []domain.Release {
	t.Helper()
	rels, err := ix.Search(ctx(t), req)
	must(t, "Search", err)
	if len(rels) == 0 {
		t.Fatal("fixture search returned nothing")
	}
	for _, p := range releaseProblems(rels) {
		t.Error(p)
	}
	return rels
}

// releaseProblems lists incomplete or duplicate releases.
func releaseProblems(rels []domain.Release) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range rels {
		if r.ID == "" || r.Title == "" || r.Protocol == "" {
			out = append(out, "incomplete release "+r.ID)
		}
		if seen[r.ID] {
			out = append(out, "duplicate release id "+r.ID)
		}
		seen[r.ID] = true
	}
	return out
}

func indexerFeed(t *testing.T, ix plugin.Indexer, kind domain.MediaKind) {
	t.Helper()
	feed, cursor, err := ix.Feed(ctx(t), kind, "")
	must(t, "Feed", err)
	if len(feed) == 0 || cursor == "" {
		t.Errorf("first feed read: %d releases, cursor %q", len(feed), cursor)
	}
	again, _, err := ix.Feed(ctx(t), kind, cursor)
	must(t, "Feed(cursor)", err)
	if len(again) != 0 {
		t.Errorf("feed with newest cursor returned %d releases", len(again))
	}
}

// ── downloader ───────────────────────────────────────────────────────────────

// Downloader checks the add/list/control/remove lifecycle, including idempotent add,
// against a stateful fake client.
func Downloader(t *testing.T, d plugin.Downloader, add plugin.AddRequest) {
	t.Helper()
	must(t, "Test", d.Test(ctx(t)))
	if len(d.Protocols()) == 0 {
		t.Error("no protocols")
	}
	for range 2 {
		must(t, "EnsureCategory", d.EnsureCategory(ctx(t), add.Category, add.SavePath))
	}
	id := downloaderAdd(t, d, add)
	if !listed(t, d, add.Category, id) {
		t.Fatalf("download %s missing from List", id)
	}
	downloaderControl(t, d, id)
	must(t, "Remove", d.Remove(ctx(t), id, true))
	if listed(t, d, add.Category, id) {
		t.Errorf("download %s still listed after Remove", id)
	}
}

func downloaderAdd(t *testing.T, d plugin.Downloader, add plugin.AddRequest) string {
	t.Helper()
	id, err := d.Add(ctx(t), add)
	must(t, "Add", err)
	if id == "" {
		t.Fatal("Add returned an empty download id")
	}
	if again, err := d.Add(ctx(t), add); err != nil || again != id {
		t.Errorf("Add is not idempotent: %q then %q (%v)", id, again, err)
	}
	return id
}

func downloaderControl(t *testing.T, d plugin.Downloader, id string) {
	t.Helper()
	must(t, "Pause", d.Pause(ctx(t), id))
	must(t, "Resume", d.Resume(ctx(t), id))
	for _, p := range []plugin.Priority{plugin.PrioTop, plugin.PrioUp, plugin.PrioDown, plugin.PrioBottom} {
		must(t, "SetPriority "+string(p), d.SetPriority(ctx(t), id, p))
	}
}

func listed(t *testing.T, d plugin.Downloader, category, id string) bool {
	t.Helper()
	snaps, err := d.List(ctx(t), []string{category})
	must(t, "List", err)
	for _, s := range snaps {
		if s.DownloadID == id {
			return true
		}
	}
	return false
}

// ── metadata ─────────────────────────────────────────────────────────────────

// Metadata checks search then lookup of one movie.
func Metadata(t *testing.T, m plugin.MovieMetadata, query string, year int) {
	t.Helper()
	must(t, "Test", m.Test(ctx(t)))
	res, err := m.SearchMovies(ctx(t), query, year)
	must(t, "SearchMovies", err)
	if len(res) == 0 || res[0].Title == "" || res[0].IDs["tmdb"] == "" {
		t.Fatalf("search result unusable: %+v", res)
	}
	rec, err := m.Movie(ctx(t), res[0].IDs)
	must(t, "Movie", err)
	if rec.Title != res[0].Title || rec.Year == 0 {
		t.Errorf("lookup mismatch: %+v vs %+v", rec, res[0])
	}
}

// ── media server ─────────────────────────────────────────────────────────────

// MediaServer checks refresh calls, including the empty set and repeats.
func MediaServer(t *testing.T, ms plugin.MediaServer, paths []string) {
	t.Helper()
	must(t, "Test", ms.Test(ctx(t)))
	must(t, "RefreshPaths(nil)", ms.RefreshPaths(ctx(t), nil))
	for range 2 {
		must(t, "RefreshPaths", ms.RefreshPaths(ctx(t), paths))
	}
}

// ── notifier ─────────────────────────────────────────────────────────────────

// Notifier checks schemes and a send.
func Notifier(t *testing.T, n plugin.Notifier, ev domain.Event, msg plugin.Message) {
	t.Helper()
	must(t, "Test", n.Test(ctx(t)))
	for _, p := range schemeProblems(n.Schemes()) {
		t.Error(p)
	}
	must(t, "Send", n.Send(ctx(t), ev, msg))
}

// schemeProblems lists missing or non-lower-case schemes.
func schemeProblems(schemes []string) []string {
	if len(schemes) == 0 {
		return []string{"no schemes"}
	}
	var out []string
	for _, s := range schemes {
		if s == "" || s != strings.ToLower(s) {
			out = append(out, "scheme "+s+" must be non-empty lower case")
		}
	}
	return out
}
