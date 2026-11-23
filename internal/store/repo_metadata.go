package store

import (
	"context"
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
