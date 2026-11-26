package tv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// Repo is the TV persistence (store.TV implements it).
type Repo interface {
	AddSeries(ctx context.Context, s domain.Series, seasons []domain.Season, eps []domain.Episode) (domain.Series, bool, error)
	Sync(ctx context.Context, s domain.Series, seasons []domain.Season, eps []domain.Episode, drop []int64) error
	MarkRefreshed(ctx context.Context, id int64, at time.Time) error
	RefreshedAt(ctx context.Context, id int64) (time.Time, error)
	Series(ctx context.Context, id int64) (domain.Series, bool, error)
	SeriesByTMDB(ctx context.Context, tmdbID int64) (domain.Series, bool, error)
	ListSeries(ctx context.Context, cursor int64, limit int) ([]domain.Series, int64, error)
	EachSeries(ctx context.Context, fn func(domain.Series) error) error
	DeleteSeries(ctx context.Context, id int64) error
	Seasons(ctx context.Context, seriesID int64) ([]domain.Season, error)
	Episodes(ctx context.Context, seriesID int64) ([]domain.Episode, error)
	Episode(ctx context.Context, id int64) (domain.Episode, bool, error)
	EachEpisode(ctx context.Context, fn func(domain.Episode) error) error
	AiringBetween(ctx context.Context, from, to time.Time, fn func(domain.Episode) error) error
	PendingAvailability(ctx context.Context, fn func(domain.Episode) error) error
	SetAvailablePublished(ctx context.Context, id int64, published bool) error
	MarkAvailable(ctx context.Context, ids []int64) error
	SetMonitored(ctx context.Context, ids []int64, monitored bool) (int, error)
	SetFile(ctx context.Context, ids []int64, fileID int64) error
}

// Files reads library files (store.Library implements it).
type Files interface {
	File(ctx context.Context, id int64) (domain.LibraryFile, bool, error)
}

// Publisher is the event bus.
type Publisher interface {
	Publish(domain.Event) (domain.Event, error)
}

// Deps wires the service.
type Deps struct {
	Repo        Repo
	Files       Files
	Pub         Publisher
	Metadata    func() (plugin.SeriesMetadata, bool) // first running TV metadata plugin
	RootFolders func() []string                      // tv root folders from config
	Modes       domain.Modes                         // media-kind modes; nil = all active
	Now         func() time.Time
	Log         *slog.Logger
}

// Service is the TV module and the only writer of series and episodes.
type Service struct {
	d       Deps
	mu      sync.Mutex
	checkMu sync.Mutex
	timer   *time.Timer
	ctx     context.Context
}

// New returns the service.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Service{d: d, ctx: context.Background()}
}

// Errors the API maps to statuses.
var (
	ErrNotFound   = errors.New("series or episode not found")
	ErrNoMetadata = errors.New("no TV metadata plugin is configured")
)

// FieldError is a validation failure on one input field.
type FieldError struct{ Field, Msg string }

func (e FieldError) Error() string { return e.Field + ": " + e.Msg }

// LookupItem is a provider match, marked when the series is in the library.
type LookupItem struct {
	plugin.SeriesRecord
	LibraryID int64 `json:"libraryId"`
}

var lookupSchemes = map[string]bool{"tmdb": true, "tvdb": true, "imdb": true}

// Lookup searches the provider; "tmdb:", "tvdb:" and "imdb:" look up one series.
func (s *Service) Lookup(ctx context.Context, term string) ([]LookupItem, error) {
	md, ok := s.d.Metadata()
	if !ok {
		return nil, ErrNoMetadata
	}
	recs, err := s.find(ctx, md, strings.TrimSpace(term))
	if err != nil {
		return nil, err
	}
	out := make([]LookupItem, len(recs))
	for i, r := range recs {
		out[i] = LookupItem{SeriesRecord: r}
		if id, _ := strconv.ParseInt(r.IDs["tmdb"], 10, 64); id > 0 {
			if have, ok, err := s.d.Repo.SeriesByTMDB(ctx, id); err == nil && ok {
				out[i].LibraryID = have.ID
			}
		}
	}
	return out, nil
}

func (s *Service) find(ctx context.Context, md plugin.SeriesMetadata, term string) ([]plugin.SeriesRecord, error) {
	scheme, id, ok := strings.Cut(term, ":")
	if !ok || !lookupSchemes[scheme] {
		return md.SearchSeries(ctx, term)
	}
	rec, err := md.Series(ctx, map[string]string{scheme: strings.TrimSpace(id)})
	if errors.Is(err, plugin.ErrNotFound) {
		return []plugin.SeriesRecord{}, nil
	}
	rec.Episodes = nil // lookups list series, not their episodes
	return []plugin.SeriesRecord{rec}, err
}

// AddRequest adds one series by TMDB id.
type AddRequest struct {
	TMDBID        int64    `json:"tmdbId"`
	RootFolder    string   `json:"rootFolder"`
	ProfileKey    string   `json:"profileKey"`
	Monitor       string   `json:"monitor"`
	MonitorNew    string   `json:"monitorNew"`
	SeasonFolders *bool    `json:"seasonFolders"`
	SeriesType    string   `json:"seriesType"`
	Tags          []string `json:"tags"`
	// Seasons, when set, monitors exactly these season numbers instead of
	// applying Monitor.
	Seasons   []int `json:"seasons,omitempty"`
	SearchNow bool  `json:"searchNow"`
}

// Add fetches the series with all its episodes, applies the monitoring mode,
// stores everything and publishes item.added (with searchNow). Adding an
// existing TMDB id returns it unchanged.
func (s *Service) Add(ctx context.Context, req AddRequest) (domain.Series, bool, error) {
	ser, mode, err := s.prepare(req)
	if err != nil {
		return ser, false, err
	}
	md, ok := s.d.Metadata()
	if !ok {
		return ser, false, ErrNoMetadata
	}
	rec, err := md.Series(ctx, map[string]string{"tmdb": strconv.FormatInt(req.TMDBID, 10)})
	if err != nil {
		return ser, false, err
	}
	ApplyRecord(&ser, rec)
	ser.Path = filepath.Join(ser.RootFolder, FolderName(ser))
	eps, seasons := ApplyMode(mode, Episodes(rec), s.d.Now())
	if req.Seasons != nil {
		eps, seasons = ApplySeasons(req.Seasons, Episodes(rec))
	}
	ser, created, err := s.d.Repo.AddSeries(ctx, ser, seasons, eps)
	if err != nil || !created {
		return ser, false, err
	}
	s.afterAdd(ctx, ser, req.SearchNow)
	return ser, true, nil
}

func (s *Service) prepare(req AddRequest) (domain.Series, domain.MonitorMode, error) {
	ser := domain.Series{TMDBID: req.TMDBID, ProfileKey: req.ProfileKey, Tags: nonNil(req.Tags), Monitored: true,
		SeasonFolders: req.SeasonFolders == nil || *req.SeasonFolders, AddedAt: s.d.Now().UTC()}
	if req.TMDBID <= 0 {
		return ser, "", FieldError{"tmdbId", "a TMDB id is required"}
	}
	mode, err := domain.ParseMonitorMode(req.Monitor)
	if err != nil {
		return ser, "", FieldError{"monitor", err.Error()}
	}
	if ser.MonitorNew, err = domain.ParseMonitorNew(req.MonitorNew); err != nil {
		return ser, "", FieldError{"monitorNew", err.Error()}
	}
	if ser.Type, err = domain.ParseSeriesType(req.SeriesType); err != nil {
		return ser, "", FieldError{"seriesType", err.Error()}
	}
	if ser.ProfileKey == "" {
		ser.ProfileKey = DefaultProfileKey
	}
	if _, ok := ProfileByKey(ser.ProfileKey); !ok {
		return ser, "", FieldError{"profileKey", "unknown quality profile " + ser.ProfileKey}
	}
	ser.RootFolder, err = s.rootFolder(req.RootFolder)
	return ser, mode, err
}

func (s *Service) rootFolder(want string) (string, error) {
	roots := s.d.RootFolders()
	if want == "" && len(roots) > 0 {
		want = roots[0]
	}
	for _, r := range roots {
		if r == want {
			if st, err := os.Stat(r); err != nil || !st.IsDir() {
				return "", FieldError{"rootFolder", r + " is not an accessible directory"}
			}
			return r, nil
		}
	}
	return "", FieldError{"rootFolder", fmt.Sprintf("%q is not a TV root folder", want)}
}

// afterAdd marks already aired episodes as announced (the search on add
// covers them), publishes item.added and re-arms the availability timer.
func (s *Service) afterAdd(ctx context.Context, ser domain.Series, searchNow bool) {
	eps, err := s.d.Repo.Episodes(ctx, ser.ID)
	s.logErr(err)
	var aired []int64
	for _, e := range eps {
		if IsAvailable(e, s.d.Now()) {
			aired = append(aired, e.ID)
		}
	}
	s.logErr(s.d.Repo.MarkAvailable(ctx, aired))
	s.publish(domain.EvItemAdded, "user", []domain.EventItem{ser.EventItem()},
		domain.ItemEventData{Origin: "user", Path: ser.Path, Monitored: ser.Monitored, SearchNow: searchNow})
	s.Kick()
}

func (s *Service) publish(t domain.EventType, actor string, items []domain.EventItem, data domain.ItemEventData) {
	_, _ = s.d.Pub.Publish(domain.Event{Type: t, Actor: actor, Kind: domain.KindTV, Items: items, Data: data})
}

func (s *Service) logErr(err error) {
	if err != nil {
		s.d.Log.Warn("tv", "err", err)
	}
}

// Series returns one series or ErrNotFound.
func (s *Service) Series(ctx context.Context, id int64) (domain.Series, error) {
	ser, ok, err := s.d.Repo.Series(ctx, id)
	if err == nil && !ok {
		err = ErrNotFound
	}
	return ser, err
}

// Episode returns one episode or ErrNotFound.
func (s *Service) Episode(ctx context.Context, id int64) (domain.Episode, error) {
	e, ok, err := s.d.Repo.Episode(ctx, id)
	if err == nil && !ok {
		err = ErrNotFound
	}
	return e, err
}

// Episodes lists a series' episodes.
func (s *Service) Episodes(ctx context.Context, seriesID int64) ([]domain.Episode, error) {
	return s.d.Repo.Episodes(ctx, seriesID)
}

// EachSeries streams every series.
func (s *Service) EachSeries(ctx context.Context, fn func(domain.Series) error) error {
	return s.d.Repo.EachSeries(ctx, fn)
}

// EpisodeItem is an episode's event snapshot named after its series.
func (s *Service) EpisodeItem(ctx context.Context, id int64) domain.EventItem {
	e, err := s.Episode(ctx, id)
	if err != nil {
		return domain.EventItem{Ref: domain.ItemRef{Kind: domain.KindTV, ID: id}}
	}
	ser, err := s.Series(ctx, e.SeriesID)
	if err != nil {
		return e.EventItem("")
	}
	return e.EventItem(ser.Title)
}

// SetEpisodeFile links episodes to their library file (importer → tv).
func (s *Service) SetEpisodeFile(ctx context.Context, ids []int64, fileID int64) error {
	return s.d.Repo.SetFile(ctx, ids, fileID)
}

// Delete removes a series with its episodes and publishes item.deleted for
// the series and every episode with a file; files stay on disk unless
// deleteFiles asks the importer to recycle them.
func (s *Service) Delete(ctx context.Context, id int64, deleteFiles bool) error {
	if deleteFiles {
		if err := s.d.Modes.CheckActive(domain.KindTV); err != nil {
			return err // shadow/disabled: no file changes
		}
	}
	ser, err := s.Series(ctx, id)
	if err != nil {
		return err
	}
	eps, err := s.d.Repo.Episodes(ctx, id)
	if err != nil {
		return err
	}
	if err := s.d.Repo.DeleteSeries(ctx, id); err != nil {
		return err
	}
	items := []domain.EventItem{ser.EventItem()}
	for _, e := range eps {
		if e.FileID != 0 {
			items = append(items, e.EventItem(ser.Title))
		}
	}
	s.publish(domain.EvItemDeleted, "user", items, domain.ItemEventData{Path: ser.Path, Monitored: ser.Monitored, FilesDeleted: deleteFiles})
	return nil
}
