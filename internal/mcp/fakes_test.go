package mcp

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/music"
	"github.com/jrand0m/corsarr/internal/musicauto"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/queue"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/tv"
	"github.com/jrand0m/corsarr/internal/tvauto"
)

var errBoom = errors.New("boom")

// calls records what the fakes were asked to do.
type calls struct{ list []string }

func (c *calls) add(s string) { c.list = append(c.list, s) }

func (c *calls) has(s string) bool {
	for _, x := range c.list {
		if x == s {
			return true
		}
	}
	return false
}

// ── movies ───────────────────────────────────────────────────────────────────

type fakeMovies struct {
	c      *calls
	movies []domain.Movie
	recs   []plugin.MovieRecord
	err    error
}

func (f *fakeMovies) Lookup(_ context.Context, term string) ([]plugin.MovieRecord, error) {
	f.c.add("movies.lookup " + term)
	if strings.HasPrefix(term, "imdb:none") {
		return nil, nil
	}
	return f.recs, f.err
}

func (f *fakeMovies) Add(_ context.Context, req movies.AddRequest) (domain.Movie, bool, error) {
	f.c.add("movies.add")
	for _, m := range f.movies {
		if m.TMDBID == req.TMDBID {
			return m, false, nil
		}
	}
	m := domain.Movie{ID: int64(len(f.movies) + 1), TMDBID: req.TMDBID, Title: "Added", Monitored: true}
	f.movies = append(f.movies, m)
	return m, true, f.err
}

func (f *fakeMovies) Get(_ context.Context, id int64) (domain.Movie, error) {
	for _, m := range f.movies {
		if m.ID == id {
			return m, nil
		}
	}
	return domain.Movie{}, movies.ErrNotFound
}

func (f *fakeMovies) Each(_ context.Context, fn func(domain.Movie) error) error {
	for _, m := range f.movies {
		if err := fn(m); err != nil {
			return err
		}
	}
	return f.err
}

func (f *fakeMovies) Update(_ context.Context, id int64, p movies.Patch) (domain.Movie, error) {
	f.c.add("movies.update")
	m, err := f.Get(context.Background(), id)
	m.Monitored = *p.Monitored
	return m, err
}

func (f *fakeMovies) Refresh(context.Context) error { f.c.add("movies.refresh"); return nil }

type fakeMovieSearch struct {
	c   *calls
	idx *automation.Index
}

func (f *fakeMovieSearch) Search(_ context.Context, id int64, _ domain.SearchOrigin) (automation.Summary, error) {
	f.c.add("movies.search")
	return automation.Summary{}, nil
}

func (f *fakeMovieSearch) Interactive(_ context.Context, id int64, em automation.Emitter) (automation.Summary, error) {
	f.c.add("movies.interactive")
	_ = em.Record(automation.Decided{Release: domain.Release{ID: "r1", Title: "r1"}, Decision: domain.Decision{ReleaseID: "r1", Reasons: []domain.Reason{}}})
	_ = em.Record(automation.Decided{Release: domain.Release{ID: "r2", Title: "r2"}, Decision: domain.Decision{ReleaseID: "r2", Reasons: []domain.Reason{}}})
	_ = em.Error("idx", errBoom)
	return automation.Summary{Indexers: 1, Releases: 2, Accepted: 1}, nil
}

func (f *fakeMovieSearch) Index() *automation.Index { return f.idx }

// ── tv ───────────────────────────────────────────────────────────────────────

type fakeTV struct {
	c        *calls
	series   []domain.Series
	episodes []domain.Episode
	lookup   []tv.LookupItem
	err      error
}

func (f *fakeTV) Lookup(_ context.Context, term string) ([]tv.LookupItem, error) {
	f.c.add("tv.lookup " + term)
	if term == "tvdb:404" {
		return nil, nil
	}
	return f.lookup, f.err
}

func (f *fakeTV) Add(_ context.Context, req tv.AddRequest) (domain.Series, bool, error) {
	f.c.add("tv.add " + req.Monitor)
	for _, s := range f.series {
		if s.TMDBID == req.TMDBID {
			return s, false, nil
		}
	}
	s := domain.Series{ID: int64(len(f.series) + 1), TMDBID: req.TMDBID, Title: "New Show", Monitored: true}
	f.series = append(f.series, s)
	return s, true, nil
}

func (f *fakeTV) Series(_ context.Context, id int64) (domain.Series, error) {
	for _, s := range f.series {
		if s.ID == id {
			return s, nil
		}
	}
	return domain.Series{}, tv.ErrNotFound
}

func (f *fakeTV) Episode(_ context.Context, id int64) (domain.Episode, error) {
	for _, e := range f.episodes {
		if e.ID == id {
			return e, nil
		}
	}
	return domain.Episode{}, tv.ErrNotFound
}

func (f *fakeTV) Episodes(_ context.Context, seriesID int64) ([]domain.Episode, error) {
	return f.episodes, f.err
}

func (f *fakeTV) EachSeries(_ context.Context, fn func(domain.Series) error) error {
	for _, s := range f.series {
		if err := fn(s); err != nil {
			return err
		}
	}
	return f.err
}

func (f *fakeTV) Detail(ctx context.Context, id int64) (tv.Detail, error) {
	s, err := f.Series(ctx, id)
	d := tv.Detail{Series: tv.Summary{Series: s, Stats: tv.Stats{EpisodeCount: 2, EpisodeFileCount: 1}},
		Seasons: []tv.SeasonView{{Number: 1}, {Number: 2}}}
	for _, e := range f.episodes {
		d.Episodes = append(d.Episodes, tv.EpisodeView{Episode: e})
	}
	return d, err
}

func (f *fakeTV) Update(ctx context.Context, id int64, _ tv.Patch) (domain.Series, error) {
	f.c.add("tv.update")
	return f.Series(ctx, id)
}

func (f *fakeTV) SetSeasonMonitored(_ context.Context, _ int64, season int, _ bool) error {
	f.c.add("tv.season")
	return nil
}

func (f *fakeTV) SetEpisodesMonitored(_ context.Context, ids []int64, _ bool) (int, error) {
	f.c.add("tv.episodes")
	return len(ids), nil
}

func (f *fakeTV) Refresh(context.Context, int64) (tv.RefreshResult, error) {
	f.c.add("tv.refresh")
	return tv.RefreshResult{}, nil
}

func (f *fakeTV) Upcoming(context.Context, time.Time, time.Time) ([]tv.CalendarItem, error) {
	return []tv.CalendarItem{{}}, f.err
}

type fakeTVSearch struct {
	c   *calls
	idx *tvauto.Index
}

func (f *fakeTVSearch) SearchSeries(context.Context, int64, domain.SearchOrigin) (tvauto.Summary, error) {
	f.c.add("tv.searchSeries")
	return tvauto.Summary{}, nil
}

func (f *fakeTVSearch) SearchSeason(context.Context, int64, int, domain.SearchOrigin) (tvauto.Summary, error) {
	f.c.add("tv.searchSeason")
	return tvauto.Summary{}, nil
}

func (f *fakeTVSearch) SearchEpisode(context.Context, int64, domain.SearchOrigin) (tvauto.Summary, error) {
	f.c.add("tv.searchEpisode")
	return tvauto.Summary{}, errBoom
}

func (f *fakeTVSearch) InteractiveEpisode(context.Context, int64, automation.Emitter) (automation.Summary, error) {
	f.c.add("tv.interactiveEpisode")
	return automation.Summary{}, nil
}

func (f *fakeTVSearch) InteractiveSeason(context.Context, int64, int, automation.Emitter) (automation.Summary, error) {
	f.c.add("tv.interactiveSeason")
	return automation.Summary{}, nil
}

func (f *fakeTVSearch) Index() *tvauto.Index { return f.idx }

// ── music ────────────────────────────────────────────────────────────────────

type fakeMusic struct {
	c       *calls
	artists []domain.Artist
	albums  []domain.Album
	lookup  []music.LookupItem
	disco   plugin.ArtistRecord
	match   music.RecordingMatch
	err     error
}

func (f *fakeMusic) Lookup(_ context.Context, term string) ([]music.LookupItem, error) {
	f.c.add("music.lookup " + term)
	if term == "nobody" {
		return nil, nil
	}
	return f.lookup, f.err
}

func (f *fakeMusic) Discography(context.Context, string) (plugin.ArtistRecord, error) {
	return f.disco, f.err
}

func (f *fakeMusic) FindRecording(_ context.Context, _, title string) (music.RecordingMatch, error) {
	if title == "Nowhere" {
		return music.RecordingMatch{}, music.ErrNoRecording
	}
	return f.match, f.err
}

func (f *fakeMusic) Add(_ context.Context, req music.AddRequest) (domain.Artist, bool, error) {
	f.c.add("music.add " + req.Monitor)
	for _, a := range f.artists {
		if a.MBID == req.MBID {
			return a, false, f.err
		}
	}
	a := domain.Artist{ID: int64(len(f.artists) + 1), MBID: req.MBID, Name: "New Artist"}
	f.artists = append(f.artists, a)
	return a, true, f.err
}

func (f *fakeMusic) Artist(_ context.Context, id int64) (domain.Artist, error) {
	for _, a := range f.artists {
		if a.ID == id {
			return a, nil
		}
	}
	return domain.Artist{}, music.ErrNotFound
}

func (f *fakeMusic) Album(_ context.Context, id int64) (domain.Album, error) {
	for _, a := range f.albums {
		if a.ID == id {
			return a, nil
		}
	}
	return domain.Album{}, music.ErrNotFound
}

func (f *fakeMusic) Albums(context.Context, int64) ([]domain.Album, error) { return f.albums, nil }

func (f *fakeMusic) EachArtist(_ context.Context, fn func(domain.Artist) error) error {
	for _, a := range f.artists {
		if err := fn(a); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeMusic) Detail(ctx context.Context, id int64) (music.Detail, error) {
	a, err := f.Artist(ctx, id)
	d := music.Detail{Artist: music.Summary{Artist: a, Stats: music.Stats{MonitoredAlbums: 1}}}
	for _, al := range f.albums {
		d.Albums = append(d.Albums, music.AlbumView{Album: al})
	}
	return d, err
}

func (f *fakeMusic) AlbumPage(ctx context.Context, id int64) (music.AlbumDetail, error) {
	al, err := f.Album(ctx, id)
	return music.AlbumDetail{Album: music.AlbumView{Album: al, FileCount: 2}, Artist: music.ArtistRef{ID: al.ArtistID, Name: "Band"}}, err
}

func (f *fakeMusic) Update(ctx context.Context, id int64, _ music.Patch) (domain.Artist, error) {
	f.c.add("music.update")
	return f.Artist(ctx, id)
}

func (f *fakeMusic) SetAlbumsMonitored(_ context.Context, ids []int64, on bool) (int, error) {
	f.c.add("music.albums")
	for i := range f.albums {
		f.albums[i].Monitored = on
	}
	return len(ids), nil
}

func (f *fakeMusic) Refresh(context.Context, int64) (music.RefreshResult, error) {
	f.c.add("music.refresh")
	return music.RefreshResult{}, nil
}

type fakeMusicSearch struct {
	c   *calls
	idx *musicauto.Index
}

func (f *fakeMusicSearch) SearchAlbum(context.Context, int64, domain.SearchOrigin) (musicauto.Summary, error) {
	f.c.add("music.searchAlbum")
	return musicauto.Summary{}, nil
}

func (f *fakeMusicSearch) SearchArtist(context.Context, int64, domain.SearchOrigin) (musicauto.Summary, error) {
	f.c.add("music.searchArtist")
	return musicauto.Summary{}, nil
}

func (f *fakeMusicSearch) Interactive(context.Context, int64, automation.Emitter) (automation.Summary, error) {
	f.c.add("music.interactive")
	return automation.Summary{}, nil
}

func (f *fakeMusicSearch) Index() *musicauto.Index { return f.idx }

// ── queue, library, history, health ──────────────────────────────────────────

type fakeQueue struct {
	c     *calls
	pages [][]store.StoredDownload
	err   error
}

func (f *fakeQueue) List(_ context.Context, cursor int64, _ int) ([]store.StoredDownload, int64, error) {
	if int(cursor) >= len(f.pages) {
		return nil, 0, f.err
	}
	next := cursor + 1
	if int(next) >= len(f.pages) {
		next = 0
	}
	return f.pages[cursor], next, f.err
}

func (f *fakeQueue) Remove(_ context.Context, id int64, o queue.RemoveOptions) error {
	if o.DeleteFiles {
		f.c.add("queue.remove+files")
	} else {
		f.c.add("queue.remove")
	}
	return f.err
}

type fakeLibrary struct{}

func (fakeLibrary) FilesOf(_ context.Context, ref domain.ItemRef) ([]domain.LibraryFile, error) {
	if ref.Kind == domain.KindMovie && ref.ID == 1 {
		return []domain.LibraryFile{{ID: 5, Item: ref, Path: "/movies/Alpha (2020)/alpha.mkv"}}, nil
	}
	return nil, nil
}

type fakeSearches struct {
	states map[domain.ItemRef]domain.SearchState
	err    error
}

func (f fakeSearches) State(_ context.Context, ref domain.ItemRef) (domain.SearchState, error) {
	return f.states[ref], nil
}

func (f fakeSearches) States(_ context.Context, kind domain.MediaKind) (map[int64]domain.SearchState, error) {
	out := map[int64]domain.SearchState{}
	for ref, st := range f.states {
		if ref.Kind == kind {
			out[ref.ID] = st
		}
	}
	return out, f.err
}

type fakeHistory struct{}

func (fakeHistory) List(context.Context, int64, int) ([]store.HistoryEntry, int64, error) {
	return []store.HistoryEntry{{ID: 2, Event: domain.Event{Type: domain.EvGrabSent}}, {ID: 1, Event: domain.Event{Type: domain.EvImportCompleted}}}, 1, nil
}

type fakeHealth struct{}

func (fakeHealth) List() []domain.IntegrationHealth {
	return []domain.IntegrationHealth{{Status: domain.HealthOK}, {Status: domain.HealthFailing}}
}

type fakeRoots struct{ err error }

func (f fakeRoots) List(context.Context) ([]domain.RootFolder, error) {
	return []domain.RootFolder{{Path: "/movies", Kind: domain.KindMovie, Accessible: true}}, f.err
}
