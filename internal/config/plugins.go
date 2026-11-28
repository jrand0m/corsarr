package config

import (
	"fmt"
	"path/filepath"
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
	)
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
}

// ModulesDir is the plugins folder.
func (b Bootstrap) ModulesDir() string { return modulesDir(b.Plugins.Dir, b.DataDir) }

// Peek reads only the data and plugins sections of the active source.
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
	for _, k := range []string{"data", "plugins"} {
		if kid, ok := r.kids[k]; ok {
			part.kids[k] = kid
		}
	}
	full := NewSchema(plugin.NewRegistry())
	s := &Schema{Root: &Node{Kind: KindObject}, Plugins: full.Plugins}
	for _, f := range full.Root.Fields {
		if f.Name == "data" || f.Name == "plugins" {
			s.Root.Fields = append(s.Root.Fields, f)
		}
	}
	t, _, _, err := s.decode(part, in.Resolver, false)
	if err != nil {
		return Bootstrap{}, err
	}
	return Bootstrap{DataDir: str(t, "data", "dir"), Plugins: pluginsOf(t)}, nil
}
