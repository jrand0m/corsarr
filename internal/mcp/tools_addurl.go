package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/queue"
)

// addURLTool adds a download by URL for a library item.
func (s *Service) addURLTool() Tool {
	return Tool{Name: "add_download_url", Title: "Add a download by URL", Write: true, OpenWorld: true,
		Description: "Send a magnet link or a .torrent or .nzb URL to a download client for a library item: a movie, " +
			"episodes of a series (a season or given episodes) or a music album. The download is tracked and imported " +
			"like a grab. In a shadow kind the add is recorded, not sent.",
		Input: object(map[string]schema{
			"kind":       kindEnum("Media kind."),
			"id":         integer("Movie id or series id."),
			"albumId":    integer("Music album id."),
			"season":     integer("TV: every episode of this season."),
			"episodes":   array("TV: these episodes.", object(map[string]schema{"season": integer("Season."), "episode": integer("Episode.")}, "season", "episode")),
			"url":        str("Magnet link or http(s) URL of a .torrent or .nzb file."),
			"title":      str("Name for the download; taken from the URL when omitted."),
			"protocol":   enum("Needed only when the URL does not end in .torrent or .nzb.", "torrent", "usenet"),
			"downloader": str("Client key from list_downloaders; the first client that takes the protocol when omitted."),
		}, "kind", "url"),
		Run: bind(s.addDownloadURL)}
}

type addURLArgs struct {
	Kind       string      `json:"kind"`
	ID         int64       `json:"id"`
	AlbumID    int64       `json:"albumId"`
	Season     *int        `json:"season"`
	Episodes   []episodeNo `json:"episodes"`
	URL        string      `json:"url"`
	Title      string      `json:"title"`
	Protocol   string      `json:"protocol"`
	Downloader string      `json:"downloader"`
}

func (s *Service) addDownloadURL(ctx context.Context, in addURLArgs) (Result, error) {
	kind, err := s.kindArg(in.Kind)
	if err != nil {
		return Result{}, err
	}
	items, err := s.urlItems(ctx, kind, in)
	if err != nil {
		return Result{}, err
	}
	g, err := s.d.Downloaders.AddURL(ctx, queue.URLRequest{Kind: kind, Items: items, URL: in.URL, Title: in.Title,
		Protocol: domain.Protocol(in.Protocol), Downloader: domain.ObjectKey(in.Downloader)})
	switch {
	case errors.Is(err, queue.ErrShadowed):
		return Result{Text: "add_download_url: recorded, not sent (shadow mode)",
			Data: AddURLOut{Recorded: true, Items: len(items)}}, nil
	case errors.Is(err, queue.ErrBadURL):
		return Result{}, invalidArg("url", err.Error())
	case err != nil:
		return Result{}, err
	}
	return Result{Text: fmt.Sprintf("sent %s to %s for %d items", g.Release.Title, g.Downloader, len(items)),
		Data: AddURLOut{Sent: true, Grab: &g, Items: len(items)}}, nil
}

// urlItems are the library items the download is for.
func (s *Service) urlItems(ctx context.Context, kind domain.MediaKind, in addURLArgs) ([]domain.EventItem, error) {
	switch kind {
	case domain.KindMovie:
		m, err := s.d.Movies.Get(ctx, in.ID)
		return []domain.EventItem{m.EventItem()}, err
	case domain.KindTV:
		return s.urlEpisodes(ctx, in)
	}
	if in.AlbumID <= 0 {
		return nil, invalidArg("albumId", "give the album id")
	}
	p, err := s.d.Music.AlbumPage(ctx, in.AlbumID)
	return []domain.EventItem{p.Album.EventItem(p.Artist.Name)}, err
}

func (s *Service) urlEpisodes(ctx context.Context, in addURLArgs) ([]domain.EventItem, error) {
	if in.Season == nil && len(in.Episodes) == 0 {
		return nil, invalidArg("season", "give a season or episodes")
	}
	pick := showRequest{Episodes: in.Episodes}
	if in.Season != nil {
		pick.Seasons = []int{*in.Season}
	}
	ser, err := s.d.TV.Series(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	ids, err := s.pickEpisodes(ctx, ser.ID, pick)
	if err != nil {
		return nil, err
	}
	eps, err := s.d.TV.Episodes(ctx, ser.ID)
	picked := map[int64]bool{}
	for _, id := range ids {
		picked[id] = true
	}
	out := make([]domain.EventItem, 0, len(ids))
	for _, e := range eps {
		if picked[e.ID] {
			out = append(out, e.EventItem(ser.Title))
		}
	}
	return out, err
}
