package config

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// Plugins is the plugins section: where plugin modules live and the resource
// ceilings every module instance runs under.
type Plugins struct {
	Dir              string // "" = <data.dir>/plugins
	DisabledBuiltins []string
	MemoryMiB        int
	CallTimeout      time.Duration
	Concurrency      int
	MaxModuleMiB     int
	// UIInstall allows installing, updating and removing modules from the
	// web UI and the API.
	UIInstall bool
	// CatalogueURL is the plugin index address; "" = the official index of
	// this release.
	CatalogueURL string
	// Install lists modules to install from the catalogue at start.
	Install []PluginInstall
	// AutoUpdate is off, patch or minor.
	AutoUpdate string
}

// PluginInstall is one plugins.install entry; Version "" means the version
// the catalogue offers.
type PluginInstall struct {
	ID      string
	Version string
}

// ModulesDir is the plugins folder, defaulting to <data.dir>/plugins.
func (c *Config) ModulesDir() string { return modulesDir(c.Plugins.Dir, c.Data.Dir) }

func modulesDir(dir, data string) string {
	if dir != "" {
		return dir
	}
	return filepath.Join(data, "plugins")
}

func pluginsNode() *Node {
	return obj("Plugin modules: WebAssembly plugins installed into the plugins folder (applies at start).",
		Field{Name: "dir", Node: &Node{Kind: KindString, Check: checkAbsPathOrEmpty,
			Doc: "Folder for plugin modules; leave empty for the plugins folder inside data.dir. Drop a .wasm file here to install it."}},
		Field{Name: "disabledBuiltins", Node: &Node{Kind: KindList, Elem: &Node{Kind: KindString},
			Doc: "Built-in plugin ids to switch off, so a plugin module with the same id can replace them; leave empty to keep every built-in."}},
		Field{Name: "memoryMiB", Node: &Node{Kind: KindInt, Default: "64", Check: atLeast(4),
			Doc: "Most memory one running module instance may use, in MiB; a module may ask for less, never more."}},
		Field{Name: "callTimeout", Node: &Node{Kind: KindDuration, Default: "10s", Check: durationAtLeast(100 * time.Millisecond),
			Doc: "Longest one call into a module may run before it is stopped; a module may ask for less, never more."}},
		Field{Name: "concurrency", Node: &Node{Kind: KindInt, Default: "4", Check: atLeast(1),
			Doc: "Most calls one plugin instance runs at the same time; each runs in its own module instance."}},
		Field{Name: "maxModuleMiB", Node: &Node{Kind: KindInt, Default: "32", Check: atLeast(1),
			Doc: "Largest module file accepted, in MiB."}},
		Field{Name: "uiInstall", Node: &Node{Kind: KindBool, Default: "true",
			Doc: "Allow installing, updating, rolling back and removing plugin modules from the web UI and the API; set false to manage modules only through this config and the plugins folder."}},
		Field{Name: "catalogue", Node: obj("Where plugin modules are offered.",
			Field{Name: "url", Node: &Node{Kind: KindString, Check: checkCatalogueURL,
				Doc: "Address of the signed plugin index; leave empty for the official index of this release. With server.egressAllow set, allow this host and the host its downloads redirect to."}},
		)},
		Field{Name: "install", Node: &Node{Kind: KindList, Doc: "Plugin modules to install from the catalogue when corsarr starts; leave empty to install nothing this way.",
			Elem: obj("",
				Field{Name: "id", Node: &Node{Kind: KindString, Required: true, Check: checkModuleID,
					Doc: "Id of the plugin in the catalogue, for example transmission."}},
				Field{Name: "version", Node: &Node{Kind: KindString, Check: checkModuleVersion,
					Doc: "Version to install, for example 1.2.0; leave empty for the version the catalogue offers."}},
			)}},
		Field{Name: "autoUpdate", Node: enum("off", "Install newer catalogue versions of installed modules once a day: patch takes 1.2.x updates, minor takes 1.x updates, off never updates on its own. Versions that ask for new permissions always wait for you.", "off", "patch", "minor")},
	)
}

var (
	moduleIDRe      = regexp.MustCompile(`^[a-z][a-z0-9]{1,31}$`)
	moduleVersionRe = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]{1,32})?$`)
)

func checkModuleID(s string) error {
	if !moduleIDRe.MatchString(s) {
		return errors.New("must be a plugin id: 2 to 32 lower-case letters and digits, starting with a letter")
	}
	return nil
}

func checkModuleVersion(s string) error {
	if s != "" && !moduleVersionRe.MatchString(s) {
		return errors.New("must be a version such as 1.2.0")
	}
	return nil
}

func checkCatalogueURL(s string) error {
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("must be an http:// or https:// address")
	}
	return nil
}

func checkAbsPathOrEmpty(s string) error {
	if s == "" {
		return nil
	}
	return checkAbsPath(s)
}

func atLeast(n int64) func(string) error {
	return func(s string) error {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil && v < n {
			return fmt.Errorf("must be at least %d", n)
		}
		return nil
	}
}

func durationAtLeast(d time.Duration) func(string) error {
	return func(s string) error {
		if v, err := time.ParseDuration(s); err == nil && v < d {
			return fmt.Errorf("must be at least %s", d)
		}
		return nil
	}
}

func pluginsOf(t tree) Plugins {
	p, _ := t["plugins"].(tree)
	out := Plugins{DisabledBuiltins: stringList(p["disabledBuiltins"])}
	out.Dir, _ = p["dir"].(string)
	out.MemoryMiB = intOf(p["memoryMiB"])
	out.Concurrency = intOf(p["concurrency"])
	out.MaxModuleMiB = intOf(p["maxModuleMiB"])
	out.CallTimeout, _ = p["callTimeout"].(time.Duration)
	out.UIInstall, _ = p["uiInstall"].(bool)
	out.AutoUpdate, _ = p["autoUpdate"].(string)
	if c, ok := p["catalogue"].(tree); ok {
		out.CatalogueURL, _ = c["url"].(string)
	}
	list, _ := p["install"].([]any)
	for _, v := range list {
		f, _ := v.(tree)
		in := PluginInstall{}
		in.ID, _ = f["id"].(string)
		in.Version, _ = f["version"].(string)
		out.Install = append(out.Install, in)
	}
	return out
}

func intOf(v any) int {
	n, _ := v.(int64)
	return int(n)
}

// Bootstrap is what the process needs before the full configuration can be
// validated: plugin modules must be registered first, because instances name
// them as their type.
type Bootstrap struct {
	DataDir string
	Plugins Plugins
	// EgressAllow is server.egressAllow, for the catalogue fetches made
	// before the full configuration is read.
	EgressAllow []string
}

// ModulesDir is the plugins folder.
func (b Bootstrap) ModulesDir() string { return modulesDir(b.Plugins.Dir, b.DataDir) }

// bootstrapKeys are the sections Peek reads.
var bootstrapKeys = map[string]bool{"server": true, "data": true, "plugins": true}

// Peek reads only the server, data and plugins sections of the active source.
func Peek(in Inputs) (Bootstrap, error) {
	src, err := Select(in)
	if err != nil {
		return Bootstrap{}, err
	}
	r, err := readSource(src, in)
	if err != nil {
		return Bootstrap{}, err
	}
	part := newRaw(r.loc)
	for k := range bootstrapKeys {
		if kid, ok := r.kids[k]; ok {
			part.kids[k] = kid
		}
	}
	t, _, _, err := bootstrapSchema().decode(part, in.Resolver, false)
	if err != nil {
		return Bootstrap{}, err
	}
	server, _ := t["server"].(tree)
	return Bootstrap{DataDir: str(t, "data", "dir"), Plugins: pluginsOf(t), EgressAllow: stringList(server["egressAllow"])}, nil
}

// bootstrapSchema is the schema of the sections Peek reads.
func bootstrapSchema() *Schema {
	full := NewSchema(plugin.NewRegistry())
	s := &Schema{Root: &Node{Kind: KindObject}, Plugins: full.Plugins}
	for _, f := range full.Root.Fields {
		if bootstrapKeys[f.Name] {
			s.Root.Fields = append(s.Root.Fields, f)
		}
	}
	return s
}
