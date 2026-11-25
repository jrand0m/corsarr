package torznab

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

func feedOf(items ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel><title>t</title><items>2</items>` +
		strings.Join(items, "") + `</channel></rss>`
}

func item(guid, title string, extra ...string) string {
	return `<item><title>` + title + `</title><guid>` + guid + `</guid><link>https://x.test/` + guid + `.torrent</link>` +
		strings.Join(extra, "") + `</item>`
}

func titles(items []rssItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Title
	}
	return out
}

func TestRSSToleratesHTMLEntitiesAndControlCharacters(t *testing.T) {
	body := "\xEF\xBB\xBF" + feedOf(
		item("1", "Show&nbsp;Name&nbsp;S01E01 &copy; Studio"),
		item("2", "Tom & Jerry &bogus; 1080p"),
		item("3", "Bell\x07Ringer\x1b 720p", "<description>a\x00b &hellip; &#x1; c</description>"),
		item("4", "Caf&eacute; &mdash; Live", `<torznab:attr name="seeders" value="5&nbsp;"/>`),
	)
	items, err := parseRSS([]byte(body), "application/rss+xml")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Show Name S01E01 © Studio", "Tom & Jerry &bogus; 1080p", "BellRinger 720p", "Café — Live"}
	if got := titles(items); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("titles %q", got)
	}
}

func TestRSSOneBrokenItemDoesNotFailThePage(t *testing.T) {
	body := feedOf(
		item("1", "Good One"),
		item("2", "Broken <b>tag</i>"),
		item("3", "Good Two", `<torznab:attr name="seeders" value="9"/>`),
	)
	items, err := parseRSS([]byte(body), "")
	if err != nil {
		t.Fatal(err)
	}
	if got := titles(items); len(got) != 2 || got[0] != "Good One" || got[1] != "Good Two" || items[1].attr("seeders") != "9" {
		t.Errorf("items %q", got)
	}
}

func TestRSSWithEveryItemBrokenIsAParseError(t *testing.T) {
	_, err := parseRSS([]byte(feedOf(item("1", "a <b>c</i>"), item("2", "d <e>f</g>"))), "")
	if plugin.ClassOf(err) != plugin.ClassParse {
		t.Errorf("%v", err)
	}
}

func TestRSSInALegacyEncoding(t *testing.T) {
	title := "\xCF\xF0\xE8\xE2\xE5\xF2 1080p" // "Привет 1080p" in windows-1251
	declared := strings.Replace(feedOf(item("1", title)), "UTF-8", "windows-1251", 1)
	items, err := parseRSS([]byte(declared), "text/xml")
	if err != nil || len(items) != 1 || items[0].Title != "Привет 1080p" {
		t.Fatalf("declared: %q %v", titles(items), err)
	}
	header := strings.Replace(feedOf(item("1", title)), ` encoding="UTF-8"`, "", 1)
	items, err = parseRSS([]byte(header), "application/xml; charset=windows-1251")
	if err != nil || len(items) != 1 || items[0].Title != "Привет 1080p" {
		t.Fatalf("header: %q %v", titles(items), err)
	}
}

func TestRSSInvalidUTF8IsReplaced(t *testing.T) {
	items, err := parseRSS([]byte(feedOf(item("1", "Bad \xff byte"))), "")
	if err != nil || len(items) != 1 || items[0].Title != "Bad � byte" {
		t.Fatalf("%q %v", titles(items), err)
	}
}

func TestSearchOverADirtyFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write([]byte("\xEF\xBB\xBF" + `<caps><searching><search available="yes" supportedParams="q"/></searching>` +
				`<categories><category id="2000" name="Movies&nbsp;All"/></categories></caps>`))
			return
		}
		_, _ = w.Write([]byte(feedOf(item("1", "Some&nbsp;Movie 2021"), item("2", "Other <x>Movie</y>"))))
	}))
	defer srv.Close()
	ix := newIndexer(t, srv.URL, nil)
	rels, err := ix.Search(t.Context(), domain.SearchRequest{Mode: domain.SearchText, Text: "Some Movie"})
	if err != nil || len(rels) != 1 || rels[0].Title != "Some Movie 2021" {
		t.Fatalf("%+v %v", rels, err)
	}
}
