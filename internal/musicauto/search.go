package musicauto

import (
	"context"
	"errors"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/music"
	"github.com/jrand0m/corsarr/internal/parse"
	"github.com/jrand0m/corsarr/internal/queue"
)

// Summary is the result of one or more album searches.
type Summary struct {
	Searches int `json:"searches"`
	automation.Summary
}

func (s *Summary) add(o automation.Summary) {
	s.Searches++
	s.Indexers += o.Indexers
	s.Failed += o.Failed
	s.Releases += o.Releases
	s.Accepted += o.Accepted
	if s.Grabbed == "" {
		s.Grabbed = o.Grabbed
	}
	if s.Shadowed == "" {
		s.Shadowed = o.Shadowed
	}
}

// scope is one album with its artist and what is on disk for it.
type scope struct {
	artist domain.Artist
	album  domain.Album
	cur    *decide.CurrentFile
}

func (s *Service) scopeOf(ctx context.Context, albumID int64) (scope, error) {
	al, err := s.d.Music.Album(ctx, albumID)
	if err != nil {
		return scope{}, err
	}
	ar, err := s.d.Music.Artist(ctx, al.ArtistID)
	if err != nil {
		return scope{}, err
	}
	e := s.entry(ctx, ar, al)
	return scope{artist: ar, album: al, cur: e.Cur}, nil
}

func (s *Service) context(ctx context.Context, sc scope, p decide.QualityProfile, rel domain.Release) decide.Context {
	c := decide.Context{Now: s.d.Now(), Payload: s.d.Payload}
	ref := sc.album.Ref()
	var err1, err2, err3 error
	c.Blocklisted, err1 = s.d.Blocklist.Has(ctx, ref, domain.IdentityOf(rel))
	c.GrabbedBefore, err2 = s.d.Queue.GrabbedBefore(ctx, ref, rel)
	c.QueuedEqualOrBetter, err3 = s.queuedEqualOrBetter(ctx, ref, p, rel)
	s.warn("decision lookups", errors.Join(err1, err2, err3), "album", sc.album.ID)
	return c
}

func (s *Service) queuedEqualOrBetter(ctx context.Context, ref domain.ItemRef, p decide.QualityProfile, rel domain.Release) (bool, error) {
	queued, err := s.d.Queue.QueuedQualities(ctx, ref)
	if err != nil || len(queued) == 0 {
		return false, err
	}
	cand, _ := decide.AudioQualityOf(parse.ParseMusic(rel.Title))
	candRank, _ := p.Rank(cand.ID)
	for _, name := range queued {
		if rank, ok := automation.RankByName(p, name); !ok || rank >= candRank {
			return true, nil
		}
	}
	return false, nil
}

// decideOne decides a release for the album of the scope.
func (s *Service) decideOne(ctx context.Context, sc scope, rel domain.Release) automation.Decided {
	p, _ := music.ProfileByKey(sc.artist.ProfileKey)
	t := music.Target{Artist: sc.artist, Album: sc.album, Current: sc.cur, Context: s.context(ctx, sc, p, rel),
		Released: music.IsReleased(sc.album, s.d.Now())}
	return automation.NewDecided(rel, music.Decide(t, p, rel))
}

// collect queries every indexer, dedupes, decides each release and streams it.
func (s *Service) collect(ctx context.Context, sc scope, req domain.SearchRequest, em automation.Emitter) ([]automation.Decided, automation.Summary) {
	var out []automation.Decided
	var sum automation.Summary
	seen := map[string]bool{}
	for _, ix := range s.d.Indexers() {
		sum.Indexers++
		rels, err := ix.Impl.Search(ctx, req)
		if err != nil {
			sum.Failed++
			_ = em.Error(string(ix.Key), err)
			continue
		}
		for _, rel := range rels {
			if key := automation.IdentityKey(rel); !seen[key] {
				seen[key] = true
				_ = s.recent.Put(ctx, rel)
				d := s.decideOne(ctx, sc, rel)
				_ = em.Record(d)
				out = append(out, d)
			}
		}
	}
	sum.Releases = len(out)
	return out, sum
}

// run performs one automatic album search and grabs the best accepted release.
func (s *Service) run(ctx context.Context, sc scope, origin domain.SearchOrigin) automation.Summary {
	req := music.AlbumRequest(sc.artist, sc.album, origin)
	ds, sum := s.collect(ctx, sc, req, automation.NopEmitter)
	if top, n, ok := automation.Best(ds); ok {
		sum.Accepted = n
		switch _, err := s.grab(ctx, sc, top, origin, false, ""); {
		case err == nil:
			sum.Grabbed = top.Release.Title
		case errors.Is(err, automation.ErrShadowed):
			sum.Shadowed = top.Release.Title
		default:
			s.warn("grab", err, "release", top.Release.Title)
		}
	}
	res := domain.SearchEventData{Origin: origin, Indexers: sum.Indexers, Failed: sum.Failed, Releases: sum.Releases,
		Accepted: sum.Accepted, Grabbed: sum.Grabbed, Rejections: automation.Rejections(ds)}
	if s.d.Searches != nil {
		err := automation.RecordSearch(ctx, s.d.Searches, sc.album.Ref(), s.d.Now().UTC(), res)
		s.warn("save search state", err, "album", sc.album.ID)
	}
	s.publish(domain.EvSearchCompleted, "automation", []domain.EventItem{sc.album.EventItem(sc.artist.Name)}, res)
	return sum
}

// grab sends a decided release to the queue linked to the album; in shadow
// mode it records decision.shadowed instead.
func (s *Service) grab(ctx context.Context, sc scope, d automation.Decided, origin domain.SearchOrigin, override bool, to domain.ObjectKey) (domain.Grab, error) {
	items := []domain.EventItem{sc.album.EventItem(sc.artist.Name)}
	switch mode := s.d.Modes.Of(domain.KindMusic); mode {
	case domain.ModeShadow:
		s.publish(domain.EvDecisionShadowed, automation.ActorOf(origin), items, domain.GrabEventData{Release: d.Release,
			Decision: d.Decision, Origin: origin, Override: override, Downloader: to})
		return domain.Grab{}, automation.ErrShadowed
	case domain.ModeDisabled:
		return domain.Grab{}, domain.ModeError{Kind: domain.KindMusic, Mode: mode}
	}
	g, err := s.d.Queue.Grab(ctx, queue.GrabRequest{Kind: domain.KindMusic, Items: items, Release: d.Release,
		Decision: d.Decision, Origin: origin, Override: override, Downloader: to})
	if err == nil {
		s.refreshAlbum(ctx, sc.album.ID)
	}
	return g, err
}

// SearchAlbum searches one album and grabs the best release.
func (s *Service) SearchAlbum(ctx context.Context, id int64, origin domain.SearchOrigin) (Summary, error) {
	if err := s.searchable(); err != nil {
		return Summary{}, err
	}
	sc, err := s.scopeOf(ctx, id)
	if err != nil {
		return Summary{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var sum Summary
	sum.add(s.run(ctx, sc, origin))
	return sum, nil
}

// SearchArtist searches every wanted album of an artist, one by one.
func (s *Service) SearchArtist(ctx context.Context, artistID int64, origin domain.SearchOrigin) (Summary, error) {
	if err := s.searchable(); err != nil {
		return Summary{}, err
	}
	if _, err := s.d.Music.Artist(ctx, artistID); err != nil {
		return Summary{}, err
	}
	var sum Summary
	for _, id := range s.index.OfArtist(artistID) {
		one, err := s.SearchAlbum(ctx, id, origin)
		s.warn("album search", err, "album", id)
		sum.add(one.Summary)
	}
	return sum, nil
}

// Interactive streams every decision for one album.
func (s *Service) Interactive(ctx context.Context, albumID int64, em automation.Emitter) (automation.Summary, error) {
	if err := s.searchable(); err != nil {
		return automation.Summary{}, err
	}
	sc, err := s.scopeOf(ctx, albumID)
	if err != nil {
		return automation.Summary{}, err
	}
	ds, sum := s.collect(ctx, sc, music.AlbumRequest(sc.artist, sc.album, domain.OriginInteractive), em)
	return automation.Explain(sum, ds, s.d.Modes.Of(domain.KindMusic)), nil
}

// GrabRelease grabs a release from a recent search by hand for an album:
// the release is decided again; accepted and "allowed" overrides go
// through, "confirm" needs confirm, "never" is refused.
func (s *Service) GrabRelease(ctx context.Context, albumID int64, releaseID string, confirm bool) (domain.Grab, automation.Decided, error) {
	return s.GrabReleaseTo(ctx, albumID, releaseID, confirm, "")
}

// GrabReleaseTo is GrabRelease sent to one download client; an empty key
// picks the first usable client for the release's protocol.
func (s *Service) GrabReleaseTo(ctx context.Context, albumID int64, releaseID string, confirm bool, to domain.ObjectKey) (domain.Grab, automation.Decided, error) {
	sc, err := s.scopeOf(ctx, albumID)
	if err != nil {
		return domain.Grab{}, automation.Decided{}, err
	}
	rel, ok := s.recent.Get(ctx, releaseID)
	if !ok {
		return domain.Grab{}, automation.Decided{}, automation.ErrUnknownRelease
	}
	d := s.decideOne(ctx, sc, rel)
	if err := automation.OverrideAllowed(d.Decision, confirm); err != nil {
		return domain.Grab{}, d, err
	}
	g, err := s.grab(ctx, sc, d, domain.OriginInteractive, !d.Decision.Accepted, to)
	return g, d, err
}
