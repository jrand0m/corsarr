package music

import (
	"context"
	"errors"

	"github.com/jrand0m/corsarr/internal/domain"
)

// RefreshResult says what one artist refresh changed.
type RefreshResult struct {
	Added int `json:"added"`
}

// Refresh re-reads one artist from the metadata plugin, adds new release
// groups per monitor-new-items and the metadata profile, keeps everything it
// had (monitoring, pins, albums gone upstream) and publishes item.updated
// when release groups arrived.
func (s *Service) Refresh(ctx context.Context, id int64) (RefreshResult, error) {
	ar, err := s.Artist(ctx, id)
	if err != nil {
		return RefreshResult{}, err
	}
	md, err := s.metadata()
	if err != nil {
		return RefreshResult{}, err
	}
	rec, err := md.Artist(ctx, ar.MBID)
	if err != nil {
		return RefreshResult{}, err
	}
	stored, err := s.d.Repo.Albums(ctx, id)
	if err != nil {
		return RefreshResult{}, err
	}
	applyArtist(&ar, rec)
	profile, _ := domain.MetadataProfileByKey(ar.MetadataProfile)
	m := MergeAlbums(ar, profile, stored, rec.Albums)
	if err := s.d.Repo.SyncArtist(ctx, ar, m.Albums); err != nil {
		return RefreshResult{}, err
	}
	s.logErr("mark refreshed", s.d.Repo.MarkRefreshed(ctx, id, s.d.Now().UTC()))
	if m.Added > 0 {
		s.publish(domain.EvItemUpdated, "system", []domain.EventItem{ar.EventItem()},
			domain.ItemEventData{Monitored: ar.Monitored, Changed: []string{"albums"}})
	}
	s.fillMonitored(ctx, id)
	return RefreshResult{Added: m.Added}, nil
}

// fillMonitored fetches the editions of monitored albums that lack tracks.
func (s *Service) fillMonitored(ctx context.Context, artistID int64) {
	albums, err := s.d.Repo.Albums(ctx, artistID)
	s.logErr("list albums", err)
	var ids []int64
	for _, a := range albums {
		if a.Monitored && a.TrackCount == 0 {
			ids = append(ids, a.ID)
		}
	}
	s.fillLater(ids)
}

// RefreshAll refreshes, one after another, every artist not refreshed within
// RefreshCadence (the scheduled job, so a large library is spread over the
// week); a failing artist does not stop the others.
func (s *Service) RefreshAll(ctx context.Context) error {
	var ids []int64
	if err := s.d.Repo.EachArtist(ctx, func(a domain.Artist) error { ids = append(ids, a.ID); return nil }); err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if at, err := s.d.Repo.RefreshedAt(ctx, id); err == nil && s.d.Now().Sub(at) < RefreshCadence {
			continue
		}
		if _, err := s.Refresh(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// CheckReleased publishes item.available (reason "released") once for every
// monitored album whose release date has come (the scheduled job).
func (s *Service) CheckReleased(ctx context.Context) error {
	now := s.d.Now()
	var due []domain.Album
	err := s.d.Repo.PendingAvailability(ctx, func(a domain.Album) error {
		if IsReleased(a, now) {
			due = append(due, a)
		}
		return nil
	})
	if err != nil || len(due) == 0 {
		return err
	}
	ids := make([]int64, len(due))
	for i, a := range due {
		ids[i] = a.ID
	}
	if err := s.d.Repo.MarkAvailable(ctx, ids); err != nil {
		return err
	}
	for _, a := range due {
		s.publish(domain.EvItemAvailable, "system", []domain.EventItem{s.AlbumItem(ctx, a.ID)},
			domain.ItemEventData{Reason: "released", Monitored: true})
	}
	return nil
}
