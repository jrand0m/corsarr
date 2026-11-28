package decide

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Catalog is every quality profile and custom format in force: the shipped
// profiles of each kind plus the owner's custom ones. It is immutable once
// built; the profile service builds a new one on every change and swaps it in.
type Catalog struct {
	builtins    map[domain.MediaKind][]QualityProfile
	customs     []QualityProfile
	customByKey map[string]QualityProfile
	formats     []CustomFormat
	formatByKey map[string]CustomFormat
}

func newCatalog() *Catalog {
	c := &Catalog{builtins: map[domain.MediaKind][]QualityProfile{}, customByKey: map[string]QualityProfile{},
		formatByKey: map[string]CustomFormat{}}
	for _, k := range []domain.MediaKind{domain.KindTV, domain.KindMovie, domain.KindMusic} {
		c.builtins[k] = DefaultProfiles(k)
	}
	return c
}

func (c *Catalog) addFormat(f CustomFormat) {
	c.formats = append(c.formats, f)
	c.formatByKey[f.key] = f
}

func (c *Catalog) addProfile(p QualityProfile) {
	c.customs = append(c.customs, p)
	c.customByKey[p.Key()] = p
}

func (c *Catalog) sort() {
	sort.SliceStable(c.formats, func(i, j int) bool { return lessName(c.formats[i].name, c.formats[j].name) })
	sort.SliceStable(c.customs, func(i, j int) bool { return lessName(c.customs[i].spec.Name, c.customs[j].spec.Name) })
}

func lessName(a, b string) bool { return strings.ToLower(a) < strings.ToLower(b) }

// Lookup finds the profile a kind's item names by key: the kind's built-in
// with that key, else a custom profile serving the kind.
func (c *Catalog) Lookup(kind domain.MediaKind, key string) (QualityProfile, bool) {
	for _, p := range c.builtins[kind] {
		if p.Key() == key {
			return p, true
		}
	}
	if p, ok := c.customByKey[key]; ok && p.Serves(kind) {
		return p, true
	}
	return QualityProfile{}, false
}

// Profiles lists the profiles a kind's items may use: built-ins first, then
// custom profiles by name.
func (c *Catalog) Profiles(kind domain.MediaKind) []QualityProfile {
	out := append([]QualityProfile(nil), c.builtins[kind]...)
	for _, p := range c.customs {
		if p.Serves(kind) {
			out = append(out, p)
		}
	}
	return out
}

// Builtins lists the shipped profiles of a kind.
func (c *Catalog) Builtins(kind domain.MediaKind) []QualityProfile {
	return append([]QualityProfile(nil), c.builtins[kind]...)
}

// Customs lists every custom profile by name.
func (c *Catalog) Customs() []QualityProfile { return append([]QualityProfile(nil), c.customs...) }

// Custom finds a custom profile by key.
func (c *Catalog) Custom(key string) (QualityProfile, bool) {
	p, ok := c.customByKey[key]
	return p, ok
}

// Formats lists every custom format by name.
func (c *Catalog) Formats() []CustomFormat { return append([]CustomFormat(nil), c.formats...) }

// Format finds a custom format by key.
func (c *Catalog) Format(key string) (CustomFormat, bool) { f, ok := c.formatByKey[key]; return f, ok }

// FormatMap is the formats by key (a copy).
func (c *Catalog) FormatMap() map[string]CustomFormat {
	out := make(map[string]CustomFormat, len(c.formatByKey))
	for k, f := range c.formatByKey {
		out[k] = f
	}
	return out
}

// UsedBy lists the custom profiles scoring a format, by name.
func (c *Catalog) UsedBy(formatKey string) []QualityProfile {
	var out []QualityProfile
	for _, p := range c.customs {
		for _, fs := range p.spec.Formats {
			if fs.Format.key == formatKey {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

var (
	builtinOnly = sync.OnceValue(newCatalog)
	current     atomic.Pointer[Catalog]
)

// CurrentCatalog is the catalog in force (the built-ins until one is set).
func CurrentCatalog() *Catalog {
	if c := current.Load(); c != nil {
		return c
	}
	return builtinOnly()
}

// SetCatalog puts a catalog in force; nil returns to the built-ins. Only the
// profile service calls it.
func SetCatalog(c *Catalog) { current.Store(c) }

// WithCatalog puts a catalog in force and returns the function restoring the
// previous one (for tests).
func WithCatalog(c *Catalog) (restore func()) {
	prev := current.Load()
	current.Store(c)
	return func() { current.Store(prev) }
}

// Lookup finds a kind's profile by key in the catalog in force.
func Lookup(kind domain.MediaKind, key string) (QualityProfile, bool) {
	return CurrentCatalog().Lookup(kind, key)
}

// ProfilesFor lists a kind's profiles in the catalog in force.
func ProfilesFor(kind domain.MediaKind) []QualityProfile { return CurrentCatalog().Profiles(kind) }
