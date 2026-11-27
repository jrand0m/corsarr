package queue

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

// Errors of downloader control the API maps to statuses.
var (
	// ErrShadowed reports a downloader write recorded as downloader.shadowed
	// instead of sent (the media kind is in shadow mode).
	ErrShadowed = errors.New("shadow mode: the action was recorded, not sent")
	// ErrUnknownClient: no configured downloader has this key.
	ErrUnknownClient = errors.New("no downloader with this key")
	// ErrNotImportable: the download is not finished, or already imported.
	ErrNotImportable = errors.New("only a finished download waiting for its import, held back from it, or whose import failed while its files are still there can be imported again")
)

// unsupported names the capability a client lacks.
func unsupported(what string) error { return fmt.Errorf("%s: %w", what, plugin.ErrUnsupported) }

// clientOp is one write to a download's client.
type clientOp struct {
	name string
	args map[string]any
	run  func(dl plugin.Downloader, clientID string) error
}

// guard decides what happens to a user's write for a download: sent for an
// active kind, recorded as downloader.shadowed for a shadow kind, refused for
// a disabled one.
func (s *Service) guard(ctx context.Context, d store.StoredDownload, action string, args map[string]any) error {
	switch mode := s.d.Modes.Of(d.Kind); mode {
	case domain.ModeShadow:
		s.publish(domain.EvClientShadowed, "user", s.d.ItemTitles(ctx, refsOf(d.Links)), domain.ClientActionData{
			Downloader: d.Downloader, DownloadID: d.ID, ClientID: d.DownloadID, Title: d.Title, Action: action, Args: args})
		return ErrShadowed
	case domain.ModeDisabled:
		return domain.ModeError{Kind: d.Kind, Mode: mode}
	}
	return nil
}

// client runs one write against a download's client: the client must be
// running and able to do it before the kind's mode is applied.
func (s *Service) client(ctx context.Context, id int64, op clientOp, can func(plugin.Downloader) bool) error {
	d, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	dl, ok := s.downloader(d.Downloader)
	if !ok {
		return ErrNoDownloader
	}
	if can != nil && !can(dl) {
		return unsupported(op.name)
	}
	if err := s.guard(ctx, d, op.name, op.args); err != nil {
		return err
	}
	if err := op.run(dl, d.DownloadID); err != nil {
		return err
	}
	s.Wake()
	return nil
}

// Pause stops a download in its client.
func (s *Service) Pause(ctx context.Context, id int64) error {
	return s.client(ctx, id, clientOp{name: "pause", run: func(c plugin.Downloader, cid string) error { return c.Pause(ctx, cid) }}, nil)
}

// Resume restarts a paused download.
func (s *Service) Resume(ctx context.Context, id int64) error {
	return s.client(ctx, id, clientOp{name: "resume", run: func(c plugin.Downloader, cid string) error { return c.Resume(ctx, cid) }}, nil)
}

// SetPriority moves a download in its client's queue (top, up, down,
// bottom) or sets its priority level (low, normal, high, force) where the
// client has levels.
func (s *Service) SetPriority(ctx context.Context, id int64, p plugin.Priority) error {
	if !p.IsMove() && !p.IsLevel() {
		return plugin.Errorf(plugin.ClassConfig, "unknown priority %q: use top, up, down, bottom, low, normal, high or force", p)
	}
	op := clientOp{name: "priority", args: map[string]any{"priority": p},
		run: func(c plugin.Downloader, cid string) error { return c.SetPriority(ctx, cid, p) }}
	if !p.IsLevel() {
		return s.client(ctx, id, op, nil)
	}
	op.run = func(c plugin.Downloader, cid string) error {
		return c.(plugin.PriorityLeveler).SetPriorityLevel(ctx, cid, p)
	}
	return s.client(ctx, id, op, is[plugin.PriorityLeveler])
}

// Recheck verifies a download's data again.
func (s *Service) Recheck(ctx context.Context, id int64) error {
	return s.client(ctx, id, clientOp{name: "recheck", run: func(c plugin.Downloader, cid string) error {
		return c.(plugin.Rechecker).Recheck(ctx, cid)
	}}, is[plugin.Rechecker])
}

// Reannounce asks the client to contact the download's trackers now.
func (s *Service) Reannounce(ctx context.Context, id int64) error {
	return s.client(ctx, id, clientOp{name: "reannounce", run: func(c plugin.Downloader, cid string) error {
		return c.(plugin.Reannouncer).Reannounce(ctx, cid)
	}}, is[plugin.Reannouncer])
}

// SetCategory moves a download to another client category; the next poll
// records it (the hold category holds it).
func (s *Service) SetCategory(ctx context.Context, id int64, category string) error {
	return s.client(ctx, id, clientOp{name: "category", args: map[string]any{"category": category},
		run: func(c plugin.Downloader, cid string) error {
			return c.(plugin.Categoriser).SetCategory(ctx, cid, category)
		}},
		is[plugin.Categoriser])
}

func is[T any](dl plugin.Downloader) bool { _, ok := dl.(T); return ok }

// setCategory is SetCategory for an API body whose category is required.
func (s *Service) setCategory(ctx context.Context, id int64, category *string) error {
	if category == nil {
		return plugin.Errorf(plugin.ClassConfig, "category is required")
	}
	return s.SetCategory(ctx, id, *category)
}

// RetryImport hands a finished download back to the import: one that waits
// for it, one the import held back, or one that failed after it finished
// while its payload is still on disk (it moves back to import-pending). A held
// download stays held.
func (s *Service) RetryImport(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	retryFailed := d.State == domain.StateFailed && s.payloadKept(d)
	if !importable[d.State] && d.State != domain.StateImportBlocked && !retryFailed {
		return fmt.Errorf("%w (download %d is %s)", ErrNotImportable, id, d.State)
	}
	if d.OnHold() {
		return fmt.Errorf("%w: %d", ErrHeld, id)
	}
	if err := s.guard(ctx, d, "retry-import", nil); err != nil {
		return err
	}
	if retryFailed {
		if err := s.transition(ctx, &d, change{to: domain.StateImportPending, actor: domain.ActorUser, reason: "import retried by user"}); err != nil {
			return err
		}
	}
	s.publish(domain.EvDownloadCompleted, "user", s.d.ItemTitles(ctx, refsOf(d.Links)), s.eventData(ctx, &d))
	return nil
}

// payloadKept reports whether a failed download had finished and its payload
// is still on disk, so its import can run again.
func (s *Service) payloadKept(d store.StoredDownload) bool {
	if d.CompletedAt == nil || d.ContentPath == "" {
		return false
	}
	if s.d.PayloadExists != nil {
		return s.d.PayloadExists(d.ContentPath)
	}
	_, err := os.Stat(d.ContentPath)
	return err == nil
}
