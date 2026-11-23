package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// SourceKind is where the configuration came from.
type SourceKind string

const (
	SourceFile     SourceKind = "file"
	SourceArgs     SourceKind = "args"
	SourceEnv      SourceKind = "env"
	SourceDefaults SourceKind = "defaults"
)

// Location says where a value was set: file line/column, an arg or an env
// name. It never holds the value itself, so it is safe to log.
type Location struct {
	Kind SourceKind `json:"kind"`
	File string     `json:"file,omitempty"`
	Line int        `json:"line,omitempty"`
	Col  int        `json:"col,omitempty"`
	Name string     `json:"name,omitempty"` // env var or --arg
}

func (l Location) String() string {
	switch l.Kind {
	case SourceFile:
		return fmt.Sprintf("%s:%d:%d", l.File, l.Line, l.Col)
	case SourceArgs, SourceEnv:
		return l.Name
	}
	return string(l.Kind)
}

// raw is the source-neutral tree: every leaf is text, every node knows where
// it was set. Lists are stored with index keys.
type raw struct {
	leaf bool
	val  string
	kids map[string]*raw
	loc  Location
}

func newRaw(loc Location) *raw { return &raw{kids: map[string]*raw{}, loc: loc} }

// set assigns a leaf at path, creating parents; a conflict (leaf vs branch)
// is reported instead of silently overwritten.
func (r *raw) set(path []string, val string, loc Location) error {
	n := r
	for i, seg := range path {
		child, ok := n.kids[seg]
		if !ok {
			child = newRaw(loc)
			n.kids[seg] = child
		}
		if i == len(path)-1 {
			if len(child.kids) > 0 {
				return fmt.Errorf("%s: %s is a section, not a value", loc, strings.Join(path, "."))
			}
			child.leaf, child.val, child.loc = true, val, loc
			return nil
		}
		if child.leaf {
			return fmt.Errorf("%s: %s is a value, not a section", loc, strings.Join(path[:i+1], "."))
		}
		n = child
	}
	return nil
}

// sortedKeys returns child keys, numeric keys in numeric order.
func (r *raw) sortedKeys() []string {
	keys := make([]string, 0, len(r.kids))
	for k := range r.kids {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, ea := strconv.Atoi(keys[i])
		b, eb := strconv.Atoi(keys[j])
		if ea == nil && eb == nil {
			return a < b
		}
		return keys[i] < keys[j]
	})
	return keys
}

// fromYAML converts a parsed YAML document into a raw tree with line/column.
func fromYAML(file string, data []byte) (*raw, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	root := newRaw(Location{Kind: SourceFile, File: file, Line: 1, Col: 1})
	if len(doc.Content) == 0 {
		return root, nil
	}
	return root, yamlInto(root, doc.Content[0], file)
}

func yamlInto(dst *raw, n *yaml.Node, file string) error {
	loc := Location{Kind: SourceFile, File: file, Line: n.Line, Col: n.Column}
	dst.loc = loc
	switch n.Kind {
	case yaml.MappingNode:
		return yamlPairs(dst, n.Content, file)
	case yaml.SequenceNode:
		for i, c := range n.Content {
			if err := yamlChild(dst, strconv.Itoa(i), c, file); err != nil {
				return err
			}
		}
		return nil
	case yaml.ScalarNode:
		if n.Tag != "!!null" {
			dst.leaf, dst.val = true, n.Value
		}
		return nil
	}
	return fmt.Errorf("%s: unsupported YAML construct (anchors/aliases are not allowed)", loc)
}

func yamlPairs(dst *raw, content []*yaml.Node, file string) error {
	for i := 0; i+1 < len(content); i += 2 {
		k := content[i]
		if _, dup := dst.kids[k.Value]; dup {
			return fmt.Errorf("%s:%d:%d: duplicate key %q", file, k.Line, k.Column, k.Value)
		}
		if err := yamlChild(dst, k.Value, content[i+1], file); err != nil {
			return err
		}
	}
	return nil
}

func yamlChild(dst *raw, key string, n *yaml.Node, file string) error {
	child := newRaw(Location{})
	dst.kids[key] = child
	return yamlInto(child, n, file)
}

// fromArgs reads `--a.b.c=value` named args. Args without '=' are not
// configuration and are ignored here (subcommand flags are parsed by cmd).
func fromArgs(args []string) (*raw, error) {
	root := newRaw(Location{Kind: SourceArgs, Name: "args"})
	for _, a := range args {
		k, v, ok := configArg(a)
		if !ok {
			continue
		}
		loc := Location{Kind: SourceArgs, Name: "--" + k}
		if err := root.set(strings.Split(k, "."), v, loc); err != nil {
			return nil, err
		}
	}
	return root, nil
}

// configArg splits `--key=value`; --configFile is not a config key.
func configArg(a string) (key, val string, ok bool) {
	if !strings.HasPrefix(a, "--") {
		return "", "", false
	}
	key, val, ok = strings.Cut(a[2:], "=")
	if !ok || key == "" || strings.EqualFold(key, "configFile") {
		return "", "", false
	}
	return key, val, true
}

const envPrefix = "CORSARR_"

// fromEnv reads CORSARR_A_B_C=value. Segments are lower-cased; field names
// are matched case-insensitively while decoding, so the walk is unambiguous
// (no segment contains '_').
func fromEnv(environ []string) (*raw, error) {
	root := newRaw(Location{Kind: SourceEnv, Name: "env"})
	for _, name := range configEnvNames(environ) {
		v := envValue(environ, name)
		segs := strings.Split(strings.ToLower(name[len(envPrefix):]), "_")
		if err := root.set(segs, v, Location{Kind: SourceEnv, Name: name}); err != nil {
			return nil, err
		}
	}
	return root, nil
}

// configEnvNames lists CORSARR_* names that are config keys: not the
// CORSARR_SECRET_ namespace and not CORSARR_CONFIGFILE. Sorted.
func configEnvNames(environ []string) []string {
	var out []string
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		up := strings.ToUpper(name)
		if !strings.HasPrefix(up, envPrefix) || strings.HasPrefix(up, envPrefix+"SECRET_") ||
			up == envPrefix+"CONFIGFILE" || len(up) == len(envPrefix) {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func envValue(environ []string, name string) string {
	for _, kv := range environ {
		if k, v, _ := strings.Cut(kv, "="); k == name {
			return v
		}
	}
	return ""
}
