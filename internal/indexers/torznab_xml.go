package indexers

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// Torznab error codes (public protocol description).
const (
	codeAuth        = 100
	codeBadParam    = 201
	codeUnsupported = 202
	codeNotFound    = 300
	codeServer      = 900
)

// Request is a parsed Torznab query.
type Request struct {
	Op     string // caps | search | get
	Search domain.SearchRequest
	Cats   []int
	Limit  int
	Offset int
}

var torznabOps = map[string]domain.SearchMode{
	"search": domain.SearchText, "tvsearch": domain.SearchTV, "tv-search": domain.SearchTV, "movie": domain.SearchMovie,
	"movie-search": domain.SearchMovie, "music": domain.SearchMusic, "audio": domain.SearchMusic, "book": domain.SearchBook,
}

var modeKinds = map[domain.SearchMode]domain.MediaKind{
	domain.SearchTV: domain.KindTV, domain.SearchMovie: domain.KindMovie, domain.SearchMusic: domain.KindMusic, domain.SearchBook: domain.KindBook,
}

// ParseRequest reads the query string of a Torznab call (pure).
func ParseRequest(q url.Values) (Request, error) {
	t := strings.ToLower(q.Get("t"))
	if t == "caps" {
		return Request{Op: "caps"}, nil
	}
	mode, ok := torznabOps[t]
	if !ok {
		return Request{}, fmt.Errorf("function %q is not supported", t)
	}
	r := Request{Op: "search", Limit: 100}
	var err error
	if r.Cats, err = ints(q.Get("cat")); err != nil {
		return Request{}, err
	}
	if r.Limit, err = bounded(q.Get("limit"), 100, 1, 500); err != nil {
		return Request{}, err
	}
	if r.Offset, err = bounded(q.Get("offset"), 0, 0, 10000); err != nil {
		return Request{}, err
	}
	r.Search = domain.SearchRequest{Mode: mode, Kind: kindOf(mode, r.Cats), Text: strings.TrimSpace(q.Get("q")), Origin: domain.OriginInteractive}
	return r, searchParams(&r.Search, q)
}

func kindOf(m domain.SearchMode, cats []int) domain.MediaKind {
	if k, ok := modeKinds[m]; ok {
		return k
	}
	for _, c := range cats {
		switch c / 1000 {
		case 2:
			return domain.KindMovie
		case 3:
			return domain.KindMusic
		case 5:
			return domain.KindTV
		case 7:
			return domain.KindBook
		}
	}
	return ""
}

func searchParams(s *domain.SearchRequest, q url.Values) error {
	ids := map[string]string{}
	if v := q.Get("imdbid"); v != "" {
		ids["imdb"] = "tt" + strings.TrimPrefix(v, "tt")
	}
	for param, ext := range map[string]string{"tmdbid": "tmdb", "tvdbid": "tvdb", "tvmazeid": "tvmaze"} {
		if v := q.Get(param); v != "" {
			ids[ext] = v
		}
	}
	if len(ids) > 0 {
		s.IDs = ids
	}
	var err error
	if s.Season, err = bounded(q.Get("season"), 0, 0, 9999); err != nil {
		return err
	}
	ep, err := bounded(q.Get("ep"), 0, 0, 99999)
	if ep > 0 {
		s.Episodes = []int{ep}
	}
	if err == nil {
		s.Year, err = bounded(q.Get("year"), 0, 0, 3000)
	}
	return err
}

func ints(s string) ([]int, error) {
	var out []int
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("category %q is not a number", p)
		}
		out = append(out, n)
	}
	return out, nil
}

func bounded(s string, def, lo, hi int) (int, error) {
	if s == "" {
		return def, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < lo {
		return 0, fmt.Errorf("%q is not a valid number", s)
	}
	return min(n, hi), nil
}

// ── documents ────────────────────────────────────────────────────────────────

type errorDoc struct {
	XMLName     xml.Name `xml:"error"`
	Code        int      `xml:"code,attr"`
	Description string   `xml:"description,attr"`
}

func writeError(w io.Writer, code int, msg string) {
	_, _ = io.WriteString(w, xml.Header)
	_ = xml.NewEncoder(w).Encode(errorDoc{Code: code, Description: msg})
}

type capsDoc struct {
	XMLName   xml.Name    `xml:"caps"`
	Server    serverEl    `xml:"server"`
	Limits    limitsEl    `xml:"limits"`
	Searching searchingEl `xml:"searching"`
	Cats      []catEl     `xml:"categories>category"`
}

type serverEl struct {
	Title string `xml:"title,attr"`
}

type limitsEl struct {
	Max     int `xml:"max,attr"`
	Default int `xml:"default,attr"`
}

type searchingEl struct {
	Search modeEl `xml:"search"`
	TV     modeEl `xml:"tv-search"`
	Movie  modeEl `xml:"movie-search"`
	Music  modeEl `xml:"music-search"`
	Audio  modeEl `xml:"audio-search"`
	Book   modeEl `xml:"book-search"`
}

type modeEl struct {
	Available string `xml:"available,attr"`
	Params    string `xml:"supportedParams,attr"`
}

type catEl struct {
	ID   int     `xml:"id,attr"`
	Name string  `xml:"name,attr"`
	Subs []catEl `xml:"subcat"`
}

var parentNames = map[int]string{1000: "Console", 2000: "Movies", 3000: "Audio", 4000: "PC", 5000: "TV", 6000: "XXX", 7000: "Books", 8000: "Other"}

var modeParams = map[domain.SearchMode]string{
	domain.SearchText: "q", domain.SearchTV: "q,season,ep", domain.SearchMovie: "q", domain.SearchMusic: "q", domain.SearchBook: "q",
}

// CapsDoc merges the capabilities of the indexers behind an endpoint (pure).
func CapsDoc(title string, caps []plugin.Caps) []byte {
	modes := map[domain.SearchMode]bool{}
	ids := map[string]bool{}
	cats := map[int]string{}
	for _, c := range caps {
		for _, m := range c.Modes {
			modes[m] = true
		}
		for _, p := range c.IDParams {
			ids[p] = true
		}
		for id, name := range c.Categories {
			cats[id] = name
		}
	}
	mode := func(m domain.SearchMode, idParams ...string) modeEl {
		if !modes[m] {
			return modeEl{Available: "no", Params: modeParams[m]}
		}
		p := modeParams[m]
		for _, id := range idParams {
			if ids[id] {
				p += "," + id
			}
		}
		return modeEl{Available: "yes", Params: p}
	}
	music := mode(domain.SearchMusic)
	doc := capsDoc{Server: serverEl{Title: title}, Limits: limitsEl{Max: 500, Default: 100},
		Searching: searchingEl{Search: mode(domain.SearchText), TV: mode(domain.SearchTV, "imdbid", "tvdbid", "tmdbid"),
			Movie: mode(domain.SearchMovie, "imdbid", "tmdbid"), Music: music, Audio: music, Book: mode(domain.SearchBook)},
		Cats: categoryTree(cats)}
	out, _ := xml.MarshalIndent(doc, "", "  ")
	return append([]byte(xml.Header), out...)
}

func categoryTree(cats map[int]string) []catEl {
	parents := map[int]*catEl{}
	for id := range cats {
		p := id / 1000 * 1000
		if parents[p] == nil {
			parents[p] = &catEl{ID: p, Name: firstNonEmpty(parentNames[p], cats[p], strconv.Itoa(p))}
		}
		if id != p {
			parents[p].Subs = append(parents[p].Subs, catEl{ID: id, Name: cats[id]})
		}
	}
	var out []catEl
	for _, c := range parents {
		sort.Slice(c.Subs, func(i, j int) bool { return c.Subs[i].ID < c.Subs[j].ID })
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

type rssDoc struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Atom    string   `xml:"xmlns:atom,attr"`
	NS      string   `xml:"xmlns:torznab,attr"`
	Channel channel  `xml:"channel"`
}

type channel struct {
	Title string `xml:"title"`
	Items []item `xml:"item"`
}

type item struct {
	Title     string    `xml:"title"`
	GUID      string    `xml:"guid"`
	Link      string    `xml:"link"`
	Comments  string    `xml:"comments,omitempty"`
	PubDate   string    `xml:"pubDate"`
	Size      int64     `xml:"size"`
	Category  []int     `xml:"category"`
	Enclosure enclosure `xml:"enclosure"`
	Attrs     []attr    `xml:"torznab:attr"`
}

type enclosure struct {
	URL    string `xml:"url,attr"`
	Length int64  `xml:"length,attr"`
	Type   string `xml:"type,attr"`
}

type attr struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// ResultsDoc renders releases as a Torznab RSS document (pure); link gives
// each release's download address through corsarr.
func ResultsDoc(title string, rels []domain.Release, link func(domain.Release) string) []byte {
	doc := rssDoc{Version: "2.0", Atom: "http://www.w3.org/2005/Atom", NS: "http://torznab.com/schemas/2015/feed", Channel: channel{Title: title}}
	for _, r := range rels {
		doc.Channel.Items = append(doc.Channel.Items, itemOf(r, link(r)))
	}
	out, _ := xml.MarshalIndent(doc, "", "  ")
	return append([]byte(xml.Header), out...)
}

func itemOf(r domain.Release, link string) item {
	it := item{Title: r.Title, GUID: r.ID, Link: link, Comments: r.DetailsURL, Size: r.Size, Category: r.Categories,
		Enclosure: enclosure{URL: link, Length: r.Size, Type: "application/x-bittorrent"}}
	if !r.PublishedAt.IsZero() {
		it.PubDate = r.PublishedAt.UTC().Format(time.RFC1123Z)
	}
	add := func(name, v string) {
		if v != "" {
			it.Attrs = append(it.Attrs, attr{name, v})
		}
	}
	for _, c := range r.Categories {
		add("category", strconv.Itoa(c))
	}
	add("size", strconv.FormatInt(r.Size, 10))
	add("seeders", strconv.Itoa(r.Seeders))
	add("peers", strconv.Itoa(r.Peers))
	add("grabs", strconv.Itoa(r.Grabs))
	add("infohash", r.InfoHash)
	if strings.HasPrefix(r.DownloadURL, "magnet:") {
		add("magneturl", r.DownloadURL)
	}
	add("imdbid", strings.TrimPrefix(r.ExternalIDs["imdb"], "tt"))
	for _, f := range r.Flags {
		if f == "freeleech" {
			add("downloadvolumefactor", "0")
		}
	}
	return it
}
