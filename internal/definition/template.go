package definition

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/charset"
)

// Template is a parsed text template: literal text with {{ name|filter:arg }}
// substitutions. There are no loops; evaluation is linear in the template and
// its output is capped.
type Template struct{ parts []part }

type part struct {
	lit     string
	name    string // variable; empty for a literal
	filters []tfilter
}

type tfilter struct{ name, arg string }

var templateFilters = map[string]func(v, arg string) string{
	"urlencode":  func(v, _ string) string { return url.QueryEscape(v) },
	"pathescape": func(v, _ string) string { return url.PathEscape(v) },
	"lower":      func(v, _ string) string { return strings.ToLower(v) },
	"upper":      func(v, _ string) string { return strings.ToUpper(v) },
	"trim":       func(v, _ string) string { return strings.TrimSpace(v) },
	"json":       jsonString,
	"jsonarray":  jsonArray,
	"default": func(v, arg string) string {
		if v == "" {
			return arg
		}
		return v
	},
	"prefix": func(v, arg string) string {
		if v == "" {
			return ""
		}
		return arg + v
	},
}

// jsonString renders v as a quoted JSON string.
func jsonString(v, _ string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// jsonArray renders a comma list as a JSON array: numbers when every element
// is a number, strings otherwise, [] when empty.
func jsonArray(v, _ string) string {
	var items []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			items = append(items, s)
		}
	}
	numeric := true
	for _, s := range items {
		if _, err := strconv.ParseFloat(s, 64); err != nil {
			numeric = false
		}
	}
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = s
		if !numeric {
			out[i] = jsonString(s, "")
		}
	}
	return "[" + strings.Join(out, ",") + "]"
}

// ParseTemplate parses src once at load.
func ParseTemplate(src string) (*Template, error) {
	if len(src) > MaxTemplate {
		return nil, fmt.Errorf("template is longer than %d bytes", MaxTemplate)
	}
	t := &Template{}
	for src != "" {
		open := strings.Index(src, "{{")
		if open < 0 {
			t.parts = append(t.parts, part{lit: src})
			break
		}
		if open > 0 {
			t.parts = append(t.parts, part{lit: src[:open]})
		}
		end := strings.Index(src[open:], "}}")
		if end < 0 {
			return nil, fmt.Errorf("unclosed {{ in template")
		}
		p, err := parseExpr(src[open+2 : open+end])
		if err != nil {
			return nil, err
		}
		t.parts = append(t.parts, p)
		src = src[open+end+2:]
	}
	return t, nil
}

func parseExpr(expr string) (part, error) {
	segs := strings.Split(expr, "|")
	name := strings.TrimSpace(segs[0])
	if name == "" {
		return part{}, fmt.Errorf("empty {{ }} in template")
	}
	p := part{name: name}
	for _, s := range segs[1:] {
		fname, arg, _ := strings.Cut(strings.TrimSpace(s), ":")
		if templateFilters[fname] == nil {
			return part{}, fmt.Errorf("unknown template filter %q", fname)
		}
		p.filters = append(p.filters, tfilter{fname, arg})
	}
	if len(p.filters) > MaxFilters {
		return part{}, fmt.Errorf("at most %d filters", MaxFilters)
	}
	return p, nil
}

// Vars are the values templates see: query, keywords, season, ep, imdbid,
// imdbnum, tmdbid, tvdbid, year, categories, limit, offset, mode, and
// settings.<key>; row templates also see the row's extracted fields.
type Vars map[string]string

// Execute renders the template; unknown variables render empty.
func (t *Template) Execute(v Vars) (string, error) { return t.ExecuteIn(v, "") }

// escapers are the filters that percent-encode; with a site encoding they
// escape the site's bytes rather than UTF-8.
var escapers = map[string]bool{"urlencode": true, "pathescape": true}

// ExecuteIn renders the template for a site whose text is in encoding ("" =
// UTF-8): urlencode and pathescape escape the value's bytes in that encoding.
func (t *Template) ExecuteIn(v Vars, encoding string) (string, error) {
	var b strings.Builder
	for _, p := range t.parts {
		s := p.lit
		if p.name != "" {
			s = applyFilters(v[p.name], p.filters, encoding)
		}
		if b.Len()+len(s) > MaxExpanded {
			return "", fmt.Errorf("template output is longer than %d bytes", MaxExpanded)
		}
		b.WriteString(s)
	}
	return b.String(), nil
}

func applyFilters(s string, fs []tfilter, encoding string) string {
	for _, f := range fs {
		if escapers[f.name] {
			s = charset.FromUTF8(s, encoding)
		}
		s = templateFilters[f.name](s, f.arg)
	}
	return s
}

// Expand parses and executes src in one step (sources were validated at load).
func Expand(src string, v Vars) (string, error) { return ExpandIn(src, v, "") }

// ExpandIn is Expand for a site text encoding (see ExecuteIn).
func ExpandIn(src string, v Vars, encoding string) (string, error) {
	t, err := ParseTemplate(src)
	if err != nil {
		return "", err
	}
	return t.ExecuteIn(v, encoding)
}
