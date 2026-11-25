package declarative

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jrand0m/corsarr/internal/charset"
	"github.com/jrand0m/corsarr/internal/definition"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// legacyDef is a small HTML tracker whose pages are in a single-byte encoding.
func legacyDef(t *testing.T, encoding string) *definition.Definition {
	t.Helper()
	src := `schema: 1
id: legacysite
name: Legacy Site
links: [https://legacy.test/]
settings:
  - {key: username, type: text, label: Username}
login:
  method: form
  path: /login
  fields: {user: "{{settings.username}}"}
  success: {contains: "Выход"}
caps:
  modes: {search: [q]}
search:
  requests:
    - path: /search
      query: {q: "{{keywords}}"}
  response:
    type: html
    rows: "tr.r"
    fields:
      title: {selector: "td.t"}
      download: {selector: "a", attr: href}
`
	if encoding != "" {
		src += "encoding: " + encoding + "\n"
	}
	d, err := definition.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// legacySite serves windows-1251 pages; header says whether the
// Content-Type names the charset. It records what it was sent.
type legacySite struct {
	mu     sync.Mutex
	query  string
	user   string
	header bool
	srv    *httptest.Server
}

func newLegacySite(t *testing.T, header bool) *legacySite {
	s := &legacySite{header: header}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *legacySite) write(w http.ResponseWriter, page string) {
	ct := "text/html"
	if s.header {
		ct += "; charset=windows-1251"
	}
	w.Header().Set("Content-Type", ct)
	_, _ = w.Write([]byte(charset.FromUTF8(page, "windows-1251")))
}

func (s *legacySite) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.URL.Path == "/login" {
		_ = r.ParseForm()
		s.user = r.PostForm.Get("user")
		s.write(w, `<a href="/logout">Выход</a>`)
		return
	}
	s.query = r.URL.Query().Get("q")
	s.write(w, `<table><tr class="r"><td class="t">Кино 2024 1080p</td><td><a href="/dl/1.torrent">get</a></td></tr></table>`)
}

func TestLegacyEncodingFromTheDefinition(t *testing.T) {
	s := newLegacySite(t, false)
	ix := instance(t, legacyDef(t, "windows-1251"), s.srv.URL, plugin.Settings{"username": "Пётр"})
	rels, err := ix.Search(context.Background(), domain.SearchRequest{Mode: domain.SearchText, Text: "Кино"})
	if err != nil || len(rels) != 1 || rels[0].Title != "Кино 2024 1080p" {
		t.Fatalf("%+v %v", rels, err)
	}
	if s.query != charset.FromUTF8("Кино", "windows-1251") {
		t.Errorf("query sent as % x", s.query)
	}
	if s.user != charset.FromUTF8("Пётр", "windows-1251") {
		t.Errorf("login sent as % x", s.user)
	}
}

func TestLegacyEncodingFromTheContentType(t *testing.T) {
	s := newLegacySite(t, true)
	ix := instance(t, legacyDef(t, ""), s.srv.URL, plugin.Settings{"username": "owner"})
	rels, err := ix.Search(context.Background(), domain.SearchRequest{Mode: domain.SearchText, Text: "film"})
	if err != nil || len(rels) != 1 || !strings.HasPrefix(rels[0].Title, "Кино") {
		t.Fatalf("%+v %v", rels, err)
	}
}
