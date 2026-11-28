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
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/msgtemplate"
	"github.com/jrand0m/corsarr/internal/naming"
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
var bootstrapSections = map[string]bool{"server": true, "data": true, "cache": true, "log": true, "auth": true, "plugins": true}

// instanceSections are the MVP plugin-instance sections, in export order.
var instanceSections = []plugin.Kind{
	plugin.KindIndexer, plugin.KindDownloader, plugin.KindMetadata,
	plugin.KindMediaServer, plugin.KindNotifier, plugin.KindSource,
}

// NewSchema assembles the MVP schema; reg contributes plugin settings.
func NewSchema(reg *plugin.Registry) *Schema {
	fields := []Field{
		{Name: "version", Node: &Node{Kind: KindInt, Default: "1", Doc: "Format version of the configuration; leave it at 1.", Check: checkVersion}},
		{Name: "server", Node: obj("Web server (applies at start).",
			Field{Name: "address", Node: &Node{Kind: KindString, Default: ":8090", Doc: "Address and port the web UI and API listen on; change the port when 8090 is taken, or add a host to listen on one network interface only.", Check: checkAddress}},
			Field{Name: "basePath", Node: &Node{Kind: KindString, Doc: "Path prefix when a reverse proxy serves corsarr under a sub-path, for example /corsarr; leave empty when it has its own host name.", Check: checkBasePath}},
			Field{Name: "egressAllow", Node: &Node{Kind: KindList, Elem: &Node{Kind: KindString, Check: checkEgressHost},
				Doc: "Hosts corsarr may contact (exact name, *.domain or an IP range); leave empty to allow every host, or list hosts to lock outgoing traffic down."}},
			Field{Name: "rateLearning", Node: obj("How corsarr finds each service's rate limit and stays below it.",
				Field{Name: "enabled", Node: &Node{Kind: KindBool, Default: "true",
					Doc: "Learn where each service starts throttling and keep requests below it; turn off to use only the configured limits."}},
				Field{Name: "headroom", Node: &Node{Kind: KindNumber, Default: "0.85", Check: checkHeadroom,
					Doc: "Share of a learned limit corsarr uses, between 0.1 and 1; lower it to stay further from the limit."}},
				Field{Name: "probeEvery", Node: &Node{Kind: KindDuration, Default: "168h", Check: nonNegative,
					Doc: "How often a limit found by throttling is gently re-checked while requests are waiting; 0 never re-checks."}},
			)},
		)},
		{Name: "data", Node: obj("Data folder (applies at start).",
			Field{Name: "dir", Node: &Node{Kind: KindString, Default: "/data", Doc: "Folder for the database, the secret key, backups and logs; point it at storage that survives restarts and upgrades."}},
			Field{Name: "umask", Node: &Node{Kind: KindString, Default: "002", Check: checkUmask,
				Doc: "Permission mask for every file corsarr creates, in octal; 002 lets a shared media group write, 022 keeps files writable by corsarr only."}},
		)},
		{Name: "cache", Node: cacheNode()},
		{Name: "log", Node: obj("Logging (applies at start).",
			Field{Name: "level", Node: enum("info", "How much is logged; use debug while tracking down a problem, warn or error to keep logs short.", "debug", "info", "warn", "error")},
		)},
		{Name: "auth", Node: obj("Sign-in (applies at start).",
			Field{Name: "mode", Node: enum("password", "How people sign in: password asks for the admin login or the API key, local lets devices on private networks in without one, proxy trusts the user name your reverse proxy sends.", "password", "local", "proxy")},
		)},
		{Name: "plugins", Node: pluginsNode()},
		{Name: "mediaKinds", Live: true, Node: obj("What corsarr does for each kind of media.",
			kindField(domain.KindTV, domain.ModeActive), kindField(domain.KindMovie, domain.ModeActive),
			kindField(domain.KindMusic, domain.ModeActive), kindField(domain.KindBook, domain.ModeDisabled),
		)},
		{Name: "rootFolders", Live: true, Node: &Node{Kind: KindList, Doc: "Library root folders.", Elem: obj("",
			Field{Name: "path", Node: &Node{Kind: KindString, Required: true, Doc: "Full path of a library folder as corsarr sees it, for example /media/movies.", Check: checkAbsPath}},
			Field{Name: "kind", Node: &Node{Kind: KindEnum, Required: true, Enum: kindNames(), Doc: "Which kind of media lives in this folder."}},
		)}},
	}
	fields = append(fields, Field{Name: "naming", Live: true, Node: namingNode()})
	fields = append(fields, Field{Name: "automation", Live: true, Node: obj("Automatic clean-up and upgrade rules.",
		Field{Name: "rules", Node: &Node{Kind: KindMap, Doc: "One entry per rule, keyed by rule id (upgradereplace, itemdelete, recyclepurge, …).",
			Elem: obj("", Field{Name: "mode", Node: &Node{Kind: KindEnum, Enum: RuleModes,
				Doc: "off never runs the rule, dry-run only records what it would do, live acts; leave unset to keep the rule's own default. Try dry-run first."}})}},
	)})
	fields = append(fields, Field{Name: "legacy", Live: true, Node: obj(
		"Compatibility API for third-party tools (request managers, dashboards) under /legacy/<app>; each app is off until enabled.",
		legacyField(LegacySonarr, "Sonarr-compatible API at /legacy/sonarr/api/v3 (TV)."),
		legacyField(LegacyRadarr, "Radarr-compatible API at /legacy/radarr/api/v3 (movies)."),
		legacyField(LegacyLidarr, "Lidarr-compatible read-only API at /legacy/lidarr/api/v1 (music)."),
		legacyField(LegacyProwlarr, "Prowlarr-compatible read-only API at /legacy/prowlarr/api/v1 (indexers)."),
	)})
	fields = append(fields, Field{Name: "mcp", Live: true, Node: mcpNode()})
	for _, k := range instanceSections {
		fields = append(fields, Field{Name: k.Section(), Live: true, Node: &Node{
			Kind: KindMap, Doc: fmt.Sprintf("Your %s connections, one entry per name you choose (lower-case letters and digits).", kindLabel(k)),
			Elem: &Node{Kind: KindInstance, Plugin: k},
		}})
	}
	return &Schema{Root: &Node{Kind: KindObject, Fields: fields}, Plugins: reg}
}

// namingTemplates are the template fields of each kind's naming, with the
// built-in template as default.
var namingTemplates = map[domain.MediaKind][]struct{ name, def, doc string }{
	domain.KindTV: {
		{"seriesFolder", naming.DefaultSeriesFolder, "Folder of a series added from now on, inside its root folder."},
		{"seasonFolder", naming.DefaultSeasonFolder, "Season folder inside the series folder (series that use season folders)."},
		{"episodeFile", naming.DefaultEpisodeFile, "Name of an imported episode file, without the extension."},
	},
	domain.KindMovie: {
		{"movieFolder", naming.DefaultMovieFolder, "Folder of a movie added from now on, inside its root folder."},
		{"movieFile", naming.DefaultMovieFile, "Name of an imported movie file, without the extension."},
	},
	domain.KindMusic: {
		{"artistFolder", naming.DefaultArtistFolder, "Folder of an artist added from now on, inside its root folder."},
		{"albumFolder", naming.DefaultAlbumFolder, "Album folder inside the artist folder."},
		{"trackFile", naming.DefaultTrackFile, "Name of an imported track file, without the extension."},
	},
}

// namingKinds are the kinds the naming section covers, in order.
var namingKinds = []domain.MediaKind{domain.KindTV, domain.KindMovie, domain.KindMusic}

func namingNode() *Node {
	var kinds []Field
	for _, k := range namingKinds {
		fields := []Field{
			{Name: "rename", Node: &Node{Kind: KindBool, Default: "true",
				Doc: "Rename imported files with the file template; false keeps each file's own name inside the item's folder."}},
			{Name: "colon", Node: enum(string(naming.ColonSmart), "What replaces a colon in names: delete drops it, dash writes -, spaceDash writes \" - \", smart writes \" - \" after a word and drops it between digits.",
				string(naming.ColonDelete), string(naming.ColonDash), string(naming.ColonSpaceDash), string(naming.ColonSmart))},
		}
		for _, t := range namingTemplates[k] {
			fields = append(fields, Field{Name: t.name, Node: &Node{Kind: KindString, Default: t.def, Doc: t.doc, Check: checkTemplate}})
		}
		kinds = append(kinds, Field{Name: string(k), Node: obj("How "+kindLabels[k]+" folders and files are named.", fields...)})
	}
	return obj("Folder and file names per kind of media; tokens in braces, e.g. {Movie Title}{ (Release Year)}, with text inside the braces written only when the token has a value.", kinds...)
}

func checkTemplate(s string) error {
	_, err := naming.ParseTemplate(s)
	return err
}

func obj(doc string, fields ...Field) *Node { return &Node{Kind: KindObject, Doc: doc, Fields: fields} }

func enum(def, doc string, opts ...string) *Node {
	return &Node{Kind: KindEnum, Default: def, Doc: doc, Enum: opts}
}

func kindField(k domain.MediaKind, def domain.KindMode) Field {
	return Field{Name: string(k), Node: obj("", Field{Name: "mode", Node: enum(string(def), fmt.Sprintf("How corsarr handles %s: disabled turns it off, shadow decides everything but only records what it would grab, active grabs and imports.", kindLabels[k]),
		string(domain.ModeDisabled), string(domain.ModeShadow), string(domain.ModeActive))})}
}

// The compatibility API apps (keys of the legacy section).
const (
	LegacySonarr   = "sonarr"
	LegacyRadarr   = "radarr"
	LegacyLidarr   = "lidarr"
	LegacyProwlarr = "prowlarr"
)

func legacyField(app, doc string) Field {
	return Field{Name: app, Node: obj(doc, Field{Name: "enabled", Node: &Node{Kind: KindBool, Default: "false",
		Doc: "Turn on to let request managers and dashboards use this API; they sign in with the corsarr API key."}})}
}

// kindLabels name each media kind in plain words.
var kindLabels = map[domain.MediaKind]string{
	domain.KindTV: "TV series", domain.KindMovie: "movies", domain.KindMusic: "music", domain.KindBook: "books",
}

// kindLabel names a plugin kind in plain words.
func kindLabel(k plugin.Kind) string {
	switch k {
	case plugin.KindDownloader:
		return "download client"
	case plugin.KindMediaServer:
		return "media server"
	case plugin.KindNotifier:
		return "notification"
	}
	return string(k)
}

// MCP scopes: read exposes only tools that change nothing; write exposes all.
const (
	MCPScopeRead  = "read"
	MCPScopeWrite = "write"
)

func mcpNode() *Node {
	return obj("Model Context Protocol endpoint at <base>/mcp for AI assistants; off until enabled.",
		Field{Name: "enabled", Node: &Node{Kind: KindBool, Default: "false",
			Doc: "Serve the MCP endpoint (authenticated with the corsarr API key or the read-only key)."}},
		Field{Name: "scope", Node: enum(MCPScopeRead, "read: only tools that change nothing; write: also tools that add, search, monitor or remove.",
			MCPScopeRead, MCPScopeWrite)},
		Field{Name: "tools", Node: &Node{Kind: KindList, Doc: "Tool names to expose; empty exposes every tool the scope allows.",
			Elem: &Node{Kind: KindString, Doc: "A tool to expose by name, e.g. find_media; leave the list empty to expose every tool the scope allows."}}},
		Field{Name: "rateLimit", Node: &Node{Kind: KindInt, Default: "60", Check: nonNegative,
			Doc: "Most MCP calls per minute per API key; 0 means no limit."}},
		Field{Name: "allowedOrigins", Node: &Node{Kind: KindList,
			Doc:  "Browser origins allowed besides the server's own, e.g. http://localhost:6274; clients that send no Origin are always allowed.",
			Elem: &Node{Kind: KindString, Doc: "A browser origin allowed to call the endpoint besides the server's own, as scheme://host[:port], e.g. http://localhost:6274."}}},
	)
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
		{Name: "type", Node: &Node{Kind: KindEnum, Required: true, Enum: ids, Doc: "Which plugin this entry uses; its own settings are listed per plugin."}},
		{Name: "name", Node: &Node{Kind: KindString, Doc: "Name shown in the web UI."}},
		{Name: "enabled", Node: &Node{Kind: KindBool, Default: "true", Doc: "Set to false to keep the entry but stop using it."}},
	}
	if k == plugin.KindIndexer {
		fields = append(fields,
			Field{Name: "queryLimit", Node: &Node{Kind: KindInt, Default: "0", Check: nonNegative,
				Doc: "Most searches sent to this indexer per queryLimitPer, for sites that limit API use; 0 means no limit. Past the limit the indexer is skipped until the window frees up."}},
			Field{Name: "queryLimitPer", Node: enum("day", "Whether queryLimit counts per hour or per day.", "hour", "day")})
	}
	if k == plugin.KindNotifier {
		fields = append(fields, Field{Name: "template", Node: obj("Message template of this connection (Go text/template); .Title and .Body hold the built-in text, plus .Event, .Items, .Grab, .Import, .Download, .Payload.",
			Field{Name: "title", Node: &Node{Kind: KindString, Check: msgtemplate.Check, Doc: "Message title as a template, for example [corsarr] {{.Title}}; leave empty for the built-in title."}},
			Field{Name: "body", Node: &Node{Kind: KindString, Check: msgtemplate.Check, Doc: "Message text as a template; leave empty for the built-in text."}},
		)})
		fields = append(fields, Field{Name: "on", Node: &Node{Kind: KindList, Elem: &Node{Kind: KindEnum, Enum: NotificationGroups},
			Doc: "Which events send a message; leave empty for grab, import, upgrade, downloadFailed, importFailed and manualAction."}})
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
		return &Node{Kind: KindList, Doc: f.Help, Required: f.Required, Elem: &Node{Kind: KindString, Check: f.Check}}
	case plugin.FieldMultiselect:
		return &Node{Kind: KindList, Doc: f.Help, Required: f.Required, Elem: &Node{Kind: KindEnum, Enum: f.Options}}
	case plugin.FieldMap:
		return &Node{Kind: KindMap, Doc: f.Help, Required: f.Required, Elem: &Node{Kind: KindString}}
	}
	n.Check = bothChecks(rangeCheck(f), f.Check)
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

func checkHeadroom(s string) error {
	if v, err := strconv.ParseFloat(s, 64); err != nil || v < 0.1 || v > 1 {
		return fmt.Errorf("must be a number between 0.1 and 1")
	}
	return nil
}

func nonNegative(s string) error {
	if strings.HasPrefix(strings.TrimSpace(s), "-") {
		return fmt.Errorf("must be 0 or more")
	}
	return nil
}

func checkUmask(s string) error {
	if _, err := ParseUmask(s); err != nil {
		return err
	}
	return nil
}

// ParseUmask reads an octal file creation mask such as 002 or 0027.
func ParseUmask(s string) (int, error) {
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil || v > 0o777 {
		return 0, fmt.Errorf("must be an octal mask between 000 and 777, e.g. 002")
	}
	return int(v), nil
}

func checkEgressHost(s string) error {
	if _, _, err := net.ParseCIDR(s); err == nil {
		return nil
	}
	host := strings.TrimPrefix(s, "*.")
	if host == "" || strings.ContainsAny(host, "/:*@ ") {
		return fmt.Errorf("must be a host name, *.domain, an IP address or a CIDR range")
	}
	return nil
}

func checkAbsPath(s string) error {
	if !strings.HasPrefix(s, "/") {
		return fmt.Errorf("must be an absolute path")
	}
	return nil
}
