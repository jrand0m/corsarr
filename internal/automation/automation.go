// Package automation is the automation and queue-health module: the wanted index, search on
// add and on availability, interactive search streamed as NDJSON, the RSS
// sync job, failure follow-up (blocklist, then re-search) and the blocklist.
package automation

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/queue"
)

// RSSJob is the RSS sync job name; RSSInterval its default cadence.
const (
	RSSJob      = "automation.rss"
	RSSInterval = 15 * time.Minute
)

// Movies is the movie module surface automation reads.
type Movies interface {
	Get(ctx context.Context, id int64) (domain.Movie, error)
	Each(ctx context.Context, fn func(domain.Movie) error) error
}

// Files reads library files (store.Library).
type Files interface {
	File(ctx context.Context, id int64) (domain.LibraryFile, bool, error)
}

// Queue is the queue module surface: grabs and the lookups the decision engine needs.
type Queue interface {
	Grab(ctx context.Context, req queue.GrabRequest) (domain.Grab, error)
	GrabbedBefore(ctx context.Context, item domain.ItemRef, rel domain.Release) (bool, error)
	QueuedQualities(ctx context.Context, item domain.ItemRef) ([]string, error)
}

// Blocklist is the blocklist persistence (store.Blocklist).
type Blocklist interface {
	Add(ctx context.Context, e domain.BlocklistEntry) (domain.BlocklistEntry, bool, error)
	Has(ctx context.Context, item domain.ItemRef, id domain.ReleaseIdentity) (bool, error)
	List(ctx context.Context, cursor int64, limit int) ([]domain.BlocklistEntry, int64, error)
	Remove(ctx context.Context, id int64) (domain.BlocklistEntry, bool, error)
}

// Cursors are the per-indexer RSS cursors (store.FeedCursors).
type Cursors interface {
	Get(ctx context.Context, indexer domain.ObjectKey, kind domain.MediaKind) (string, error)
	Set(ctx context.Context, indexer domain.ObjectKey, kind domain.MediaKind, cursor string) error
}

// Indexer is one running indexer instance.
type Indexer struct {
	Key   domain.ObjectKey
	Impl  plugin.Indexer
	Limit Limit // query limit (zero = none)
}

// Publisher is the event bus.
type Publisher interface {
	Publish(domain.Event) (domain.Event, error)
}

// Deps wires the service.
type Deps struct {
	Movies    Movies
	Files     Files
	Queue     Queue
	Blocklist Blocklist
	Cursors   Cursors
	Searches  Searches
	Recent    RecentStore
	Indexers  func() []Indexer
	Pub       Publisher
	Payload   decide.PayloadPolicy
	Modes     domain.Modes // media-kind modes; nil = all active
	Now       func() time.Time
	Log       *slog.Logger
}

// Service is the automation module.
type Service struct {
	d     Deps
	index *Index
	queue *searchQueue
	mu    sync.Mutex // one automatic search or RSS batch at a time
}

// New returns the service with an empty index (call Rebuild at start).
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Service{d: d, index: NewIndex(), queue: newSearchQueue()}
}

// Index exposes the wanted index (read-only use).
func (s *Service) Index() *Index { return s.index }

func (s *Service) publish(t domain.EventType, actor string, items []domain.EventItem, data any) {
	_, _ = s.d.Pub.Publish(domain.Event{Type: t, Actor: actor, Kind: domain.KindOf(items, domain.KindMovie), Items: items, Data: data})
}

func (s *Service) warn(msg string, err error, args ...any) {
	if err != nil {
		s.d.Log.Warn(msg, append(args, "err", err)...)
	}
}

// Recent-release cache bounds: a release shown by a search can be grabbed by
// id for RecentTTL; at most RecentMax are kept.
const (
	RecentTTL = 24 * time.Hour
	RecentMax = 500
)

// RecentStore persists the releases shown by searches (store.RecentReleases),
// so a manual grab survives a restart; download links stay server-side.
type RecentStore interface {
	PutMany(ctx context.Context, rels []domain.Release, t, expire time.Time, max int) error
	Get(ctx context.Context, id string, since time.Time) (domain.Release, bool, error)
}

// remember stores releases a search showed.
func (s *Service) remember(ctx context.Context, rels []domain.Release) {
	if len(rels) == 0 {
		return
	}
	now := s.d.Now()
	s.warn("remember releases", s.d.Recent.PutMany(ctx, rels, now, now.Add(-RecentTTL), RecentMax))
}
