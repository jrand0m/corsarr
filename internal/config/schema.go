// Package config is corsarr's one configuration tree: a schema, exactly one source per start (file, args, env or defaults),
// mechanical key names in every form, secret references, validation that
// reports every error with its source location, deterministic export as YAML
// or as an env list, JSON Schema generation and a polling file watch.
//
// Data flow: source → raw tree (string leaves + locations) → decode against
// the schema (typed tree + all errors) → Config (typed accessors) → export.
package config

import (
	"fmt"
	"net"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/msgtemplate"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// NodeKind is the shape of one schema node.
type NodeKind int

const (
	KindObject   NodeKind = iota // fixed, ordered fields
	KindMap                      // user-chosen object keys ([a-z][a-z0-9]*) → Elem
	KindList                     // indices → Elem
	KindInstance                 // plugin instance: type/name/enabled + manifest settings
	KindString
	KindInt
	KindNumber
	KindBool
	KindEnum
	KindSecret
	KindDuration
)

// Node is one schema node. Leaves carry their default as text in the one
// documented form shared by every source.
type Node struct {
	Kind     NodeKind
	Doc      string
	Fields   []Field // KindObject
	Elem     *Node   // KindMap, KindList
	Enum     []string
	Default  string
	Required bool
	Plugin   plugin.Kind // KindInstance
	Check    func(string) error
}

// Field is a named child of an object node; Name is camelCase.
type Field struct {
	Name string
	Node *Node
	Live bool // top-level sections only: hot-reloaded from a watched file
}

// Schema is the assembled tree plus the plugin registry that contributes the
// settings of every plugin instance.
type Schema struct {
	Root    *Node
	Plugins *plugin.Registry
}

// Bootstrap sections apply at start only.
var bootstrapSections = map[string]bool{"server": true, "data": true, "log": true, "auth": true}

// instanceSections are the MVP plugin-instance sections, in export order.
var instanceSections = []plugin.Kind{
	plugin.KindIndexer, plugin.KindDownloader, plugin.KindMetadata,
	plugin.KindMediaServer, plugin.KindNotifier, plugin.KindSource,
}

// NewSchema assembles the MVP schema; reg contributes plugin settings.
func NewSchema(reg *plugin.Registry) *Schema {
	fields := []Field{
		{Name: "version", Node: &Node{Kind: KindInt, Default: "1", Doc: "Schema version of this file.", Check: checkVersion}},
		{Name: "server", Node: obj("HTTP server (bootstrap).",
			Field{Name: "address", Node: &Node{Kind: KindString, Default: ":8090", Doc: "Listen address host:port.", Check: checkAddress}},
			Field{Name: "basePath", Node: &Node{Kind: KindString, Doc: "URL path prefix behind a reverse proxy, e.g. /corsarr.", Check: checkBasePath}},
		)},
		{Name: "data", Node: obj("Data directory (bootstrap).",
			Field{Name: "dir", Node: &Node{Kind: KindString, Default: "/data", Doc: "Holds corsarr.db, secret.key, backups and logs."}},
		)},
		{Name: "log", Node: obj("Logging (bootstrap).",
			Field{Name: "level", Node: enum("info", "Minimum log level.", "debug", "info", "warn", "error")},
		)},
		{Name: "auth", Node: obj("Access control (bootstrap).",
			Field{Name: "mode", Node: enum("password", "password: admin login or API key; local: trust private networks; proxy: trust the reverse proxy user header.", "password", "local", "proxy")},
		)},
		{Name: "mediaKinds", Live: true, Node: obj("Per media kind mode.",
			kindField(domain.KindTV, domain.ModeActive), kindField(domain.KindMovie, domain.ModeActive),
			kindField(domain.KindMusic, domain.ModeActive), kindField(domain.KindBook, domain.ModeDisabled),
		)},
		{Name: "rootFolders", Live: true, Node: &Node{Kind: KindList, Doc: "Library root folders.", Elem: obj("",
			Field{Name: "path", Node: &Node{Kind: KindString, Required: true, Doc: "Absolute path.", Check: checkAbsPath}},
			Field{Name: "kind", Node: &Node{Kind: KindEnum, Required: true, Enum: kindNames(), Doc: "Media kind stored here."}},
		)}},
	}
	fields = append(fields, Field{Name: "automation", Live: true, Node: obj("Automation.",
		Field{Name: "rules", Node: &Node{Kind: KindMap, Doc: "Per rule id (upgradereplace, itemdelete, recyclepurge, …).",
			Elem: obj("", Field{Name: "mode", Node: &Node{Kind: KindEnum, Enum: RuleModes,
				Doc: "off: never runs; dry-run: records what it would do; live: acts. Unset keeps the rule's default."}})}},
	)})
	for _, k := range instanceSections {
		fields = append(fields, Field{Name: k.Section(), Live: true, Node: &Node{
			Kind: KindMap, Doc: fmt.Sprintf("%s plugin instances keyed by [a-z][a-z0-9]*.", k),
			Elem: &Node{Kind: KindInstance, Plugin: k},
		}})
	}
	return &Schema{Root: &Node{Kind: KindObject, Fields: fields}, Plugins: reg}
}

func obj(doc string, fields ...Field) *Node { return &Node{Kind: KindObject, Doc: doc, Fields: fields} }

func enum(def, doc string, opts ...string) *Node {
	return &Node{Kind: KindEnum, Default: def, Doc: doc, Enum: opts}
}

func kindField(k domain.MediaKind, def domain.KindMode) Field {
	return Field{Name: string(k), Node: obj("", Field{Name: "mode", Node: enum(string(def), "disabled | shadow | active.",
		string(domain.ModeDisabled), string(domain.ModeShadow), string(domain.ModeActive))})}
}

func kindNames() []string {
	return []string{string(domain.KindTV), string(domain.KindMovie), string(domain.KindMusic), string(domain.KindBook)}
}

// commonInstanceFields precede the plugin's own settings in every instance.
func commonInstanceFields(k plugin.Kind, reg *plugin.Registry) []Field {
	var ids []string
	for _, m := range reg.List(k) {
		ids = append(ids, m.ID)
	}
	fields := []Field{
		{Name: "type", Node: &Node{Kind: KindEnum, Required: true, Enum: ids, Doc: "Plugin id."}},
		{Name: "name", Node: &Node{Kind: KindString, Doc: "Display name."}},
		{Name: "enabled", Node: &Node{Kind: KindBool, Default: "true", Doc: "Whether the instance runs."}},
	}
	if k == plugin.KindIndexer {
		fields = append(fields,
			Field{Name: "queryLimit", Node: &Node{Kind: KindInt, Default: "0", Check: nonNegative,
				Doc: "Most searches sent to this indexer per queryLimitPer; 0 means no limit. Searches past it skip the indexer until the window frees up."}},
			Field{Name: "queryLimitPer", Node: enum("day", "Window of queryLimit.", "hour", "day")})
	}
	if k == plugin.KindNotifier {
		fields = append(fields, Field{Name: "template", Node: obj("Message template of this connection (Go text/template); .Title and .Body hold the built-in text, plus .Event, .Items, .Grab, .Import, .Download, .Payload.",
			Field{Name: "title", Node: &Node{Kind: KindString, Check: msgtemplate.Check, Doc: "Title template, e.g. [corsarr] {{.Title}}."}},
			Field{Name: "body", Node: &Node{Kind: KindString, Check: msgtemplate.Check, Doc: "Body template."}},
		)})
		fields = append(fields, Field{Name: "on", Node: &Node{Kind: KindList, Elem: &Node{Kind: KindEnum, Enum: NotificationGroups},
			Doc: "Event groups this connection notifies; empty means grab, import, upgrade, downloadFailed, importFailed and manualAction."}})
	}
	return fields
}

// RuleModes are the modes of an automation rule.
var RuleModes = []string{"off", "dry-run", "live"}

// NotificationGroups are the event toggles of a notification connection.
var NotificationGroups = []string{"grab", "import", "upgrade", "rename", "delete", "itemAdded", "itemDeleted",
	"downloadFailed", "importFailed", "manualAction", "health", "automation", "config"}

// pluginFields converts a manifest's settings schema into schema fields.
func pluginFields(m plugin.Manifest) []Field {
	out := make([]Field, 0, len(m.Settings))
	for _, f := range m.Settings {
		out = append(out, Field{Name: f.Key, Node: fieldNode(f)})
	}
	return out
}

var leafKinds = map[plugin.FieldType]NodeKind{
	plugin.FieldSecret: KindSecret, plugin.FieldNumber: KindNumber, plugin.FieldBool: KindBool,
	plugin.FieldEnum: KindEnum, plugin.FieldDuration: KindDuration,
}

func fieldNode(f plugin.Field) *Node {
	n := &Node{Kind: KindString, Doc: f.Help, Required: f.Required, Enum: f.Options}
	if k, ok := leafKinds[f.Type]; ok {
		n.Kind = k
	}
	if f.Default != nil {
		n.Default = fmt.Sprint(f.Default)
	}
	switch f.Type {
	case plugin.FieldList:
		return &Node{Kind: KindList, Doc: f.Help, Required: f.Required, Elem: &Node{Kind: KindString}}
	case plugin.FieldMultiselect:
		return &Node{Kind: KindList, Doc: f.Help, Required: f.Required, Elem: &Node{Kind: KindEnum, Enum: f.Options}}
	case plugin.FieldMap:
		return &Node{Kind: KindMap, Doc: f.Help, Required: f.Required, Elem: &Node{Kind: KindString}}
	}
	n.Check = rangeCheck(f)
	return n
}

func checkVersion(s string) error {
	if s != "1" {
		return fmt.Errorf("schema version %s requires a newer corsarr (this build reads version 1)", s)
	}
	return nil
}

func checkAddress(s string) error {
	if _, _, err := net.SplitHostPort(s); err != nil {
		return fmt.Errorf("must be host:port, e.g. :8090")
	}
	return nil
}

func checkBasePath(s string) error {
	if s == "" {
		return nil
	}
	if !strings.HasPrefix(s, "/") || strings.HasSuffix(s, "/") {
		return fmt.Errorf("must start with / and not end with /, e.g. /corsarr")
	}
	return nil
}

func nonNegative(s string) error {
	if strings.HasPrefix(strings.TrimSpace(s), "-") {
		return fmt.Errorf("must be 0 or more")
	}
	return nil
}

func checkAbsPath(s string) error {
	if !strings.HasPrefix(s, "/") {
		return fmt.Errorf("must be an absolute path")
	}
	return nil
}
