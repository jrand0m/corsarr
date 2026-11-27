package config

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// Placeholders stand for parts of a key the owner chooses.
const (
	NamePlaceholder  = "<name>" // a map key such as an indexer name
	IndexPlaceholder = "<n>"    // a list position, starting at 0
)

// secretDefault is shown instead of a secret's value.
const secretDefault = "(secret, unset)"

// Row is one documented setting.
type Row struct {
	Path    []string // key segments, placeholders included
	Default string   // documented text form; "" when there is none
	Secret  bool
	Live    bool
	Doc     string // what it does and when to change it
	Note    string // allowed values or units, derived from the type
}

// Key is the dotted key, e.g. server.address.
func (r Row) Key() string { return strings.Join(r.Path, ".") }

// Env is the environment variable form, e.g. CORSARR_SERVER_ADDRESS.
func (r Row) Env() string { return envPrefix + envName(r.Path) }

// Flag is the command-line form, e.g. --server.address=.
func (r Row) Flag() string { return "--" + r.Key() + "=" }

// ShownDefault is the default as printed: secrets never show a value.
func (r Row) ShownDefault() string {
	if r.Secret {
		return secretDefault
	}
	return r.Default
}

// Explanation is the documentation plus allowed values or units.
func (r Row) Explanation() string { return strings.TrimSpace(r.Doc + " " + r.Note) }

// PluginTable lists the settings one plugin adds to its instances.
type PluginTable struct {
	Kind    plugin.Kind
	Section string
	ID      string
	Name    string
	Rows    []Row
}

// Reference is every setting of the schema: the main rows in schema order,
// then the settings of each registered plugin grouped by kind.
type Reference struct {
	Rows    []Row
	Plugins []PluginTable
}

// Reference documents the whole schema; output order is stable.
func (s *Schema) Reference() Reference {
	w := refWalker{s: s}
	for _, f := range s.Root.Fields {
		w.live = f.Live
		w.walk(f.Node, []string{f.Name}, "")
	}
	ref := Reference{Rows: w.rows}
	for _, k := range instanceSections {
		for _, m := range s.Plugins.List(k) {
			ref.Plugins = append(ref.Plugins, pluginTable(k, m))
		}
	}
	return ref
}

// MissingDocs lists the keys whose row has no documentation.
func (r Reference) MissingDocs() []string {
	var out []string
	for _, row := range r.all() {
		if strings.TrimSpace(row.Doc) == "" {
			out = append(out, row.Key())
		}
	}
	return out
}

func (r Reference) all() []Row {
	out := append([]Row{}, r.Rows...)
	for _, t := range r.Plugins {
		out = append(out, t.Rows...)
	}
	return out
}

type refWalker struct {
	s    *Schema
	live bool
	rows []Row
}

// walk adds one row per leaf below n. inherit is the documentation of the
// map or list a leaf is the element of.
func (w *refWalker) walk(n *Node, path []string, inherit string) {
	switch n.Kind {
	case KindObject:
		for _, f := range n.Fields {
			w.walk(f.Node, extend(path, f.Name), "")
		}
	case KindMap:
		w.walk(n.Elem, extend(path, NamePlaceholder), n.Doc)
	case KindList:
		w.walk(n.Elem, extend(path, IndexPlaceholder), n.Doc)
	case KindInstance:
		w.instance(n, path)
	default:
		w.rows = append(w.rows, leafRow(n, path, inherit, w.live))
	}
}

// instance documents the fields every instance has, then points at the
// per-plugin tables for the rest.
func (w *refWalker) instance(n *Node, path []string) {
	for _, f := range commonInstanceFields(n.Plugin, w.s.Plugins) {
		w.walk(f.Node, extend(path, f.Name), "")
	}
	w.rows = append(w.rows, Row{Path: extend(path, "<setting>"), Live: w.live,
		Doc: fmt.Sprintf("Settings of the chosen %s plugin; each plugin's settings are listed below.", kindLabel(n.Plugin))})
}

func leafRow(n *Node, path []string, inherit string, live bool) Row {
	doc := n.Doc
	if doc == "" {
		doc = inherit
	}
	return Row{Path: path, Default: canonicalDefault(n), Secret: n.Kind == KindSecret, Live: live, Doc: doc, Note: leafNote(n)}
}

func leafNote(n *Node) string {
	switch n.Kind {
	case KindEnum:
		if len(n.Enum) > 0 {
			return "One of: " + strings.Join(n.Enum, ", ") + "."
		}
	case KindDuration:
		return "A duration such as 90s, 15m or 2h."
	case KindBool:
		return "true or false."
	case KindSecret:
		return "Give the value, ${env:NAME} or ${file:/path}."
	}
	return ""
}

// pluginTable documents one plugin's settings as keys of its section.
func pluginTable(k plugin.Kind, m plugin.Manifest) PluginTable {
	t := PluginTable{Kind: k, Section: k.Section(), ID: m.ID, Name: m.Name}
	base := []string{k.Section(), NamePlaceholder}
	for _, f := range m.Settings {
		w := refWalker{live: true}
		w.walk(fieldNode(f), extend(base, f.Key), f.Help)
		for i := range w.rows {
			w.rows[i].Secret = w.rows[i].Secret || f.IsSecret()
			if w.rows[i].Default == "" && f.Default != nil {
				w.rows[i].Default = formatAny(f.Default)
			}
		}
		t.Rows = append(t.Rows, w.rows...)
	}
	return t
}

// formatAny renders a plugin default in the documented text form: lists as
// comma-separated values, maps as sorted key=value pairs.
func formatAny(v any) string {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		parts := make([]string, rv.Len())
		for i := range parts {
			parts[i] = formatAny(rv.Index(i).Interface())
		}
		return strings.Join(parts, ",")
	case reflect.Map:
		parts := make([]string, 0, rv.Len())
		for _, key := range rv.MapKeys() {
			parts = append(parts, fmt.Sprint(key.Interface())+"="+formatAny(rv.MapIndex(key).Interface()))
		}
		sort.Strings(parts)
		return strings.Join(parts, " ")
	}
	return fmt.Sprint(v)
}

func extend(path []string, seg string) []string {
	return append(append(make([]string, 0, len(path)+1), path...), seg)
}
