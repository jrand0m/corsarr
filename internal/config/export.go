package config

import (
	"bytes"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExportOptions controls export.
type ExportOptions struct {
	Full bool // include keys still at their default
}

// secretRef is the reference export writes for a secret without one:
// ${env:CORSARR_SECRET_<SECTION>_<KEY>_<FIELD>}.
func secretRef(path []string) string { return "${env:" + secretVar(path) + "}" }

func secretVar(path []string) string { return envPrefix + "SECRET_" + envName(path) }

func envName(path []string) string { return strings.ToUpper(strings.Join(path, "_")) }

// ExportYAML writes the configuration deterministically: schema field order,
// sorted object keys, defaults omitted unless Full, secrets as references,
// section headers as comments. Loading the output yields the same export.
func (c *Config) ExportYAML(o ExportOptions) []byte {
	doc := c.document(o)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	_ = enc.Encode(doc) // encoding a node tree we built cannot fail
	_ = enc.Close()
	return buf.Bytes()
}

// ExportEnv writes the same state as CORSARR_* lines in the same order.
// Secrets without an original reference become `NAME=` placeholders.
func (c *Config) ExportEnv(o ExportOptions) []byte {
	var buf bytes.Buffer
	flatten(c.document(o).Content[0], nil, &buf)
	return buf.Bytes()
}

// RequiredVars lists the environment variables an export references.
func (c *Config) RequiredVars() []string {
	set := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case tree:
			for _, cv := range x {
				walk(cv)
			}
		case []any:
			for _, cv := range x {
				walk(cv)
			}
		case Secret:
			if scheme, target, ok := parseRef(x.Ref); ok && scheme == "env" {
				set[target] = true
			}
		}
	}
	walk(c.withSecretRefs())
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (c *Config) document(o ExportOptions) *yaml.Node {
	e := exporter{s: c.schema, full: o.Full}
	root := e.object(c.schema.Root.Fields, c.withSecretRefs(), nil)
	if root == nil {
		root = mapping()
	}
	if cs := root.Content; len(cs) == 0 || cs[0].Value != "version" {
		root.Content = append([]*yaml.Node{scalar("version"), scalar("1")}, cs...) // files always carry it
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		root.Content[i].HeadComment = docOf(c.schema.Root.Fields, root.Content[i].Value)
	}
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	if vars := c.RequiredVars(); len(vars) > 0 {
		doc.HeadComment = "corsarr configuration. Set these environment variables: " + strings.Join(vars, ", ")
	}
	return doc
}

// withSecretRefs returns the tree with every reference-less secret given its
// CORSARR_SECRET_* reference.
func (c *Config) withSecretRefs() tree {
	return refSecrets(c.tree, nil).(tree)
}

func refSecrets(v any, path []string) any {
	switch x := v.(type) {
	case tree:
		out := tree{}
		for k, cv := range x {
			out[k] = refSecrets(cv, append(append([]string{}, path...), k))
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, cv := range x {
			out[i] = refSecrets(cv, append(append([]string{}, path...), strconv.Itoa(i)))
		}
		return out
	case Secret:
		if x.Ref == "" {
			x.Ref = secretRef(path)
		}
		return x
	}
	return v
}

func docOf(fields []Field, name string) string {
	for _, f := range fields {
		if f.Name == name {
			return f.Node.Doc
		}
	}
	return ""
}

type exporter struct {
	s    *Schema
	full bool
}

func mapping() *yaml.Node { return &yaml.Node{Kind: yaml.MappingNode} }

func scalar(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Value: s} }

// node renders one value; nil means "nothing to write".
func (e exporter) node(n *Node, v any, path []string) *yaml.Node {
	switch n.Kind {
	case KindObject:
		return e.object(n.Fields, v.(tree), path)
	case KindInstance:
		return e.object(e.instanceFields(n, v.(tree)), v.(tree), path)
	case KindMap:
		return e.mapEntries(n, v.(tree), path)
	case KindList:
		return e.list(n, v.([]any), path)
	}
	return e.leaf(n, v)
}

func (e exporter) instanceFields(n *Node, v tree) []Field {
	fields := commonInstanceFields(n.Plugin, e.s.Plugins)
	if reg, ok := e.s.Plugins.Get(v["type"].(string)); ok {
		fields = append(fields, pluginFields(reg.Manifest)...)
	}
	return fields
}

func (e exporter) object(fields []Field, v tree, path []string) *yaml.Node {
	out := mapping()
	for _, f := range fields {
		cv, ok := v[f.Name]
		if !ok {
			continue
		}
		if child := e.node(f.Node, cv, append(path, f.Name)); child != nil {
			out.Content = append(out.Content, scalar(f.Name), child)
		}
	}
	return nonEmpty(out)
}

func (e exporter) mapEntries(n *Node, v tree, path []string) *yaml.Node {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := mapping()
	for _, k := range keys {
		child := e.node(n.Elem, v[k], append(path, k))
		if child == nil {
			child = mapping() // a named entry exists even with all defaults
		}
		out.Content = append(out.Content, scalar(k), child)
	}
	return nonEmpty(out)
}

func (e exporter) list(n *Node, v []any, path []string) *yaml.Node {
	out := &yaml.Node{Kind: yaml.SequenceNode}
	for i, cv := range v {
		child := e.node(n.Elem, cv, append(path, strconv.Itoa(i)))
		if child == nil {
			child = mapping()
		}
		out.Content = append(out.Content, child)
	}
	return nonEmpty(out)
}

func (e exporter) leaf(n *Node, v any) *yaml.Node {
	text := formatLeaf(v)
	if !e.full && n.Kind != KindSecret && text == canonicalDefault(n) && n.Default != "" {
		return nil
	}
	return scalar(text)
}

func nonEmpty(n *yaml.Node) *yaml.Node {
	if len(n.Content) == 0 {
		return nil
	}
	return n
}

// flatten writes CORSARR_<PATH>=value lines from the export document.
func flatten(n *yaml.Node, path []string, buf *bytes.Buffer) {
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			flatten(n.Content[i+1], append(path, n.Content[i].Value), buf)
		}
	case yaml.SequenceNode:
		for i, c := range n.Content {
			flatten(c, append(path, strconv.Itoa(i)), buf)
		}
	case yaml.ScalarNode:
		if len(path) == 1 && path[0] == "version" {
			return
		}
		val := n.Value
		if val == secretRef(path) {
			val = "" // placeholder: the env source takes the secret directly
		}
		buf.WriteString(envPrefix + envName(path) + "=" + val + "\n")
	}
}
