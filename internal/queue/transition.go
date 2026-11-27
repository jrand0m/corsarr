package queue

import (
	"context"
	"fmt"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

// change describes one requested transition.
type change struct {
	to     domain.DownloadState
	actor  domain.Actor
	reason string
	extra  func(*domain.DownloadEventData) // cause, flags for the follow-up event
}

// transition is the single transition operation: it checks
// the move, persists it and publishes download.state_changed plus the
// target's own event. Callers hold s.mu.
func (s *Service) transition(ctx context.Context, d *store.StoredDownload, c change) error {
	if c.actor == domain.ActorAutomation && d.OnHold() {
		return fmt.Errorf("%w: %d", ErrHeld, d.ID) // rules and follow-ups never touch a held download
	}
	if err := domain.CheckTransition(d.State, c.to, c.actor); err != nil {
		s.d.Log.Warn("download transition rejected", "download", d.ID, "err", err)
		return err
	}
	from := d.State
	d.State, d.StatusReason = c.to, c.reason
	if c.to == domain.StateCompleted {
		at := s.d.Now().UTC()
		d.CompletedAt = &at
	}
	if err := s.d.Repo.Save(ctx, *d); err != nil {
		return err
	}
	s.warn("record download transition", s.d.Repo.AddTransition(ctx, d.ID, store.Transition{From: from, To: c.to, Actor: c.actor,
		Reason: c.reason, At: s.d.Now().UTC()}), "download", d.ID)
	data := s.eventData(ctx, d)
	data.From, data.To, data.Actor, data.Reason = from, c.to, c.actor, c.reason
	if c.extra != nil {
		c.extra(&data)
	}
	items := s.d.ItemTitles(ctx, refsOf(d.Links))
	actor := actorName(c.actor)
	s.publish(domain.EvDownloadState, actor, items, data)
	if t, ok := targetEvents[c.to]; ok {
		s.publish(t, actor, items, data)
	}
	return nil
}

var targetEvents = map[domain.DownloadState]domain.EventType{
	domain.StateCompleted: domain.EvDownloadCompleted,
	domain.StateFailed:    domain.EvDownloadFailed,
	domain.StateRemoved:   domain.EvDownloadRemoved,
}

func actorName(a domain.Actor) string {
	switch a {
	case domain.ActorUser:
		return "user"
	case domain.ActorDownloader:
		return "plugin:downloader"
	}
	return "automation"
}

func (s *Service) eventData(ctx context.Context, d *store.StoredDownload) domain.DownloadEventData {
	data := domain.DownloadEventData{DownloadID: d.ID, GrabID: d.GrabID, Downloader: d.Downloader, ClientID: d.DownloadID,
		Title: d.Title, Progress: d.Progress, Speed: d.Speed, ETASeconds: d.ETASeconds, SavePath: d.SavePath,
		ContentPath: d.ContentPath, Protocol: d.Protocol, Release: domain.ReleaseIdentity{Title: d.Title, InfoHash: d.DownloadID},
		Held: d.OnHold()}
	if g, ok, err := s.d.Repo.Grab(ctx, d.GrabID); err == nil && ok {
		data.Release = domain.IdentityOf(g.Release)
	}
	return data
}

// Transition applies a transition requested by another module (import,
// automation) or the user.
func (s *Service) Transition(ctx context.Context, id int64, to domain.DownloadState, by domain.Actor, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	return s.transition(ctx, &d, change{to: to, actor: by, reason: reason})
}

var linkOutcome = map[string]domain.LinkStatus{
	"imported": domain.LinkImported, "upgraded": domain.LinkImported, "skipped": domain.LinkSkipped,
	"blocked": domain.LinkBlocked, "failed": domain.LinkFailed,
}

var importTarget = map[domain.ImportStatus]domain.DownloadState{
	domain.ImportImported: domain.StateImported, domain.ImportPartial: domain.StateImportBlocked,
	domain.ImportBlocked: domain.StateImportBlocked, domain.ImportFailed: domain.StateFailed,
}

// ReportImport records an ImportOutcome: link statuses, then the import's
// transition (imported only when every link is resolved).
func (s *Service) ReportImport(ctx context.Context, o domain.ImportOutcome) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load(ctx, o.DownloadID)
	if err != nil {
		return err
	}
	for _, it := range o.Items {
		for i := range d.Links {
			if d.Links[i].Item == it.Item {
				d.Links[i].Status = linkOutcome[it.Outcome]
			}
		}
	}
	return s.transition(ctx, &d, change{to: importTarget[o.Status], actor: domain.ActorImport, reason: o.ReasonCode})
}

// RemoveOptions are the user's choices when removing from the queue.
type RemoveOptions struct {
	DeleteFiles bool `json:"deleteFiles"`
	Blocklist   bool `json:"blocklist"`
	Research    bool `json:"research"`
	SearchAgain bool `json:"searchAgain,omitempty"` // same as Research
}

func (o RemoveOptions) args() map[string]any {
	return map[string]any{"deleteFiles": o.DeleteFiles, "blocklist": o.Blocklist, "searchAgain": o.Research || o.SearchAgain}
}

// Remove deletes the download from its client and marks it removed; the
// blocklist and re-search choices travel on download.removed to automation.
func (s *Service) Remove(ctx context.Context, id int64, o RemoveOptions) error {
	return s.removeAs(ctx, id, domain.ActorUser, "removed by user", "user", o)
}

func (s *Service) removeAs(ctx context.Context, id int64, actor domain.Actor, reason, cause string, o RemoveOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load(ctx, id)
	if err != nil || d.State == domain.StateRemoved {
		return err
	}
	// shadow/disabled kinds and held downloads get no automatic client writes;
	// the safety action is the one exception (an executable payload is removed
	// whatever the mode or hold); a user's remove in a shadow kind is recorded
	if err := s.writable(ctx, d, actor, o); err != nil {
		return err
	}
	if dl, ok := s.downloader(d.Downloader); ok {
		if err := dl.Remove(ctx, d.DownloadID, o.DeleteFiles); err != nil {
			return err
		}
	}
	return s.transition(ctx, &d, change{to: domain.StateRemoved, actor: actor, reason: reason, extra: func(e *domain.DownloadEventData) {
		e.FilesDeleted, e.Cause, e.Blocklist, e.Research = o.DeleteFiles, cause, o.Blocklist, o.Research || o.SearchAgain
	}})
}

// writable refuses client writes for inactive kinds and, for automation, held
// downloads; the safety action is never refused, a user's write in a shadow
// kind is recorded instead of sent.
func (s *Service) writable(ctx context.Context, d store.StoredDownload, actor domain.Actor, o RemoveOptions) error {
	switch {
	case actor == domain.ActorSafety:
		return nil
	case actor == domain.ActorUser:
		return s.guard(ctx, d, "remove", o.args())
	case actor == domain.ActorAutomation && d.OnHold():
		return fmt.Errorf("%w: %d", ErrHeld, d.ID)
	}
	return s.d.Modes.CheckActive(d.Kind)
}

// Hold sets or clears the hold flag. Releasing a download
// that finished while held hands it to the import at once.
func (s *Service) Hold(ctx context.Context, id int64, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load(ctx, id)
	if err != nil || d.Held == on {
		return err
	}
	d.Held = on
	if err := s.d.Repo.Save(ctx, d); err != nil {
		return err
	}
	if !d.OnHold() && importable[d.State] {
		s.publish(domain.EvDownloadCompleted, "user", s.d.ItemTitles(ctx, refsOf(d.Links)), s.eventData(ctx, &d))
	}
	return nil
}

// importable are the states the import picks up on download.completed.
var importable = map[domain.DownloadState]bool{domain.StateCompleted: true, domain.StateImportPending: true}

// OnPayloadRejected is the safety action: stop and remove
// the download with its files at once — not a rule, never dry-run.
func (s *Service) OnPayloadRejected(ctx context.Context, ev domain.Event) {
	data, ok := ev.Data.(domain.PayloadEventData)
	if !ok {
		return
	}
	err := s.removeAs(ctx, data.DownloadID, domain.ActorSafety, "unsafe payload: "+first(data.Files), "unsafe_payload",
		RemoveOptions{DeleteFiles: true})
	s.warn("safety removal failed", err, "download", data.DownloadID)
}

func first(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return xs[0]
}
