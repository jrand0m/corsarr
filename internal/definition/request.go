package definition

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/charset"
	"github.com/jrand0m/corsarr/internal/domain"
)

// modeNames maps corsarr search modes to definition mode names.
var modeNames = map[domain.SearchMode]string{
	domain.SearchText: "search", domain.SearchTV: "tv", domain.SearchMovie: "movie",
	domain.SearchMusic: "music", domain.SearchBook: "book",
}

// ModeOf returns the definition mode for a corsarr search mode.
func ModeOf(m domain.SearchMode) string { return modeNames[m] }

// kindRanges are the standard category thousands of each media kind.
var kindRanges = map[domain.MediaKind]int{domain.KindTV: 5, domain.KindMovie: 2, domain.KindMusic: 3, domain.KindBook: 7}

// KindOf maps a standard category to a media kind ("" when none).
func KindOf(standard int) domain.MediaKind {
	for k, r := range kindRanges {
		if standard/1000 == r {
			return k
		}
	}
	return ""
}

// SiteCategories lists the site categories mapped into a media kind.
func (d *Definition) SiteCategories(kind domain.MediaKind) []string {
	var out []string
	for _, c := range d.Caps.Categories {
		if KindOf(c.Standard) == kind && kind != "" {
			out = append(out, c.Site)
		}
	}
	return out
}

// SiteCategoriesFor lists the site categories mapped to any of the standard
// ids (a parent id such as 2000 also selects its children 2010-2999).
func (d *Definition) SiteCategoriesFor(standard []int) []string {
	var out []string
	for _, c := range d.Caps.Categories {
		for _, s := range standard {
			if c.Standard == s || (s%1000 == 0 && c.Standard/1000 == s/1000) {
				out = append(out, c.Site)
				break
			}
		}
	}
	return out
}

// Query is a search translated for one definition: the mode it runs in and
// the template variables.
type Query struct {
	Mode string
	Vars Vars
}

// Translate builds the richest query the definition supports for req: the
// request's own mode when declared (with only the parameters the mode
// accepts), else a text search whose keywords carry the season/episode or
// year. categories are site categories; settings are the instance's values.
func (d *Definition) Translate(req domain.SearchRequest, categories []string, settings map[string]string) Query {
	mode := ModeOf(req.Mode)
	params, ok := d.Caps.Modes[mode]
	if !ok {
		mode, params = "search", d.Caps.Modes["search"]
	}
	accepts := map[string]bool{}
	for _, p := range params {
		accepts[p] = true
	}
	v := Vars{"mode": mode, "categories": strings.Join(categories, ",")}
	for k, s := range settings {
		v["settings."+k] = s
	}
	text := req.Text
	if text == "" && len(req.Titles) > 0 {
		text = req.Titles[0]
	}
	v["query"] = text
	setIDs(v, req, accepts)
	v["keywords"] = keywords(text, req, accepts)
	return Query{Mode: mode, Vars: v}
}

func setIDs(v Vars, req domain.SearchRequest, accepts map[string]bool) {
	set := func(param, val string) {
		if accepts[param] && val != "" && val != "0" {
			v[param] = val
		}
	}
	if id := req.IDs["imdb"]; id != "" {
		set("imdbid", "tt"+strings.TrimPrefix(id, "tt"))
		if v["imdbid"] != "" {
			v["imdbnum"] = strings.TrimPrefix(id, "tt")
		}
	}
	set("tmdbid", req.IDs["tmdb"])
	set("tvdbid", req.IDs["tvdb"])
	if req.Season > 0 {
		set("season", strconv.Itoa(req.Season))
	}
	if len(req.Episodes) == 1 {
		set("ep", strconv.Itoa(req.Episodes[0]))
	}
	if req.Year > 0 {
		set("year", strconv.Itoa(req.Year))
	}
}

// keywords is the text form: the query plus S01E02 / S01 or the year when the
// mode cannot take them as parameters.
func keywords(text string, req domain.SearchRequest, accepts map[string]bool) string {
	switch {
	case text == "":
		return ""
	case req.Mode == domain.SearchTV && req.Season > 0 && !accepts["season"]:
		return text + episodeTag(req)
	case req.Mode == domain.SearchMovie && req.Year > 0 && !accepts["year"]:
		if year := strconv.Itoa(req.Year); !strings.Contains(text, year) {
			return text + " " + year
		}
	}
	return text
}

func episodeTag(req domain.SearchRequest) string {
	tag := fmt.Sprintf(" S%02d", req.Season)
	if len(req.Episodes) == 1 {
		tag += fmt.Sprintf("E%02d", req.Episodes[0])
	}
	return tag
}

// Pick returns the first request matching the query's mode and condition.
func (d *Definition) Pick(q Query) (Request, bool) {
	for _, r := range d.Search.Requests {
		if (len(r.Modes) == 0 || contains(r.Modes, q.Mode)) && (r.When == "" || q.Vars[r.When] != "") {
			return r, true
		}
	}
	return Request{}, false
}

// HTTPRequest is a fully rendered request.
type HTTPRequest struct {
	Method      string
	URL         string
	Body        string
	ContentType string
	Headers     map[string]string
}

// Render expands a request template against the site base address.
//
// With a site encoding, the path, query values and a form body carry the
// text in that encoding (a JSON body stays UTF-8).
func (d *Definition) Render(base *url.URL, r Request, v Vars) (HTTPRequest, error) {
	u, err := d.renderURL(base, r, v)
	if err != nil {
		return HTTPRequest{}, err
	}
	out := HTTPRequest{Method: r.Method, URL: u, ContentType: r.Type}
	if out.Body, out.ContentType, err = d.renderBody(r, v); err != nil {
		return HTTPRequest{}, err
	}
	out.Headers, err = expandAll(v, "", d.Headers, r.Headers)
	return out, err
}

// renderURL expands the path and query (empty query values are left out).
func (d *Definition) renderURL(base *url.URL, r Request, v Vars) (string, error) {
	path, err := ExpandIn(r.Path, v, d.Encoding)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(d.EncodeText(path))
	if err != nil {
		return "", fmt.Errorf("request path %q: %w", path, err)
	}
	u = base.ResolveReference(u)
	q := u.Query()
	query, err := expandAll(v, d.Encoding, r.Query)
	if err != nil {
		return "", err
	}
	for k, val := range query {
		if val != "" {
			q.Set(k, d.EncodeText(val))
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// renderBody expands the body; a body without a type is a form.
func (d *Definition) renderBody(r Request, v Vars) (body, ctype string, err error) {
	if body, err = ExpandIn(r.Body, v, d.Encoding); err != nil {
		return "", "", err
	}
	ctype = r.Type
	if body != "" && ctype == "" {
		ctype = FormType
	}
	if ctype == FormType {
		body = d.EncodeText(body)
	}
	return body, ctype, nil
}

// FormType is the content type of a form-encoded body.
const FormType = "application/x-www-form-urlencoded"

// EncodeText converts text to the site's encoding (unchanged for UTF-8 sites).
func (d *Definition) EncodeText(s string) string { return charset.FromUTF8(s, d.Encoding) }

// expandAll renders the values of string maps into one map (later maps win).
func expandAll(v Vars, encoding string, maps ...Strings) (map[string]string, error) {
	out := map[string]string{}
	for _, m := range maps {
		for k, t := range m {
			val, err := ExpandIn(t, v, encoding)
			if err != nil {
				return nil, err
			}
			out[k] = val
		}
	}
	return out, nil
}
