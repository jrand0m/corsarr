package torznab

import (
	"context"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

type rssDoc struct {
	Items []rssItem `xml:"channel>item"`
}

type rssItem struct {
	Title     string   `xml:"title"`
	GUID      string   `xml:"guid"`
	Link      string   `xml:"link"`
	Comments  string   `xml:"comments"`
	PubDate   string   `xml:"pubDate"`
	Size      string   `xml:"size"`
	Cats      []string `xml:"category"`
	Enclosure struct {
		URL    string `xml:"url,attr"`
		Length string `xml:"length,attr"`
	} `xml:"enclosure"`
	Attrs []struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	} `xml:"attr"`
}

// items runs a query and parses the RSS document.
func (ix *Indexer) items(ctx context.Context, q url.Values) ([]rssItem, error) {
	resp, err := ix.get(ctx, q, maxXML)
	if err != nil {
		return nil, err
	}
	return parseRSS(resp.Body, resp.Header.Get("Content-Type"))
}

// parseRSS reads a results page. When the page as a whole does not parse,
// each item is read on its own and the broken ones are dropped; the page
// fails only when no item can be read.
func parseRSS(body []byte, contentType string) ([]rssItem, error) {
	body = cleanXML(body, contentType)
	if err := serverError(body); err != nil {
		return nil, err
	}
	var d rssDoc
	err := unmarshal(body, &d)
	if err == nil {
		return d.Items, nil
	}
	if items := itemsOneByOne(body); len(items) > 0 {
		return items, nil
	}
	return nil, plugin.Wrap(plugin.ClassParse, "results page", err)
}

func (it rssItem) attr(name string) string {
	for _, a := range it.Attrs {
		if a.Name == name {
			return a.Value
		}
	}
	return ""
}

func (it rssItem) attrAll(name string) []string {
	var out []string
	for _, a := range it.Attrs {
		if a.Name == name {
			out = append(out, a.Value)
		}
	}
	return out
}

var dateLayouts = []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, "Mon, 2 Jan 2006 15:04:05 -0700"}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// ratio reads a seed ratio; anything unparsable or negative is "not reported".
func ratio(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func (it rssItem) size() int64 {
	for _, s := range []string{it.attr("size"), it.Size, it.Enclosure.Length} {
		if n := atoi64(s); n > 0 {
			return n
		}
	}
	return 0
}

func (it rssItem) categories() []int {
	seen := map[int]bool{}
	var out []int
	for _, s := range append(it.attrAll("category"), it.Cats...) {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// downloadFlags names the share of the download that counts against the
// account's ratio.
var downloadFlags = map[float64]string{0: "freeleech", 0.25: "freeleech75", 0.5: "halfleech", 0.75: "freeleech25"}

// flags reads the indexer flags from the volume factors: how much of the
// download counts (freeleech and partial freeleech) and double upload credit.
func (it rssItem) flags() []string {
	var out []string
	if f, err := strconv.ParseFloat(it.attr("downloadvolumefactor"), 64); err == nil {
		if flag, ok := downloadFlags[f]; ok {
			out = append(out, flag)
		}
	}
	if f, err := strconv.ParseFloat(it.attr("uploadvolumefactor"), 64); err == nil && f >= 2 {
		out = append(out, "doubleupload")
	}
	return out
}

func (it rssItem) externalIDs() map[string]string {
	ids := map[string]string{}
	set := func(key, v string) {
		if v != "" && v != "0" {
			ids[key] = v
		}
	}
	if imdb := it.attr("imdbid"); imdb != "" && imdb != "0" {
		ids["imdb"] = "tt" + strings.TrimPrefix(imdb, "tt")
	}
	set("tmdb", it.attr("tmdbid"))
	set("tvdb", it.attr("tvdbid"))
	set("tvmaze", it.attr("tvmazeid"))
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// release maps one item; ok is false for items without a title or identity.
func (ix *Indexer) release(it rssItem) (domain.Release, target, bool) {
	id := firstNonEmpty(it.GUID, it.Link, it.Enclosure.URL)
	if strings.TrimSpace(it.Title) == "" || id == "" {
		return domain.Release{}, target{}, false
	}
	seeders, _ := strconv.Atoi(it.attr("seeders"))
	peers, _ := strconv.Atoi(it.attr("peers"))
	grabs, _ := strconv.Atoi(it.attr("grabs"))
	rel := domain.Release{
		ID: id, GUID: it.GUID, Title: strings.TrimSpace(it.Title), Protocol: domain.ProtoTorrent,
		Size: it.size(), PublishedAt: parseDate(it.PubDate), Indexers: []domain.IndexerRef{ix.ref},
		Categories: it.categories(), InfoHash: strings.ToLower(it.attr("infohash")), DetailsURL: it.Comments,
		Seeders: seeders, Peers: peers, Grabs: grabs, Flags: it.flags(), ExternalIDs: it.externalIDs(),
		SwarmReported: it.attr("seeders") != "" || it.attr("peers") != "",
		Files:         max(0, atoi(it.attr("files"))), MinimumRatio: ratio(it.attr("minimumratio")),
	}
	tg := target{link: firstNonEmpty(it.Link, it.Enclosure.URL), magnet: it.attr("magneturl")}
	rel.DownloadURL = firstNonEmpty(magnetOf(tg), tg.link)
	return rel, tg, true
}

// releases maps items and remembers their download targets for Resolve.
func (ix *Indexer) releases(items []rssItem) []domain.Release {
	var rels []domain.Release
	for _, it := range items {
		rel, tg, ok := ix.release(it)
		if !ok {
			continue
		}
		ix.links.put(rel.ID, tg)
		rels = append(rels, rel)
	}
	return rels
}

// Feed returns releases newer than the cursor ("<pubDate RFC3339>|<guid>").
func (ix *Indexer) Feed(ctx context.Context, kind domain.MediaKind, cursor string) ([]domain.Release, string, error) {
	items, err := ix.items(ctx, ix.base(kind, "search"))
	if err != nil {
		return nil, cursor, err
	}
	rels := ix.releases(items)
	return newerThan(rels, cursor)
}

func splitCursor(c string) (time.Time, string) {
	ts, guid, _ := strings.Cut(c, "|")
	t, _ := time.Parse(time.RFC3339Nano, ts)
	return t, guid
}

func isNewer(r domain.Release, at time.Time, guid string) bool {
	return r.PublishedAt.After(at) || (r.PublishedAt.Equal(at) && r.ID != guid)
}

func newerThan(rels []domain.Release, cursor string) ([]domain.Release, string, error) {
	at, guid := splitCursor(cursor)
	var fresh []domain.Release
	newest := domain.Release{}
	for _, r := range rels {
		if cursor == "" || isNewer(r, at, guid) {
			fresh = append(fresh, r)
		}
		if r.PublishedAt.After(newest.PublishedAt) {
			newest = r
		}
	}
	if newest.ID == "" {
		return fresh, cursor, nil
	}
	if cursor != "" && !newest.PublishedAt.After(at) {
		return fresh, cursor, nil
	}
	return fresh, newest.PublishedAt.Format(time.RFC3339Nano) + "|" + newest.ID, nil
}
