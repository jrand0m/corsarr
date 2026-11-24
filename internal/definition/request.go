package definition

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

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
	if text == "" {
		return ""
	}
	switch {
	case req.Mode == domain.SearchTV && req.Season > 0 && !accepts["season"]:
		text += fmt.Sprintf(" S%02d", req.Season)
		if len(req.Episodes) == 1 {
			text += fmt.Sprintf("E%02d", req.Episodes[0])
		}
	case req.Mode == domain.SearchMovie && req.Year > 0 && !accepts["year"] && !strings.Contains(text, strconv.Itoa(req.Year)):
		text += " " + strconv.Itoa(req.Year)
	}
	return text
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
func (d *Definition) Render(base *url.URL, r Request, v Vars) (HTTPRequest, error) {
	path, err := Expand(r.Path, v)
	if err != nil {
		return HTTPRequest{}, err
	}
	u, err := url.Parse(path)
	if err != nil {
		return HTTPRequest{}, fmt.Errorf("request path %q: %w", path, err)
	}
	u = base.ResolveReference(u)
	q := u.Query()
	for k, t := range r.Query {
		val, err := Expand(t, v)
		if err != nil {
			return HTTPRequest{}, err
		}
		if val != "" {
			q.Set(k, val)
		}
	}
	u.RawQuery = q.Encode()
	out := HTTPRequest{Method: r.Method, URL: u.String(), ContentType: r.Type, Headers: map[string]string{}}
	if out.Body, err = Expand(r.Body, v); err != nil {
		return HTTPRequest{}, err
	}
	if out.Body != "" && out.ContentType == "" {
		out.ContentType = "application/x-www-form-urlencoded"
	}
	for _, hs := range []Strings{d.Headers, r.Headers} {
		for k, t := range hs {
			if out.Headers[k], err = Expand(t, v); err != nil {
				return HTTPRequest{}, err
			}
		}
	}
	return out, nil
}
