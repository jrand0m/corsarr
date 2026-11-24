package definition

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Problem is one reason a definition file is invalid, located by field path
// and line (0 when unknown).
type Problem struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Msg  string `json:"message"`
}

func (p Problem) String() string {
	loc := p.Path
	if p.Line > 0 {
		loc = fmt.Sprintf("%s (line %d)", p.Path, p.Line)
	}
	if loc == "" {
		return p.Msg
	}
	return loc + ": " + p.Msg
}

// Invalid lists every problem found in one file.
type Invalid struct{ Problems []Problem }

func (e *Invalid) Error() string {
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = p.String()
	}
	return "invalid definition: " + strings.Join(parts, "; ")
}

var yamlLine = regexp.MustCompile(`line (\d+)`)

// Parse reads a definition (YAML or JSON, which is valid YAML), rejects
// unknown keys and validates it. Errors are *Invalid.
func Parse(data []byte) (*Definition, error) {
	if len(data) > MaxFileSize {
		return nil, &Invalid{Problems: []Problem{{Msg: fmt.Sprintf("file is larger than %d bytes", MaxFileSize)}}}
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, &Invalid{Problems: []Problem{yamlProblem(err)}}
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var d Definition
	if err := dec.Decode(&d); err != nil {
		return nil, &Invalid{Problems: []Problem{yamlProblem(err)}}
	}
	d.normalize()
	if probs := d.Validate(); len(probs) > 0 {
		for i := range probs {
			probs[i].Line = lineOf(&root, probs[i].Path)
		}
		return nil, &Invalid{Problems: probs}
	}
	return &d, nil
}

func yamlProblem(err error) Problem {
	p := Problem{Msg: err.Error()}
	if m := yamlLine.FindStringSubmatch(err.Error()); m != nil {
		p.Line, _ = strconv.Atoi(m[1])
	}
	return p
}

// lineOf finds the line of a dotted path (a.b[2].c) in a YAML tree; 0 when absent.
func lineOf(root *yaml.Node, path string) int {
	n := root
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	line := n.Line
	for _, seg := range splitPath(path) {
		next := child(n, seg)
		if next == nil {
			return line
		}
		n, line = next, next.Line
	}
	return line
}

func child(n *yaml.Node, seg string) *yaml.Node {
	if n.Kind == yaml.SequenceNode {
		return item(n, seg)
	}
	for i := 0; i+1 < len(n.Content) && n.Kind == yaml.MappingNode; i += 2 {
		if n.Content[i].Value == seg {
			return n.Content[i+1]
		}
	}
	return nil
}

func item(n *yaml.Node, seg string) *yaml.Node {
	i, err := strconv.Atoi(seg)
	if err != nil || i < 0 || i >= len(n.Content) {
		return nil
	}
	return n.Content[i]
}

func splitPath(path string) []string {
	path = strings.NewReplacer("[", ".", "]", "").Replace(path)
	var out []string
	for _, s := range strings.Split(path, ".") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// UnmarshalYAML reads "2s", "500ms" or a number of seconds.
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	if s, err := strconv.ParseFloat(n.Value, 64); err == nil {
		*d = Duration(time.Duration(s * float64(time.Second)))
		return nil
	}
	v, err := time.ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: %q is not a duration", n.Line, n.Value)
	}
	*d = Duration(v)
	return nil
}

func (d *Definition) normalize() {
	if d.Version == "" {
		d.Version = "1"
	}
	if d.Privacy == "" {
		d.Privacy = Public
	}
	if d.Login.Method == "" {
		d.Login.Method = LoginNone
	}
	for i := range d.Search.Requests {
		r := &d.Search.Requests[i]
		r.Method = strings.ToUpper(r.Method)
		if r.Method == "" {
			r.Method = "GET"
		}
	}
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9]{1,39}$`)

// Validate lists every problem in an already decoded definition.
func (d *Definition) Validate() []Problem {
	var v validator
	v.identity(d)
	v.settings(d.Settings)
	v.login(d.Login)
	v.caps(d.Caps)
	v.search(d.Search)
	if d.Download != nil {
		v.selector("download.selector", d.Download.Selector)
	}
	return v.out
}

type validator struct{ out []Problem }

func (v *validator) add(path, format string, a ...any) {
	v.out = append(v.out, Problem{Path: path, Msg: fmt.Sprintf(format, a...)})
}

// when adds a problem when bad holds.
func (v *validator) when(bad bool, path, format string, a ...any) {
	if bad {
		v.add(path, format, a...)
	}
}

var privacies = map[Privacy]bool{Public: true, SemiPrivate: true, Private: true}

func (v *validator) identity(d *Definition) {
	v.when(d.Schema != SchemaVersion, "schema", "schema version %d is not supported (this build reads %d)", d.Schema, SchemaVersion)
	v.when(!idPattern.MatchString(d.ID), "id", "id %q must be 2-40 lower-case letters or digits, starting with a letter", d.ID)
	v.when(strings.TrimSpace(d.Name) == "", "name", "a display name is required")
	v.when(!privacies[d.Privacy], "privacy", "privacy must be public, semi-private or private")
	v.when(len(d.Links) == 0, "links", "at least one site address is required")
	for i, l := range d.Links {
		v.when(!isHTTP(l), fmt.Sprintf("links[%d]", i), "%q is not an http(s) address", l)
	}
}

func isHTTP(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

var settingKey = regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`)

var settingTypes = map[string]bool{"text": true, "password": true, "checkbox": true, "select": true}

var reservedSettings = map[string]bool{"baseUrl": true, "priority": true, "cookie": true, "categories": true}

func (v *validator) settings(ss []Setting) {
	seen := map[string]bool{}
	for i, s := range ss {
		p := fmt.Sprintf("settings[%d]", i)
		switch {
		case !settingKey.MatchString(s.Key):
			v.add(p+".key", "key %q must be camelCase letters and digits", s.Key)
		case reservedSettings[s.Key]:
			v.add(p+".key", "key %q is reserved", s.Key)
		case seen[s.Key]:
			v.add(p+".key", "key %q is declared twice", s.Key)
		}
		seen[s.Key] = true
		if !settingTypes[s.Type] {
			v.add(p+".type", "type must be text, password, checkbox or select")
		}
		if s.Type == "select" && len(s.Options) == 0 {
			v.add(p+".options", "a select needs options")
		}
	}
}

func (v *validator) login(l Login) {
	switch l.Method {
	case LoginNone, LoginCookie:
	case LoginForm:
		if l.Path == "" {
			v.add("login.path", "form login needs the path credentials are posted to")
		}
		if len(l.Fields) == 0 {
			v.add("login.fields", "form login needs the fields to post")
		}
		if l.Success.Empty() {
			v.add("login.success", "form login needs a success check")
		}
	default:
		v.add("login.method", "method must be none, cookie or form")
	}
	for name, c := range map[string]Check{"success": l.Success, "failure": l.Failure, "expired": l.Expired} {
		v.selector("login."+name+".selector", c.Selector)
	}
	for k, t := range l.Fields {
		v.template("login.fields."+k, t)
	}
}

func (v *validator) caps(c Caps) {
	v.when(len(c.Modes) == 0, "caps.modes", "at least one search mode is required")
	for m, params := range c.Modes {
		v.when(!knownModes[m], "caps.modes."+m, "unknown mode (search, tv, movie, music, book)")
		for _, p := range params {
			v.when(!knownParams[p], "caps.modes."+m, "unknown parameter %q", p)
		}
	}
	v.categories(c.Categories)
}

func (v *validator) categories(cats []Category) {
	seen := map[string]bool{}
	for i, cat := range cats {
		p := fmt.Sprintf("caps.categories[%d]", i)
		v.when(cat.Site == "" || seen[cat.Site], p+".site", "site category must be set and unique")
		seen[cat.Site] = true
		v.when(cat.Standard < 1000 || cat.Standard > 9999, p+".standard", "standard category %d is outside 1000-9999", cat.Standard)
	}
}

func (v *validator) search(s Search) {
	if len(s.Requests) == 0 {
		v.add("search.requests", "at least one request is required")
	}
	for i, r := range s.Requests {
		v.request(fmt.Sprintf("search.requests[%d]", i), r)
	}
	v.response(s.Response)
}

func (v *validator) request(p string, r Request) {
	if r.Method != "GET" && r.Method != "POST" {
		v.add(p+".method", "method must be GET or POST")
	}
	if r.Body != "" && r.Method != "POST" {
		v.add(p+".body", "only a POST request has a body")
	}
	for _, m := range r.Modes {
		if !knownModes[m] {
			v.add(p+".modes", "unknown mode %q", m)
		}
	}
	v.template(p+".path", r.Path)
	v.template(p+".body", r.Body)
	for k, t := range r.Query {
		v.template(p+".query."+k, t)
	}
	for k, t := range r.Headers {
		v.template(p+".headers."+k, t)
	}
}

func (v *validator) response(r Response) {
	switch r.Type {
	case ResponseJSON:
		if _, err := ParsePath(r.Rows); err != nil {
			v.add("search.response.rows", "%v", err)
		}
	case ResponseHTML:
		if r.Rows == "" {
			v.add("search.response.rows", "an HTML response needs a row selector")
		}
		v.selector("search.response.rows", r.Rows)
	default:
		v.add("search.response.type", "type must be json or html")
	}
	if _, ok := r.Fields[FieldTitle]; !ok {
		v.add("search.response.fields", "a title field is required")
	}
	if !hasAny(r.Fields, FieldDownload, FieldMagnet, FieldInfoHash, FieldDetails) {
		v.add("search.response.fields", "one of download, magnet, infohash or details is required")
	}
	for name, f := range r.Fields {
		v.field(r.Type, name, f)
	}
}

func hasAny(m map[string]FieldRule, keys ...string) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func (v *validator) field(t ResponseType, name string, f FieldRule) {
	p := "search.response.fields." + name
	v.when(!knownFields[name] && !strings.HasPrefix(name, "_"), p, "unknown field (helper fields start with _)")
	v.when(t == ResponseJSON && f.Selector != "", p+".selector", "a JSON response uses path, not selector")
	v.when(t == ResponseHTML && f.Path != "", p+".path", "an HTML response uses selector, not path")
	if _, err := ParsePath(f.Path); err != nil {
		v.add(p+".path", "%v", err)
	}
	v.selector(p+".selector", f.Selector)
	v.template(p+".value", f.Value)
	v.filters(p, f.Filters)
}

func (v *validator) filters(p string, fs []Filter) {
	v.when(len(fs) > MaxFilters, p+".filters", "at most %d filters", MaxFilters)
	for i, fl := range fs {
		if err := fl.check(); err != nil {
			v.add(fmt.Sprintf("%s.filters[%d]", p, i), "%v", err)
		}
	}
}

func (v *validator) selector(p, s string) {
	if s == "" {
		return
	}
	if _, err := ParseSelector(s); err != nil {
		v.add(p, "%v", err)
	}
}

func (v *validator) template(p, s string) {
	if s == "" {
		return
	}
	if _, err := ParseTemplate(s); err != nil {
		v.add(p, "%v", err)
	}
}

// errorsAs is errors.As for *Invalid, for callers listing problems.
func errorsAs(err error) (*Invalid, bool) {
	var inv *Invalid
	ok := errors.As(err, &inv)
	return inv, ok
}

// Problems returns the problems carried by a Parse error (or the error as one problem).
func Problems(err error) []Problem {
	if inv, ok := errorsAs(err); ok {
		return inv.Problems
	}
	return []Problem{{Msg: err.Error()}}
}
