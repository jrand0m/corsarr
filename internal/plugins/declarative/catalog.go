package declarative

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/jrand0m/corsarr/internal/definition"
	"github.com/jrand0m/corsarr/internal/plugin"
)

//go:embed bundled/*.yaml
var bundledFS embed.FS

// Origins of a definition.
const (
	OriginBundled = "bundled"
	OriginUser    = "user"
)

// UserDir is where owner definitions live, relative to the data directory.
const UserDir = "plugins/definitions"

// Entry is one definition file as the catalogue lists it, valid or not.
type Entry struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Language    string               `json:"language,omitempty"`
	Privacy     definition.Privacy   `json:"privacy,omitempty"`
	Version     string               `json:"version,omitempty"`
	Origin      string               `json:"origin"`
	File        string               `json:"file"`
	Overrides   bool                 `json:"overrides,omitempty"` // a user file replacing a bundled id
	Valid       bool                 `json:"valid"`
	Problems    []definition.Problem `json:"problems,omitempty"`
	Login       string               `json:"login,omitempty"`
	Modes       []string             `json:"modes,omitempty"`
}

// Catalog keeps the bundled and user definitions and their plugin
// registrations in step. It is the only writer of declarative registrations.
type Catalog struct {
	mu      sync.Mutex
	reg     *plugin.Registry
	bundled map[string]*definition.Definition
	defs    map[string]*definition.Definition // what is registered now
	entries []Entry
	userIDs map[string]bool
}

// Default is the catalogue registered into plugin.Default.
var Default = mustBundled(plugin.Default)

func mustBundled(reg *plugin.Registry) *Catalog {
	c, err := NewCatalog(reg, bundledFS)
	if err != nil {
		panic(err) // a broken bundled definition is a build bug, caught by tests
	}
	return c
}

// NewCatalog registers the bundled definitions of fsys (files bundled/*.yaml).
func NewCatalog(reg *plugin.Registry, fsys fs.FS) (*Catalog, error) {
	c := &Catalog{reg: reg, bundled: map[string]*definition.Definition{}, defs: map[string]*definition.Definition{}, userIDs: map[string]bool{}}
	files, _ := fs.Glob(fsys, "bundled/*.yaml")
	for _, f := range files {
		data, _ := fs.ReadFile(fsys, f)
		d, err := definition.Parse(data)
		if err != nil {
			return nil, err
		}
		if err := reg.Put(Registration(d)); err != nil {
			return nil, err
		}
		c.bundled[d.ID], c.defs[d.ID] = d, d
		c.entries = append(c.entries, entryOf(d, OriginBundled, filepath.Base(f)))
	}
	c.sortEntries()
	return c, nil
}

func entryOf(d *definition.Definition, origin, file string) Entry {
	e := Entry{ID: d.ID, Name: d.Name, Description: d.Description, Language: d.Language, Privacy: d.Privacy, Version: d.Version,
		Origin: origin, File: file, Valid: true, Login: string(d.Login.Method)}
	for _, m := range modeOrder {
		if _, ok := d.Caps.Modes[m]; ok {
			e.Modes = append(e.Modes, m)
		}
	}
	return e
}

// Entries lists every definition file (sorted by id, then origin).
func (c *Catalog) Entries() []Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Entry(nil), c.entries...)
}

// Definition returns the registered definition of an id.
func (c *Catalog) Definition(id string) (*definition.Definition, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d, ok := c.defs[id]
	return d, ok
}

// Load re-reads the user definitions in dir (*.yaml, *.yml, *.json):
// valid ones are registered (overriding bundled ids), invalid ones are listed
// with their problems, and user definitions that disappeared are dropped
// (restoring the bundled one they overrode). A missing dir means none.
func (c *Catalog) Load(dir string) []Entry {
	user := map[string]*definition.Definition{}
	var listed []Entry
	for _, path := range definitionFiles(dir) {
		e, d := c.read(path)
		if d != nil {
			if _, dup := user[d.ID]; dup {
				e = Entry{ID: d.ID, Name: d.Name, Origin: OriginUser, File: e.File,
					Problems: []definition.Problem{{Path: "id", Msg: "another user file already defines " + d.ID}}}
				d = nil
			} else {
				user[d.ID] = d
			}
		}
		listed = append(listed, e)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.apply(user, listed)
	return append([]Entry(nil), c.entries...)
}

func definitionFiles(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !e.IsDir() && (ext == ".yaml" || ext == ".yml" || ext == ".json") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

func (c *Catalog) read(path string) (Entry, *definition.Definition) {
	file := filepath.Base(path)
	data, err := readBounded(path)
	if err == nil {
		var d *definition.Definition
		if d, err = definition.Parse(data); err == nil {
			e := entryOf(d, OriginUser, file)
			_, e.Overrides = c.bundled[d.ID]
			return e, d
		}
	}
	id := strings.TrimSuffix(file, filepath.Ext(file))
	return Entry{ID: id, Name: id, Origin: OriginUser, File: file, Problems: definition.Problems(err)}, nil
}

func readBounded(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return plugin.ReadLimited(f, definition.MaxFileSize+1)
}

// apply swaps the registered set to bundled ∪ user (user wins).
func (c *Catalog) apply(user map[string]*definition.Definition, listed []Entry) {
	for id := range c.userIDs {
		if _, still := user[id]; still {
			continue
		}
		if b, ok := c.bundled[id]; ok {
			_ = c.reg.Put(Registration(b))
			c.defs[id] = b
		} else {
			c.reg.Remove(id)
			delete(c.defs, id)
		}
	}
	c.userIDs = map[string]bool{}
	for id, d := range user {
		if existing, ok := c.reg.Get(id); ok && existing.Manifest.Delivery != Delivery {
			continue // never shadow a built-in plugin
		}
		if c.reg.Put(Registration(d)) == nil {
			c.defs[id], c.userIDs[id] = d, true
		}
	}
	entries := []Entry{}
	for _, e := range c.entries {
		if e.Origin == OriginBundled {
			entries = append(entries, e)
		}
	}
	c.entries = append(entries, listed...)
	c.sortEntries()
}

func (c *Catalog) sortEntries() {
	sort.SliceStable(c.entries, func(i, j int) bool {
		if c.entries[i].ID != c.entries[j].ID {
			return c.entries[i].ID < c.entries[j].ID
		}
		return c.entries[i].Origin < c.entries[j].Origin
	})
}
