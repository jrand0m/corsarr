package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/store"
)

// Category groups plan entries; the owner chooses the scope by category.
type Category string

const (
	CatLibrary     Category = "library"
	CatFiles       Category = "files"
	CatProfiles    Category = "profiles"
	CatRootFolders Category = "rootFolders"
	CatIndexers    Category = "indexers"
	CatClients     Category = "clients"
	CatNaming      Category = "naming"
	CatDownloads   Category = "downloads"
	CatBlocklist   Category = "blocklist"
	CatHistory     Category = "history"
	CatModes       Category = "modes"
)

// Categories lists every scope category in plan order.
var Categories = []Category{CatModes, CatProfiles, CatRootFolders, CatIndexers, CatClients, CatNaming, CatLibrary, CatFiles,
	CatDownloads, CatBlocklist, CatHistory}

// Scope is the set of categories a run covers.
type Scope map[Category]bool

// DefaultScope is every category except history, which is opt-in.
func DefaultScope() Scope {
	s := Scope{}
	for _, c := range Categories {
		s[c] = c != CatHistory
	}
	return s
}

// ParseScope reads a comma list of categories ("" = the default scope).
func ParseScope(list string) (Scope, error) {
	if strings.TrimSpace(list) == "" {
		return DefaultScope(), nil
	}
	known := map[Category]bool{}
	for _, c := range Categories {
		known[c] = true
	}
	s := Scope{CatModes: true}
	for _, part := range strings.Split(list, ",") {
		c := Category(strings.TrimSpace(part))
		if !known[c] {
			return nil, fmt.Errorf("unknown category %q", c)
		}
		s[c] = true
	}
	if s[CatLibrary] {
		s[CatFiles] = true
	}
	return s, nil
}

// Has reports whether the category is in scope.
func (s Scope) Has(c Category) bool { return s[c] }

// Outcome is what applying the plan does with one source object.
type Outcome string

const (
	OutCreate       Outcome = "create"
	OutUpdate       Outcome = "update"
	OutUnchanged    Outcome = "unchanged"
	OutSkip         Outcome = "skip"
	OutApproximate  Outcome = "approximate"
	OutUnsupported  Outcome = "unsupported"
	OutConflict     Outcome = "conflict"
	OutSecretNeeded Outcome = "secretNeeded"
	OutFileMissing  Outcome = "fileMissing"
	OutManaged      Outcome = "managedByConfig"
	OutGone         Outcome = "gone"
	OutFailed       Outcome = "failed"
)

// writes reports whether applying the entry changes the database.
func (o Outcome) writes() bool { return o == OutCreate || o == OutUpdate || o == OutApproximate }

// Entry is one source object's outcome with its reason.
type Entry struct {
	ID       string   `json:"id"` // <app>:<type>:<source id>, stable across runs
	Category Category `json:"category"`
	Source   App      `json:"source"`
	Name     string   `json:"name"`
	Outcome  Outcome  `json:"outcome"`
	Reason   string   `json:"reason"`
	Target   string   `json:"target,omitempty"`
	op       op       // what apply does; nil for report-only entries
}

// Report is a plan or an apply result.
type Report struct {
	RunID    int64           `json:"runId,omitempty"`
	Mode     string          `json:"mode"` // plan | apply
	Sources  []SourceInfo    `json:"sources"`
	Entries  []Entry         `json:"entries"`
	Counts   map[Outcome]int `json:"counts"`
	Config   string          `json:"config,omitempty"` // YAML for config-managed objects
	Env      []string        `json:"env,omitempty"`    // variables the YAML references
	Naming   []TokenMapping  `json:"naming,omitempty"` // how each source naming token maps
	PlanHash string          `json:"planHash"`
	Started  time.Time       `json:"startedAt"`
	Finished time.Time       `json:"finishedAt"`
}

// count recomputes the per-outcome counts.
func (r *Report) count() {
	r.Counts = map[Outcome]int{}
	for _, e := range r.Entries {
		r.Counts[e.Outcome]++
	}
}

// hash fingerprints the entries' ids and outcomes: an apply can tell that
// the live state moved since the plan the owner reviewed.
func (r *Report) hash() string {
	h := sha256.New()
	for _, e := range r.Entries {
		fmt.Fprintf(h, "%s=%s;", e.ID, e.Outcome)
	}
	return hex.EncodeToString(h.Sum(nil)[:12])
}

// Options shape a plan.
type Options struct {
	Scope       Scope
	Rewrite     Rewrite
	HistoryDays int
	Activate    bool            // imported kinds go active instead of shadow
	Exclude     map[string]bool // entry ids the owner left out
}

// Rewrite replaces a path prefix of the source with a corsarr path.
type Rewrite struct{ From, To string }

// ParseRewrite reads "from:to" ("" = none).
func ParseRewrite(s string) (Rewrite, error) {
	if s == "" {
		return Rewrite{}, nil
	}
	from, to, ok := strings.Cut(s, ":")
	if !ok || !strings.HasPrefix(from, "/") || !strings.HasPrefix(to, "/") {
		return Rewrite{}, fmt.Errorf("path rewrite %q must be /source/prefix:/corsarr/prefix", s)
	}
	return Rewrite{From: cleanPath(from), To: cleanPath(to)}, nil
}

// Apply rewrites a path when it starts with the prefix (at a separator).
func (r Rewrite) Apply(p string) string {
	if r.From == "" || (p != r.From && !strings.HasPrefix(p, r.From+"/")) {
		return p
	}
	return r.To + strings.TrimPrefix(p, r.From)
}

// ClientHit is a download found in a corsarr download client.
type ClientHit struct {
	Downloader string
	Category   string
	SavePath   string
}

// State is what corsarr holds, read before planning.
type State struct {
	Prov      map[store.ProvenanceKey]store.Provenance
	Current   map[store.ProvenanceKey]string // current target hash of each provenance target; absent = deleted
	Index     store.LibraryIndex
	Live      map[string]bool      // download ids corsarr already tracks
	Client    map[string]ClientHit // hash → where a corsarr client holds it; nil = no client configured
	Stat      func(path string) (size int64, ok bool)
	Config    ConfigView
	Overrides map[string]string // "kind:tv" | "rule:*" → mode already chosen
	Now       time.Time
}

// ConfigView is what the active config already sets.
type ConfigView struct {
	RootFolders  map[string]bool   // path
	Indexers     map[string]bool   // url
	ManagerRefs  map[string]string // indexer host → the API key reference an indexer there uses
	Clients      map[string]bool   // url
	KindExplicit map[string]bool   // media kind whose mode the config sets
	KindMode     map[string]string // effective mode of each kind the config sets
	Naming       map[string]bool   // media kind whose naming the config sets
}

// hashOf fingerprints any value through its JSON form.
func hashOf(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:12])
}

func entryID(a App, typ string, id any) string { return fmt.Sprintf("%s:%s:%v", a, typ, id) }

// sortEntries orders a report by category, then id.
func sortEntries(list []Entry) {
	rank := map[Category]int{}
	for i, c := range Categories {
		rank[c] = i
	}
	sort.SliceStable(list, func(i, j int) bool {
		if rank[list[i].Category] != rank[list[j].Category] {
			return rank[list[i].Category] < rank[list[j].Category]
		}
		return list[i].ID < list[j].ID
	})
}
