package definition

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/jrand0m/corsarr/internal/domain"
)

// ErrNoRows says a non-empty results page yielded no rows where the
// definition expects some: the site layout probably changed.
var ErrNoRows = errors.New("results page could not be parsed")

// Row is one result's extracted field values.
type Row map[string]string

// Rows reads the result rows of a response body.
func (d *Definition) Rows(body []byte) ([]Row, error) {
	if len(body) > MaxResponseSize {
		return nil, fmt.Errorf("response is larger than %d bytes", MaxResponseSize)
	}
	var rows []Row
	var err error
	if d.Search.Response.Type == ResponseHTML {
		rows, err = d.htmlRows(body)
	} else {
		rows, err = d.jsonRows(body)
	}
	if err != nil {
		return nil, err
	}
	return d.keep(rows), nil
}

func (d *Definition) jsonRows(body []byte) ([]Row, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoRows, err)
	}
	path, _ := ParsePath(d.Search.Response.Rows)
	v, ok := path.Get(doc)
	if !ok {
		return nil, fmt.Errorf("%w: no %q in the response", ErrNoRows, d.Search.Response.Rows)
	}
	list, ok := v.([]any)
	if !ok {
		if v == nil {
			return nil, nil // an API answering null for "nothing found"
		}
		return nil, fmt.Errorf("%w: %q is not a list", ErrNoRows, d.Search.Response.Rows)
	}
	var rows []Row
	for _, item := range list[:min(len(list), MaxRows)] {
		rows = append(rows, d.row(func(name string, f FieldRule) (string, bool) {
			p, _ := ParsePath(f.Path)
			v, ok := p.Get(item)
			return scalar(v), ok
		}))
	}
	return rows, nil
}

func (d *Definition) htmlRows(body []byte) ([]Row, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoRows, err)
	}
	sel, _ := ParseSelector(d.Search.Response.Rows)
	nodes := sel.All(doc)
	if len(nodes) == 0 && len(bytes.TrimSpace(body)) > 0 && !d.Search.Response.NoResults.holdsOn(doc, body) {
		return nil, ErrNoRows
	}
	var rows []Row
	for _, n := range nodes[:min(len(nodes), MaxRows)] {
		rows = append(rows, d.row(func(name string, f FieldRule) (string, bool) { return htmlField(n, f) }))
	}
	return rows, nil
}

func htmlField(row *html.Node, f FieldRule) (string, bool) {
	n := row
	if f.Selector != "" {
		sel, _ := ParseSelector(f.Selector)
		if n = sel.First(row); n == nil {
			return "", false
		}
	}
	if f.Attr != "" {
		return attrOK(n, strings.ToLower(f.Attr))
	}
	return Text(n), true
}

// row extracts every field: direct fields first, then value templates that
// may refer to them (and to helper fields).
func (d *Definition) row(read func(string, FieldRule) (string, bool)) Row {
	r := Row{}
	names := make([]string, 0, len(d.Search.Response.Fields))
	for name := range d.Search.Response.Fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if f := d.Search.Response.Fields[name]; f.Value == "" {
			v, _ := read(name, f)
			r[name] = finish(f, v)
		}
	}
	for _, name := range names {
		if f := d.Search.Response.Fields[name]; f.Value != "" {
			v, _ := Expand(f.Value, Vars(r))
			r[name] = finish(f, v)
		}
	}
	return r
}

func finish(f FieldRule, v string) string {
	v = applyAll(f.Filters, strings.TrimSpace(v))
	if v == "" {
		return f.Default
	}
	return v
}

// keep drops rows matching a skip condition or without a title.
func (d *Definition) keep(rows []Row) []Row {
	out := rows[:0]
	for _, r := range rows {
		if r[FieldTitle] != "" && !d.skipped(r) {
			out = append(out, r)
		}
	}
	return out
}

func (d *Definition) skipped(r Row) bool {
	for _, c := range d.Search.Response.Skip {
		if (c.Empty && r[c.Field] == "") || (!c.Empty && r[c.Field] == c.Equals) {
			return true
		}
	}
	return false
}

// Holds evaluates a check against a response (status, redirect target, body).
func (c Check) Holds(status int, location string, body []byte) bool {
	if c.redirects(status, location) || c.contains(body) {
		return true
	}
	if c.Selector == "" {
		return false
	}
	doc, err := html.Parse(bytes.NewReader(body))
	return err == nil && c.holdsOn(doc, nil)
}

func (c Check) redirects(status int, location string) bool {
	return c.Redirect != "" && status >= 300 && status < 400 && strings.Contains(location, c.Redirect)
}

func (c Check) contains(body []byte) bool {
	return c.Contains != "" && bytes.Contains(body, []byte(c.Contains))
}

func (c Check) holdsOn(doc *html.Node, body []byte) bool {
	if c.Contains != "" && body != nil && bytes.Contains(body, []byte(c.Contains)) {
		return true
	}
	if c.Selector == "" {
		return false
	}
	sel, err := ParseSelector(c.Selector)
	return err == nil && sel.First(doc) != nil
}

// Message returns the text of the check's selector on the page ("" when absent).
func (c Check) Message(body []byte) string { return c.Attr(body, "") }

// Attr returns an attribute (or, for "", the text) of the first element
// matching the check's selector ("" when absent).
func (c Check) Attr(body []byte, name string) string {
	if c.Selector == "" {
		return ""
	}
	doc, err := html.Parse(bytes.NewReader(body))
	sel, serr := ParseSelector(c.Selector)
	if err != nil || serr != nil {
		return ""
	}
	n := sel.First(doc)
	switch {
	case n == nil:
		return ""
	case name == "":
		return Text(n)
	}
	return attr(n, strings.ToLower(name))
}

// Link is where a release's payload comes from.
type Link struct {
	Download string // fetchable .torrent URL
	Magnet   string
	Details  string // page holding the link, for the follow-up step
}

var hexHash = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Release maps a row onto a release. ok is false when the row has no title
// or nothing to download.
func (d *Definition) Release(r Row, base *url.URL, now time.Time) (domain.Release, Link, bool) {
	link, hash := d.link(r, base)
	id := firstOf(hash, link.Details, link.Download)
	if id == "" || !d.fetchable(link) {
		return domain.Release{}, Link{}, false
	}
	rel := domain.Release{
		ID: id, Title: r[FieldTitle], Protocol: domain.ProtoTorrent, Size: ParseSize(r[FieldSize]), InfoHash: hash,
		DetailsURL: link.Details, Seeders: ParseInt(r[FieldSeeders]), Peers: peers(r), Grabs: ParseInt(r[FieldGrabs]),
		Categories: d.standardCategories(r[FieldCategory]), ExternalIDs: imdb(r[FieldIMDB]),
		DownloadURL: firstOf(link.Magnet, link.Download), SwarmReported: swarmReported(r), Files: ParseInt(r[FieldFiles]),
	}
	rel.PublishedAt, rel.Flags = publishDate(r[FieldDate], d.Search.Response.Fields[FieldDate].Formats, now)
	if ParseFloat(r[FieldDLFactor], 1) == 0 {
		rel.Flags = append(rel.Flags, "freeleech")
	}
	return rel, link, true
}

// link collects a row's payload links and info hash (a magnet is built from
// the hash when the row has none).
func (d *Definition) link(r Row, base *url.URL) (Link, string) {
	link := Link{Download: resolveURL(base, r[FieldDownload]), Magnet: r[FieldMagnet], Details: resolveURL(base, r[FieldDetails])}
	if strings.HasPrefix(link.Download, "magnet:") {
		link.Magnet, link.Download = link.Download, ""
	}
	hash := strings.ToLower(r[FieldInfoHash])
	if !hexHash.MatchString(hash) {
		hash = magnetHash(link.Magnet)
	}
	if link.Magnet == "" && hash != "" {
		link.Magnet = d.magnet(hash, r[FieldTitle])
	}
	return link, hash
}

// fetchable: a magnet or file link, or a details page the definition follows.
func (d *Definition) fetchable(l Link) bool {
	return l.Magnet != "" || l.Download != "" || (l.Details != "" && d.Download != nil)
}

func publishDate(raw string, layouts []string, now time.Time) (time.Time, []string) {
	t := ParseDate(raw, layouts, now)
	if t.After(now) {
		return now.UTC(), []string{"future-date"}
	}
	return t, nil
}

func peers(r Row) int {
	if v, ok := r[FieldPeers]; ok && v != "" {
		return ParseInt(v)
	}
	if v, ok := r[FieldLeechers]; ok && v != "" {
		return ParseInt(r[FieldSeeders]) + ParseInt(v)
	}
	return 0
}

// swarmReported is true when the row carried any seeder or peer count.
func swarmReported(r Row) bool {
	return r[FieldSeeders] != "" || r[FieldPeers] != "" || r[FieldLeechers] != ""
}

func imdb(v string) map[string]string {
	digits := nonDigits.ReplaceAllString(v, "")
	if digits == "" || strings.Trim(digits, "0") == "" {
		return nil
	}
	return map[string]string{"imdb": "tt" + digits}
}

func firstOf(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func resolveURL(base *url.URL, ref string) string {
	if ref == "" || strings.HasPrefix(ref, "magnet:") || base == nil {
		return ref
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	return base.ResolveReference(u).String()
}

func magnetHash(m string) string {
	u, err := url.Parse(m)
	if err != nil || u.Scheme != "magnet" {
		return ""
	}
	for _, xt := range u.Query()["xt"] {
		if h := strings.ToLower(strings.TrimPrefix(xt, "urn:btih:")); hexHash.MatchString(h) {
			return h
		}
	}
	return ""
}

func (d *Definition) magnet(hash, title string) string {
	q := url.Values{"dn": {title}}
	for _, tr := range d.MagnetTrackers {
		q.Add("tr", tr)
	}
	return "magnet:?xt=urn:btih:" + hash + "&" + q.Encode()
}

// standardCategories maps a comma list of site categories to standard ids.
func (d *Definition) standardCategories(site string) []int {
	var out []int
	for _, s := range strings.Split(site, ",") {
		s = strings.TrimSpace(s)
		for _, c := range d.Caps.Categories {
			if c.Site == s && !containsInt(out, c.Standard) {
				out = append(out, c.Standard)
			}
		}
	}
	return out
}

func containsInt(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
