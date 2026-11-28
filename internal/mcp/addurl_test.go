package mcp

import (
	"context"
	"fmt"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/queue"
)

func (f *fakeDownloaders) AddURL(_ context.Context, req queue.URLRequest) (domain.Grab, error) {
	if f.shadow {
		return domain.Grab{}, queue.ErrShadowed
	}
	if req.URL == "bad" {
		return domain.Grab{}, queue.ErrBadURL
	}
	refs := ""
	for _, it := range req.Items {
		refs += fmt.Sprintf(" %s/%d", it.Ref.Kind, it.Ref.ID)
	}
	f.c.add(fmt.Sprintf("addurl %s %s%s", req.Kind, req.Downloader, refs))
	to := req.Downloader
	if to == "" {
		to = "qbit"
	}
	return domain.Grab{Downloader: to, Release: domain.Release{Title: "From.URL"}}, nil
}

func TestAddDownloadURL(t *testing.T) {
	h := newHarness(t)
	magnet := "magnet:?xt=urn:btih:abc"
	h.ok(t, "add_download_url", map[string]any{"kind": "movie", "id": 2, "url": magnet}, "sent From.URL to qbit for 1 items")
	h.ok(t, "add_download_url", map[string]any{"kind": "tv", "id": 1, "season": 1, "url": magnet, "downloader": "sab"}, "for 2 items")
	h.ok(t, "add_download_url", map[string]any{"kind": "tv", "id": 1, "episodes": []map[string]int{{"season": 2, "episode": 1}}, "url": magnet}, "for 1 items")
	h.ok(t, "add_download_url", map[string]any{"kind": "music", "albumId": 100, "url": magnet}, "for 1 items")
	for _, want := range []string{"addurl movie  movie/2", "addurl tv sab tv/11 tv/12", "addurl tv  tv/21", "addurl music  music/100"} {
		if !h.c.has(want) {
			t.Fatalf("missing %q in %v", want, h.c.list)
		}
	}
	h.fails(t, "add_download_url", map[string]any{"kind": "tv", "id": 1, "url": magnet}, "season")
	h.fails(t, "add_download_url", map[string]any{"kind": "music", "url": magnet}, "albumId")
	h.fails(t, "add_download_url", map[string]any{"kind": "movie", "id": 9, "url": magnet}, "not found")
	h.fails(t, "add_download_url", map[string]any{"kind": "movie", "id": 2, "url": "bad"}, "url:")
	h.dl.shadow = true
	h.ok(t, "add_download_url", map[string]any{"kind": "movie", "id": 2, "url": magnet}, "recorded, not sent")
	h.modes[domain.KindMovie] = domain.ModeDisabled
	h.fails(t, "add_download_url", map[string]any{"kind": "movie", "id": 2, "url": magnet}, "disabled")
}

func TestAddDownloadURLNeedsWrite(t *testing.T) {
	h := newHarness(t)
	if !h.s.byName["add_download_url"].Write {
		t.Fatal("add_download_url is a write tool")
	}
	ro := h.s.NewSession(KeyRead)
	out := h.callOn(t, ro, "add_download_url", map[string]any{"kind": "movie", "id": 2, "url": "magnet:?xt=urn:btih:abc"})
	if !out.IsError || len(h.c.list) != 0 {
		t.Fatal("a read-only key cannot add downloads")
	}
}
