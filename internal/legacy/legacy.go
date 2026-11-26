// Package legacy is the compatibility API under /legacy/<app>: the subset of
// the Sonarr, Radarr, Lidarr and Prowlarr HTTP APIs that request managers
// (Jellyseerr, Overseerr) and dashboards (Homepage) use, so they can talk to
// corsarr unchanged. Each app is off until legacy.<app>.enabled is set; a
// disabled app answers 404 like any unknown path. Requests authenticate with
// the corsarr API key (X-Api-Key header or apikey query).
//
// The package only translates: every read and write goes through the TV,
// movie, music, queue and indexer services the native API uses, so adding a
// series here behaves exactly as adding it in the UI (shadow mode included).
// Clients address quality profiles, root folders, tags and indexers by
// integer; a persisted id map keeps those ids stable across restarts.
package legacy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/indexers"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/musicauto"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/tv"
	"github.com/jrand0m/corsarr/internal/tvauto"
)

// TV is the tv module surface the Sonarr shim uses (tv.Service).
type TV interface {
	Lookup(ctx context.Context, term string) ([]tv.LookupItem, error)
	Add(ctx context.Context, req tv.AddRequest) (domain.Series, bool, error)
	Update(ctx context.Context, id int64, p tv.Patch) (domain.Series, error)
	SetSeasonMonitored(ctx context.Context, id int64, season int, monitored bool) error
	Detail(ctx context.Context, id int64) (tv.Detail, error)
	Series(ctx context.Context, id int64) (domain.Series, error)
	EachSeries(ctx context.Context, fn func(domain.Series) error) error
	Episode(ctx context.Context, id int64) (domain.Episode, error)
	Upcoming(ctx context.Context, from, to time.Time) ([]tv.CalendarItem, error)
}

// TVSearch runs TV searches and holds the wanted index (tvauto.Service).
type TVSearch interface {
	SearchSeries(ctx context.Context, id int64, origin domain.SearchOrigin) (tvauto.Summary, error)
	SearchSeason(ctx context.Context, id int64, season int, origin domain.SearchOrigin) (tvauto.Summary, error)
	SearchEpisode(ctx context.Context, id int64, origin domain.SearchOrigin) (tvauto.Summary, error)
	Index() *tvauto.Index
}

// Movies is the movie module surface the Radarr shim uses (movies.Service).
type Movies interface {
	Lookup(ctx context.Context, term string) ([]plugin.MovieRecord, error)
	Add(ctx context.Context, req movies.AddRequest) (domain.Movie, bool, error)
	Get(ctx context.Context, id int64) (domain.Movie, error)
	Each(ctx context.Context, fn func(domain.Movie) error) error
	Update(ctx context.Context, id int64, p movies.Patch) (domain.Movie, error)
}

// MovieSearch queues automatic movie searches (automation.Service).
type MovieSearch interface {
	Request(ctx context.Context, movieID int64)
}

// Music lists artists (music.Service).
type Music interface {
	EachArtist(ctx context.Context, fn func(domain.Artist) error) error
}

// MusicWanted holds the wanted albums (musicauto.Service).
type MusicWanted interface {
	Index() *musicauto.Index
}

// Queue pages the download queue (queue.Service).
type Queue interface {
	List(ctx context.Context, cursor int64, limit int) ([]store.StoredDownload, int64, error)
}

// RootFolders lists the checked root folders (rootfolders.Checker).
type RootFolders interface {
	List(ctx context.Context) ([]domain.RootFolder, error)
}

// Indexers lists the indexers (indexers.Service).
type Indexers interface {
	List(ctx context.Context) ([]indexers.Row, error)
}

// QueryLog counts queries sent to an indexer (store.Searches).
type QueryLog interface {
	Queries(ctx context.Context, indexer domain.ObjectKey, since time.Time) (int, time.Time, error)
}

// GrabLog counts grabs per indexer (store.Queue).
type GrabLog interface {
	GrabsByIndexer(ctx context.Context) (map[domain.ObjectKey]store.IndexerGrabs, error)
}

// IDs is the persisted integer id map (store.LegacyIDs).
type IDs interface {
	ID(ctx context.Context, space, key string) (int64, error)
	Key(ctx context.Context, space string, id int64) (string, bool, error)
	Keys(ctx context.Context, space string) ([]string, error)
}

// Deps wire the shims.
type Deps struct {
	Config      func() *config.Config
	Modes       domain.Modes
	TV          TV
	TVSearch    TVSearch
	Movies      Movies
	MovieSearch MovieSearch
	Music       Music
	MusicWanted MusicWanted
	Queue       Queue
	RootFolders RootFolders
	Indexers    Indexers
	Queries     QueryLog
	Grabs       GrabLog
	IDs         IDs
	// Go runs a command in the background; nil runs it on its own goroutine.
	Go  func(func(context.Context))
	Now func() time.Time
	Log *slog.Logger
}

// Service serves the compatibility API.
type Service struct {
	d    Deps
	apps map[string]*http.ServeMux
	cmd  commands
}

// New builds the shims.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if d.Go == nil {
		d.Go = func(fn func(context.Context)) { go fn(context.Background()) }
	}
	s := &Service{d: d, apps: map[string]*http.ServeMux{}}
	for app, routes := range map[string]func(*http.ServeMux){
		config.LegacySonarr: s.sonarrRoutes, config.LegacyRadarr: s.radarrRoutes,
		config.LegacyLidarr: s.lidarrRoutes, config.LegacyProwlarr: s.prowlarrRoutes,
	} {
		mux := http.NewServeMux()
		routes(mux)
		mux.HandleFunc("/legacy/"+app+"/", notFound)
		s.apps[app] = mux
	}
	return s
}

// Mount registers /legacy/<app>/ for every app; the routes authenticate by
// API key and answer only while their app is enabled.
func (s *Service) Mount(m api.Mounter) {
	km, ok := m.(api.KeyMounter)
	if !ok {
		return
	}
	for app := range s.apps {
		km.HandleAPIKey("/legacy/"+app+"/", s.serve(app), s.deny(app))
	}
}

// Enabled reports whether an app's compatibility API is switched on.
func (s *Service) Enabled(app string) bool { return s.d.Config().LegacyEnabled(app) }

func (s *Service) serve(app string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.Enabled(app) {
			disabled(w)
			return
		}
		s.apps[app].ServeHTTP(w, r)
	}
}

func (s *Service) deny(app string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.Enabled(app) {
			disabled(w)
			return
		}
		writeJSON(w, http.StatusUnauthorized, message{"Unauthorized: send the API key in X-Api-Key or apikey"})
	}
}

// ── responses ────────────────────────────────────────────────────────────────

type message struct {
	Message string `json:"message"`
}

// fieldError is one entry of a 400 validation response.
type fieldError struct {
	PropertyName string `json:"propertyName"`
	ErrorMessage string `json:"errorMessage"`
}

// disabled answers like the reserved, unmounted prefix.
func disabled(w http.ResponseWriter) {
	api.WriteError(w, http.StatusNotFound, "not_found", "legacy compatibility routes are reserved and not enabled", "")
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, message{"NotFound"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, v any) { writeJSON(w, http.StatusOK, v) }

func invalid(w http.ResponseWriter, field, msg string) {
	writeJSON(w, http.StatusBadRequest, []fieldError{{PropertyName: field, ErrorMessage: msg}})
}

// writeErr maps module errors onto statuses.
func (s *Service) writeErr(w http.ResponseWriter, err error) {
	var tfe tv.FieldError
	var mfe movies.FieldError
	var me domain.ModeError
	switch {
	case errors.As(err, &tfe):
		invalid(w, tfe.Field, tfe.Msg)
	case errors.As(err, &mfe):
		invalid(w, mfe.Field, mfe.Msg)
	case errors.As(err, &me):
		writeJSON(w, http.StatusConflict, message{err.Error()})
	case errors.Is(err, tv.ErrNotFound), errors.Is(err, movies.ErrNotFound), errors.Is(err, plugin.ErrNotFound):
		writeJSON(w, http.StatusNotFound, message{err.Error()})
	case errors.Is(err, tv.ErrNoMetadata), errors.Is(err, movies.ErrNoMetadata):
		writeJSON(w, http.StatusServiceUnavailable, message{err.Error()})
	default:
		s.d.Log.Warn("legacy api", "err", err)
		writeJSON(w, http.StatusInternalServerError, message{err.Error()})
	}
}

// decode reads a bounded JSON body; unknown fields are ignored because the
// clients send whole resources back.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		invalid(w, "body", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

// writable refuses writes while a media kind is disabled.
func (s *Service) writable(w http.ResponseWriter, k domain.MediaKind) bool {
	if mode := s.d.Modes.Of(k); mode == domain.ModeDisabled {
		s.writeErr(w, domain.ModeError{Kind: k, Mode: mode})
		return false
	}
	return true
}
