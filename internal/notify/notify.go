package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

// Delivery defaults for notifications and media-server refreshes.
const (
	MaxAttempts = 5
	// MaxRefreshAttempts caps the retries of one media-server refresh.
	MaxRefreshAttempts = 5
	BaseBackoff        = 30 * time.Second
	MaxBackoff         = 2 * time.Hour
	MergeWindow        = 30 * time.Second
)

// Outbox is the delivery persistence (store.Outbox).
type Outbox interface {
	Enqueue(ctx context.Context, e store.OutboxEntry) (bool, error)
	Due(ctx context.Context, t time.Time, limit int) ([]store.OutboxEntry, error)
	Batch(ctx context.Context, connection, key string) ([]store.OutboxEntry, error)
	NextDue(ctx context.Context) (time.Time, bool, error)
	Finish(ctx context.Context, e store.OutboxEntry) error
}

// Refreshes are the pending media-server refreshes (store.Refreshes).
type Refreshes interface {
	Add(ctx context.Context, server, path string, due time.Time) error
	Due(ctx context.Context, t time.Time) (map[string][]string, error)
	NextDue(ctx context.Context) (time.Time, bool, error)
	Done(ctx context.Context, server string, paths []string) error
	Retry(ctx context.Context, server string, paths []string, next time.Time) error
	Attempts(ctx context.Context, server string, paths []string) (int, error)
}

// Reporter records integration health (health.Registry).
type Reporter interface {
	Report(ref domain.IntegrationRef, o health.Observation) domain.IntegrationHealth
}

// Publisher is the event bus.
type Publisher interface {
	Publish(domain.Event) (domain.Event, error)
}

// Connection is one notification connection.
type Connection struct {
	Key      domain.ObjectKey
	Impl     plugin.Notifier
	Toggles  []string
	Template Template // the connection's own title/body; zero = built-in
}

// MediaServer is one media server instance.
type MediaServer struct {
	Key  domain.ObjectKey
	Impl plugin.MediaServer
}

// Deps wires the service.
type Deps struct {
	Outbox       Outbox
	Refreshes    Refreshes
	Connections  func() []Connection
	MediaServers func() []MediaServer
	Health       Reporter  // media-server health; may be nil
	Pub          Publisher // History for abandoned refreshes; may be nil
	MergeWindow  time.Duration
	BaseBackoff  time.Duration
	Modes        domain.Modes // media-kind modes; nil = all active
	Now          func() time.Time
	Log          *slog.Logger
}

// Service is the notify module.
type Service struct {
	d    Deps
	wake chan struct{}
}

// New returns the service with defaults.
func New(d Deps) *Service {
	if d.MergeWindow <= 0 {
		d.MergeWindow = MergeWindow
	}
	if d.BaseBackoff <= 0 {
		d.BaseBackoff = BaseBackoff
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Service{d: d, wake: make(chan struct{}, 1)}
}

func (s *Service) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) warn(msg string, err error, args ...any) {
	if err != nil {
		s.d.Log.Warn(msg, append(args, "err", err)...)
	}
}

// OnEvent writes one outbox row per subscribed connection and queues media
// refreshes; it never blocks on delivery.
func (s *Service) OnEvent(ctx context.Context, ev domain.Event) {
	if !s.d.Modes.AllActive(ev.Items) {
		return // no notifications or library refreshes for shadow/disabled kinds
	}
	s.enqueue(ctx, ev)
	s.queueRefresh(ctx, ev)
	s.poke()
}

func (s *Service) enqueue(ctx context.Context, ev domain.Event) {
	env, _ := json.Marshal(ev) // event payloads are plain data
	key, at := BatchKey(ev), s.d.Now()
	if key != "" {
		at = at.Add(s.d.MergeWindow) // wait for the download's next events
	}
	for _, c := range s.d.Connections() {
		if !Wants(c.Toggles, ev) {
			continue
		}
		title, body := RenderWith(c.Template, ev)
		_, err := s.d.Outbox.Enqueue(ctx, store.OutboxEntry{EventID: ev.EventID, Connection: string(c.Key), EventType: string(ev.Type),
			Title: title, Body: body, Envelope: string(env), BatchKey: key, NextAt: at})
		s.warn("enqueue notification", err, "connection", c.Key)
	}
}

// Preview is what a connection's Test sends.
type Preview struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Sent  bool   `json:"sent"`
	Error string `json:"error,omitempty"`
}

// SampleEvent is the import the Test preview renders.
func SampleEvent(now time.Time) domain.Event {
	item := domain.EventItem{Ref: domain.ItemRef{Kind: domain.KindMovie, ID: 0}, Title: "Example Movie"}
	return domain.Event{SchemaVersion: 1, EventID: "test", Type: domain.EvImportCompleted, OccurredAt: now, Actor: "system",
		Kind: domain.KindMovie, Items: []domain.EventItem{item}, Data: domain.ImportEventData{Outcome: domain.ImportOutcome{
			Status: domain.ImportImported, Items: []domain.ImportItem{{Item: item.Ref, Outcome: "imported", Quality: "WEBDL-1080p",
				NewPaths: []string{"/media/movies/Example Movie (2021)/Example Movie (2021) [WEBDL-1080p].mkv"}}}}}}
}

// Test renders the sample event through a connection's template and, with
// send, delivers it.
func (s *Service) Test(ctx context.Context, key domain.ObjectKey, send bool) (Preview, error) {
	for _, c := range s.d.Connections() {
		if c.Key != key {
			continue
		}
		ev := SampleEvent(s.d.Now())
		p := Preview{}
		p.Title, p.Body = RenderWith(c.Template, ev)
		if send {
			err := c.Impl.Send(ctx, ev, plugin.Message{Title: p.Title, Body: p.Body})
			p.Sent = err == nil
			if err != nil {
				p.Error = err.Error()
			}
		}
		return p, nil
	}
	return Preview{}, ErrUnknownConnection
}

// ErrUnknownConnection says no running connection has the key.
var ErrUnknownConnection = errors.New("no notification connection with this key")

// refreshFailed retries with growing jittered backoff; after
// MaxRefreshAttempts it drops the paths, marks the server unhealthy and
// records refresh.abandoned in History.
func (s *Service) refreshFailed(ctx context.Context, ms MediaServer, paths []string, err error) {
	key := string(ms.Key)
	s.warn("media server refresh", err, "server", key)
	n, aerr := s.d.Refreshes.Attempts(ctx, key, paths)
	s.warn("read refresh attempts", aerr, "server", key)
	n++
	if n < MaxRefreshAttempts {
		s.warn("retry refresh", s.d.Refreshes.Retry(ctx, key, paths, s.d.Now().Add(Backoff(s.d.BaseBackoff, n, rand.Int64N))))
		return
	}
	s.warn("drop refresh", s.d.Refreshes.Done(ctx, key, paths), "server", key)
	reason := fmt.Sprintf("library refresh gave up after %d attempts: %v", n, err)
	if s.d.Health != nil {
		s.d.Health.Report(domain.IntegrationRef{Type: string(plugin.KindMediaServer), Key: ms.Key},
			health.Observation{Class: failureClass(err), Message: reason})
	}
	if s.d.Pub != nil {
		_, _ = s.d.Pub.Publish(domain.Event{Type: domain.EvRefreshAbandoned, Actor: "system",
			Data: domain.RefreshEventData{Server: ms.Key, Paths: paths, Attempts: n, Reason: reason}})
	}
}

// failureClass is the error's failure class, unreachable when it has none.
func failureClass(err error) string {
	if c := plugin.ClassOf(err); c != "" {
		return string(c)
	}
	return health.ClassUnreachable
}

// RefreshFolders are the library folders an event changed.
func RefreshFolders(ev domain.Event) []string {
	switch d := ev.Data.(type) {
	case domain.ImportEventData:
		if ev.Type == domain.EvImportCompleted {
			return d.Folders
		}
	case domain.FileEventData:
		if d.OldPath != "" && filepath.Dir(d.OldPath) != filepath.Dir(d.Path) {
			return []string{filepath.Dir(d.Path), filepath.Dir(d.OldPath)} // moved: both folders change
		}
		return []string{filepath.Dir(d.Path)}
	}
	return nil
}

func (s *Service) queueRefresh(ctx context.Context, ev domain.Event) {
	due := s.d.Now().Add(s.d.MergeWindow)
	for _, folder := range RefreshFolders(ev) {
		for _, ms := range s.d.MediaServers() {
			s.warn("queue refresh", s.d.Refreshes.Add(ctx, string(ms.Key), folder, due), "server", ms.Key)
		}
	}
}

// Run delivers due notifications and refreshes until ctx ends. It sleeps
// until the next due time or a new event; nothing pending means no timer.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTimer(time.Hour)
	defer t.Stop()
	for {
		next := s.Flush(ctx)
		t.Stop()
		var timer <-chan time.Time
		if !next.IsZero() {
			t.Reset(max(time.Until(next), 10*time.Millisecond))
			timer = t.C
		}
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-timer:
		}
	}
}

// Flush delivers everything due now and returns the next due time (zero = none).
func (s *Service) Flush(ctx context.Context) time.Time {
	s.deliverDue(ctx)
	s.refreshDue(ctx)
	a, okA, _ := s.d.Outbox.NextDue(ctx)
	b, okB, _ := s.d.Refreshes.NextDue(ctx)
	return earliest(a, okA, b, okB)
}

// earliest folds two optional times (zero when neither is set).
func earliest(a time.Time, okA bool, b time.Time, okB bool) time.Time {
	switch {
	case okB && (!okA || b.Before(a)):
		return b
	case okA:
		return a
	}
	return time.Time{}
}

func (s *Service) deliverDue(ctx context.Context) {
	due, err := s.d.Outbox.Due(ctx, s.d.Now(), 50)
	s.warn("read outbox", err)
	conns := map[string]Connection{}
	for _, c := range s.d.Connections() {
		conns[string(c.Key)] = c
	}
	done := map[int64]bool{}
	for _, e := range due {
		if done[e.ID] {
			continue // delivered with an earlier entry of its batch
		}
		batch := s.batchOf(ctx, e)
		sent := s.attempt(ctx, conns, Merge(batch))
		for _, b := range batch {
			done[b.ID] = true
			b.Status, b.Attempts, b.NextAt, b.LastError = sent.Status, sent.Attempts, sent.NextAt, sent.LastError
			s.warn("finish delivery", s.d.Outbox.Finish(ctx, b), "id", b.ID)
		}
	}
}

// batchOf is the entry plus every pending entry of its batch.
func (s *Service) batchOf(ctx context.Context, e store.OutboxEntry) []store.OutboxEntry {
	if e.BatchKey == "" {
		return []store.OutboxEntry{e}
	}
	batch, err := s.d.Outbox.Batch(ctx, e.Connection, e.BatchKey)
	if err != nil || len(batch) == 0 {
		s.warn("read batch", err, "id", e.ID)
		return []store.OutboxEntry{e}
	}
	return batch
}

// BatchKey groups the events of one download (pure): grab, import and
// failures of the same download become one message per connection.
func BatchKey(ev domain.Event) string {
	var id int64
	switch d := ev.Data.(type) {
	case domain.GrabEventData:
		id = d.DownloadID
	case domain.ImportEventData:
		id = d.Outcome.DownloadID
	case domain.DownloadEventData:
		id = d.DownloadID
	case domain.PayloadEventData:
		id = d.DownloadID
	}
	if id == 0 {
		return ""
	}
	return "download:" + strconv.FormatInt(id, 10)
}

// Merge folds a batch (oldest first) into one delivery (pure): the latest
// title, every body in order, the latest event as the envelope; attempts
// count from the most retried entry.
func Merge(batch []store.OutboxEntry) store.OutboxEntry {
	out := batch[len(batch)-1]
	bodies := make([]string, 0, len(batch))
	for _, b := range batch {
		if b.Body != "" {
			bodies = append(bodies, b.Body)
		}
		out.Attempts = max(out.Attempts, b.Attempts)
	}
	out.Body = strings.Join(bodies, "\n")
	return out
}

// attempt delivers one entry and returns its new state.
func (s *Service) attempt(ctx context.Context, conns map[string]Connection, e store.OutboxEntry) store.OutboxEntry {
	e.Attempts++
	c, ok := conns[e.Connection]
	if !ok {
		e.Status, e.LastError = "failed", "connection no longer configured"
		return e
	}
	var ev domain.Event
	_ = json.Unmarshal([]byte(e.Envelope), &ev) // written by enqueue
	err := c.Impl.Send(ctx, ev, plugin.Message{Title: e.Title, Body: e.Body})
	switch {
	case err == nil:
		e.Status, e.LastError = "sent", ""
	case e.Attempts >= MaxAttempts:
		e.Status, e.LastError = "failed", err.Error()
	default:
		e.LastError, e.NextAt = err.Error(), s.d.Now().Add(Backoff(s.d.BaseBackoff, e.Attempts, rand.Int64N))
	}
	return e
}

// Backoff is the jittered exponential retry delay after n attempts.
func Backoff(base time.Duration, n int, jitter func(int64) int64) time.Duration {
	d := base << min(n-1, 16)
	d = min(d, MaxBackoff)
	return d/2 + time.Duration(jitter(int64(d/2)+1))
}

func (s *Service) refreshDue(ctx context.Context) {
	due, err := s.d.Refreshes.Due(ctx, s.d.Now())
	s.warn("read refreshes", err)
	servers := map[string]MediaServer{}
	for _, ms := range s.d.MediaServers() {
		servers[string(ms.Key)] = ms
	}
	for key, paths := range due {
		ms, ok := servers[key]
		if !ok {
			s.warn("drop refresh", s.d.Refreshes.Done(ctx, key, paths), "server", key)
			continue
		}
		if err := ms.Impl.RefreshPaths(ctx, paths); err != nil {
			s.refreshFailed(ctx, ms, paths, err)
			continue
		}
		s.warn("refresh done", s.d.Refreshes.Done(ctx, key, paths), "server", key)
	}
}
