package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

type weekHistory struct{ err error }

func (w weekHistory) List(context.Context, int64, int) ([]store.HistoryEntry, int64, error) {
	at := day.Add(-24 * time.Hour)
	return []store.HistoryEntry{
		{Event: domain.Event{Type: domain.EvGrabSent, OccurredAt: at}},
		{Event: domain.Event{Type: domain.EvGrabSent, OccurredAt: at}},
		{Event: domain.Event{Type: domain.EvImportCompleted, OccurredAt: at}},
		{Event: domain.Event{Type: domain.EvImportCompleted, OccurredAt: day.AddDate(0, 0, -30)}},
	}, 0, w.err
}

func (h *harness) prompt(t *testing.T, name string, args map[string]string) (string, *rpcError) {
	t.Helper()
	out := h.rpc(t, "prompts/get", map[string]any{"name": name, "arguments": args})
	if out.Error != nil {
		return "", out.Error
	}
	var r struct {
		Messages []struct {
			Role    string `json:"role"`
			Content struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(out.Result, &r); err != nil || len(r.Messages) != 1 || r.Messages[0].Role != "user" || r.Messages[0].Content.Type != "text" {
		t.Fatalf("%s: %s", name, out.Result)
	}
	return r.Messages[0].Content.Text, nil
}

func TestPromptsAreListed(t *testing.T) {
	h := newHarness(t)
	out := h.rpc(t, "prompts/list", nil)
	for _, name := range []string{"weekly_digest", "find_something_to_watch", "why_not_downloaded", `"required":true`} {
		if !strings.Contains(string(out.Result), name) {
			t.Errorf("missing %s in %s", name, out.Result)
		}
	}
	init := h.rpc(t, "initialize", map[string]any{"protocolVersion": "2025-06-18"})
	if !strings.Contains(string(init.Result), `"prompts"`) {
		t.Fatal("the prompts capability is announced")
	}
	if _, err := h.prompt(t, "nope", nil); err == nil || !strings.Contains(err.Message, "unknown prompt") {
		t.Fatal(err)
	}
	if out := h.rpc(t, "prompts/get", "x"); out.Error == nil {
		t.Fatal("bad params")
	}
}

func TestWeeklyDigestPrompt(t *testing.T) {
	h := newHarness(t)
	h.s.d.History = weekHistory{}
	text, err := h.prompt(t, "weekly_digest", nil)
	if err != nil || !strings.Contains(text, "2 grab.sent, 1 import.completed") || !strings.Contains(text, "movie releases between") ||
		!strings.Contains(text, "integrations") {
		t.Fatalf("%v %s", err, text)
	}
	h.s.d.History = weekHistory{err: errBoom}
	if text, _ := h.prompt(t, "weekly_digest", nil); !strings.Contains(text, "unavailable") {
		t.Fatal(text)
	}
	h.s.d.History = fakeHistory{}
	if text, _ := h.prompt(t, "weekly_digest", nil); !strings.Contains(text, "nothing recorded") {
		t.Fatal(text)
	}
	h.s.d.RootFolders = fakeRoots{err: errBoom}
	if _, err := h.prompt(t, "weekly_digest", nil); err == nil {
		t.Fatal("a failing part fails the prompt")
	}
}

func TestSomethingToWatchPrompt(t *testing.T) {
	h := newHarness(t)
	text, err := h.prompt(t, "find_something_to_watch", map[string]string{"mood": "cosy"})
	if err != nil || !strings.Contains(text, "cosy") || !strings.Contains(text, "[movie 1] Alpha (2020)") || !strings.Contains(text, "[tv 1] Show") {
		t.Fatalf("%v %s", err, text)
	}
	text, _ = h.prompt(t, "find_something_to_watch", map[string]string{"kind": "tv"})
	if strings.Contains(text, "Alpha") || !strings.Contains(text, "anything good") {
		t.Fatal(text)
	}
	if _, err := h.prompt(t, "find_something_to_watch", map[string]string{"kind": "game"}); err == nil {
		t.Fatal("bad kind")
	}
	h.modes[domain.KindTV] = domain.ModeDisabled
	if text, _ := h.prompt(t, "find_something_to_watch", nil); strings.Contains(text, "[tv") {
		t.Fatal("disabled kinds are left out")
	}
	h.mv.err = errBoom
	if _, err := h.prompt(t, "find_something_to_watch", nil); err == nil {
		t.Fatal("library errors fail the prompt")
	}
}

func TestWhyNotDownloadedPrompt(t *testing.T) {
	h := newHarness(t)
	text, err := h.prompt(t, "why_not_downloaded", map[string]string{"kind": "movie", "title": "beta"})
	if err != nil || !strings.Contains(text, `"Beta"`) || !strings.Contains(text, "all_rejected") ||
		!strings.Contains(text, "Rejected 3× by quality.notWanted") || !strings.Contains(text, "3 releases, 0 accepted") {
		t.Fatalf("%v %s", err, text)
	}
	text, _ = h.prompt(t, "why_not_downloaded", map[string]string{"kind": "tv", "id": "1"})
	if !strings.Contains(text, "Download: Show.S01 on qbit") || !strings.Contains(text, "No search has finished") {
		t.Fatal(text)
	}
	for _, args := range []map[string]string{{"kind": "movie"}, {"kind": "movie", "id": "x"}, {"kind": "movie", "title": "zzz"},
		{"kind": "game"}, {"kind": "movie", "id": "99"}} {
		if _, err := h.prompt(t, "why_not_downloaded", args); err == nil {
			t.Errorf("%v should fail", args)
		}
	}
}

func TestPromptsAreRateLimited(t *testing.T) {
	h := newHarness(t)
	cfg := h.cfg.Load().MCP
	cfg.RateLimit = 1
	h.setMCP(cfg)
	h.prompt(t, "find_something_to_watch", nil)
	if _, err := h.prompt(t, "find_something_to_watch", nil); err == nil || err.Code != codeRateLimited {
		t.Fatal(err)
	}
}
