package queue

import (
	"context"
	"fmt"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

// GrabRequest asks the queue to send one decided release to a downloader.
type GrabRequest struct {
	Kind     domain.MediaKind
	Items    []domain.EventItem // the N items the release covers
	Release  domain.Release
	Decision domain.Decision
	Origin   domain.SearchOrigin
	Override bool // grabbed by hand despite a rejection
	// Downloader sends it to this client instead of the first usable one
	// for the release's protocol.
	Downloader domain.ObjectKey

	payload *plugin.Payload // given by hand (AddURL); else resolved through the indexer
}

// Grab resolves the payload through the release's indexer, adds it to a
// downloader with the kind's category, then persists one Grab with one
// Download and N links and publishes grab.sent. Any failure publishes
// grab.failed so automation can blocklist and re-search.
func (s *Service) Grab(ctx context.Context, req GrabRequest) (domain.Grab, error) {
	dl, clientID, cat, err := s.send(ctx, req)
	if err != nil {
		s.publish(domain.EvGrabFailed, actorOf(req.Origin), req.Items, domain.GrabEventData{Release: req.Release,
			Decision: req.Decision, Origin: req.Origin, Downloader: dl.Key, Override: req.Override, Reason: err.Error()})
		return domain.Grab{}, err
	}
	g, d := s.records(req, dl.Key, clientID, cat)
	s.mu.Lock()
	g, sd, err := s.d.Repo.CreateGrab(ctx, g, d)
	s.mu.Unlock()
	if err != nil {
		return g, err
	}
	s.publish(domain.EvGrabSent, actorOf(req.Origin), req.Items, domain.GrabEventData{GrabID: g.ID, DownloadID: sd.ID,
		Release: req.Release, Decision: req.Decision, Origin: req.Origin, Downloader: dl.Key, ClientID: clientID, Override: req.Override})
	s.Wake()
	return g, nil
}

func actorOf(o domain.SearchOrigin) string {
	if o == domain.OriginInteractive {
		return "user"
	}
	return "automation"
}

// send picks a downloader, resolves the payload and adds it.
func (s *Service) send(ctx context.Context, req GrabRequest) (Downloader, string, string, error) {
	dl, ok := s.pickFor(req.Release, req.Downloader)
	if !ok {
		return dl, "", "", noDownloader(req.Downloader)
	}
	payload, err := s.payloadOf(ctx, req)
	if err != nil {
		return dl, "", "", fmt.Errorf("payload: %w", err)
	}
	cat := categoryFor(dl.Impl, req.Kind)
	s.ensureCategory(ctx, dl, cat)
	id, err := dl.Impl.Add(ctx, plugin.AddRequest{Payload: payload, Category: cat})
	if err != nil {
		return dl, "", "", fmt.Errorf("downloader rejected the release: %w", err)
	}
	return dl, lower(id), cat, nil
}

// pickFor is the chosen client when it takes the release, else (with no
// choice) the first usable one.
func (s *Service) pickFor(rel domain.Release, key domain.ObjectKey) (Downloader, bool) {
	if key == "" {
		return s.pick(rel)
	}
	for _, d := range s.d.Plugins.Downloaders() {
		if d.Key == key && handles(d.Impl, rel) {
			return d, true
		}
	}
	return Downloader{Key: key}, false
}

func (s *Service) payloadOf(ctx context.Context, req GrabRequest) (plugin.Payload, error) {
	if req.payload != nil {
		return *req.payload, nil
	}
	return s.resolve(ctx, req.Release)
}

func noDownloader(key domain.ObjectKey) error {
	if key == "" {
		return ErrNoDownloader
	}
	return fmt.Errorf("%w: %s is not running or does not take this release", ErrNoDownloader, key)
}

func (s *Service) resolve(ctx context.Context, rel domain.Release) (plugin.Payload, error) {
	if len(rel.Indexers) == 0 {
		return plugin.Payload{}, fmt.Errorf("release %q names no indexer", rel.Title)
	}
	ix, ok := s.d.Plugins.Indexer(rel.Indexers[0].Key)
	if !ok {
		return plugin.Payload{}, fmt.Errorf("indexer %s is not running", rel.Indexers[0].Key)
	}
	return ix.Resolve(ctx, rel)
}

// ensureCategory creates the category once per downloader per process; a
// failure is logged (most clients create a missing category on add).
func (s *Service) ensureCategory(ctx context.Context, dl Downloader, cat string) {
	key := string(dl.Key) + "/" + cat
	s.pollMu.Lock()
	done := s.cats[key]
	s.cats[key] = true
	s.pollMu.Unlock()
	if !done {
		s.warn("ensure downloader category", dl.Impl.EnsureCategory(ctx, cat, ""), "downloader", dl.Key, "category", cat)
	}
}

func (s *Service) records(req GrabRequest, dl domain.ObjectKey, clientID, cat string) (domain.Grab, domain.Download) {
	now := s.d.Now().UTC()
	refs := make([]domain.ItemRef, len(req.Items))
	links := make([]domain.DownloadLink, len(req.Items))
	for i, it := range req.Items {
		refs[i], links[i] = it.Ref, domain.DownloadLink{Item: it.Ref, Status: domain.LinkPending}
	}
	g := domain.Grab{Kind: req.Kind, ItemRefs: refs, Release: req.Release, Decision: req.Decision, Origin: req.Origin,
		Downloader: dl, DownloadID: clientID, SentAt: now}
	d := domain.Download{Downloader: dl, DownloadID: clientID, Protocol: req.Release.Protocol, Kind: req.Kind,
		Title: req.Release.Title, State: domain.StateQueued, Links: links, Size: req.Release.Size, Category: cat, AddedAt: now}
	return g, d
}

// GrabbedBefore is the History lookup "grabbed before" for the decision engine.
func (s *Service) GrabbedBefore(ctx context.Context, item domain.ItemRef, rel domain.Release) (bool, error) {
	return s.d.Repo.GrabbedBefore(ctx, item, domain.IdentityOf(rel))
}

// QueuedQualities lists the qualities of live downloads for an item;
// automation compares them to a candidate for "equal or better already queued".
func (s *Service) QueuedQualities(ctx context.Context, item domain.ItemRef) ([]string, error) {
	return s.d.Repo.QueuedQualities(ctx, item)
}

// GrabOf returns the grab behind a download.
func (s *Service) GrabOf(ctx context.Context, d store.StoredDownload) (domain.Grab, bool, error) {
	return s.d.Repo.Grab(ctx, d.GrabID)
}
