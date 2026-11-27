package mcp

import (
	"context"
	"fmt"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/tv"
)

// ── set_monitored ────────────────────────────────────────────────────────────

type monitorArgs struct {
	Kind       string  `json:"kind"`
	ID         int64   `json:"id"`
	Monitored  bool    `json:"monitored"`
	Season     *int    `json:"season"`
	EpisodeIDs []int64 `json:"episodeIds"`
	AlbumIDs   []int64 `json:"albumIds"`
}

func (s *Service) setMonitored(ctx context.Context, in monitorArgs) (Result, error) {
	kind, err := s.kindArg(in.Kind)
	if err != nil {
		return Result{}, err
	}
	set := map[domain.MediaKind]func(context.Context, monitorArgs) (string, error){
		domain.KindMovie: s.monitorMovie, domain.KindTV: s.monitorTV, domain.KindMusic: s.monitorMusic,
	}[kind]
	what, err := set(ctx, in)
	if err != nil {
		return Result{}, err
	}
	verb := "unmonitored"
	if in.Monitored {
		verb = "monitored"
	}
	return Result{Text: verb + " " + what, Data: map[string]any{"kind": kind, "monitored": in.Monitored, "changed": what}}, nil
}

func (s *Service) monitorMovie(ctx context.Context, in monitorArgs) (string, error) {
	m, err := s.d.Movies.Update(ctx, in.ID, movies.Patch{Monitored: &in.Monitored})
	return m.Title, err
}

func (s *Service) monitorTV(ctx context.Context, in monitorArgs) (string, error) {
	switch {
	case len(in.EpisodeIDs) > 0:
		n, err := s.d.TV.SetEpisodesMonitored(ctx, in.EpisodeIDs, in.Monitored)
		return fmt.Sprintf("%d episodes", n), err
	case in.Season != nil:
		return fmt.Sprintf("season %d of series %d", *in.Season, in.ID), s.d.TV.SetSeasonMonitored(ctx, in.ID, *in.Season, in.Monitored)
	}
	ser, err := s.d.TV.Update(ctx, in.ID, tv.Patch{Monitored: &in.Monitored})
	return ser.Title, err
}

func (s *Service) monitorMusic(ctx context.Context, in monitorArgs) (string, error) {
	if len(in.AlbumIDs) > 0 {
		n, err := s.d.Music.SetAlbumsMonitored(ctx, in.AlbumIDs, in.Monitored)
		return fmt.Sprintf("%d albums", n), err
	}
	return fmt.Sprintf("artist %d", in.ID), s.setArtistMonitored(ctx, in.ID, in.Monitored)
}

// ── start_search ─────────────────────────────────────────────────────────────

type searchArgs struct {
	Kind      string `json:"kind"`
	ID        int64  `json:"id"`
	Season    *int   `json:"season"`
	EpisodeID int64  `json:"episodeId"`
	AlbumID   int64  `json:"albumId"`
}

func (s *Service) startSearch(_ context.Context, in searchArgs) (Result, error) {
	kind, err := s.kindArg(in.Kind)
	if err != nil {
		return Result{}, err
	}
	if in.ID <= 0 && in.EpisodeID <= 0 && in.AlbumID <= 0 {
		return Result{}, invalidArg("id", "give id, episodeId or albumId")
	}
	run := s.searchFor(kind, in)
	s.d.Go(func(ctx context.Context) { s.logErr("search", run(ctx)) })
	note := ""
	if s.d.Modes.Of(kind) == domain.ModeShadow {
		note = "; the kind is in shadow mode, so the decision is recorded and nothing is grabbed"
	}
	return Result{Text: "search started" + note, Data: map[string]any{"started": true, "kind": kind, "mode": s.d.Modes.Of(kind)}}, nil
}

// searchFor picks the search the arguments ask for.
func (s *Service) searchFor(kind domain.MediaKind, in searchArgs) func(context.Context) error {
	o := domain.OriginAutomatic
	switch {
	case kind == domain.KindMovie:
		return func(ctx context.Context) error { _, err := s.d.MovieSearch.Search(ctx, in.ID, o); return err }
	case in.EpisodeID > 0:
		return func(ctx context.Context) error {
			_, err := s.d.TVSearch.SearchEpisode(ctx, in.EpisodeID, o)
			return err
		}
	case kind == domain.KindTV && in.Season != nil:
		return func(ctx context.Context) error {
			_, err := s.d.TVSearch.SearchSeason(ctx, in.ID, *in.Season, o)
			return err
		}
	case kind == domain.KindTV:
		return func(ctx context.Context) error { _, err := s.d.TVSearch.SearchSeries(ctx, in.ID, o); return err }
	case in.AlbumID > 0:
		return func(ctx context.Context) error { _, err := s.d.MusicSearch.SearchAlbum(ctx, in.AlbumID, o); return err }
	}
	return func(ctx context.Context) error { _, err := s.d.MusicSearch.SearchArtist(ctx, in.ID, o); return err }
}

// ── rescan_item ──────────────────────────────────────────────────────────────

type rescanArgs struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
}

func (s *Service) rescanItem(_ context.Context, in rescanArgs) (Result, error) {
	kind, err := s.kindArg(in.Kind)
	if err != nil {
		return Result{}, err
	}
	if kind != domain.KindMovie && in.ID <= 0 {
		return Result{}, invalidArg("id", "give the series or artist id")
	}
	run := map[domain.MediaKind]func(context.Context) error{
		domain.KindMovie: s.d.Movies.Refresh,
		domain.KindTV:    func(ctx context.Context) error { _, err := s.d.TV.Refresh(ctx, in.ID); return err },
		domain.KindMusic: func(ctx context.Context) error { _, err := s.d.Music.Refresh(ctx, in.ID); return err },
	}[kind]
	s.d.Go(func(ctx context.Context) { s.logErr("refresh", run(ctx)) })
	return Result{Text: "refresh started", Data: map[string]any{"started": true, "kind": kind, "id": in.ID}}, nil
}
