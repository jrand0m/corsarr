package music

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// Repo is the music persistence (store.Music implements it).
type Repo interface {
	AddArtist(ctx context.Context, a domain.Artist, albums []domain.Album) (domain.Artist, bool, error)
	SyncArtist(ctx context.Context, a domain.Artist, albums []domain.Album) error
	MarkRefreshed(ctx context.Context, id int64, at time.Time) error
	RefreshedAt(ctx context.Context, id int64) (time.Time, error)
	Artist(ctx context.Context, id int64) (domain.Artist, bool, error)
	ArtistByMBID(ctx context.Context, mbid string) (domain.Artist, bool, error)
	ListArtists(ctx context.Context, cursor int64, limit int) ([]domain.Artist, int64, error)
	EachArtist(ctx context.Context, fn func(domain.Artist) error) error
	DeleteArtist(ctx context.Context, id int64) error
	Albums(ctx context.Context, artistID int64) ([]domain.Album, error)
	Album(ctx context.Context, id int64) (domain.Album, bool, error)
	SetAlbumsMonitored(ctx context.Context, ids []int64, monitored bool, pin domain.PinReason) (int, error)
	SetEdition(ctx context.Context, a domain.Album, tracks []domain.Track) error
	Tracks(ctx context.Context, albumID int64) ([]domain.Track, error)
	SetTrackFile(ctx context.Context, trackID, fileID int64) error
	PendingAvailability(ctx context.Context, fn func(domain.Album) error) error
	MarkAvailable(ctx context.Context, ids []int64) error
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
	Metadata    func() (plugin.MusicMetadata, bool) // first running music metadata plugin
	RootFolders func() []string                     // music root folders from config
	Modes       domain.Modes                        // media-kind modes; nil = all active
	Now         func() time.Time
	Log         *slog.Logger
}

// Jobs owned by the music module.
const (
	RefreshJob       = "music.refresh"
	RefreshInterval  = 24 * time.Hour
	ReleaseJob       = "music.released"
	ReleaseInterval  = time.Hour
	RefreshCadence   = 7 * 24 * time.Hour // each artist's metadata is refreshed this often
	maxFillPerArtist = 200                // editions fetched in the background per add or refresh
)

// Service is the music module and the only writer of artists, albums and tracks.
type Service struct {
	d   Deps
	mu  sync.Mutex
	ctx context.Context
	wg  sync.WaitGroup
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
	ErrNotFound   = errors.New("artist, album or track not found")
	ErrNoMetadata = errors.New("no music metadata plugin is configured")
)

// FieldError is a validation failure on one input field.
type FieldError struct{ Field, Msg string }

func (e FieldError) Error() string { return e.Field + ": " + e.Msg }

// Start lets background edition fetches run until Stop.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()
}

// Stop waits for background edition fetches.
func (s *Service) Stop() { s.wg.Wait() }

func (s *Service) logErr(msg string, err error) {
	if err != nil {
		s.d.Log.Warn("music: "+msg, "err", err)
	}
}

func (s *Service) publish(t domain.EventType, actor string, items []domain.EventItem, data domain.ItemEventData) {
	_, _ = s.d.Pub.Publish(domain.Event{Type: t, Actor: actor, Kind: domain.KindMusic, Items: items, Data: data})
}

func (s *Service) metadata() (plugin.MusicMetadata, error) {
	md, ok := s.d.Metadata()
	if !ok {
		return nil, ErrNoMetadata
	}
	return md, nil
}

// LookupItem is a provider match, marked when the artist is in the library.
type LookupItem struct {
	plugin.ArtistRecord
	LibraryID int64 `json:"libraryId"`
}

// Lookup searches artists by name; "mbid:<id>" looks up one artist.
func (s *Service) Lookup(ctx context.Context, term string) ([]LookupItem, error) {
	md, err := s.metadata()
	if err != nil {
		return nil, err
	}
	recs, err := find(ctx, md, strings.TrimSpace(term))
	if err != nil {
		return nil, err
	}
	out := make([]LookupItem, len(recs))
	for i, r := range recs {
		out[i] = LookupItem{ArtistRecord: r}
		if have, ok, err := s.d.Repo.ArtistByMBID(ctx, r.MBID); err == nil && ok {
			out[i].LibraryID = have.ID
		}
	}
	return out, nil
}

func find(ctx context.Context, md plugin.MusicMetadata, term string) ([]plugin.ArtistRecord, error) {
	id, ok := strings.CutPrefix(term, "mbid:")
	if !ok {
		return md.SearchArtists(ctx, term)
	}
	rec, err := md.Artist(ctx, strings.TrimSpace(id))
	if errors.Is(err, plugin.ErrNotFound) {
		return []plugin.ArtistRecord{}, nil
	}
	rec.Albums = nil // lookups list artists, not their albums
	return []plugin.ArtistRecord{rec}, err
}

// AddRequest adds one artist by MusicBrainz id.
type AddRequest struct {
	MBID            string   `json:"mbid"`
	RootFolder      string   `json:"rootFolder"`
	ProfileKey      string   `json:"profileKey"`
	MetadataProfile string   `json:"metadataProfile"`
	Monitor         string   `json:"monitor"`
	MonitorNew      string   `json:"monitorNew"`
	Tags            []string `json:"tags"`
	SearchNow       bool     `json:"searchNow"`
}

// Add fetches the artist with every release group, applies the monitoring
// choice through the metadata profile, stores everything and publishes
// item.added (with searchNow). The editions and tracks of monitored albums
// follow in the background. Adding a known MusicBrainz id returns it unchanged.
func (s *Service) Add(ctx context.Context, req AddRequest) (domain.Artist, bool, error) {
	ar, mode, err := s.prepare(req)
	if err != nil {
		return ar, false, err
	}
	md, err := s.metadata()
	if err != nil {
		return ar, false, err
	}
	rec, err := md.Artist(ctx, req.MBID)
	if err != nil {
		return ar, false, err
	}
	applyArtist(&ar, rec)
	ar.Path = filepath.Join(ar.RootFolder, ArtistFolder(ar))
	profile, _ := domain.MetadataProfileByKey(ar.MetadataProfile)
	albums := make([]domain.Album, len(rec.Albums))
	for i, r := range rec.Albums {
		albums[i] = AlbumOf(r)
	}
	ar, created, err := s.d.Repo.AddArtist(ctx, ar, ApplyMonitor(mode, profile, albums, s.d.Now()))
	if err != nil || !created {
		return ar, false, err
	}
	s.afterAdd(ctx, ar, req.SearchNow)
	return ar, true, nil
}

func applyArtist(a *domain.Artist, r plugin.ArtistRecord) {
	a.MBID, a.Name, a.SortName, a.Disambiguation, a.Type, a.Country = r.MBID, r.Name, r.SortName, r.Disambiguation, r.Type, r.Country
	if a.SortName == "" {
		a.SortName = r.Name
	}
}

func (s *Service) prepare(req AddRequest) (domain.Artist, domain.AlbumMonitor, error) {
	ar := domain.Artist{MBID: req.MBID, ProfileKey: req.ProfileKey, MetadataProfile: req.MetadataProfile, Tags: nonNil(req.Tags),
		Monitored: true, AddedAt: s.d.Now().UTC()}
	if strings.TrimSpace(req.MBID) == "" {
		return ar, "", FieldError{"mbid", "a MusicBrainz artist id is required"}
	}
	mode, err := domain.ParseAlbumMonitor(req.Monitor)
	if err != nil {
		return ar, "", FieldError{"monitor", err.Error()}
	}
	if ar.MonitorNew, err = domain.ParseMonitorNewItems(req.MonitorNew); err != nil {
		return ar, "", FieldError{"monitorNew", err.Error()}
	}
	if err := defaults(&ar); err != nil {
		return ar, "", err
	}
	ar.RootFolder, err = s.rootFolder(req.RootFolder)
	return ar, mode, err
}

func defaults(ar *domain.Artist) error {
	if ar.ProfileKey == "" {
		ar.ProfileKey = DefaultProfileKey
	}
	if ar.MetadataProfile == "" {
		ar.MetadataProfile = domain.DefaultMetadataProfile
	}
	if _, ok := ProfileByKey(ar.ProfileKey); !ok {
		return FieldError{"profileKey", "unknown quality profile " + ar.ProfileKey}
	}
	if _, ok := domain.MetadataProfileByKey(ar.MetadataProfile); !ok {
		return FieldError{"metadataProfile", "unknown metadata profile " + ar.MetadataProfile}
	}
	return nil
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
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
	return "", FieldError{"rootFolder", fmt.Sprintf("%q is not a music root folder", want)}
}

// afterAdd marks albums already out as available (the search on add covers
// them), publishes item.added and fetches monitored editions in the background.
func (s *Service) afterAdd(ctx context.Context, ar domain.Artist, searchNow bool) {
	albums, err := s.d.Repo.Albums(ctx, ar.ID)
	s.logErr("list albums", err)
	var out, fill []int64
	for _, a := range albums {
		if IsReleased(a, s.d.Now()) {
			out = append(out, a.ID)
		}
		if a.Monitored {
			fill = append(fill, a.ID)
		}
	}
	s.logErr("mark released", s.d.Repo.MarkAvailable(ctx, out))
	s.publish(domain.EvItemAdded, "user", []domain.EventItem{ar.EventItem()},
		domain.ItemEventData{Origin: "user", Path: ar.Path, Monitored: ar.Monitored, SearchNow: searchNow})
	s.fillLater(fill)
}

// fillLater fetches editions and tracks of albums one after another in the
// background; the metadata plugin paces the requests.
func (s *Service) fillLater(ids []int64) {
	if len(ids) == 0 {
		return
	}
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for _, id := range ids[:min(len(ids), maxFillPerArtist)] {
			if ctx.Err() != nil {
				return
			}
			_, _, err := s.EnsureEdition(ctx, id)
			s.logErr("fetch edition", err)
		}
	}()
}

// EnsureEdition returns an album with its tracks, choosing and storing an
// edition from the metadata plugin when it has none yet.
func (s *Service) EnsureEdition(ctx context.Context, albumID int64) (domain.Album, []domain.Track, error) {
	al, err := s.Album(ctx, albumID)
	if err != nil {
		return al, nil, err
	}
	tracks, err := s.d.Repo.Tracks(ctx, albumID)
	if err != nil || len(tracks) > 0 {
		return al, tracks, err
	}
	md, err := s.metadata()
	if err != nil {
		return al, nil, err
	}
	rels, err := md.Releases(ctx, al.MBID)
	if err != nil {
		return al, nil, err
	}
	rel, ok := ChooseEdition(rels)
	if !ok {
		return al, nil, nil // no edition upstream: matching falls back to names
	}
	al, tracks = Edition(al, rel)
	if err := s.d.Repo.SetEdition(ctx, al, tracks); err != nil {
		return al, nil, err
	}
	tracks, err = s.d.Repo.Tracks(ctx, albumID)
	return al, tracks, err
}

// Artist returns one artist or ErrNotFound.
func (s *Service) Artist(ctx context.Context, id int64) (domain.Artist, error) {
	a, ok, err := s.d.Repo.Artist(ctx, id)
	if err == nil && !ok {
		err = ErrNotFound
	}
	return a, err
}

// Album returns one album or ErrNotFound.
func (s *Service) Album(ctx context.Context, id int64) (domain.Album, error) {
	a, ok, err := s.d.Repo.Album(ctx, id)
	if err == nil && !ok {
		err = ErrNotFound
	}
	return a, err
}

// Albums lists an artist's albums.
func (s *Service) Albums(ctx context.Context, artistID int64) ([]domain.Album, error) {
	return s.d.Repo.Albums(ctx, artistID)
}

// Tracks lists an album's tracks.
func (s *Service) Tracks(ctx context.Context, albumID int64) ([]domain.Track, error) {
	return s.d.Repo.Tracks(ctx, albumID)
}

// EachArtist streams every artist.
func (s *Service) EachArtist(ctx context.Context, fn func(domain.Artist) error) error {
	return s.d.Repo.EachArtist(ctx, fn)
}

// AlbumItem is an album's event snapshot named after its artist.
func (s *Service) AlbumItem(ctx context.Context, id int64) domain.EventItem {
	al, err := s.Album(ctx, id)
	if err != nil {
		return domain.EventItem{Ref: domain.ItemRef{Kind: domain.KindMusic, ID: id}}
	}
	ar, err := s.Artist(ctx, al.ArtistID)
	if err != nil {
		return al.EventItem("")
	}
	return al.EventItem(ar.Name)
}

// SetTrackFile links a track to its library file (importer → music).
func (s *Service) SetTrackFile(ctx context.Context, trackID, fileID int64) error {
	return s.d.Repo.SetTrackFile(ctx, trackID, fileID)
}

// SetAlbumsMonitored changes monitoring by hand: monitoring pins the album
// with reason "owner" so profile changes never touch it; unmonitoring unpins.
func (s *Service) SetAlbumsMonitored(ctx context.Context, ids []int64, monitored bool) (int, error) {
	pin := domain.PinNone
	if monitored {
		pin = domain.PinOwner
	}
	n, err := s.d.Repo.SetAlbumsMonitored(ctx, ids, monitored, pin)
	if err != nil || n == 0 {
		return n, err
	}
	items := make([]domain.EventItem, len(ids))
	for i, id := range ids {
		items[i] = s.AlbumItem(ctx, id)
	}
	s.publish(domain.EvItemUpdated, "user", items, domain.ItemEventData{Monitored: monitored, Changed: []string{"monitoring"}})
	if monitored {
		s.fillLater(ids)
	}
	return n, nil
}

// Delete removes an artist with its albums and publishes item.deleted for
// the artist and every track with a file; files stay on disk unless
// deleteFiles asks the importer to recycle them.
func (s *Service) Delete(ctx context.Context, id int64, deleteFiles bool) error {
	if deleteFiles {
		if err := s.d.Modes.CheckActive(domain.KindMusic); err != nil {
			return err // shadow/disabled: no file changes
		}
	}
	ar, err := s.Artist(ctx, id)
	if err != nil {
		return err
	}
	items, err := s.fileItems(ctx, ar)
	if err != nil {
		return err
	}
	if err := s.d.Repo.DeleteArtist(ctx, id); err != nil {
		return err
	}
	s.publish(domain.EvItemDeleted, "user", items, domain.ItemEventData{Path: ar.Path, Monitored: ar.Monitored, FilesDeleted: deleteFiles})
	return nil
}

// fileItems are the artist plus every track that has a file.
func (s *Service) fileItems(ctx context.Context, ar domain.Artist) ([]domain.EventItem, error) {
	items := []domain.EventItem{ar.EventItem()}
	albums, err := s.d.Repo.Albums(ctx, ar.ID)
	for _, al := range albums {
		tracks, terr := s.d.Repo.Tracks(ctx, al.ID)
		err = errors.Join(err, terr)
		for _, t := range tracks {
			if t.FileID != 0 {
				items = append(items, domain.EventItem{Ref: t.Ref(), Title: ar.Name + " - " + t.Title})
			}
		}
	}
	return items, err
}
