package imagecache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jrand0m/corsarr/internal/store"
)

// Timings.
const (
	missFor    = 24 * time.Hour // a provider miss is remembered this long
	touchEvery = time.Hour      // last access is written at most this often
)

// ErrNotFound is an unknown or tampered token, or an image the provider
// does not have.
var ErrNotFound = errors.New("no such image")

// Index is the cache index (store.Images).
type Index interface {
	Get(ctx context.Context, key string, variant int) (store.ImageEntry, bool, error)
	Put(ctx context.Context, e store.ImageEntry) error
	Touch(ctx context.Context, key string, variant int, at time.Time) error
	Delete(ctx context.Context, key string, variant int) error
	Variants(ctx context.Context, key string) ([]store.ImageEntry, error)
	DeleteAll(ctx context.Context) error
	DeleteMisses(ctx context.Context, before time.Time) error
	Stored(ctx context.Context) ([]store.ImageEntry, error)
	Usage(ctx context.Context) (store.ImageUsage, error)
}

// Metadata is the provider-record cache, whose size counts toward the cap
// and whose records of items outside the library are evicted first. Its
// writer is the module that owns it.
type Metadata interface {
	Bytes(ctx context.Context) (int64, error)
	EvictUnused(ctx context.Context, want int64) (int, int64, error)
}

// Deps are the cache's collaborators; Metadata, Library and Kick may be nil.
type Deps struct {
	Settings Settings
	Index    Index
	Metadata Metadata
	HTTP     Doer
	Signer   Signer
	// Library lists the origin addresses of artwork used by library items.
	Library func(ctx context.Context) (map[string]bool, error)
	// Kick asks for a degradation run soon (the scheduled job).
	Kick func()
	Now  func() time.Time
	Log  *slog.Logger
}

// Cache is the artwork cache. It is the only writer of the index and of the
// files under its folder; every write holds mu.
type Cache struct {
	d        Deps
	mu       sync.Mutex
	flightMu sync.Mutex
	flight   map[string]*call
	requests atomic.Int64
	hits     atomic.Int64
	last     atomic.Pointer[RunStats]
}

type call struct {
	done chan struct{}
	e    store.ImageEntry
	err  error
}

// New returns a cache.
func New(d Deps) *Cache {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Cache{d: d, flight: map[string]*call{}}
}

// Link turns a provider address into the cached image path ("" for none).
func (c *Cache) Link(role Role, origin string) string { return c.d.Signer.Link(role, origin) }

// Image is what the image route serves: a stored file, or bytes when the
// cache is off.
type Image struct {
	Path    string
	Data    []byte
	Format  string
	ETag    string
	ModTime time.Time
	Stored  bool
}

// Image resolves a token and an optional requested width to an image,
// fetching and storing it on first use.
func (c *Cache) Image(ctx context.Context, token string, w int) (Image, error) {
	role, origin, ok := c.d.Signer.Open(token)
	if !ok {
		return Image{}, ErrNotFound
	}
	c.requests.Add(1)
	if !c.d.Settings.Enabled {
		return c.passThrough(ctx, role, origin, w)
	}
	key := Key(origin)
	e, hit, err := c.original(ctx, key, role, origin)
	if err != nil {
		return Image{}, err
	}
	if b := Bucket(w, e.Width); b > 0 {
		if e, err = c.variant(ctx, e, b); err != nil {
			return Image{}, err
		}
	}
	if hit {
		c.hits.Add(1)
	}
	c.touch(ctx, e)
	return c.served(e), nil
}

func (c *Cache) served(e store.ImageEntry) Image {
	return Image{Path: c.path(e), Format: e.Format, ModTime: e.CreatedAt, Stored: true,
		ETag: fmt.Sprintf(`"%s-%d-%d-%d"`, e.Key[:16], e.Variant, e.Quality, e.Width)}
}

// passThrough fetches and re-encodes without storing anything.
func (c *Cache) passThrough(ctx context.Context, role Role, origin string, w int) (Image, error) {
	data, err := fetch(ctx, c.d.HTTP, origin)
	if err != nil {
		return Image{}, c.upstream(err)
	}
	width := c.d.Settings.maxWidth(role)
	if b := Bucket(w, width); b > 0 {
		width = b
	}
	enc, err := process(data, width, c.d.Settings.Quality)
	if err != nil {
		return Image{}, err
	}
	return Image{Data: enc.Data, Format: enc.Format, ModTime: c.d.Now()}, nil
}

func (c *Cache) upstream(err error) error {
	if errors.Is(err, ErrMissing) {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return err
}

// original returns the stored original, fetching it once even when many
// requests ask at the same time; hit reports that it was already stored.
func (c *Cache) original(ctx context.Context, key string, role Role, origin string) (store.ImageEntry, bool, error) {
	e, ok, err := c.d.Index.Get(ctx, key, 0)
	switch {
	case err != nil:
		return e, false, err
	case ok && e.Format != "":
		if _, serr := os.Stat(c.path(e)); serr == nil {
			return e, true, nil
		}
	case ok && c.d.Now().Before(e.MissUntil):
		return e, false, ErrNotFound
	}
	e, err = c.once(key, func() (store.ImageEntry, error) { return c.fill(ctx, key, role, origin) })
	return e, false, err
}

// once runs fn for the first caller of a key and shares its result.
func (c *Cache) once(key string, fn func() (store.ImageEntry, error)) (store.ImageEntry, error) {
	c.flightMu.Lock()
	if f, ok := c.flight[key]; ok {
		c.flightMu.Unlock()
		<-f.done
		return f.e, f.err
	}
	f := &call{done: make(chan struct{})}
	c.flight[key] = f
	c.flightMu.Unlock()
	f.e, f.err = fn()
	c.flightMu.Lock()
	delete(c.flight, key)
	c.flightMu.Unlock()
	close(f.done)
	return f.e, f.err
}

// fill fetches, re-encodes and stores an original; a provider miss is
// remembered so the provider is not asked again for a day.
func (c *Cache) fill(ctx context.Context, key string, role Role, origin string) (store.ImageEntry, error) {
	now := c.d.Now()
	data, err := fetch(ctx, c.d.HTTP, origin)
	if errors.Is(err, ErrMissing) {
		miss := store.ImageEntry{Key: key, Origin: origin, Role: roleName(role), LastAccess: now, MissUntil: now.Add(missFor), CreatedAt: now}
		c.mu.Lock()
		perr := c.d.Index.Put(ctx, miss)
		c.mu.Unlock()
		return miss, errors.Join(c.upstream(err), perr)
	}
	if err != nil {
		return store.ImageEntry{}, err
	}
	enc, err := process(data, c.d.Settings.maxWidth(role), c.d.Settings.Quality)
	if err != nil {
		return store.ImageEntry{}, err
	}
	e := entryOf(key, 0, origin, roleName(role), enc, now)
	return e, c.store(ctx, e, enc.Data)
}

// variant returns the smaller copy of an original at width b, making it
// from the stored original on first use.
func (c *Cache) variant(ctx context.Context, orig store.ImageEntry, b int) (store.ImageEntry, error) {
	if v, ok, err := c.d.Index.Get(ctx, orig.Key, b); err != nil || ok {
		if _, serr := os.Stat(c.path(v)); err != nil || serr == nil {
			return v, err
		}
	}
	return c.once(orig.Key+"-"+strconv.Itoa(b), func() (store.ImageEntry, error) {
		data, err := os.ReadFile(c.path(orig))
		if err != nil {
			return store.ImageEntry{}, err
		}
		enc, err := process(data, b, max(orig.Quality, c.d.Settings.Degrade.QualityFloor))
		if err != nil {
			return store.ImageEntry{}, err
		}
		v := entryOf(orig.Key, b, orig.Origin, orig.Role, enc, c.d.Now())
		return v, c.store(ctx, v, enc.Data)
	})
}

func entryOf(key string, variant int, origin, role string, enc Encoded, now time.Time) store.ImageEntry {
	return store.ImageEntry{Key: key, Variant: variant, Origin: origin, Role: role, Format: enc.Format, Bytes: int64(len(enc.Data)),
		Width: enc.Width, Height: enc.Height, Quality: enc.Quality, LastAccess: now, CreatedAt: now}
}

// store writes a file and its index row. A store that would push usage over
// the cap first shrinks and evicts until the file fits, so the cap is never
// exceeded; crossing the high-water mark asks for a degradation run.
func (c *Cache) store(ctx context.Context, e store.ImageEntry, data []byte) error {
	s := c.d.Settings
	c.mu.Lock()
	defer c.mu.Unlock()
	usage, err := c.usage(ctx)
	if err != nil {
		return err
	}
	if usage+e.Bytes > s.MaxSize {
		target := min(s.low(), s.MaxSize-e.Bytes)
		if target < 0 {
			return fmt.Errorf("%w: one image is larger than the cache", ErrTooLarge)
		}
		st, err := c.degrade(ctx, usage, target)
		c.record(st)
		if err != nil {
			return err
		}
		usage = st.After
	}
	if err := c.write(c.path(e), data); err != nil {
		return err
	}
	if err := c.d.Index.Put(ctx, e); err != nil {
		_ = os.Remove(c.path(e))
		return err
	}
	if usage+e.Bytes >= s.high() && c.d.Kick != nil {
		c.d.Kick()
	}
	return nil
}

// usage is the stored image bytes plus the provider-record bytes.
func (c *Cache) usage(ctx context.Context) (int64, error) {
	u, err := c.d.Index.Usage(ctx)
	if err != nil || c.d.Metadata == nil {
		return u.Bytes, err
	}
	m, err := c.d.Metadata.Bytes(ctx)
	return u.Bytes + m, err
}

// touch records an access when the stored one is older than touchEvery.
func (c *Cache) touch(ctx context.Context, e store.ImageEntry) {
	now := c.d.Now()
	if now.Sub(e.LastAccess) < touchEvery {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.d.Index.Touch(ctx, e.Key, e.Variant, now); err != nil {
		c.d.Log.Warn("image cache: record access", "err", err)
	}
}

func (c *Cache) imagesDir() string { return filepath.Join(c.d.Settings.Dir, "images") }

func (c *Cache) path(e store.ImageEntry) string {
	ext := ".jpg"
	if e.Format == FormatPNG {
		ext = ".png"
	}
	if len(e.Key) < 2 {
		return filepath.Join(c.imagesDir(), "invalid")
	}
	return filepath.Join(c.imagesDir(), e.Key[:2], e.Key+"-"+strconv.Itoa(e.Variant)+ext)
}

// write replaces a file atomically, so a reader never sees half of it.
func (c *Cache) write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	if err := errors.Join(werr, tmp.Close()); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

// remove deletes an entry's file and row.
func (c *Cache) remove(ctx context.Context, e store.ImageEntry) error {
	if err := os.Remove(c.path(e)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return c.d.Index.Delete(ctx, e.Key, e.Variant)
}

// Clear removes every stored image and index row; provider records stay,
// because they are the refresh ledger.
func (c *Cache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.d.Index.DeleteAll(ctx); err != nil {
		return err
	}
	return os.RemoveAll(c.imagesDir())
}
