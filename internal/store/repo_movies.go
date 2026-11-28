package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Movies stores the movie items (single writer: the movies module).
type Movies struct{ s *Store }

// Movies returns the movie repository.
func (s *Store) Movies() Movies { return Movies{s} }

const movieCols = `id, tmdb_id, imdb_id, title, original_title, year, alt_titles, runtime_min, in_cinemas, digital,
	physical, monitored, min_availability, profile_key, root_folder, path, file_id, added_at, poster_url, fanart_url`

type scanner interface{ Scan(dest ...any) error }

func scanMovie(sc scanner) (domain.Movie, error) {
	var m domain.Movie
	var alt, avail string
	var cin, dig, phy sql.NullInt64
	var added int64
	err := sc.Scan(&m.ID, &m.TMDBID, &m.IMDBID, &m.Title, &m.OriginalTitle, &m.Year, &alt, &m.RuntimeMin, &cin, &dig,
		&phy, &m.Monitored, &avail, &m.ProfileKey, &m.RootFolder, &m.Path, &m.FileID, &added, &m.PosterURL, &m.FanartURL)
	m.InCinemas, m.Digital, m.Physical = ptrMS(cin), ptrMS(dig), ptrMS(phy)
	m.MinAvailability, m.AddedAt = domain.MinAvailability(avail), fromMS(added)
	_ = json.Unmarshal([]byte(alt), &m.AltTitles) // written by this repository as a JSON list
	return m, err
}

func jsonText(v any) string {
	b, _ := json.Marshal(v) // plain data shapes always marshal
	return string(b)
}

// Add inserts a movie keyed by its TMDB id. Adding the same TMDB id again
// returns the existing movie with created=false (idempotent).
func (r Movies) Add(ctx context.Context, m domain.Movie) (domain.Movie, bool, error) {
	var created bool
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO movies (tmdb_id, imdb_id, title, original_title, year, alt_titles,
			runtime_min, in_cinemas, digital, physical, monitored, min_availability, profile_key, root_folder, path,
			added_at, poster_url, fanart_url) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (tmdb_id) DO NOTHING`,
			m.TMDBID, m.IMDBID, m.Title, m.OriginalTitle, m.Year, jsonText(m.AltTitles), m.RuntimeMin, nullMS(m.InCinemas),
			nullMS(m.Digital), nullMS(m.Physical), m.Monitored, string(m.MinAvailability), m.ProfileKey, m.RootFolder,
			m.Path, ms(m.AddedAt), m.PosterURL, m.FanartURL)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		created = n == 1
		return nil
	})
	if err != nil {
		return domain.Movie{}, false, err
	}
	out, err := scanMovie(r.s.R().QueryRowContext(ctx, "SELECT "+movieCols+" FROM movies WHERE tmdb_id = ?", m.TMDBID))
	return out, created, err
}

// Get returns a movie by id.
func (r Movies) Get(ctx context.Context, id int64) (domain.Movie, bool, error) {
	m, err := scanMovie(r.s.R().QueryRowContext(ctx, "SELECT "+movieCols+" FROM movies WHERE id = ?", id))
	ok, err := found(err)
	return m, ok, err
}

// List pages movies by ascending id after cursor; next is 0 on the last page.
func (r Movies) List(ctx context.Context, cursor int64, limit int) ([]domain.Movie, int64, error) {
	out := []domain.Movie{}
	err := r.query(ctx, func(m domain.Movie) error { out = append(out, m); return nil },
		"SELECT "+movieCols+" FROM movies WHERE id > ? ORDER BY id LIMIT ?", cursor, limit)
	var next int64
	if len(out) == limit && limit > 0 {
		next = out[len(out)-1].ID
	}
	return out, next, err
}

// Each streams every movie in id order without holding the list in memory.
func (r Movies) Each(ctx context.Context, fn func(domain.Movie) error) error {
	return r.query(ctx, fn, "SELECT "+movieCols+" FROM movies ORDER BY id")
}

// PendingAvailability streams monitored movies whose item.available has not
// been published yet.
func (r Movies) PendingAvailability(ctx context.Context, fn func(domain.Movie) error) error {
	return r.query(ctx, fn, "SELECT "+movieCols+" FROM movies WHERE available_published = 0 AND monitored = 1 ORDER BY id")
}

func (r Movies) query(ctx context.Context, fn func(domain.Movie) error, q string, args ...any) error {
	rows, err := r.s.R().QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		m, err := scanMovie(rows)
		if err != nil {
			return err
		}
		if err := fn(m); err != nil {
			return err
		}
	}
	return rows.Err()
}

// Update writes the editable and metadata fields of a movie.
func (r Movies) Update(ctx context.Context, m domain.Movie) error {
	return r.exec(ctx, `UPDATE movies SET imdb_id = ?, title = ?, original_title = ?, year = ?, alt_titles = ?,
		runtime_min = ?, in_cinemas = ?, digital = ?, physical = ?, monitored = ?, min_availability = ?, profile_key = ?,
		root_folder = ?, path = ?, poster_url = ?, fanart_url = ? WHERE id = ?`, m.IMDBID, m.Title, m.OriginalTitle, m.Year,
		jsonText(m.AltTitles), m.RuntimeMin, nullMS(m.InCinemas), nullMS(m.Digital), nullMS(m.Physical), m.Monitored,
		string(m.MinAvailability), m.ProfileKey, m.RootFolder, m.Path, m.PosterURL, m.FanartURL, m.ID)
}

// SetFile links the movie's current library file (0 = none).
func (r Movies) SetFile(ctx context.Context, id, fileID int64) error {
	return r.exec(ctx, "UPDATE movies SET file_id = ? WHERE id = ?", fileID, id)
}

// SetAvailablePublished records whether item.available was published.
func (r Movies) SetAvailablePublished(ctx context.Context, id int64, published bool) error {
	return r.exec(ctx, "UPDATE movies SET available_published = ? WHERE id = ?", published, id)
}

// Delete removes a movie; deleting twice is fine.
func (r Movies) Delete(ctx context.Context, id int64) error {
	return r.exec(ctx, "DELETE FROM movies WHERE id = ?", id)
}

func (r Movies) exec(ctx context.Context, q string, args ...any) error {
	return r.s.exec(ctx, q, args...)
}

// exec runs one statement on the writer.
func (s *Store) exec(ctx context.Context, q string, args ...any) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, args...)
		return err
	})
}

// now is the repositories' clock (unix ms on write).
var now = time.Now
