package definition

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/jrand0m/corsarr/internal/domain"
)

const jsonDef = `
schema: 1
id: sample
name: Sample JSON
links: [https://api.example.test/]
headers: {Accept: application/json}
caps:
  modes:
    search: [q]
    movie: [q, imdbid]
    tv: [q]
  categories:
    - {site: "201", standard: 2000, name: Movies}
    - {site: "205", standard: 5000, name: TV}
    - {site: "101", standard: 3000, name: Music}
magnetTrackers: [udp://tracker.example.test:1337]
search:
  requests:
    - modes: [movie]
      when: imdbid
      path: /q
      query: {q: "{{imdbid}}"}
    - path: /q
      query: {q: "{{keywords}}", cat: "{{categories}}"}
  response:
    type: json
    rows: ""
    skip: [{field: _id, equals: "0"}]
    fields:
      _id: {path: id}
      title: {path: name}
      infohash: {path: info_hash}
      size: {path: size}
      seeders: {path: seeders}
      leechers: {path: leechers}
      date: {path: added}
      category: {path: category}
      imdb: {path: imdb}
      details: {value: "/item/{{_id}}"}
`

func mustParse(t *testing.T, src string) *Definition {
	t.Helper()
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestParseFillsDefaults(t *testing.T) {
	d := mustParse(t, jsonDef)
	if d.Version != "1" || d.Privacy != Public || d.Login.Method != LoginNone || d.Search.Requests[0].Method != "GET" {
		t.Fatalf("defaults not applied: %+v", d)
	}
}

func TestParseReportsProblemsWithLines(t *testing.T) {
	src := strings.Replace(jsonDef, "id: sample", "id: Bad_Id", 1)
	src = strings.Replace(src, "type: json", "type: xml", 1)
	_, err := Parse([]byte(src))
	probs := Problems(err)
	if len(probs) != 2 {
		t.Fatalf("problems = %v", probs)
	}
	got := map[string]int{}
	for _, p := range probs {
		got[p.Path] = p.Line
	}
	if got["id"] != 3 || got["search.response.type"] == 0 {
		t.Errorf("lines = %v", got)
	}
	if !strings.Contains(err.Error(), "id (line 3)") {
		t.Errorf("message %q", err)
	}
}

func TestParseRejectsUnknownKeysSyntaxAndSize(t *testing.T) {
	for name, src := range map[string]string{
		"unknown key": jsonDef + "\nbogus: 1\n",
		"syntax":      "schema: [1\n",
		"too big":     strings.Repeat("#", MaxFileSize+1),
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if p := Problems(errors.New("plain")); p[0].Msg != "plain" {
		t.Error(p)
	}
}

func TestValidateCatchesEachSection(t *testing.T) {
	d := &Definition{Schema: 2, Privacy: "secret", Links: []string{"ftp://x"},
		Settings: []Setting{{Key: "baseUrl", Type: "text"}, {Key: "a", Type: "nope"}, {Key: "a", Type: "select"}, {Key: "9", Type: "text"}},
		Login:    Login{Method: LoginForm, Success: Check{Selector: "[["}, Fields: Strings{}},
		Caps:     Caps{Modes: map[string][]string{"radio": {"zz"}}, Categories: []Category{{Site: "", Standard: 1}}},
		Search: Search{Requests: []Request{{Method: "PUT", Body: "x", Modes: []string{"x"}, Path: "{{", Query: Strings{"a": "{{a|bogus}}"}, Headers: Strings{"h": "{{}}"}}},
			Response: Response{Type: ResponseHTML, Fields: map[string]FieldRule{"weird": {Path: "a[x]", Filters: []Filter{{Name: "nope"}}}}}},
		Download: &Download{Selector: ">"},
	}
	probs := d.Validate()
	if len(probs) < 20 {
		for _, p := range probs {
			t.Log(p)
		}
		t.Fatalf("only %d problems", len(probs))
	}
	d2 := &Definition{Login: Login{Method: "magic"}, Search: Search{Response: Response{Type: ResponseJSON, Rows: "a[", Fields: map[string]FieldRule{"title": {Selector: "a"}}}}}
	if len(d2.Validate()) < 5 {
		t.Fatal(d2.Validate())
	}
}

func TestTemplates(t *testing.T) {
	v := Vars{"query": "Some Show", "categories": "1,2", "names": "a,b", "empty": ""}
	cases := map[string]string{
		"q={{query|urlencode}}":                  "q=Some+Show",
		"{{query|lower|trim}}":                   "some show",
		"{{query|upper}}":                        "SOME SHOW",
		"{{categories|jsonarray}}":               "[1,2]",
		"{{names|jsonarray}}":                    `["a","b"]`,
		"{{empty|jsonarray}}":                    "[]",
		"{{query|json}}":                         `"Some Show"`,
		"{{empty|default:x}}":                    "x",
		"{{query|default:x}}":                    "Some Show",
		"{{empty|prefix:&p=}}{{query|prefix:/}}": "/Some Show",
		"{{query|pathescape}}":                   "Some%20Show",
		"{{missing}}":                            "",
	}
	for src, want := range cases {
		got, err := Expand(src, v)
		if err != nil || got != want {
			t.Errorf("%s = %q, %v; want %q", src, got, err, want)
		}
	}
	for _, bad := range []string{"{{", "{{ }}", "{{a|nope}}", strings.Repeat("x", MaxTemplate+1), "{{a" + strings.Repeat("|trim", MaxFilters+1) + "}}"} {
		if _, err := ParseTemplate(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	big := Vars{"x": strings.Repeat("y", MaxExpanded)}
	if _, err := Expand("{{x}}{{x}}", big); err == nil {
		t.Error("oversize output accepted")
	}
}

func TestFilters(t *testing.T) {
	f := func(name string, args ...string) Filter {
		fl := Filter{Name: name, Args: args}
		if err := fl.check(); err != nil {
			t.Fatal(err)
		}
		return fl
	}
	cases := []struct {
		f    Filter
		in   string
		want string
	}{
		{f("trim"), "  a \n b ", "a b"},
		{f("lower"), "AB", "ab"},
		{f("upper"), "ab", "AB"},
		{f("urldecode"), "a%20b", "a b"},
		{f("urldecode"), "%zz", "%zz"},
		{f("urlencode"), "a b", "a+b"},
		{f("digits"), "id-123x", "123"},
		{f("regex", `id=(\d+)`), "x?id=42", "42"},
		{f("regex", `\d+`), "ab12", "12"},
		{f("regex", `\d+`), "ab", ""},
		{f("prepend", "/"), "a", "/a"},
		{f("append", "/"), "a", "a/"},
		{f("remove", "-"), "a-b", "ab"},
		{f("replace", ",", "."), "1,5", "1.5"},
		{f("split", "|", "1"), "a|b|c", "b"},
		{f("split", "|", "-1"), "a|b|c", "c"},
		{f("split", "|", "9"), "a|b", ""},
	}
	for _, c := range cases {
		if got := c.f.Apply(c.in); got != c.want {
			t.Errorf("%s(%q) = %q, want %q", c.f.Name, c.in, got, c.want)
		}
	}
	if got := (Filter{Name: "regex", Args: []string{"("}}).Apply("x"); got != "" {
		t.Error(got)
	}
	for _, bad := range []Filter{{Name: "x"}, {Name: "trim", Args: []string{"a"}}, {Name: "split", Args: []string{",", "x"}}, {Name: "regex", Args: []string{"("}}} {
		if bad.check() == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
	if got := applyAll(nil, strings.Repeat("a", MaxValue+5)); len(got) != MaxValue {
		t.Error(len(got))
	}
}

func TestFilterYAMLForms(t *testing.T) {
	src := strings.Replace(jsonDef, "title: {path: name}", "title:\n        path: name\n        filters: [trim, {regex: \"(.+)\"}, {replace: [a, b]}]", 1)
	d := mustParse(t, src)
	fl := d.Search.Response.Fields["title"].Filters
	if len(fl) != 3 || fl[1].re == nil || fl[2].Args[1] != "b" {
		t.Fatalf("%+v", fl)
	}
	for _, bad := range []string{"filters: [{a: 1, b: 2}]", "filters: [[x]]", "filters: [{regex: \"(\"}]"} {
		s := strings.Replace(jsonDef, "title: {path: name}", "title:\n        path: name\n        "+bad, 1)
		if _, err := Parse([]byte(s)); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestDurations(t *testing.T) {
	for in, want := range map[string]time.Duration{"2s": 2 * time.Second, "1.5": 1500 * time.Millisecond} {
		d := mustParse(t, jsonDef+"rateLimit: {interval: "+in+"}\n")
		if time.Duration(d.RateLimit.Interval) != want {
			t.Errorf("%s = %v", in, d.RateLimit.Interval)
		}
	}
	if _, err := Parse([]byte(jsonDef + "rateLimit: {interval: soon}\n")); err == nil {
		t.Error("bad duration accepted")
	}
}

func TestPaths(t *testing.T) {
	doc := map[string]any{"a": []any{map[string]any{"b": "x"}}, "n": nil}
	for path, want := range map[string]string{"a[0].b": "x", "": "", "a[1].b": "", "a.b": "", "a[0].b[0]": "", "n.x": ""} {
		p, err := ParsePath(path)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := p.Get(doc)
		if path == "" {
			continue
		}
		if got := scalar(v); got != want {
			t.Errorf("%s = %q", path, got)
		}
	}
	for _, bad := range []string{"a[x]", "a[0", "a[0]b"} {
		if _, err := ParsePath(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if scalar(true) != "true" || scalar([]any{"a", nil, "b"}) != "a,b" || scalar(map[string]any{"k": 1}) != `{"k":1}` {
		t.Error("scalar")
	}
}

func TestSizesNumbersDates(t *testing.T) {
	for in, want := range map[string]int64{
		"1.5 GiB": 1610612736, "700 MB": 734003200, "1,234 KB": 1263616, "1.234,5 KB": 1264128, "1,5 GB": 1610612736,
		"123456": 123456, "10 bytes": 10, "x": 0, "1 PB": 1 << 50, "99999999999 PB": 0,
	} {
		if got := ParseSize(in); got != want {
			t.Errorf("ParseSize(%q) = %d, want %d", in, got, want)
		}
	}
	if ParseInt("1,234") != 1234 || ParseInt("-") != 0 || ParseFloat("0.5", 1) != 0.5 || ParseFloat("", 1) != 1 {
		t.Error("numbers")
	}
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Time{
		"2026-05-01 10:00:00": time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
		"1746000000":          time.Unix(1746000000, 0).UTC(),
		"1746000000000":       time.UnixMilli(1746000000000).UTC(),
		"3 hours ago":         now.Add(-3 * time.Hour),
		"a day ago":           time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC),
		"today 08:30":         time.Date(2026, 5, 10, 8, 30, 0, 0, time.UTC),
		"yesterday":           time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC),
		"":                    {},
		"soonish":             {},
	} {
		if got := ParseDate(in, nil, now); !got.Equal(want) {
			t.Errorf("ParseDate(%q) = %v, want %v", in, got, want)
		}
	}
	if got := ParseDate("10/05/26", []string{"02/01/06"}, now); got.Day() != 10 || got.Month() != 5 {
		t.Error(got)
	}
}

const apiRows = `[
 {"id":"7","name":"Movie Title 2024 1080p","info_hash":"0123456789ABCDEF0123456789ABCDEF01234567","size":"1073741824","seeders":"12","leechers":"3","added":"1746000000","category":"201","imdb":"tt0111161"},
 {"id":"8","name":"No hash","info_hash":"","size":"1","seeders":"0","leechers":"0","added":"0","category":"999","imdb":""},
 {"id":"0","name":"No results returned","info_hash":"0000000000000000000000000000000000000000"}
]`

func TestJSONRowsToReleases(t *testing.T) {
	d := mustParse(t, jsonDef)
	rows, err := d.Rows([]byte(apiRows))
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows %v %v", rows, err)
	}
	base, _ := url.Parse(d.Links[0])
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	rel, link, ok := d.Release(rows[0], base, now)
	if !ok {
		t.Fatal("not mapped")
	}
	if rel.ID != "0123456789abcdef0123456789abcdef01234567" || rel.Size != 1<<30 || rel.Seeders != 12 || rel.Peers != 15 ||
		rel.Categories[0] != 2000 || rel.ExternalIDs["imdb"] != "tt0111161" || rel.DetailsURL != "https://api.example.test/item/7" {
		t.Errorf("release %+v", rel)
	}
	if !strings.HasPrefix(link.Magnet, "magnet:?xt=urn:btih:0123") || !strings.Contains(link.Magnet, "tr=udp") || rel.DownloadURL != link.Magnet {
		t.Errorf("magnet %q", link.Magnet)
	}
	if !rel.PublishedAt.Equal(now) || rel.Flags[0] != "future-date" {
		t.Errorf("future date not clamped: %v %v", rel.PublishedAt, rel.Flags)
	}
	if _, _, ok := d.Release(rows[1], base, now); ok {
		t.Error("row without any link mapped")
	}
}

func TestJSONRowErrors(t *testing.T) {
	d := mustParse(t, strings.Replace(jsonDef, `rows: ""`, "rows: data.items", 1))
	for body, wantErr := range map[string]bool{`{"data":{"items":[]}}`: false, `{"data":{"items":null}}`: false,
		`{"data":{}}`: true, `{"data":{"items":3}}`: true, `<html>`: true} {
		_, err := d.Rows([]byte(body))
		if (err != nil) != wantErr || (err != nil && !errors.Is(err, ErrNoRows)) {
			t.Errorf("%s: %v", body, err)
		}
	}
	if _, err := d.Rows(make([]byte, MaxResponseSize+1)); err == nil {
		t.Error("oversize accepted")
	}
}

func TestReleaseVariants(t *testing.T) {
	d := mustParse(t, jsonDef)
	d.Download = &Download{Selector: "a"}
	base, _ := url.Parse("https://site.test/")
	now := time.Now()
	rel, link, ok := d.Release(Row{"title": "T", "download": "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567", "peers": "4", "downloadFactor": "0", "imdb": "0"}, base, now)
	if !ok || rel.InfoHash == "" || link.Download != "" || rel.Peers != 4 || rel.Flags[0] != "freeleech" || rel.ExternalIDs != nil {
		t.Errorf("%+v %+v", rel, link)
	}
	rel, link, ok = d.Release(Row{"title": "T", "details": "/t/1"}, base, now)
	if !ok || link.Details != "https://site.test/t/1" || rel.DownloadURL != "" {
		t.Errorf("details-only %+v", link)
	}
	if magnetHash("http://x") != "" || magnetHash("magnet:?xt=urn:btih:short") != "" || resolveURL(base, "%zz") != "" {
		t.Error("helpers")
	}
}

const htmlDef = `
schema: 1
id: pagesite
name: Page Site
links: [https://pages.example.test/]
caps:
  modes: {search: [q]}
  categories: [{site: "4", standard: 3000}]
search:
  requests: [{path: "/search/{{keywords|pathescape}}/"}]
  response:
    type: html
    rows: "table#results > tbody > tr.row"
    noResults: {selector: "div.empty"}
    fields:
      title: {selector: "td.name a.title"}
      details: {selector: "td.name a.title", attr: href}
      download: {selector: "a[href$=\".torrent\"]", attr: href}
      size: {selector: "td:nth-child(3)"}
      seeders: {selector: "td.s"}
      leechers: {selector: "td:last-child"}
      category: {selector: "td.cat a", attr: href, filters: [{regex: "cat=(\\d+)"}]}
      date: {selector: "td.date", attr: title, formats: ["02 Jan 2006"]}
`

const htmlPage = `<html><body>
<table id="results"><tbody>
<tr class="head"><th>Name</th></tr>
<tr class="row"><td class="name"><a class="title" href="/t/1">Album One FLAC</a> <a href="/dl/1.torrent">dl</a></td>
  <td class="cat"><a href="/browse?cat=4">Music</a></td><td>512 MB</td><td class="s">7</td><td class="date" title="03 Mar 2025">x</td><td>2</td></tr>
<tr class="row"><td class="name"><a class="title" href="/t/2">Album Two</a> <a href="/dl/2.torrent">dl</a></td>
  <td class="cat"><a href="/browse?cat=9">Other</a></td><td>1 GB</td><td class="s">1</td><td class="date" title="">x</td><td>0</td></tr>
</tbody></table></body></html>`

func TestHTMLRows(t *testing.T) {
	d := mustParse(t, htmlDef)
	rows, err := d.Rows([]byte(htmlPage))
	if err != nil || len(rows) != 2 {
		t.Fatalf("%v %v", rows, err)
	}
	base, _ := url.Parse(d.Links[0])
	rel, link, ok := d.Release(rows[0], base, time.Now())
	if !ok || rel.Title != "Album One FLAC" || rel.Size != 512<<20 || rel.Seeders != 7 || rel.Peers != 9 ||
		link.Download != "https://pages.example.test/dl/1.torrent" || rel.Categories[0] != 3000 || rel.PublishedAt.Year() != 2025 {
		t.Errorf("%+v %+v", rel, link)
	}
	if _, err := d.Rows([]byte(`<html><body><p>changed layout</p></body></html>`)); !errors.Is(err, ErrNoRows) {
		t.Errorf("layout change: %v", err)
	}
	if rows, err := d.Rows([]byte(`<html><div class="empty">Nothing</div></html>`)); err != nil || len(rows) != 0 {
		t.Errorf("no results: %v %v", rows, err)
	}
}

func TestSelectors(t *testing.T) {
	page := `<div id="a" class="x y"><p data-k="v1">one</p><p>two</p><span><p data-k="w">three</p></span></div>`
	d := mustParse(t, htmlDef)
	_ = d
	cases := map[string]int{
		"p": 3, "div > p": 2, "#a p": 3, ".x.y p:first-child": 2, "p:nth-child(2)": 1, "p:last-child": 1,
		"[data-k]": 2, "[data-k=v1]": 1, "[data-k^=v]": 1, "[data-k*=1]": 1, "p, span": 4, "*": 8, "div.z": 0,
	}
	doc := parseHTML(t, page)
	for s, want := range cases {
		sel, err := ParseSelector(s)
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		if got := len(sel.All(doc)); got != want {
			t.Errorf("%s matched %d, want %d", s, got, want)
		}
	}
	for _, bad := range []string{"", "> p", "p >", "a > > b", "#", "[x", "[a b]", ":hover", ":nth-child(x)", ":nth-child(1", "p!"} {
		if _, err := ParseSelector(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	sel, _ := ParseSelector("em")
	if sel.First(doc) != nil {
		t.Error("first of nothing")
	}
}

func TestChecks(t *testing.T) {
	page := []byte(`<html><div class="err">Wrong password</div></html>`)
	if !(Check{Selector: "div.err"}).Holds(200, "", page) || (Check{Selector: "div.ok"}).Holds(200, "", page) {
		t.Error("selector")
	}
	if !(Check{Contains: "Wrong"}).Holds(200, "", page) || !(Check{Redirect: "login"}).Holds(302, "/login.php", nil) || (Check{}).Holds(200, "", page) {
		t.Error("contains/redirect")
	}
	if (Check{Redirect: "login"}).Holds(200, "/login", nil) {
		t.Error("redirect without 3xx")
	}
	if (Check{Selector: "div.err"}).Message(page) != "Wrong password" || (Check{}).Message(page) != "" || (Check{Selector: "b"}).Message(page) != "" || (Check{Selector: "[["}).Message(page) != "" {
		t.Error("message")
	}
}

func TestTranslatePickRender(t *testing.T) {
	d := mustParse(t, jsonDef)
	base, _ := url.Parse(d.Links[0])
	movie := domain.SearchRequest{Kind: domain.KindMovie, Mode: domain.SearchMovie, Titles: []string{"Film"}, Year: 1994, IDs: map[string]string{"imdb": "tt0111161"}}
	q := d.Translate(movie, d.SiteCategories(domain.KindMovie), map[string]string{"k": "v"})
	if q.Mode != "movie" || q.Vars["imdbid"] != "tt0111161" || q.Vars["imdbnum"] != "0111161" || q.Vars["keywords"] != "Film 1994" || q.Vars["settings.k"] != "v" {
		t.Fatalf("%+v", q)
	}
	r, ok := d.Pick(q)
	hr, err := d.Render(base, r, q.Vars)
	if !ok || err != nil || hr.URL != "https://api.example.test/q?q=tt0111161" || hr.Headers["Accept"] != "application/json" {
		t.Fatalf("%+v %v", hr, err)
	}
	tv := domain.SearchRequest{Kind: domain.KindTV, Mode: domain.SearchTV, Text: "Show", Season: 1, Episodes: []int{2}}
	q = d.Translate(tv, d.SiteCategories(domain.KindTV), nil)
	r, _ = d.Pick(q)
	hr, _ = d.Render(base, r, q.Vars)
	if hr.URL != "https://api.example.test/q?cat=205&q=Show+S01E02" {
		t.Errorf("tv %s", hr.URL)
	}
	book := domain.SearchRequest{Mode: domain.SearchBook, Text: ""}
	if q = d.Translate(book, nil, nil); q.Mode != "search" || q.Vars["keywords"] != "" {
		t.Errorf("book fallback %+v", q)
	}
	d.Search.Requests = d.Search.Requests[:1]
	if _, ok := d.Pick(d.Translate(tv, nil, nil)); ok {
		t.Error("picked a movie-only request for tv")
	}
	if _, err := d.Render(base, Request{Path: "{{"}, nil); err == nil {
		t.Error("bad path rendered")
	}
	post, _ := d.Render(base, Request{Method: "POST", Path: "/s", Body: `{"q":{{query|json}}}`, Type: "application/json"}, Vars{"query": "x"})
	if post.Body != `{"q":"x"}` || post.ContentType != "application/json" {
		t.Errorf("%+v", post)
	}
	form, _ := d.Render(base, Request{Method: "POST", Path: "/s", Body: "a=1"}, nil)
	if form.ContentType != "application/x-www-form-urlencoded" {
		t.Error(form.ContentType)
	}
}

func TestCategories(t *testing.T) {
	d := mustParse(t, jsonDef)
	if KindOf(5040) != domain.KindTV || KindOf(9000) != "" {
		t.Error("KindOf")
	}
	if got := d.SiteCategoriesFor([]int{2000, 3000}); len(got) != 2 {
		t.Error(got)
	}
	if got := d.SiteCategories(""); got != nil {
		t.Error(got)
	}
	if ModeOf(domain.SearchMusic) != "music" {
		t.Error("ModeOf")
	}
}

func parseHTML(t *testing.T, s string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
