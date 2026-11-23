package automation

import (
	"context"

	"github.com/jrand0m/corsarr/internal/domain"
)

// failure is what a failure event says: which release to blocklist for which
// items, and whether to search again.
type failure struct {
	release  domain.ReleaseIdentity
	source   string
	reason   string
	block    bool
	research bool
}

// failureOf reads a failure event (pure); ok=false when nothing follows.
func failureOf(ev domain.Event) (failure, bool) {
	switch data := ev.Data.(type) {
	case domain.DownloadEventData:
		return downloadFailure(ev.Type, data)
	case domain.GrabEventData:
		// no usable downloader is our configuration, not the release's fault
		ok := ev.Type == domain.EvGrabFailed && data.Downloader != ""
		return failure{release: domain.IdentityOf(data.Release), source: "grab", reason: data.Reason, block: true, research: true}, ok
	case domain.PayloadEventData:
		// blocklist now; the re-search waits for the safety removal (download.removed,
		// cause unsafe_payload) so it never sees the rejected download as queued
		return failure{release: data.Release, source: "payload", reason: "dangerous files: " + first(data.Files), block: true},
			ev.Type == domain.EvPayloadRejected
	}
	return failure{}, false
}

func downloadFailure(t domain.EventType, d domain.DownloadEventData) (failure, bool) {
	switch {
	case t == domain.EvDownloadFailed && d.Held:
		return failure{}, false // held: automation leaves it alone
	case t == domain.EvDownloadFailed:
		return failure{release: d.Release, source: "downloader", reason: d.Reason, block: true, research: true}, true
	case t == domain.EvDownloadRemoved && d.Cause == "unsafe_payload":
		return failure{release: d.Release, research: true}, true // blocklisted on payload.rejected
	case t == domain.EvDownloadRemoved && d.Cause == "user":
		return failure{release: d.Release, source: "user", reason: "removed from the queue", block: d.Blocklist, research: d.Research},
			d.Blocklist || d.Research
	}
	return failure{}, false
}

func first(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return xs[0]
}

// OnFailure is the failure follow-up for download.failed, grab.failed,
// payload.rejected and user removals: blocklist first,
// then re-search each item.
func (s *Service) OnFailure(ctx context.Context, ev domain.Event) {
	f, ok := failureOf(ev)
	if !ok {
		return
	}
	for _, it := range ev.Items {
		if f.block {
			s.warn("blocklist", s.block(ctx, it, f), "item", it.Ref.String())
		}
		if f.research && it.Ref.Kind == domain.KindMovie && s.searchable() == nil {
			s.Request(ctx, it.Ref.ID)
		}
	}
}

func (s *Service) block(ctx context.Context, it domain.EventItem, f failure) error {
	_, err := s.AddBlock(ctx, domain.BlocklistEntry{Item: it.Ref, Release: f.release, Source: f.source, Reason: f.reason}, it)
	return err
}

// AddBlock stores a blocklist entry and publishes blocklist.added once.
func (s *Service) AddBlock(ctx context.Context, e domain.BlocklistEntry, it domain.EventItem) (domain.BlocklistEntry, error) {
	if e.AddedAt.IsZero() {
		e.AddedAt = s.d.Now().UTC()
	}
	e, created, err := s.d.Blocklist.Add(ctx, e)
	if err == nil && created {
		s.publish(domain.EvBlocklistAdded, "automation", []domain.EventItem{it}, domain.BlocklistEventData{Entry: e})
	}
	return e, err
}

// RemoveBlock deletes an entry and publishes blocklist.removed.
func (s *Service) RemoveBlock(ctx context.Context, id int64) (domain.BlocklistEntry, bool, error) {
	e, ok, err := s.d.Blocklist.Remove(ctx, id)
	if err == nil && ok {
		s.publish(domain.EvBlocklistRemoved, "user", []domain.EventItem{{Ref: e.Item}}, domain.BlocklistEventData{Entry: e})
	}
	return e, ok, err
}

// OnEvent keeps the wanted index current and runs the searches on add and on
// availability.
func (s *Service) OnEvent(ctx context.Context, ev domain.Event) {
	for _, it := range ev.Items {
		if it.Ref.Kind == domain.KindMovie {
			s.refresh(ctx, it.Ref.ID)
		}
	}
	if !searchTrigger(ev) || s.searchable() != nil {
		return
	}
	for _, it := range ev.Items {
		if it.Ref.Kind == domain.KindMovie {
			s.Request(ctx, it.Ref.ID)
		}
	}
}

func searchTrigger(ev domain.Event) bool {
	data, _ := ev.Data.(domain.ItemEventData)
	return ev.Type == domain.EvItemAvailable || (ev.Type == domain.EvItemAdded && data.SearchNow)
}
