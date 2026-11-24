// Package queue owns grab execution and the Download aggregate.
// It is the only writer of downloads: every state change goes through the
// one transition operation, guarded by domain.CheckTransition, and
// publishes download.* events. Polling is adaptive through the scheduler.
package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

// Job names registered with the scheduler.
const (
	PollJob  = "queue.poll"
	SweepJob = "queue.sweep"
)

// Default polling cadence.
const (
	FastPoll      = 5 * time.Second
	SlowPoll      = 60 * time.Second
	SweepInterval = 6 * time.Hour
	// MissingGrace keeps a just-added download from counting as removed
	// externally before the client lists it.
	MissingGrace = 2 * time.Minute
)

// Repo is the queue persistence (store.Queue implements it).
type Repo interface {
	CreateGrab(ctx context.Context, g domain.Grab, d domain.Download) (domain.Grab, store.StoredDownload, error)
	Download(ctx context.Context, id int64) (store.StoredDownload, bool, error)
	Downloads(ctx context.Context, states ...domain.DownloadState) ([]store.StoredDownload, error)
	List(ctx context.Context, cursor int64, limit int) ([]store.StoredDownload, int64, error)
	Save(ctx context.Context, d store.StoredDownload) error
	PayloadChecked(ctx context.Context, id int64) (bool, error)
	SetPayloadChecked(ctx context.Context, id int64) error
	Grab(ctx context.Context, id int64) (domain.Grab, bool, error)
	QueuedQualities(ctx context.Context, item domain.ItemRef) ([]string, error)
	GrabbedBefore(ctx context.Context, item domain.ItemRef, id domain.ReleaseIdentity) (bool, error)
}

// Downloader is one running downloader instance.
type Downloader struct {
	Key  domain.ObjectKey
	Impl plugin.Downloader
}

// Plugins are the running indexers and downloaders.
type Plugins interface {
	Indexer(key domain.ObjectKey) (plugin.Indexer, bool)
	Downloaders() []Downloader // usable ones, in preference order
}

// Scheduler is the part of the job scheduler polling needs.
type Scheduler interface {
	SetInterval(name string, d time.Duration) error
	SetEnabled(name string, on bool) error
	RunNow(name string) error
}

// Publisher is the event bus.
type Publisher interface {
	Publish(domain.Event) (domain.Event, error)
}

// Deps wires the service.
type Deps struct {
	Repo       Repo
	Plugins    Plugins
	Pub        Publisher
	Jobs       Scheduler
	UIClients  func() int // connected SSE clients ("UI connected")
	ItemTitles func(ctx context.Context, refs []domain.ItemRef) []domain.EventItem
	Payload    decide.PayloadPolicy
	Modes      domain.Modes // media-kind modes; nil = all active
	FastPoll   time.Duration
	SlowPoll   time.Duration
	Now        func() time.Time
	Log        *slog.Logger
}

// Service is the queue module.
type Service struct {
	d  Deps
	mu sync.Mutex // the single writer of the Download aggregate

	pollMu   sync.Mutex
	interval time.Duration
	enabled  bool
	idleRuns int             // consecutive polls in which nothing moved
	cats     map[string]bool // downloader/category pairs ensured once
}

// New returns the service with defaults filled in.
func New(d Deps) *Service {
	if d.FastPoll <= 0 {
		d.FastPoll = FastPoll
	}
	if d.SlowPoll <= 0 {
		d.SlowPoll = SlowPoll
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.UIClients == nil {
		d.UIClients = func() int { return 0 }
	}
	if d.ItemTitles == nil {
		d.ItemTitles = refsOnly
	}
	return &Service{d: d, enabled: true, cats: map[string]bool{}}
}

func refsOnly(_ context.Context, refs []domain.ItemRef) []domain.EventItem {
	out := make([]domain.EventItem, len(refs))
	for i, r := range refs {
		out[i] = domain.EventItem{Ref: r}
	}
	return out
}

// Errors the API maps to statuses.
var (
	ErrNotFound     = errors.New("download not found")
	ErrNoDownloader = errors.New("no usable downloader for this protocol")
	ErrHeld         = errors.New("download is on hold: automation leaves it alone")
)

// DefaultCategories is the downloader category per kind.
var DefaultCategories = map[domain.MediaKind]string{domain.KindMovie: "movies", domain.KindTV: "tv", domain.KindMusic: "music", domain.KindBook: "books"}

// categoryFor asks the downloader for its configured category, else the default.
func categoryFor(dl plugin.Downloader, kind domain.MediaKind) string {
	if c, ok := dl.(interface{ CategoryFor(domain.MediaKind) string }); ok && c.CategoryFor(kind) != "" {
		return c.CategoryFor(kind)
	}
	return DefaultCategories[kind]
}

func (s *Service) downloader(key domain.ObjectKey) (plugin.Downloader, bool) {
	for _, d := range s.d.Plugins.Downloaders() {
		if d.Key == key {
			return d.Impl, true
		}
	}
	return nil, false
}

func (s *Service) pick(rel domain.Release) (Downloader, bool) {
	for _, d := range s.d.Plugins.Downloaders() {
		if handles(d.Impl, rel) {
			return d, true
		}
	}
	return Downloader{}, false
}

// handles reports whether a downloader takes a release: it speaks the
// release's protocol, and a downloader that owns releases (a direct
// source) takes only its own.
func handles(dl plugin.Downloader, rel domain.Release) bool {
	if o, ok := dl.(interface{ Owns(domain.Release) bool }); ok && !o.Owns(rel) {
		return false
	}
	for _, p := range dl.Protocols() {
		if p == rel.Protocol {
			return true
		}
	}
	return false
}

func (s *Service) publish(t domain.EventType, actor string, items []domain.EventItem, data any) {
	_, _ = s.d.Pub.Publish(domain.Event{Type: t, Actor: actor, Kind: domain.KindOf(items, domain.KindMovie), Items: items, Data: data})
}

func (s *Service) warn(msg string, err error, args ...any) {
	if err != nil {
		s.d.Log.Warn(msg, append(args, "err", err)...)
	}
}

func lower(s string) string { return strings.ToLower(s) }

func refsOf(links []domain.DownloadLink) []domain.ItemRef {
	out := make([]domain.ItemRef, len(links))
	for i, l := range links {
		out[i] = l.Item
	}
	return out
}

func (s *Service) load(ctx context.Context, id int64) (store.StoredDownload, error) {
	d, ok, err := s.d.Repo.Download(ctx, id)
	if err == nil && !ok {
		err = fmt.Errorf("%w: %d", ErrNotFound, id)
	}
	return d, err
}

// Get returns one download.
func (s *Service) Get(ctx context.Context, id int64) (store.StoredDownload, error) {
	return s.load(ctx, id)
}

// List pages the queue newest first.
func (s *Service) List(ctx context.Context, cursor int64, limit int) ([]store.StoredDownload, int64, error) {
	return s.d.Repo.List(ctx, cursor, limit)
}
