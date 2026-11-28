package movies

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
	"github.com/jrand0m/corsarr/internal/naming"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// Repo is the movie persistence (store.Movies implements it).
type Repo interface {
	Add(ctx context.Context, m domain.Movie) (domain.Movie, bool, error)
	Get(ctx context.Context, id int64) (domain.Movie, bool, error)
	List(ctx context.Context, cursor int64, limit int) ([]domain.Movie, int64, error)
	Each(ctx context.Context, fn func(domain.Movie) error) error
	PendingAvailability(ctx context.Context, fn func(domain.Movie) error) error
	Update(ctx context.Context, m domain.Movie) error
	SetFile(ctx context.Context, id, fileID int64) error
	SetAvailablePublished(ctx context.Context, id int64, published bool) error
	Delete(ctx context.Context, id int64) error
}

// Publisher is the event bus.
type Publisher interface {
	Publish(domain.Event) (domain.Event, error)
}

// Deps wires the service.
type Deps struct {
	Repo        Repo
	Pub         Publisher
	Metadata    func() (plugin.MovieMetadata, bool) // first running movie metadata plugin
	Cache       MetaCache                           // metadata cache; nil = no caching
	Folders     Folders                             // moves item folders (the importer owns files)
	RootFolders func() []string                     // movie root folders from config
	Modes       domain.Modes                        // media-kind modes; nil = all active
	Art         domain.ArtLink                      // artwork addresses in API answers; nil = unchanged
	Now         func() time.Time
	Log         *slog.Logger
}

// Service is the movie module and the only writer of Movie.
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
	ErrNotFound   = errors.New("movie not found")
	ErrNoMetadata = errors.New("no movie metadata plugin is configured")
)

// FieldError is a validation failure on one input field.
type FieldError struct{ Field, Msg string }

func (e FieldError) Error() string { return e.Field + ": " + e.Msg }

// Lookup searches the metadata provider; "tmdb:<id>" and "imdb:<id>" look up
// one movie. Cached records answer while the provider is down.
func (s *Service) Lookup(ctx context.Context, term string) ([]plugin.MovieRecord, error) {
	if scheme, id, ok := strings.Cut(strings.TrimSpace(term), ":"); ok && (scheme == "tmdb" || scheme == "imdb") {
		rec, err := s.movie(ctx, map[string]string{scheme: id})
		if errors.Is(err, plugin.ErrNotFound) {
			return []plugin.MovieRecord{}, nil
		}
		if err != nil {
			return nil, err
		}
		return []plugin.MovieRecord{rec}, nil
	}
	return s.search(ctx, term)
}

// AddRequest adds one movie by TMDB id.
type AddRequest struct {
	TMDBID          int64  `json:"tmdbId"`
	RootFolder      string `json:"rootFolder"`
	ProfileKey      string `json:"profileKey"`
	Monitored       *bool  `json:"monitored"`
	MinAvailability string `json:"minAvailability"`
	SearchNow       bool   `json:"searchNow"`
}

// Add fetches the metadata, validates and stores the movie, then publishes
// item.added (with searchNow for automation). Adding an existing TMDB id returns it.
func (s *Service) Add(ctx context.Context, req AddRequest) (domain.Movie, bool, error) {
	m, err := s.prepare(req)
	if err != nil {
		return m, false, err
	}
	rec, err := s.movie(ctx, map[string]string{"tmdb": strconv.FormatInt(req.TMDBID, 10)})
	if err != nil {
		return m, false, err
	}
	applyRecord(&m, rec)
	m.Path = filepath.Join(m.RootFolder, FolderName(m))
	m, created, err := s.d.Repo.Add(ctx, m)
	if err != nil || !created {
		return m, false, err
	}
	s.afterAdd(ctx, m, req.SearchNow)
	return m, true, nil
}

func (s *Service) prepare(req AddRequest) (domain.Movie, error) {
	avail, err := domain.ParseMinAvailability(req.MinAvailability)
	if err != nil {
		return domain.Movie{}, FieldError{"minAvailability", err.Error()}
	}
	m := domain.Movie{TMDBID: req.TMDBID, Monitored: req.Monitored == nil || *req.Monitored, MinAvailability: avail,
		ProfileKey: req.ProfileKey, RootFolder: req.RootFolder, AddedAt: s.d.Now().UTC()}
	if req.TMDBID <= 0 {
		return m, FieldError{"tmdbId", "a TMDB id is required"}
	}
	if m.ProfileKey == "" {
		m.ProfileKey = DefaultProfileKey
	}
	if _, ok := ProfileByKey(m.ProfileKey); !ok {
		return m, FieldError{"profileKey", "unknown quality profile " + m.ProfileKey}
	}
	m.RootFolder, err = s.rootFolder(req.RootFolder)
	return m, err
}

// rootFolder validates the chosen (or first) movie root folder.
func (s *Service) rootFolder(want string) (string, error) {
	roots := s.d.RootFolders()
	if want == "" && len(roots) > 0 {
		want = roots[0]
	}
	for _, r := range roots {
		if r == want {
			return r, accessible(r)
		}
	}
	return "", FieldError{"rootFolder", fmt.Sprintf("%q is not a movie root folder", want)}
}

func accessible(dir string) error {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return FieldError{"rootFolder", dir + " is not an accessible directory"}
	}
	return nil
}

func applyRecord(m *domain.Movie, rec plugin.MovieRecord) {
	m.IMDBID, m.Title, m.OriginalTitle, m.Year = rec.IDs["imdb"], rec.Title, rec.OriginalTitle, rec.Year
	m.AltTitles, m.RuntimeMin = rec.AltTitles, rec.RuntimeMin
	m.InCinemas, m.Digital, m.Physical = rec.InCinemas, rec.Digital, rec.Physical
	m.PosterURL, m.FanartURL = rec.PosterURL, rec.FanartURL
}

// FolderName renders the movie folder from the default template.
func FolderName(m domain.Movie) string {
	name, _ := naming.Preview(naming.DefaultMovieFolder, naming.Values{MovieTitle: m.Title, ReleaseYear: m.Year})
	return name
}

func (s *Service) afterAdd(ctx context.Context, m domain.Movie, searchNow bool) {
	if IsAvailable(m, s.d.Now()) {
		// available at add: search on add covers it, so no item.available later.
		s.logErr(s.d.Repo.SetAvailablePublished(ctx, m.ID, true))
	}
	s.publish(domain.EvItemAdded, m, domain.ItemEventData{Origin: "user", Path: m.Path, Monitored: m.Monitored, SearchNow: searchNow})
	s.Kick()
}

func (s *Service) publish(t domain.EventType, m domain.Movie, data domain.ItemEventData) {
	_, _ = s.d.Pub.Publish(domain.Event{Type: t, Actor: "user", Kind: domain.KindMovie, Items: []domain.EventItem{m.EventItem()}, Data: data})
}

func (s *Service) logErr(err error) {
	if err != nil {
		s.d.Log.Warn("movies", "err", err)
	}
}

// Get returns one movie or ErrNotFound.
func (s *Service) Get(ctx context.Context, id int64) (domain.Movie, error) {
	m, ok, err := s.d.Repo.Get(ctx, id)
	if err == nil && !ok {
		err = ErrNotFound
	}
	return m, err
}

// List pages movies by id.
func (s *Service) List(ctx context.Context, cursor int64, limit int) ([]domain.Movie, int64, error) {
	return s.d.Repo.List(ctx, cursor, limit)
}

// Each streams every movie.
func (s *Service) Each(ctx context.Context, fn func(domain.Movie) error) error {
	return s.d.Repo.Each(ctx, fn)
}

// Patch is a single-movie edit; nil fields stay. A new root folder moves the
// movie's folder there (same folder name).
type Patch struct {
	Monitored       *bool   `json:"monitored"`
	ProfileKey      *string `json:"profileKey"`
	MinAvailability *string `json:"minAvailability"`
	RootFolder      *string `json:"rootFolder"`
}

// Folders moves an item folder and previews the move (importer.Service).
type Folders interface {
	Move(ctx context.Context, item domain.ItemRef, from, to string) (any, error)
	Plan(ctx context.Context, item domain.ItemRef, from, to string) (any, error)
}

// ErrNoMover says folder moves are not wired (tests, tools).
var ErrNoMover = errors.New("moving folders is not available")

// Update applies a patch and publishes item.updated with the changed fields.
func (s *Service) Update(ctx context.Context, id int64, p Patch) (domain.Movie, error) {
	m, err := s.Get(ctx, id)
	if err != nil {
		return m, err
	}
	changed, err := apply(&m, p)
	if err != nil {
		return m, err
	}
	moved, err := s.relocate(ctx, &m, p.RootFolder)
	if err != nil {
		return m, err
	}
	if moved {
		changed = append(changed, "rootFolder", "path")
	}
	if len(changed) == 0 {
		return m, nil
	}
	if err := s.d.Repo.Update(ctx, m); err != nil {
		return m, err
	}
	if !IsAvailable(m, s.d.Now()) {
		s.logErr(s.d.Repo.SetAvailablePublished(ctx, m.ID, false))
	}
	s.publish(domain.EvItemUpdated, m, domain.ItemEventData{Monitored: m.Monitored, Changed: changed})
	s.Kick()
	return m, nil
}

// target validates a new root folder and returns the movie's path under it;
// ok=false when the root folder does not change.
func (s *Service) target(m domain.Movie, root *string) (string, string, bool, error) {
	if root == nil || *root == "" || *root == m.RootFolder {
		return "", "", false, nil
	}
	r, err := s.rootFolder(*root)
	if err != nil {
		return "", "", false, err
	}
	if err := s.d.Modes.CheckActive(domain.KindMovie); err != nil {
		return "", "", false, err // no file changes outside active mode
	}
	if s.d.Folders == nil {
		return "", "", false, ErrNoMover
	}
	return r, filepath.Join(r, filepath.Base(m.Path)), true, nil
}

// relocate moves the movie folder into a new root folder and updates the movie.
func (s *Service) relocate(ctx context.Context, m *domain.Movie, root *string) (bool, error) {
	r, to, ok, err := s.target(*m, root)
	if !ok || err != nil {
		return false, err
	}
	if _, err := s.d.Folders.Move(ctx, m.Ref(), m.Path, to); err != nil {
		return false, err
	}
	m.RootFolder, m.Path = r, to
	return true, nil
}

// PreviewMove lists the old and new paths of moving a movie to a root folder.
func (s *Service) PreviewMove(ctx context.Context, id int64, root string) (any, error) {
	m, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	_, to, ok, err := s.target(m, &root)
	if err != nil || !ok {
		return map[string]any{"from": m.Path, "to": m.Path, "files": []any{}}, err
	}
	return s.d.Folders.Plan(ctx, m.Ref(), m.Path, to)
}

func apply(m *domain.Movie, p Patch) ([]string, error) {
	var changed []string
	for _, f := range []func(*domain.Movie, Patch) (string, error){applyMonitored, applyProfile, applyAvailability} {
		name, err := f(m, p)
		if err != nil {
			return nil, err
		}
		if name != "" {
			changed = append(changed, name)
		}
	}
	return changed, nil
}

func applyMonitored(m *domain.Movie, p Patch) (string, error) {
	if p.Monitored == nil || *p.Monitored == m.Monitored {
		return "", nil
	}
	m.Monitored = *p.Monitored
	return "monitored", nil
}

func applyProfile(m *domain.Movie, p Patch) (string, error) {
	if p.ProfileKey == nil || *p.ProfileKey == m.ProfileKey {
		return "", nil
	}
	if _, ok := ProfileByKey(*p.ProfileKey); !ok {
		return "", FieldError{"profileKey", "unknown quality profile " + *p.ProfileKey}
	}
	m.ProfileKey = *p.ProfileKey
	return "profile", nil
}

func applyAvailability(m *domain.Movie, p Patch) (string, error) {
	if p.MinAvailability == nil || domain.MinAvailability(*p.MinAvailability) == m.MinAvailability {
		return "", nil
	}
	a, err := domain.ParseMinAvailability(*p.MinAvailability)
	if err != nil {
		return "", FieldError{"minAvailability", err.Error()}
	}
	m.MinAvailability = a
	return "minAvailability", nil
}

// Delete removes a movie and publishes item.deleted; files stay on disk
// unless deleteFiles asks the importer to recycle them.
func (s *Service) Delete(ctx context.Context, id int64, deleteFiles bool) error {
	if deleteFiles {
		if err := s.d.Modes.CheckActive(domain.KindMovie); err != nil {
			return err // shadow/disabled: no file changes
		}
	}
	m, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.d.Repo.Delete(ctx, id); err != nil {
		return err
	}
	s.publish(domain.EvItemDeleted, m, domain.ItemEventData{Path: m.Path, Monitored: m.Monitored, FilesDeleted: deleteFiles})
	return nil
}

// SetFile links the current library file (importer → movies).
func (s *Service) SetFile(ctx context.Context, id, fileID int64) error {
	return s.d.Repo.SetFile(ctx, id, fileID)
}
