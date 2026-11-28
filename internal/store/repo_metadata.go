package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// MetadataEntry is one cached provider record (the record as JSON).
type MetadataEntry struct {
	Source      string
	Kind        domain.MediaKind
	ExternalID  string
	Title       string
	Record      string
	FetchedAt   time.Time
	NextRefresh time.Time
}

// Metadata is the metadata cache (writer: the media-kind module).
type Metadata struct{ s *Store }

// Metadata returns the metadata cache repository.
func (s *Store) Metadata() Metadata { return Metadata{s} }

// Put upserts a record.
func (r Metadata) Put(ctx context.Context, e MetadataEntry) error {
	return r.s.exec(ctx, `INSERT INTO metadata_cache (source, kind, external_id, title, record, fetched_at, next_refresh)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (source, kind, external_id) DO UPDATE SET title = excluded.title,
		record = excluded.record, fetched_at = excluded.fetched_at, next_refresh = excluded.next_refresh`,
		e.Source, string(e.Kind), e.ExternalID, e.Title, e.Record, ms(e.FetchedAt), ms(e.NextRefresh))
}

const metaCols = "source, kind, external_id, title, record, fetched_at, next_refresh"

func scanMeta(sc scanner) (MetadataEntry, error) {
	var e MetadataEntry
	var kind string
	var fetched, next int64
	err := sc.Scan(&e.Source, &kind, &e.ExternalID, &e.Title, &e.Record, &fetched, &next)
	e.Kind, e.FetchedAt, e.NextRefresh = domain.MediaKind(kind), fromMS(fetched), fromMS(next)
	return e, err
}

// Get returns one cached record.
func (r Metadata) Get(ctx context.Context, source string, kind domain.MediaKind, id string) (MetadataEntry, bool, error) {
	e, err := scanMeta(r.s.R().QueryRowContext(ctx, "SELECT "+metaCols+` FROM metadata_cache
		WHERE source = ? AND kind = ? AND external_id = ?`, source, string(kind), id))
	ok, err := found(err)
	return e, ok, err
}

// Search returns up to limit cached records of a kind whose title contains term.
func (r Metadata) Search(ctx context.Context, kind domain.MediaKind, term string, limit int) ([]MetadataEntry, error) {
	like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.ToLower(term)) + "%"
	return r.list(ctx, "SELECT "+metaCols+` FROM metadata_cache WHERE kind = ? AND lower(title) LIKE ? ESCAPE '\'
		ORDER BY title LIMIT ?`, string(kind), like, limit)
}

func (r Metadata) list(ctx context.Context, q string, args ...any) ([]MetadataEntry, error) {
	rows, err := r.s.R().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MetadataEntry
	for rows.Next() {
		e, err := scanMeta(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Bytes is the size of every cached record.
func (r Metadata) Bytes(ctx context.Context) (int64, error) {
	var n sql.NullInt64
	err := r.s.R().QueryRowContext(ctx, "SELECT SUM(length(record)) FROM metadata_cache").Scan(&n)
	return n.Int64, err
}

// unusedMeta selects records of items that are not in the library, oldest first.
const unusedMeta = `SELECT rowid, length(record) FROM metadata_cache m WHERE NOT (
	(m.kind = 'movie' AND EXISTS (SELECT 1 FROM movies WHERE CAST(movies.tmdb_id AS TEXT) = m.external_id)) OR
	(m.kind = 'tv' AND EXISTS (SELECT 1 FROM series WHERE CAST(series.tmdb_id AS TEXT) = m.external_id)) OR
	(m.kind = 'music' AND EXISTS (SELECT 1 FROM artists WHERE artists.mbid = m.external_id)))
	ORDER BY fetched_at, rowid`

// EvictUnused deletes the oldest records of items not in the library until at
// least want bytes are freed or none are left; it returns the rows and bytes freed.
func (r Metadata) EvictUnused(ctx context.Context, want int64) (int, int64, error) {
	var n int
	var freed int64
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		n, freed = 0, 0
		rows, err := tx.QueryContext(ctx, unusedMeta)
		if err != nil {
			return err
		}
		var ids []int64
		for rows.Next() && freed < want {
			var id, size int64
			if err := rows.Scan(&id, &size); err != nil {
				rows.Close()
				return err
			}
			ids, freed = append(ids, id), freed+size
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, "DELETE FROM metadata_cache WHERE rowid = ?", id); err != nil {
				return err
			}
		}
		n = len(ids)
		return nil
	})
	return n, freed, err
}
