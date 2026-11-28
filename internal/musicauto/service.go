// Package musicauto is the music side of automation: the wanted index over
// albums, searches on add and on release, re-search after failures,
// interactive album search and manual grabs. It reuses the kind-neutral
// pieces of the automation package and the music module's adapter.
package musicauto

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/music"
)

// Music is the music module surface automation reads.
type Music interface {
	Artist(ctx context.Context, id int64) (domain.Artist, error)
	Album(ctx context.Context, id int64) (domain.Album, error)
	Albums(ctx context.Context, artistID int64) ([]domain.Album, error)
	Track(ctx context.Context, id int64) (domain.Track, error)
	AlbumFiles(ctx context.Context, albumID int64) ([]domain.LibraryFile, error)
	EachArtist(ctx context.Context, fn func(domain.Artist) error) error
}

// Blocklist answers the blocklist lookup (store.Blocklist).
type Blocklist interface {
	Has(ctx context.Context, item domain.ItemRef, id domain.ReleaseIdentity) (bool, error)
}

// Publisher is the event bus.
type Publisher interface {
	Publish(domain.Event) (domain.Event, error)
}

// Deps wires the service.
type Deps struct {
	Music     Music
	Queue     automation.Queue
	Blocklist Blocklist
	Indexers  func() []automation.Indexer
	Pub       Publisher
	Payload   decide.PayloadPolicy
	Modes     domain.Modes           // nil = all active
	Recent    automation.RecentStore // nil = in memory
	Searches  automation.StateStore  // nil = search results are not kept
	Now       func() time.Time
	Log       *slog.Logger
}

// Service is the music automation module.
type Service struct {
	d      Deps
	index  *Index
	recent automation.Recent
	mu     sync.Mutex // one automatic search at a time
}

// New returns the service with an empty index (call Rebuild at start).
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Service{d: d, index: NewIndex(), recent: automation.NewRecent(d.Recent, d.Now)}
}

// Index exposes the wanted index (read-only use).
func (s *Service) Index() *Index { return s.index }

func (s *Service) warn(msg string, err error, args ...any) {
	if err != nil {
		s.d.Log.Warn(msg, append(args, "err", err)...)
	}
}

func (s *Service) publish(t domain.EventType, actor string, items []domain.EventItem, data any) {
	_, _ = s.d.Pub.Publish(domain.Event{Type: t, Actor: actor, Kind: domain.KindMusic, Items: items, Data: data})
}

// searchable refuses searches for a disabled kind; shadow searches run.
func (s *Service) searchable() error {
	if mode := s.d.Modes.Of(domain.KindMusic); mode == domain.ModeDisabled {
		return domain.ModeError{Kind: domain.KindMusic, Mode: mode}
	}
	return nil
}

// WantedReason is the pure rule: the artist and the album are monitored, the
// album is out, and some of its tracks lack a file or its files are below
// the profile's cutoff.
func WantedReason(ar domain.Artist, al domain.Album, files []domain.LibraryFile, now time.Time) automation.Reason {
	if !ar.Monitored || !al.Monitored || !music.IsReleased(al, now) {
		return automation.ReasonNotWanted
	}
	if len(files) == 0 || len(files) < al.TrackCount {
		return automation.ReasonMissing
	}
	p, _ := music.ProfileByKey(ar.ProfileKey)
	if cur := music.Current(p, files); cur != nil && automation.BelowCutoff(p, *cur) {
		return automation.ReasonCutoffUnmet
	}
	return automation.ReasonNotWanted
}

// Entry is one wanted album.
type Entry struct {
	Album  domain.Album        `json:"album"`
	Artist music.ArtistRef     `json:"artist"`
	Reason automation.Reason   `json:"reason"`
	Files  int                 `json:"files"`
	Cur    *decide.CurrentFile `json:"current,omitempty"`
}

// Index is the in-memory wanted index of albums.
type Index struct {
	mu       sync.RWMutex
	albums   map[int64]Entry
	byArtist map[int64]map[int64]bool
}

// NewIndex returns an empty index.
func NewIndex() *Index {
	return &Index{albums: map[int64]Entry{}, byArtist: map[int64]map[int64]bool{}}
}

// PutArtist replaces an artist's wanted albums.
func (x *Index) PutArtist(artistID int64, wanted []Entry) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.dropArtist(artistID)
	for _, e := range wanted {
		x.put(e)
	}
}

// PutAlbum files one album (dropping it when not wanted).
func (x *Index) PutAlbum(e Entry) {
	x.mu.Lock()
	defer x.mu.Unlock()
	delete(x.albums, e.Album.ID)
	delete(x.byArtist[e.Album.ArtistID], e.Album.ID)
	x.put(e)
}

func (x *Index) put(e Entry) {
	if e.Reason == automation.ReasonNotWanted {
		return
	}
	x.albums[e.Album.ID] = e
	if x.byArtist[e.Album.ArtistID] == nil {
		x.byArtist[e.Album.ArtistID] = map[int64]bool{}
	}
	x.byArtist[e.Album.ArtistID][e.Album.ID] = true
}

// RemoveArtist drops an artist's albums.
func (x *Index) RemoveArtist(id int64) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.dropArtist(id)
}

func (x *Index) dropArtist(id int64) {
	for aid := range x.byArtist[id] {
		delete(x.albums, aid)
	}
	delete(x.byArtist, id)
}

// Wanted reports whether an album is in the index.
func (x *Index) Wanted(id int64) (Entry, bool) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	e, ok := x.albums[id]
	return e, ok
}

// OfArtist lists an artist's wanted album ids, ascending.
func (x *Index) OfArtist(id int64) []int64 {
	x.mu.RLock()
	defer x.mu.RUnlock()
	out := make([]int64, 0, len(x.byArtist[id]))
	for aid := range x.byArtist[id] {
		out = append(out, aid)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Page lists wanted albums of one reason by ascending album id after cursor.
func (x *Index) Page(reason automation.Reason, cursor int64, limit int) ([]Entry, int64) {
	x.mu.RLock()
	defer x.mu.RUnlock()
	ids := make([]int64, 0, len(x.albums))
	for id, e := range x.albums {
		if id > cursor && e.Reason == reason {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var next int64
	if len(ids) > limit {
		ids, next = ids[:limit], ids[limit-1]
	}
	out := make([]Entry, len(ids))
	for i, id := range ids {
		out[i] = x.albums[id]
	}
	return out, next
}

// Len is the number of wanted albums.
func (x *Index) Len() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return len(x.albums)
}

// entry derives one album's index entry.
func (s *Service) entry(ctx context.Context, ar domain.Artist, al domain.Album) Entry {
	files, err := s.d.Music.AlbumFiles(ctx, al.ID)
	s.warn("album files", err, "album", al.ID)
	p, _ := music.ProfileByKey(ar.ProfileKey)
	return Entry{Album: al, Artist: music.RefOf(ar), Reason: WantedReason(ar, al, files, s.d.Now()), Files: len(files),
		Cur: music.Current(p, files)}
}

// Rebuild fills the index from every artist (start-up).
func (s *Service) Rebuild(ctx context.Context) error {
	return s.d.Music.EachArtist(ctx, func(ar domain.Artist) error { return s.loadArtist(ctx, ar) })
}

func (s *Service) loadArtist(ctx context.Context, ar domain.Artist) error {
	albums, err := s.d.Music.Albums(ctx, ar.ID)
	if err != nil {
		return err
	}
	entries := make([]Entry, len(albums))
	for i, al := range albums {
		entries[i] = s.entry(ctx, ar, al)
	}
	s.index.PutArtist(ar.ID, entries)
	return nil
}

// refreshArtist re-derives an artist's albums (or drops them when gone).
func (s *Service) refreshArtist(ctx context.Context, id int64) {
	ar, err := s.d.Music.Artist(ctx, id)
	if err != nil {
		s.index.RemoveArtist(id)
		return
	}
	s.warn("index artist", s.loadArtist(ctx, ar), "artist", id)
}

// refreshAlbum re-derives one album's entry.
func (s *Service) refreshAlbum(ctx context.Context, id int64) {
	al, err := s.d.Music.Album(ctx, id)
	if err != nil {
		return // gone with its artist; the artist event drops it
	}
	if ar, err := s.d.Music.Artist(ctx, al.ArtistID); err == nil {
		s.index.PutAlbum(s.entry(ctx, ar, al))
	}
}
