package musicauto

import (
	"context"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/domain"
)

// refs are the music item references of an event, by level.
type refs struct {
	artists, albums []int64
}

// split separates artist, album and track references; a track counts as
// its album.
func (s *Service) split(ctx context.Context, items []domain.EventItem) refs {
	var r refs
	for _, it := range items {
		switch {
		case it.Ref.Kind != domain.KindMusic:
		case it.Ref.Variant == domain.VariantArtist:
			r.artists = append(r.artists, it.Ref.ID)
		case it.Ref.Variant == domain.VariantTrack:
			if t, err := s.d.Music.Track(ctx, it.Ref.ID); err == nil {
				r.albums = append(r.albums, t.AlbumID)
			}
		default:
			r.albums = append(r.albums, it.Ref.ID)
		}
	}
	return r
}

// OnEvent keeps the wanted index current and runs the searches on add (when
// asked) and when an album comes out.
func (s *Service) OnEvent(ctx context.Context, ev domain.Event) {
	r := s.split(ctx, ev.Items)
	for _, id := range r.artists {
		s.refreshArtist(ctx, id)
	}
	for _, id := range r.albums {
		s.refreshAlbum(ctx, id)
	}
	if s.searchable() != nil {
		return
	}
	data, _ := ev.Data.(domain.ItemEventData)
	switch {
	case ev.Type == domain.EvItemAdded && data.SearchNow:
		for _, id := range r.artists {
			_, err := s.SearchArtist(ctx, id, domain.OriginAutomatic)
			s.warn("search on add", err, "artist", id)
		}
	case ev.Type == domain.EvItemAvailable:
		s.searchWanted(ctx, r.albums)
	}
}

// OnFailure searches again for the albums of a failed or removed download
// (the blocklist entry is added by the shared automation).
func (s *Service) OnFailure(ctx context.Context, ev domain.Event) {
	if !automation.Researches(ev) || s.searchable() != nil {
		return
	}
	r := s.split(ctx, ev.Items)
	for _, id := range r.albums {
		s.refreshAlbum(ctx, id)
	}
	s.searchWanted(ctx, r.albums)
}

func (s *Service) searchWanted(ctx context.Context, ids []int64) {
	for _, id := range ids {
		if _, ok := s.index.Wanted(id); ok {
			_, err := s.SearchAlbum(ctx, id, domain.OriginAutomatic)
			s.warn("album search", err, "album", id)
		}
	}
}
