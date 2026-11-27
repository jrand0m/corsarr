package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/testkit"
)

// mcpClient speaks Streamable HTTP to the stack's /mcp endpoint.
type mcpClient struct {
	t       *testing.T
	s       *stack
	session string
	next    int
}

// enableMCP switches the endpoint on (write scope) as a config reload does
// and initializes a session.
func (s *stack) enableMCP(t *testing.T) *mcpClient {
	t.Helper()
	s.yaml += "mcp:\n  enabled: true\n  scope: write\n  rateLimit: 0\n"
	cfg := s.load(t, s.yaml)
	s.cfg.Store(cfg)
	s.app.OnConfig(cfg)
	c := &mcpClient{t: t, s: s}
	c.rpc("initialize", map[string]any{"protocolVersion": "2025-06-18", "clientInfo": map[string]string{"name": "e2e"}})
	return c
}

func (c *mcpClient) rpc(method string, params any) json.RawMessage {
	c.t.Helper()
	c.next++
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.next, "method": method, "params": params})
	req, _ := http.NewRequest("POST", c.s.api.URL+"/mcp", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+c.s.key)
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.session != "" {
		req.Header.Set("Mcp-Session-Id", c.session)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		c.t.Fatalf("%s: %d %s", method, resp.StatusCode, b)
	}
	c.session = resp.Header.Get("Mcp-Session-Id")
	var out struct {
		Result json.RawMessage `json:"result"`
		Error  any             `json:"error"`
	}
	_ = json.Unmarshal(b, &out)
	if out.Error != nil {
		c.t.Fatalf("%s: %s", method, b)
	}
	return out.Result
}

type toolOut struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Structured map[string]any `json:"structuredContent"`
	IsError    bool           `json:"isError"`
}

func (o toolOut) text() string {
	if len(o.Content) == 0 {
		return ""
	}
	return o.Content[0].Text
}

func (c *mcpClient) tool(name string, args map[string]any) toolOut {
	c.t.Helper()
	var out toolOut
	_ = json.Unmarshal(c.rpc("tools/call", map[string]any{"name": name, "arguments": args}), &out)
	return out
}

func (c *mcpClient) mustTool(name string, args map[string]any) toolOut {
	c.t.Helper()
	out := c.tool(name, args)
	if out.IsError {
		c.t.Fatalf("%s: %s", name, out.text())
	}
	return out
}

func (c *mcpClient) state(args map[string]any) string {
	st, _ := c.mustTool("request_status", args).Structured["state"].(string)
	return st
}

// A movie requested through MCP is grabbed and imported via the fake
// downloader, and request_status follows it to imported.
func TestMCPRequestsMovieEndToEnd(t *testing.T) {
	s := start(t, []string{"movie.mkv"})
	c := s.enableMCP(t)
	found := c.mustTool("find_media", map[string]any{"query": "example", "kind": "movie"})
	if !strings.Contains(found.text(), movieTitle) {
		t.Fatalf("find_media: %s", found.text())
	}
	if out := c.mustTool("request_movie", map[string]any{"tmdbId": 550}); out.Structured["created"] != true {
		t.Fatalf("request_movie: %v", out.Structured)
	}
	testkit.Eventually(t, 20*time.Second, "movie imported", func() bool { return c.state(map[string]any{"kind": "movie", "tmdbId": 550}) == "imported" })
	again := c.mustTool("request_movie", map[string]any{"tmdbId": 550})
	if again.Structured["created"] == true || !strings.Contains(again.text(), "already in the library") {
		t.Fatalf("requests are idempotent: %v", again.Structured)
	}
	st := c.mustTool("request_status", map[string]any{"kind": "movie", "id": 1}).Structured
	if files, _ := st["files"].([]any); len(files) != 1 || !strings.HasPrefix(files[0].(string), s.library) {
		t.Fatalf("imported path: %v", st)
	}
	if !has(s.history(t), "mcp.called") {
		t.Fatal("tool calls are audited in History")
	}
	if out := c.tool("request_book", map[string]any{"isbn": "9780593135204"}); !out.IsError || !strings.Contains(out.text(), "book") {
		t.Fatalf("books are refused with the reason: %+v", out)
	}
}

// A show requested through MCP is grabbed as a season pack and imported.
func TestMCPRequestsShowEndToEnd(t *testing.T) {
	s := start(t, nil)
	c := s.enableMCP(t)
	c.mustTool("request_show", map[string]any{"tmdbId": 77, "monitor": "all"})
	testkit.Eventually(t, 20*time.Second, "show imported", func() bool { return c.state(map[string]any{"kind": "tv", "tmdbId": 77}) == "imported" })
	if !s.qb.added(showHash) {
		t.Fatal("the season pack was not sent to the downloader")
	}
}

// An album and a single track requested through MCP: the track resolves to
// the album that holds it.
func TestMCPRequestsMusicEndToEnd(t *testing.T) {
	m := startMusic(t, "active")
	c := m.enableMCP(t)
	track := c.mustTool("find_media", map[string]any{"query": "Closing", "kind": "music", "level": "track", "artist": artistName})
	if !strings.Contains(track.text(), "Closing") {
		t.Fatalf("track lookup: %s", track.text())
	}
	out := c.mustTool("request_music", map[string]any{"artistMbid": artistMBID, "track": "Closing", "profileKey": "lossless"})
	if !strings.Contains(out.text(), "First Album") {
		t.Fatalf("track resolves to its album: %s", out.text())
	}
	album := map[string]any{"kind": "music", "mbid": albumMBID}
	testkit.Eventually(t, 20*time.Second, "album imported", func() bool { return c.state(album) == "imported" })
	if !m.qb.added(albumHash) {
		t.Fatal("the album was not sent to the downloader")
	}
	dir := filepath.Join(m.music, artistName, "First Album (2020)")
	if _, err := os.Stat(filepath.Join(dir, "Example Artist - First Album - 02 - Closing.flac")); err != nil {
		t.Fatal(err)
	}
	again := c.mustTool("request_music", map[string]any{"artistMbid": artistMBID, "albumMbid": albumMBID})
	if again.Structured["created"] == true {
		t.Fatalf("album requests are idempotent: %v", again.Structured)
	}
	if out := c.tool("request_music", map[string]any{"artistMbid": artistMBID, "track": "Nowhere"}); !out.IsError {
		t.Fatalf("an unknown track is refused: %+v", out)
	}
}

// The downloader tools read the qBittorrent fake through the queue service;
// in shadow mode a client-wide write is recorded and not sent.
func TestMCPDownloaderTools(t *testing.T) {
	s := startIn(t, "shadow", []string{"movie.mkv"})
	c := s.enableMCP(t)
	if out := c.mustTool("list_downloaders", nil); !strings.Contains(out.text(), "1 download clients") {
		t.Fatalf("list_downloaders: %s", out.text())
	}
	if out := c.mustTool("get_queue", map[string]any{"downloader": "qbit"}); !strings.Contains(out.text(), "0 downloads") {
		t.Fatalf("get_queue: %s", out.text())
	}
	out := c.mustTool("pause_all", map[string]any{"downloader": "qbit"})
	if out.Structured["recorded"] != true || out.Structured["sent"] != false {
		t.Fatalf("shadow pause_all: %v", out.Structured)
	}
	if out := c.tool("get_download", map[string]any{"id": 99}); !out.IsError {
		t.Fatal("unknown download")
	}
}
