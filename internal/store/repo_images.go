package store

import (
	"context"
	"database/sql"
	"time"
)

// ImageEntry is one row of the artwork cache index. Variant 0 is the stored
// original; other values are the width of a smaller copy. An entry with an
// empty Format is a remembered miss.
type ImageEntry struct {
	Key        string
	Variant    int
	Origin     string
	Role       string // poster | backdrop
	Format     string // jpeg | png | "" (miss)
	Bytes      int64
	Width      int
	Height     int
	Quality    int
	LastAccess time.Time
	MissUntil  time.Time
	CreatedAt  time.Time
}

// ImageUsage sums the index.
type ImageUsage struct {
	Bytes     int64
	Originals int
	Variants  int
	Misses    int
}

// Images is the artwork cache index (single writer: the image cache).
type Images struct{ s *Store }

// Images returns the artwork cache index.
func (s *Store) Images() Images { return Images{s} }

const imageCols = "key, variant, origin, role, format, bytes, width, height, quality, last_access, miss_until, created_at"

func scanImage(sc scanner) (ImageEntry, error) {
	var e ImageEntry
	var last, miss, created int64
	err := sc.Scan(&e.Key, &e.Variant, &e.Origin, &e.Role, &e.Format, &e.Bytes, &e.Width, &e.Height, &e.Quality, &last, &miss, &created)
	e.LastAccess, e.MissUntil, e.CreatedAt = fromMS(last), fromMS(miss), fromMS(created)
	return e, err
}

// Get returns one entry.
func (r Images) Get(ctx context.Context, key string, variant int) (ImageEntry, bool, error) {
	e, err := scanImage(r.s.R().QueryRowContext(ctx, "SELECT "+imageCols+" FROM image_cache WHERE key = ? AND variant = ?", key, variant))
	ok, err := found(err)
	return e, ok, err
}

// Put inserts or replaces an entry.
func (r Images) Put(ctx context.Context, e ImageEntry) error {
	return r.s.exec(ctx, `INSERT INTO image_cache (`+imageCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (key, variant) DO UPDATE SET origin = excluded.origin, role = excluded.role, format = excluded.format,
		bytes = excluded.bytes, width = excluded.width, height = excluded.height, quality = excluded.quality,
		last_access = excluded.last_access, miss_until = excluded.miss_until`,
		e.Key, e.Variant, e.Origin, e.Role, e.Format, e.Bytes, e.Width, e.Height, e.Quality, ms(e.LastAccess), ms(e.MissUntil), ms(e.CreatedAt))
}

// Touch records an access.
func (r Images) Touch(ctx context.Context, key string, variant int, at time.Time) error {
	return r.s.exec(ctx, "UPDATE image_cache SET last_access = ? WHERE key = ? AND variant = ?", ms(at), key, variant)
}

// Delete removes one entry; deleting twice is fine.
func (r Images) Delete(ctx context.Context, key string, variant int) error {
	return r.s.exec(ctx, "DELETE FROM image_cache WHERE key = ? AND variant = ?", key, variant)
}

// Variants lists the smaller copies of an image.
func (r Images) Variants(ctx context.Context, key string) ([]ImageEntry, error) {
	return r.list(ctx, "SELECT "+imageCols+" FROM image_cache WHERE key = ? AND variant > 0 ORDER BY variant", key)
}

// DeleteAll empties the index.
func (r Images) DeleteAll(ctx context.Context) error {
	return r.s.exec(ctx, "DELETE FROM image_cache")
}

// DeleteMisses forgets the misses that expired before t.
func (r Images) DeleteMisses(ctx context.Context, t time.Time) error {
	return r.s.exec(ctx, "DELETE FROM image_cache WHERE format = '' AND miss_until <= ?", ms(t))
}

// Stored lists the stored files, least recently used first.
func (r Images) Stored(ctx context.Context) ([]ImageEntry, error) {
	return r.list(ctx, "SELECT "+imageCols+" FROM image_cache WHERE format != '' ORDER BY last_access, key, variant")
}

func (r Images) list(ctx context.Context, q string, args ...any) ([]ImageEntry, error) {
	rows, err := r.s.R().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ImageEntry
	for rows.Next() {
		e, err := scanImage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Usage sums the stored bytes and counts the entries.
func (r Images) Usage(ctx context.Context) (ImageUsage, error) {
	var u ImageUsage
	var bytes sql.NullInt64
	err := r.s.R().QueryRowContext(ctx, `SELECT SUM(bytes),
		COUNT(*) FILTER (WHERE format != '' AND variant = 0),
		COUNT(*) FILTER (WHERE format != '' AND variant > 0),
		COUNT(*) FILTER (WHERE format = '') FROM image_cache`).Scan(&bytes, &u.Originals, &u.Variants, &u.Misses)
	u.Bytes = bytes.Int64
	return u, err
}

// ArtworkOrigins lists the provider addresses of movie and series artwork in
// the library.
func (s *Store) ArtworkOrigins(ctx context.Context) ([]string, error) {
	rows, err := s.R().QueryContext(ctx, `SELECT poster_url FROM movies WHERE poster_url != ''
		UNION SELECT fanart_url FROM movies WHERE fanart_url != ''
		UNION SELECT poster_url FROM series WHERE poster_url != ''
		UNION SELECT fanart_url FROM series WHERE fanart_url != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
