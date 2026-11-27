package queue

import (
	"context"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

// activeStates are polled for client snapshots; later states belong to import.
var activeStates = []domain.DownloadState{domain.StateQueued, domain.StateDownloading, domain.StateStalled}

// sweepStates are reconciled against the client on the long sweep.
var sweepStates = append(append([]domain.DownloadState{}, activeStates...), domain.StateCompleted,
	domain.StateImportPending, domain.StateImportBlocked, domain.StateFailed)

// waitingStates are finished downloads the import has not taken yet.
var waitingStates = []domain.DownloadState{domain.StateCompleted, domain.StateImportPending}

// Polling back-off for idle downloads.
const (
	// IdleGrace is how many idle polls run at the normal cadence before the
	// interval starts doubling.
	IdleGrace = 3
	// MaxIdlePoll caps the back-off.
	MaxIdlePoll = 15 * time.Minute
	// ReannounceAfter is how long a finished download may wait for its import
	// before a poll hands it to the import again (a missed event).
	ReannounceAfter = time.Minute
)

// Load is what the polling cadence depends on.
type Load struct {
	Active   int // queued, downloading or stalled downloads
	Waiting  int // finished downloads waiting for the import
	UI       int // connected UI clients
	IdleRuns int // consecutive polls in which nothing moved
}

// Interval is the adaptive polling rule: off when nothing is active or
// waiting; fast while a UI is connected, slow without; finished downloads
// waiting for the import keep that base cadence so they are reconciled
// promptly; otherwise, after IdleGrace polls in which nothing moved, the
// interval doubles per idle poll up to MaxIdlePoll and drops back on activity.
func Interval(l Load, fast, slow time.Duration) (time.Duration, bool) {
	if l.Active+l.Waiting == 0 {
		return 0, false
	}
	base := slow
	if l.UI > 0 {
		base = fast
	}
	if l.Waiting > 0 || l.IdleRuns <= IdleGrace {
		return base, true
	}
	steps := min(l.IdleRuns-IdleGrace, 20)
	return max(min(base<<steps, MaxIdlePoll), base), true
}

// Poll reads one batched snapshot per downloader for the active downloads
// and applies it, hands finished downloads that missed their import back to
// it, then adapts the poll interval.
func (s *Service) Poll(ctx context.Context) error {
	moved, err := s.reconcile(ctx, activeStates)
	s.pollMu.Lock()
	if moved {
		s.idleRuns = 0
	} else {
		s.idleRuns++
	}
	s.pollMu.Unlock()
	s.reannounce(ctx)
	s.Adjust(ctx)
	return err
}

// Sweep reconciles every unfinished download, catching external removals.
func (s *Service) Sweep(ctx context.Context) error {
	_, err := s.reconcile(ctx, sweepStates)
	s.Adjust(ctx)
	return err
}

// reconcile applies client snapshots; it reports whether any download moved
// (progress or state).
func (s *Service) reconcile(ctx context.Context, states []domain.DownloadState) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.d.Repo.Downloads(ctx, states...)
	if err != nil {
		return false, err
	}
	moved := false
	for key, group := range byDownloader(list) {
		dl, ok := s.downloader(key)
		if !ok {
			continue
		}
		snaps, err := dl.List(ctx, categories(group))
		if err != nil {
			s.warn("downloader list failed", err, "downloader", key)
			continue
		}
		moved = s.applyAll(ctx, dl, group, index(snaps)) || moved
	}
	return moved, nil
}

// waiting lists finished downloads the import should take: not held and of
// an active media kind.
func (s *Service) waiting(ctx context.Context) []store.StoredDownload {
	list, err := s.d.Repo.Downloads(ctx, waitingStates...)
	s.warn("list waiting imports", err)
	var out []store.StoredDownload
	for _, d := range list {
		if !d.OnHold() && s.d.Modes.Active(d.Kind) {
			out = append(out, d)
		}
	}
	return out
}

// reannounce publishes download.completed again for finished downloads that
// waited longer than ReannounceAfter, so a missed event costs one poll, not a
// sweep. The import is idempotent per download.
func (s *Service) reannounce(ctx context.Context) {
	for _, d := range s.waiting(ctx) {
		if d.CompletedAt != nil && s.d.Now().Sub(*d.CompletedAt) >= ReannounceAfter {
			s.publish(domain.EvDownloadCompleted, "system", s.d.ItemTitles(ctx, refsOf(d.Links)), s.eventData(ctx, &d))
		}
	}
}

func byDownloader(list []store.StoredDownload) map[domain.ObjectKey][]store.StoredDownload {
	out := map[domain.ObjectKey][]store.StoredDownload{}
	for _, d := range list {
		out[d.Downloader] = append(out[d.Downloader], d)
	}
	return out
}

// categories are the categories to list: the downloads' own plus the hold
// category, so a download the user parked in hold is still seen.
func categories(list []store.StoredDownload) []string {
	seen := map[string]bool{domain.HoldCategory: true}
	out := []string{domain.HoldCategory}
	for _, d := range list {
		if !seen[d.Category] {
			seen[d.Category] = true
			out = append(out, d.Category)
		}
	}
	return out
}

func index(snaps []plugin.Snapshot) map[string]plugin.Snapshot {
	out := make(map[string]plugin.Snapshot, len(snaps))
	for _, sn := range snaps {
		out[lower(sn.DownloadID)] = sn
	}
	return out
}

func (s *Service) applyAll(ctx context.Context, dl plugin.Downloader, group []store.StoredDownload, snaps map[string]plugin.Snapshot) bool {
	moved := false
	for i := range group {
		d := &group[i]
		snap, ok := snaps[lower(d.DownloadID)]
		if !ok {
			s.missing(ctx, d)
			continue
		}
		state, progress, done := d.State, d.Progress, d.Downloaded
		s.warn("apply snapshot", s.apply(ctx, dl, d, snap), "download", d.ID)
		moved = moved || d.State != state || d.Progress != progress || d.Downloaded != done
	}
	return moved
}

// missing marks a download the client no longer lists as removed externally,
// after a grace period for clients that list new torrents late.
func (s *Service) missing(ctx context.Context, d *store.StoredDownload) {
	if s.d.Now().Sub(d.AddedAt) < MissingGrace {
		return
	}
	_ = s.transition(ctx, d, change{to: domain.StateRemoved, actor: domain.ActorDownloader, reason: "removed in the download client",
		extra: func(e *domain.DownloadEventData) { e.Cause = "external" }})
}

// apply copies the snapshot, runs the payload check once metadata is known,
// then moves the state when the client reports a different one.
func (s *Service) apply(ctx context.Context, dl plugin.Downloader, d *store.StoredDownload, snap plugin.Snapshot) error {
	moved := copySnapshot(d, snap)
	if rejected, err := s.checkPayload(ctx, dl, d); rejected || err != nil {
		return err
	}
	if snap.State == d.State {
		d.StatusReason = snap.Error // the client's warning follows it while the state holds
	}
	if snap.State == d.State || domain.CheckTransition(d.State, snap.State, domain.ActorDownloader) != nil {
		if moved {
			s.progress(ctx, d)
		}
		return s.d.Repo.Save(ctx, *d)
	}
	return s.transition(ctx, d, change{to: snap.State, actor: domain.ActorDownloader, reason: snap.Error,
		extra: func(e *domain.DownloadEventData) { e.Cause = causeOf(snap.State) }})
}

func causeOf(st domain.DownloadState) string {
	if st == domain.StateFailed {
		return "downloader"
	}
	return ""
}

// copySnapshot updates the progress fields; it reports whether progress moved.
func copySnapshot(d *store.StoredDownload, sn plugin.Snapshot) bool {
	moved := sn.Progress != d.Progress || sn.Downloaded != d.Downloaded
	d.Progress, d.Downloaded, d.Speed, d.ETASeconds = sn.Progress, sn.Downloaded, sn.Speed, sn.ETASeconds
	d.Seeders, d.Peers, d.SavePath, d.LastActivityAt = sn.Seeders, sn.Peers, sn.SavePath, sn.LastActivityAt
	d.UpSpeed, d.Ratio = sn.UpSpeed, sn.Ratio
	if sn.Size > 0 {
		d.Size = sn.Size
	}
	if sn.ContentPath != "" {
		d.ContentPath = sn.ContentPath
	}
	if sn.Category != "" {
		d.Category = sn.Category // moving it into the hold category holds it
	}
	return moved
}

func (s *Service) progress(ctx context.Context, d *store.StoredDownload) {
	s.publish(domain.EvDownloadProgress, "plugin:downloader", s.d.ItemTitles(ctx, refsOf(d.Links)), domain.DownloadEventData{
		DownloadID: d.ID, GrabID: d.GrabID, Downloader: d.Downloader, ClientID: d.DownloadID, Title: d.Title,
		Progress: d.Progress, Speed: d.Speed, ETASeconds: d.ETASeconds})
}

// checkPayload asks the decision engine for the payload verdict once the file list is known
// and publishes payload.rejected (pre-download) when it is
// dangerous; the safety action then removes the download.
func (s *Service) checkPayload(ctx context.Context, dl plugin.Downloader, d *store.StoredDownload) (bool, error) {
	lister, ok := dl.(plugin.FileLister)
	if !ok {
		return false, nil
	}
	if done, err := s.d.Repo.PayloadChecked(ctx, d.ID); done || err != nil {
		return false, err
	}
	files, err := lister.Files(ctx, d.DownloadID)
	if err != nil || len(files) == 0 {
		return false, err // metadata not known yet: check on a later poll
	}
	if err := s.d.Repo.SetPayloadChecked(ctx, d.ID); err != nil {
		return false, err
	}
	v := s.d.Payload.Verdict(payloadFiles(files), d.Kind == domain.KindMovie || d.Kind == domain.KindTV)
	if v.Status != decide.PayloadDangerous {
		return false, nil
	}
	data := s.eventData(ctx, d)
	s.publish(domain.EvPayloadRejected, "automation", s.d.ItemTitles(ctx, refsOf(d.Links)), domain.PayloadEventData{
		DownloadID: d.ID, Stage: domain.StagePreDownload, Verdict: string(v.Status), Files: v.Files, Release: data.Release})
	return true, s.d.Repo.Save(ctx, *d)
}

func payloadFiles(files []plugin.FileEntry) []decide.PayloadFile {
	out := make([]decide.PayloadFile, len(files))
	for i, f := range files {
		out[i] = decide.PayloadFile{Path: f.Path, Size: f.Size}
	}
	return out
}

// Adjust sets the poll job's cadence from the load: active and waiting
// downloads, UI presence and the idle streak.
func (s *Service) Adjust(ctx context.Context) {
	list, err := s.d.Repo.Downloads(ctx, activeStates...)
	if err != nil {
		return
	}
	waiting := len(s.waiting(ctx))
	s.pollMu.Lock()
	defer s.pollMu.Unlock()
	d, on := Interval(Load{Active: len(list), Waiting: waiting, UI: s.d.UIClients(), IdleRuns: s.idleRuns}, s.d.FastPoll, s.d.SlowPoll)
	if on && d != s.interval {
		s.warn("set poll interval", s.d.Jobs.SetInterval(PollJob, d))
		s.interval = d
	}
	if on != s.enabled {
		s.warn("toggle polling", s.d.Jobs.SetEnabled(PollJob, on))
		s.enabled = on
	}
}

// Wake resumes polling at once at the normal cadence (grab, Queue opened,
// user action).
func (s *Service) Wake() {
	s.pollMu.Lock()
	s.idleRuns = 0
	if !s.enabled {
		s.warn("enable polling", s.d.Jobs.SetEnabled(PollJob, true))
		s.enabled = true
	}
	s.pollMu.Unlock()
	s.warn("run poll", s.d.Jobs.RunNow(PollJob))
}
