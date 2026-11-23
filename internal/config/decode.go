package config

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// FieldError is one validation failure with its key and source location.
type FieldError struct {
	Path string   `json:"path"`
	Loc  Location `json:"location"`
	Msg  string   `json:"message"`
}

func (e FieldError) Error() string { return fmt.Sprintf("%s (%s): %s", e.Path, e.Loc, e.Msg) }

// Errors is every validation failure of one load, sorted by path.
type Errors []FieldError

func (es Errors) Error() string {
	lines := make([]string, len(es))
	for i, e := range es {
		lines[i] = e.Error()
	}
	return strings.Join(lines, "\n")
}

// decoder walks a raw tree against the schema, collecting every error.
type decoder struct {
	s     *Schema
	res   Resolver
	errs  Errors
	warns []string
	locs  map[string]Location
	file  bool // literal secrets in a file earn a warning
}

// tree is a decoded value: map[string]any for objects/maps/instances, []any
// for lists, and string/int64/float64/bool/time.Duration/Secret leaves.
type tree = map[string]any

func (d *decoder) fail(path string, loc Location, format string, a ...any) {
	d.errs = append(d.errs, FieldError{Path: path, Loc: loc, Msg: fmt.Sprintf(format, a...)})
}

func join(path, seg string) string {
	if path == "" {
		return seg
	}
	return path + "." + seg
}

// value decodes r (nil = absent) against n. ok=false means "no value".
func (d *decoder) value(n *Node, r *raw, path string, parent Location) (any, bool) {
	if r != nil {
		d.locs[path] = r.loc
		parent = r.loc
	}
	switch n.Kind {
	case KindObject:
		return d.object(n.Fields, r, path, parent), true
	case KindMap:
		return d.mapping(n, r, path), true
	case KindList:
		return d.list(n, r, path), true
	case KindInstance:
		return d.instance(n, r, path, parent)
	}
	return d.leaf(n, r, path, parent)
}

func (d *decoder) branch(r *raw, path string) bool {
	if r != nil && r.leaf {
		d.fail(path, r.loc, "must be a section, not a value")
		return false
	}
	return r != nil
}

func (d *decoder) object(fields []Field, r *raw, path string, parent Location) tree {
	out := tree{}
	present := d.branch(r, path)
	byName := map[string]*raw{}
	if present {
		byName = d.matchFields(fields, r, path)
	}
	for _, f := range fields {
		if v, ok := d.value(f.Node, byName[strings.ToLower(f.Name)], join(path, f.Name), parent); ok {
			out[f.Name] = v
		}
	}
	return out
}

// matchFields maps lower-cased field names to raw children; field names
// match case-insensitively in every source, unknown keys are errors.
func (d *decoder) matchFields(fields []Field, r *raw, path string) map[string]*raw {
	known := map[string]bool{}
	for _, f := range fields {
		known[strings.ToLower(f.Name)] = true
	}
	out := map[string]*raw{}
	for _, k := range r.sortedKeys() {
		lk := strings.ToLower(k)
		if !known[lk] {
			d.fail(join(path, k), r.kids[k].loc, "unknown key")
			continue
		}
		out[lk] = r.kids[k]
	}
	return out
}

func (d *decoder) mapping(n *Node, r *raw, path string) tree {
	out := tree{}
	if !d.branch(r, path) {
		return out
	}
	for _, k := range r.sortedKeys() {
		child := r.kids[k]
		if _, err := domain.ParseObjectKey(k); err != nil {
			d.fail(join(path, k), child.loc, "object key must match [a-z][a-z0-9]*; try %q with name: %q", suggestKey(k), k)
			continue
		}
		if v, ok := d.value(n.Elem, child, join(path, k), child.loc); ok {
			out[k] = v
		}
	}
	return out
}

var nonKey = regexp.MustCompile(`[^a-z0-9]`)

func suggestKey(k string) string {
	s := nonKey.ReplaceAllString(strings.ToLower(k), "")
	if s == "" || s[0] < 'a' {
		s = "x" + s
	}
	return s
}

func (d *decoder) list(n *Node, r *raw, path string) []any {
	out := []any{}
	if !d.branch(r, path) {
		return out
	}
	for _, k := range r.sortedKeys() {
		child := r.kids[k]
		if _, err := strconv.Atoi(k); err != nil {
			d.fail(join(path, k), child.loc, "list index must be a number")
			continue
		}
		if v, ok := d.value(n.Elem, child, join(path, strconv.Itoa(len(out))), child.loc); ok {
			out = append(out, v)
		}
	}
	return out
}

// instance decodes a plugin instance: `type` picks the manifest whose
// settings schema defines the remaining fields.
func (d *decoder) instance(n *Node, r *raw, path string, parent Location) (any, bool) {
	if !d.branch(r, path) {
		return nil, false
	}
	m, ok := d.manifestFor(n.Plugin, r, path)
	if !ok {
		return nil, false
	}
	fields := append(commonInstanceFields(n.Plugin, d.s.Plugins), pluginFields(m)...)
	return d.object(fields, r, path, parent), true
}

func (d *decoder) manifestFor(k plugin.Kind, r *raw, path string) (plugin.Manifest, bool) {
	var typ *raw
	for key, c := range r.kids {
		if strings.EqualFold(key, "type") {
			typ = c
		}
	}
	if typ == nil || !typ.leaf || typ.val == "" {
		d.fail(join(path, "type"), r.loc, "is required (a %s plugin id)", k)
		return plugin.Manifest{}, false
	}
	reg, found := d.s.Plugins.Get(typ.val)
	if !found || reg.Manifest.Kind != k {
		d.fail(join(path, "type"), typ.loc, "unknown %s plugin %q (available: %s)", k, typ.val, pluginIDs(d.s.Plugins, k))
		return plugin.Manifest{}, false
	}
	return reg.Manifest, true
}

func pluginIDs(reg *plugin.Registry, k plugin.Kind) string {
	var ids []string
	for _, m := range reg.List(k) {
		ids = append(ids, m.ID)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return "none registered"
	}
	return strings.Join(ids, ", ")
}

func (d *decoder) leaf(n *Node, r *raw, path string, parent Location) (any, bool) {
	if r != nil && !r.leaf {
		d.fail(path, r.loc, "must be a value, not a section")
		return nil, false
	}
	text, loc := n.Default, parent
	if r != nil {
		text, loc = r.val, r.loc
	}
	if text == "" {
		if n.Required {
			d.fail(path, loc, "is required")
		}
		return nil, false
	}
	if n.Kind == KindSecret {
		return d.secret(text, path, loc, r != nil)
	}
	v, err := parseLeaf(n, text)
	if err != nil {
		d.fail(path, loc, "%v", err)
		return nil, false
	}
	return v, true
}

func (d *decoder) secret(text, path string, loc Location, given bool) (any, bool) {
	s, err := d.res.resolve(text)
	if err != nil {
		d.fail(path, loc, "%v", err)
		return nil, false
	}
	if given && s.Ref == "" && d.file {
		d.warns = append(d.warns, fmt.Sprintf("%s (%s): clear-text secret in a file; prefer ${env:…} or ${file:…}", path, loc))
	}
	return s, true
}

// decode runs the whole walk and returns the typed tree.
func (s *Schema) decode(r *raw, res Resolver, file bool) (tree, map[string]Location, []string, error) {
	d := &decoder{s: s, res: res, locs: map[string]Location{}, file: file}
	t := d.object(s.Root.Fields, r, "", r.loc)
	if len(d.errs) > 0 {
		sort.SliceStable(d.errs, func(i, j int) bool { return d.errs[i].Path < d.errs[j].Path })
		return nil, nil, d.warns, d.errs
	}
	return t, d.locs, d.warns, nil
}
