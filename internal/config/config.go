package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// maxFileSize is the size above which a config file is refused.
const maxFileSize = 5 << 20

// Source is the one source chosen for this start.
type Source struct {
	Kind    SourceKind `json:"kind"`
	Path    string     `json:"path,omitempty"`
	Ignored []string   `json:"ignored,omitempty"` // CORSARR_* / --args ignored because a file won
}

// Inputs is everything source selection may look at; cmd fills it from the
// process, tests fill it by hand.
type Inputs struct {
	Args     []string // config args (subcommand flags removed)
	Environ  []string // KEY=VALUE
	Resolver Resolver
	ReadFile func(string) ([]byte, error)
	Exists   func(string) bool
	Home     string
	XDG      string // $XDG_CONFIG_HOME
}

// OSInputs builds Inputs from the running process.
func OSInputs(args []string) Inputs {
	home, _ := os.UserHomeDir()
	return Inputs{
		Args: args, Environ: os.Environ(), Resolver: OSResolver(), ReadFile: os.ReadFile,
		Exists: func(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() },
		Home:   home, XDG: os.Getenv("XDG_CONFIG_HOME"),
	}
}

// RootFolder is one library root.
type RootFolder struct {
	Path string           `json:"path"`
	Kind domain.MediaKind `json:"kind"`
}

// Instance is one configured plugin instance.
type Instance struct {
	Section  string           `json:"section"`
	Key      domain.ObjectKey `json:"key"`
	Type     string           `json:"type"`
	Name     string           `json:"name"`
	Enabled  bool             `json:"enabled"`
	Settings plugin.Settings  `json:"-"` // secrets resolved to plain strings for the plugin
	Loc      Location         `json:"location"`
}

// Config is the validated configuration. Bootstrap values are typed fields;
// the full tree is kept for export and live-change detection.
type Config struct {
	Server struct {
		Address     string
		BasePath    string
		EgressAllow []string
	}
	Data struct {
		Dir   string
		Umask int
	}
	Log         struct{ Level string }
	Auth        struct{ Mode string }
	MediaKinds  map[domain.MediaKind]domain.KindMode
	RootFolders []RootFolder
	// RuleModes maps rule id → configured mode (automation.rules.<id>.mode).
	RuleModes map[string]string
	// Legacy maps a compatibility API app (sonarr, radarr, …) → enabled.
	Legacy map[string]bool
	// Instances maps section (indexers, downloadClients, …) → key → instance.
	Instances map[string]map[domain.ObjectKey]Instance
	Source    Source
	Warnings  []string

	schema *Schema
	tree   tree
	locs   map[string]Location
}

// Location returns where a dotted key was set (zero when defaulted).
func (c *Config) Location(path string) Location { return c.locs[path] }

// Load selects the source, reads it, validates everything and returns the
// Config, or Errors listing every failure.
func Load(in Inputs, reg *plugin.Registry) (*Config, error) {
	src, err := Select(in)
	if err != nil {
		return nil, err
	}
	return LoadFrom(src, in, reg)
}

// LoadFrom reads an already-selected source (the watcher reloads this way).
func LoadFrom(src Source, in Inputs, reg *plugin.Registry) (*Config, error) {
	r, err := readSource(src, in)
	if err != nil {
		return nil, err
	}
	s := NewSchema(reg)
	t, locs, warns, err := s.decode(r, in.Resolver, src.Kind == SourceFile)
	if err != nil {
		return nil, err
	}
	if src.Kind == SourceFile {
		src.Ignored = ignoredKeys(in, r)
	}
	c := build(s, t)
	c.Source, c.Warnings, c.locs = src, warns, locs
	for section, m := range c.Instances {
		for k, inst := range m {
			inst.Loc = locs[section+"."+string(k)]
			m[k] = inst
		}
	}
	if len(src.Ignored) > 0 {
		c.Warnings = append(c.Warnings, "config file in use; ignored keys from other sources: "+strings.Join(src.Ignored, ", "))
	}
	return c, nil
}

func readSource(src Source, in Inputs) (*raw, error) {
	switch src.Kind {
	case SourceFile:
		data, err := in.ReadFile(src.Path)
		if err != nil {
			return nil, fmt.Errorf("config file %s: %w", src.Path, err)
		}
		if len(data) > maxFileSize {
			return nil, fmt.Errorf("config file %s is larger than 5 MiB", src.Path)
		}
		return fromYAML(src.Path, data)
	case SourceArgs:
		return fromArgs(in.Args)
	case SourceEnv:
		return fromEnv(in.Environ)
	}
	return newRaw(Location{Kind: SourceDefaults}), nil
}

// Select picks the one config source in this order: explicit file → default file locations →
// named args → CORSARR_* env → defaults.
func Select(in Inputs) (Source, error) {
	if p := explicitFile(in); p != "" {
		if !in.Exists(p) {
			return Source{}, fmt.Errorf("config file %s not found", p)
		}
		return Source{Kind: SourceFile, Path: p}, nil
	}
	for _, p := range defaultFiles(in) {
		if in.Exists(p) {
			return Source{Kind: SourceFile, Path: p}, nil
		}
	}
	for _, a := range in.Args {
		if _, _, ok := configArg(a); ok {
			return Source{Kind: SourceArgs}, nil
		}
	}
	if len(configEnvNames(in.Environ)) > 0 {
		return Source{Kind: SourceEnv}, nil
	}
	return Source{Kind: SourceDefaults}, nil
}

func explicitFile(in Inputs) string {
	for _, a := range in.Args {
		if k, v, ok := strings.Cut(strings.TrimPrefix(a, "--"), "="); ok && strings.HasPrefix(a, "--") && strings.EqualFold(k, "configFile") {
			return v
		}
	}
	for _, kv := range in.Environ {
		if k, v, _ := strings.Cut(kv, "="); strings.EqualFold(k, envPrefix+"CONFIGFILE") && v != "" {
			return v
		}
	}
	return ""
}

func defaultFiles(in Inputs) []string {
	dirs := []string{"/etc/corsarr"}
	if in.XDG != "" {
		dirs = append(dirs, filepath.Join(in.XDG, "corsarr"))
	}
	if in.Home != "" {
		dirs = append(dirs, filepath.Join(in.Home, ".config", "corsarr"))
	}
	var out []string
	for _, d := range dirs {
		out = append(out, filepath.Join(d, "corsarr.yml"), filepath.Join(d, "corsarr.yaml"))
	}
	return append(out, "corsarr.yml", "corsarr.yaml")
}

// ignoredKeys names CORSARR_* env keys and --args a file source ignores,
// minus variables the file references through ${env:…}.
func ignoredKeys(in Inputs, r *raw) []string {
	refs := map[string]bool{}
	collectEnvRefs(r, refs)
	var out []string
	for _, n := range configEnvNames(in.Environ) {
		if !refs[n] {
			out = append(out, n)
		}
	}
	for _, a := range in.Args {
		if k, _, ok := configArg(a); ok {
			out = append(out, "--"+k)
		}
	}
	sort.Strings(out)
	return out
}

func collectEnvRefs(r *raw, refs map[string]bool) {
	if scheme, target, ok := parseRef(r.val); r.leaf && ok && scheme == "env" {
		refs[target] = true
	}
	for _, c := range r.kids {
		collectEnvRefs(c, refs)
	}
}

// build lifts the typed tree into Config's accessors.
func build(s *Schema, t tree) *Config {
	c := &Config{schema: s, tree: t, MediaKinds: map[domain.MediaKind]domain.KindMode{},
		Instances: map[string]map[domain.ObjectKey]Instance{}}
	c.Server.Address = str(t, "server", "address")
	c.Server.BasePath = str(t, "server", "basePath")
	c.Data.Dir = str(t, "data", "dir")
	c.Data.Umask, _ = ParseUmask(str(t, "data", "umask")) // validated by the schema
	for _, h := range t["server"].(tree)["egressAllow"].([]any) {
		c.Server.EgressAllow = append(c.Server.EgressAllow, h.(string))
	}
	c.Log.Level = str(t, "log", "level")
	c.Auth.Mode = str(t, "auth", "mode")
	for k, v := range t["mediaKinds"].(tree) {
		c.MediaKinds[domain.MediaKind(k)] = domain.KindMode(v.(tree)["mode"].(string))
	}
	for _, v := range t["rootFolders"].([]any) {
		f := v.(tree)
		c.RootFolders = append(c.RootFolders, RootFolder{Path: f["path"].(string), Kind: domain.MediaKind(f["kind"].(string))})
	}
	for _, k := range instanceSections {
		c.Instances[k.Section()] = instances(k.Section(), t[k.Section()].(tree))
	}
	c.RuleModes = ruleModes(t)
	c.Legacy = legacyApps(t)
	return c
}

func ruleModes(t tree) map[string]string {
	out := map[string]string{}
	rules, _ := t["automation"].(tree)["rules"].(tree)
	for id, v := range rules {
		if mode, ok := v.(tree)["mode"].(string); ok {
			out[id] = mode
		}
	}
	return out
}

func legacyApps(t tree) map[string]bool {
	out := map[string]bool{}
	apps, _ := t["legacy"].(tree)
	for app, v := range apps {
		on, _ := v.(tree)["enabled"].(bool)
		out[app] = on
	}
	return out
}

// LegacyEnabled reports whether a compatibility API app is switched on.
func (c *Config) LegacyEnabled(app string) bool { return c != nil && c.Legacy[app] }

func str(t tree, section, key string) string {
	v, _ := t[section].(tree)[key].(string)
	return v
}

func instances(section string, t tree) map[domain.ObjectKey]Instance {
	out := map[domain.ObjectKey]Instance{}
	for key, v := range t {
		f := v.(tree)
		inst := Instance{Section: section, Key: domain.ObjectKey(key), Type: f["type"].(string), Enabled: f["enabled"].(bool), Settings: plugin.Settings{}}
		inst.Name, _ = f["name"].(string)
		for k, fv := range f {
			if k != "type" && k != "name" && k != "enabled" {
				inst.Settings[k] = plainSetting(fv)
			}
		}
		out[inst.Key] = inst
	}
	return out
}

func plainSetting(v any) any {
	if s, ok := v.(Secret); ok {
		return s.Value()
	}
	return v
}

// Live reports whether a top-level section is hot-reloaded.
func Live(section string) bool { return !bootstrapSections[section] && section != "version" }
