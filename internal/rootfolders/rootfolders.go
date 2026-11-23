// Package rootfolders tracks the library root folders: it
// stores each configured folder with its last checked state (accessible,
// writable, free space), checks them on a schedule, at start and on config
// change, reports one IntegrationHealth record per folder (missing,
// read-only, full) and serves GET /api/v1/rootfolders. The rule that turns a
// probe into a problem is pure; the probe is the only file-system I/O.
package rootfolders

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
)

// CheckJob is the scheduled check; CheckInterval its cadence.
const (
	CheckJob      = "rootfolders.check"
	CheckInterval = 15 * time.Minute
)

// Reserve is the free space every root folder keeps (default 100 MB): below it the folder is full and imports block.
const Reserve int64 = 100 * 1000 * 1000

// HealthType is the IntegrationHealth ref type of a root folder.
const HealthType = "rootFolder"

// Ref is the health ref of the folder at path.
func Ref(path string) domain.IntegrationRef {
	return domain.IntegrationRef{Type: HealthType, Key: domain.ObjectKey(path)}
}

// Configured is one root folder from the config.
type Configured struct {
	Path string
	Kind domain.MediaKind
}

// Probe is what the file system says about a folder.
type Probe struct {
	Dir      bool // exists and is a directory
	Writable bool
	Free     int64
}

// Assess is the pure rule: missing, then read-only, then full (free space
// below reserve), else fine. It returns the problem and the health observation.
func Assess(p Probe, reserve int64) (domain.RootFolderProblem, health.Observation) {
	switch {
	case !p.Dir:
		return domain.RootMissing, health.Observation{Class: health.ClassPath, Message: "the folder is missing or not a directory"}
	case !p.Writable:
		return domain.RootReadOnly, health.Observation{Class: health.ClassPath, Message: "corsarr may not write to the folder"}
	case p.Free < reserve:
		return domain.RootFull, health.Observation{Class: health.ClassDiskFull, Message: "free space is below the import reserve"}
	}
	return domain.RootOK, health.Observation{OK: true}
}

// Repo stores the checked folders (store.RootFolders).
type Repo interface {
	Replace(ctx context.Context, list []domain.RootFolder) error
	List(ctx context.Context) ([]domain.RootFolder, error)
}

// Reporter records health (health.Registry).
type Reporter interface {
	Report(ref domain.IntegrationRef, o health.Observation) domain.IntegrationHealth
}

// Deps wires the checker.
type Deps struct {
	Roots   func() []Configured
	Repo    Repo
	Health  Reporter
	Probe   func(path string) Probe // default ProbePath
	Reserve int64                   // default Reserve
	Now     func() time.Time
	Log     *slog.Logger
}

// Checker owns the root-folder records.
type Checker struct {
	d    Deps
	mu   sync.Mutex // one check at a time; the single writer
	seen map[string]bool
}

// New returns a checker with defaults filled in.
func New(d Deps) *Checker {
	if d.Probe == nil {
		d.Probe = ProbePath
	}
	if d.Reserve <= 0 {
		d.Reserve = Reserve
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Checker{d: d, seen: map[string]bool{}}
}

// Check probes every configured folder, stores the result and reports health;
// folders dropped from the config are removed and their health disabled.
func (c *Checker) Check(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.d.Now().UTC()
	list := []domain.RootFolder{}
	next := map[string]bool{}
	for _, r := range c.d.Roots() {
		f, o := c.check(r, now)
		c.d.Health.Report(Ref(r.Path), o)
		list, next[r.Path] = append(list, f), true
	}
	for p := range c.seen {
		if !next[p] {
			c.d.Health.Report(Ref(p), health.Observation{Disabled: true})
		}
	}
	c.seen = next
	return c.d.Repo.Replace(ctx, list)
}

func (c *Checker) check(r Configured, now time.Time) (domain.RootFolder, health.Observation) {
	p := c.d.Probe(r.Path)
	problem, o := Assess(p, c.d.Reserve)
	return domain.RootFolder{Path: r.Path, Kind: r.Kind, Accessible: p.Dir, Writable: p.Dir && p.Writable,
		FreeSpace: p.Free, Problem: problem, Managed: true, LastChecked: &now}, o
}

// List returns the stored folders.
func (c *Checker) List(ctx context.Context) ([]domain.RootFolder, error) { return c.d.Repo.List(ctx) }

// Mount registers GET /api/v1/rootfolders.
func (c *Checker) Mount(m api.Mounter) {
	m.Handle("GET /api/v1/rootfolders", func(w http.ResponseWriter, r *http.Request) {
		list, err := c.List(r.Context())
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "internal", err.Error(), "")
			return
		}
		api.WriteJSON(w, http.StatusOK, map[string]any{"items": list})
	})
}
