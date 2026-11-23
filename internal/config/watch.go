package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// WatchInterval is the MVP poll period; one stable extra poll debounces, so a
// burst of writes (or a ConfigMap symlink swap) becomes one change.
const WatchInterval = 2 * time.Second

// fileSig identifies a file version: the resolved target (symlink swaps),
// its modification time and size.
type fileSig struct {
	target string
	mod    time.Time
	size   int64
	exists bool
}

func statSig(path string) fileSig {
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fileSig{}
	}
	st, err := os.Stat(target)
	if err != nil {
		return fileSig{}
	}
	return fileSig{target: target, mod: st.ModTime(), size: st.Size(), exists: true}
}

// debounce is the pure core of the watcher: a new signature must be seen on
// two consecutive polls before it fires.
type debounce struct {
	last, pending fileSig
	waiting       bool
}

func (d *debounce) step(cur fileSig) bool {
	switch {
	case cur == d.last:
		d.waiting = false
		return false
	case d.waiting && cur == d.pending:
		d.last, d.waiting = cur, false
		return true
	}
	d.pending, d.waiting = cur, true
	return false
}

// Watch polls path every interval until ctx ends and calls onChange(exists)
// once per settled change. It never busy-loops.
func Watch(ctx context.Context, path string, interval time.Duration, onChange func(exists bool)) {
	d := &debounce{last: statSig(path)}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if cur := statSig(path); d.step(cur) {
				onChange(cur.exists)
			}
		}
	}
}

// Change is what a watched file reload produced.
type Change struct {
	Config    *Config  // nil when the new file is invalid or missing
	Err       error    // validation errors (last applied state stays)
	Live      []string // live sections that changed
	Bootstrap []string // bootstrap sections that changed: restart needed
	Missing   bool
}

// WatchFile reloads a file source on change and reports the section diff
// against the last good config. Only a file source is watched.
func WatchFile(ctx context.Context, cur *Config, in Inputs, reg *plugin.Registry, interval time.Duration, apply func(Change)) {
	if cur.Source.Kind != SourceFile {
		return
	}
	Watch(ctx, cur.Source.Path, interval, func(exists bool) {
		ch := reload(cur, in, reg, exists)
		if ch.Config != nil {
			cur = ch.Config
		}
		apply(ch)
	})
}

func reload(cur *Config, in Inputs, reg *plugin.Registry, exists bool) Change {
	if !exists {
		return Change{Missing: true}
	}
	next, err := LoadFrom(cur.Source, in, reg)
	if err != nil {
		return Change{Err: err}
	}
	ch := Change{Config: next}
	ch.Bootstrap, ch.Live = Diff(cur, next)
	return ch
}

// Diff lists the top-level sections whose exported form differs, split into
// bootstrap and live sections.
func Diff(a, b *Config) (bootstrap, live []string) {
	for _, f := range a.schema.Root.Fields {
		if sectionYAML(a, f) == sectionYAML(b, f) {
			continue
		}
		if Live(f.Name) {
			live = append(live, f.Name)
		} else {
			bootstrap = append(bootstrap, f.Name)
		}
	}
	return bootstrap, live
}

func sectionYAML(c *Config, f Field) string {
	v, ok := c.withSecretRefs()[f.Name]
	if !ok {
		return ""
	}
	n := exporter{s: c.schema, full: true}.node(f.Node, v, []string{f.Name})
	if n == nil {
		return ""
	}
	b, _ := yaml.Marshal(n)
	// secrets compare by resolved value too, never by printing them
	return string(b) + secretDigest(c.tree[f.Name])
}

func secretDigest(v any) string {
	switch x := v.(type) {
	case tree:
		s := ""
		for _, k := range sortedTreeKeys(x) {
			s += k + ":" + secretDigest(x[k]) + ";"
		}
		return s
	case []any:
		s := ""
		for _, cv := range x {
			s += secretDigest(cv) + ";"
		}
		return s
	case Secret:
		return hashString(x.value)
	}
	return ""
}

func sortedTreeKeys(t tree) []string {
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
