// Package mcp is corsarr's Model Context Protocol server: AI assistants
// connect over Streamable HTTP at <base>/mcp (or over stdio through
// `corsarr mcp stdio`) and ask about or act on the library through tools and
// resources.
//
// The package only translates: every tool calls the same TV, movie, music,
// queue, search and history services the native API uses, so a request made
// here behaves exactly as one made in the UI, shadow mode included. Tools that
// change anything need a write-scoped key and mcp.scope: write; every call is
// audited to History and rate limited per key.
package mcp

import (
	"context"
	"io"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/events"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/music"
	"github.com/jrand0m/corsarr/internal/musicauto"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/queue"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/tv"
	"github.com/jrand0m/corsarr/internal/tvauto"
)

// Movies is the movie module surface (movies.Service).
type Movies interface {
	Lookup(ctx context.Context, term string) ([]plugin.MovieRecord, error)
	Add(ctx context.Context, req movies.AddRequest) (domain.Movie, bool, error)
	Get(ctx context.Context, id int64) (domain.Movie, error)
	Each(ctx context.Context, fn func(domain.Movie) error) error
	Update(ctx context.Context, id int64, p movies.Patch) (domain.Movie, error)
	Refresh(ctx context.Context) error
}

// MovieSearch runs movie searches and holds the wanted movies (automation.Service).
type MovieSearch interface {
	Search(ctx context.Context, movieID int64, origin domain.SearchOrigin) (automation.Summary, error)
	Interactive(ctx context.Context, movieID int64, em automation.Emitter) (automation.Summary, error)
	Index() *automation.Index
	ManualGrab
}

// TV is the tv module surface (tv.Service).
type TV interface {
	Lookup(ctx context.Context, term string) ([]tv.LookupItem, error)
	Add(ctx context.Context, req tv.AddRequest) (domain.Series, bool, error)
	Series(ctx context.Context, id int64) (domain.Series, error)
	Episode(ctx context.Context, id int64) (domain.Episode, error)
	Episodes(ctx context.Context, seriesID int64) ([]domain.Episode, error)
	EachSeries(ctx context.Context, fn func(domain.Series) error) error
	Detail(ctx context.Context, id int64) (tv.Detail, error)
	Update(ctx context.Context, id int64, p tv.Patch) (domain.Series, error)
	SetSeasonMonitored(ctx context.Context, id int64, season int, monitored bool) error
	SetEpisodesMonitored(ctx context.Context, ids []int64, monitored bool) (int, error)
	Refresh(ctx context.Context, id int64) (tv.RefreshResult, error)
	Upcoming(ctx context.Context, from, to time.Time) ([]tv.CalendarItem, error)
}

// TVSearch runs TV searches and holds the wanted episodes (tvauto.Service).
type TVSearch interface {
	SearchSeries(ctx context.Context, id int64, origin domain.SearchOrigin) (tvauto.Summary, error)
	SearchSeason(ctx context.Context, id int64, season int, origin domain.SearchOrigin) (tvauto.Summary, error)
	SearchEpisode(ctx context.Context, id int64, origin domain.SearchOrigin) (tvauto.Summary, error)
	InteractiveEpisode(ctx context.Context, id int64, em automation.Emitter) (automation.Summary, error)
	InteractiveSeason(ctx context.Context, seriesID int64, season int, em automation.Emitter) (automation.Summary, error)
	Index() *tvauto.Index
	ManualGrab
}

// Music is the music module surface (music.Service).
type Music interface {
	Lookup(ctx context.Context, term string) ([]music.LookupItem, error)
	Discography(ctx context.Context, artistMBID string) (plugin.ArtistRecord, error)
	FindRecording(ctx context.Context, artistMBID, title string) (music.RecordingMatch, error)
	Add(ctx context.Context, req music.AddRequest) (domain.Artist, bool, error)
	Artist(ctx context.Context, id int64) (domain.Artist, error)
	Album(ctx context.Context, id int64) (domain.Album, error)
	Albums(ctx context.Context, artistID int64) ([]domain.Album, error)
	EachArtist(ctx context.Context, fn func(domain.Artist) error) error
	Detail(ctx context.Context, id int64) (music.Detail, error)
	AlbumPage(ctx context.Context, id int64) (music.AlbumDetail, error)
	Update(ctx context.Context, id int64, p music.Patch) (domain.Artist, error)
	SetAlbumsMonitored(ctx context.Context, ids []int64, monitored bool) (int, error)
	Refresh(ctx context.Context, id int64) (music.RefreshResult, error)
}

// MusicSearch runs album searches and holds the wanted albums (musicauto.Service).
type MusicSearch interface {
	SearchAlbum(ctx context.Context, id int64, origin domain.SearchOrigin) (musicauto.Summary, error)
	SearchArtist(ctx context.Context, artistID int64, origin domain.SearchOrigin) (musicauto.Summary, error)
	Interactive(ctx context.Context, albumID int64, em automation.Emitter) (automation.Summary, error)
	Index() *musicauto.Index
	ManualGrab
}

// Queue pages and removes downloads (queue.Service).
type Queue interface {
	List(ctx context.Context, cursor int64, limit int) ([]store.StoredDownload, int64, error)
	Remove(ctx context.Context, id int64, o queue.RemoveOptions) error
}

// Library lists an item's imported files (store.Library).
type Library interface {
	FilesOf(ctx context.Context, item domain.ItemRef) ([]domain.LibraryFile, error)
}

// SearchStates reads an item's search state (store.Searches).
type SearchStates interface {
	State(ctx context.Context, item domain.ItemRef) (domain.SearchState, error)
	States(ctx context.Context, kind domain.MediaKind) (map[int64]domain.SearchState, error)
}

// History pages History (store.History).
type History interface {
	List(ctx context.Context, cursor int64, limit int) ([]store.HistoryEntry, int64, error)
}

// Health lists integration health (health.Registry).
type Health interface {
	List() []domain.IntegrationHealth
}

// RootFolders lists the checked root folders (rootfolders.Checker).
type RootFolders interface {
	List(ctx context.Context) ([]domain.RootFolder, error)
}

// Bus publishes the audit events and feeds resource change notifications
// (events.Bus).
type Bus interface {
	Publish(ev domain.Event) (domain.Event, error)
	Subscribe(n int, filter func(domain.EventType) bool) *events.Sub
}

// Deps wire the server to the services.
type Deps struct {
	Config      func() *config.Config
	Modes       domain.Modes
	Movies      Movies
	MovieSearch MovieSearch
	TV          TV
	TVSearch    TVSearch
	Music       Music
	MusicSearch MusicSearch
	Queue       Queue
	Downloaders Downloaders // nil leaves the download-client tools out
	Library     Library
	Searches    SearchStates
	History     History
	Health      Health
	RootFolders RootFolders
	Bus         Bus
	Version     string
	// Go runs a command in the background; nil runs it on its own goroutine.
	Go  func(func(context.Context))
	Now func() time.Time
	Log *slog.Logger
}

// Service is the MCP server.
type Service struct {
	d      Deps
	tools  []Tool
	byName map[string]*Tool
	limit  *limiter

	mu       sync.Mutex
	sessions map[string]*Session
}

// New builds the server and its tool set.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if d.Version == "" {
		d.Version = buildVersion()
	}
	if d.Go == nil {
		d.Go = func(fn func(context.Context)) { go fn(context.Background()) }
	}
	s := &Service{d: d, limit: newLimiter(d.Now), sessions: map[string]*Session{}}
	s.register(s.requestTools()...)
	s.register(s.libraryTools()...)
	s.register(s.downloaderTools()...)
	outputs := toolOutputs()
	for i := range s.tools {
		if s.tools[i].Output == nil {
			s.tools[i].Output = outputs[s.tools[i].Name]
		}
	}
	return s
}

func (s *Service) register(tools ...Tool) {
	s.tools = append(s.tools, tools...)
	s.byName = make(map[string]*Tool, len(s.tools))
	for i := range s.tools {
		s.byName[s.tools[i].Name] = &s.tools[i]
	}
}

// Tools lists every tool this build has, whatever the scope.
func (s *Service) Tools() []Tool { return append([]Tool{}, s.tools...) }

func (s *Service) cfg() config.MCP { return s.d.Config().MCP }

// buildVersion is the module version the binary was built from.
func buildVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		return bi.Main.Version
	}
	return "dev"
}
