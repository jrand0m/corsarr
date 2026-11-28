package tvauto

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/tv"
)

// RSSJob is the TV RSS sync job name; RSSInterval its default cadence.
const (
	RSSJob      = "tvauto.rss"
	RSSInterval = 15 * time.Minute
)

// DefaultPackThreshold is the share of a fully aired season's monitored
// episodes that must be missing before a season pack is searched first.
const DefaultPackThreshold = 0.5

// TV is the tv module surface automation reads.
type TV interface {
	Series(ctx context.Context, id int64) (domain.Series, error)
	Episode(ctx context.Context, id int64) (domain.Episode, error)
	Episodes(ctx context.Context, seriesID int64) ([]domain.Episode, error)
	EachSeries(ctx context.Context, fn func(domain.Series) error) error
}

// Blocklist answers the blocklist lookup (store.Blocklist).
type Blocklist interface {
	Has(ctx context.Context, item domain.ItemRef, id domain.ReleaseIdentity) (bool, error)
}

// Publisher is the event bus.
type Publisher interface {
	Publish(domain.Event) (domain.Event, error)
}

// Deps wires the service.
type Deps struct {
	TV            TV
	Files         automation.Files
	Queue         automation.Queue
	Blocklist     Blocklist
	Cursors       automation.Cursors
	Indexers      func() []automation.Indexer
	Pub           Publisher
	Payload       decide.PayloadPolicy
	Modes         domain.Modes           // nil = all active
	Recent        automation.RecentStore // nil = in memory
	PackThreshold float64                // 0 = DefaultPackThreshold
	Searches      automation.StateStore  // nil = search results are not kept
	Art           domain.ArtLink         // artwork addresses in API answers; nil = unchanged
	Now           func() time.Time
	Log           *slog.Logger
}

// Service is the TV automation module.
type Service struct {
	d      Deps
	index  *Index
	recent automation.Recent
	mu     sync.Mutex // one automatic search or RSS batch at a time
}

// New returns the service with an empty index (call Rebuild at start).
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.PackThreshold <= 0 {
		d.PackThreshold = DefaultPackThreshold
	}
	return &Service{d: d, index: NewIndex(), recent: automation.NewRecent(d.Recent, d.Now)}
}

// Index exposes the wanted index (read-only use).
func (s *Service) Index() *Index { return s.index }

func (s *Service) warn(msg string, err error, args ...any) {
	if err != nil {
		s.d.Log.Warn(msg, append(args, "err", err)...)
	}
}

func (s *Service) publish(t domain.EventType, actor string, items []domain.EventItem, data any) {
	_, _ = s.d.Pub.Publish(domain.Event{Type: t, Actor: actor, Kind: domain.KindTV, Items: items, Data: data})
}

// current is an episode's library file as the decision engine sees it.
func (s *Service) current(ctx context.Context, e domain.Episode) *decide.CurrentFile {
	if e.FileID == 0 {
		return nil
	}
	f, ok, err := s.d.Files.File(ctx, e.FileID)
	if err != nil || !ok {
		return nil
	}
	return &decide.CurrentFile{Quality: decide.QualityID(f.Quality), FormatScore: f.FormatScore, Group: f.Group, Revision: f.Revision}
}

func (s *Service) entry(ctx context.Context, ser domain.Series, e domain.Episode) Entry {
	cur, now := s.current(ctx, e), s.d.Now()
	return Entry{Episode: tv.EpisodeView{Episode: e, Status: tv.Status(e, now)}, Series: tv.RefOf(ser),
		Reason: WantedReason(ser, e, cur, now), Available: tv.IsAvailable(e, now), Current: cur}
}

// Rebuild fills the index from every series (start-up).
func (s *Service) Rebuild(ctx context.Context) error {
	return s.d.TV.EachSeries(ctx, func(ser domain.Series) error {
		return s.loadSeries(ctx, ser)
	})
}

func (s *Service) loadSeries(ctx context.Context, ser domain.Series) error {
	eps, err := s.d.TV.Episodes(ctx, ser.ID)
	if err != nil {
		return err
	}
	entries := make([]Entry, len(eps))
	for i, e := range eps {
		entries[i] = s.entry(ctx, ser, e)
	}
	s.index.PutSeries(ser, entries)
	return nil
}

// refreshSeries re-derives a series and its episodes (or drops it when gone).
func (s *Service) refreshSeries(ctx context.Context, id int64) {
	ser, err := s.d.TV.Series(ctx, id)
	if err != nil {
		s.index.RemoveSeries(id)
		return
	}
	s.warn("index series", s.loadSeries(ctx, ser), "series", id)
}

// refreshEpisode re-derives one episode's entry.
func (s *Service) refreshEpisode(ctx context.Context, id int64) {
	e, err := s.d.TV.Episode(ctx, id)
	if err != nil {
		return // gone with its series; the series event drops it
	}
	ser, err := s.d.TV.Series(ctx, e.SeriesID)
	if err == nil {
		s.index.PutEpisode(s.entry(ctx, ser, e))
	}
}

// searchable refuses searches for a disabled kind; shadow searches run.
func (s *Service) searchable() error {
	if mode := s.d.Modes.Of(domain.KindTV); mode == domain.ModeDisabled {
		return domain.ModeError{Kind: domain.KindTV, Mode: mode}
	}
	return nil
}
