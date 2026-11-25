package music

import (
	"context"
	"fmt"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// editionMargin is how much lower another edition's misfit must be before
// an album switches to it: half a file or track.
const editionMargin = 0.5

// misfit is how badly a payload fits an edition: one for each file left
// unmatched and each track left missing, plus the distance of every
// placement, less one for each file whose tags name that edition.
func misfit(files []AudioFile, mbid string, tracks []domain.Track) float64 {
	m := MatchFiles(files, tracks)
	cost := float64(len(m.Unmatched) + len(m.Missing))
	for _, p := range m.Placements {
		cost += p.Distance
	}
	return cost - float64(taggedFor(files, mbid))
}

func taggedFor(files []AudioFile, mbid string) int {
	n := 0
	for _, f := range files {
		if mbid != "" && strings.EqualFold(f.Tags.MusicBrainz.Album, mbid) {
			n++
		}
	}
	return n
}

// BetterEdition scores a payload against every audio edition of an album
// and returns the one it fits clearly better than the album's current
// tracks (pure); ok=false when the current edition fits as well.
func BetterEdition(files []AudioFile, al domain.Album, current []domain.Track, rels []plugin.ReleaseRecord) (plugin.ReleaseRecord, bool) {
	limit := misfit(files, al.EditionMBID, current) - editionMargin
	var best plugin.ReleaseRecord
	found := false
	for _, r := range audioEditions(rels) {
		if r.MBID == al.EditionMBID || r.TrackCount() == 0 {
			continue
		}
		_, tracks := Edition(al, r)
		if cost := misfit(files, r.MBID, tracks); cost <= limit {
			best, limit, found = r, cost, true
		}
	}
	return best, found
}

// EditionFit is the edition a payload is imported against.
type EditionFit struct {
	Album    domain.Album
	Tracks   []domain.Track
	Switched string // says which edition the album moved to; "" when it kept its own
}

// FitEdition returns the album and tracks to import a payload against. It
// scores the payload against every edition of the album and switches the
// album to another one when that fits clearly better, recording the change
// as item.updated. An edition chosen by hand, or one whose tracks already
// have files, is kept; so is the current one when the editions cannot be
// fetched.
func (s *Service) FitEdition(ctx context.Context, albumID int64, files []AudioFile) (EditionFit, error) {
	al, tracks, err := s.EnsureEdition(ctx, albumID)
	fit := EditionFit{Album: al, Tracks: tracks}
	if err != nil || !mayRefit(al, tracks) {
		return fit, err
	}
	md, err := s.metadata()
	if err != nil {
		return fit, nil
	}
	rels, err := md.Releases(ctx, al.MBID)
	if err != nil {
		s.logErr("fetch editions", err)
		return fit, nil
	}
	if r, ok := BetterEdition(files, al, tracks, rels); ok {
		return s.switchEdition(ctx, al, r)
	}
	return fit, nil
}

// mayRefit reports whether an import may move the album to another edition.
func mayRefit(al domain.Album, tracks []domain.Track) bool {
	if al.EditionLocked || len(tracks) == 0 {
		return false
	}
	for _, t := range tracks {
		if t.FileID != 0 {
			return false
		}
	}
	return true
}

func (s *Service) switchEdition(ctx context.Context, al domain.Album, r plugin.ReleaseRecord) (EditionFit, error) {
	from := al.EditionMBID
	next, tracks := Edition(al, r)
	if err := s.d.Repo.SetEdition(ctx, next, tracks); err != nil {
		return EditionFit{}, err
	}
	stored, err := s.d.Repo.Tracks(ctx, al.ID)
	if err != nil {
		return EditionFit{}, err
	}
	note := fmt.Sprintf("edition changed from %s to %s (%d tracks), which fits the downloaded files better", orNone(from), r.MBID,
		next.TrackCount)
	s.publish(domain.EvItemUpdated, "system", []domain.EventItem{s.AlbumItem(ctx, al.ID)},
		domain.ItemEventData{Monitored: next.Monitored, Changed: []string{"edition"}, Reason: note})
	return EditionFit{Album: next, Tracks: stored, Switched: note}, nil
}

func orNone(mbid string) string {
	if mbid == "" {
		return "none"
	}
	return mbid
}
