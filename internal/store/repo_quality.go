package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// QualityObject is one stored custom format or quality profile: its key,
// name and JSON document (the decide package's document shape).
type QualityObject struct {
	Key       string
	Name      string
	Doc       json.RawMessage
	CreatedAt time.Time
	UpdatedAt time.Time
}

// QualityTable names the two document tables.
type QualityTable string

const (
	TableFormats  QualityTable = "custom_formats"
	TableProfiles QualityTable = "quality_profiles"
)

// Quality is the repository of custom formats and profiles (writer: the
// profile service; migration writes through it too).
type Quality struct{ s *Store }

// Quality returns the custom format and profile repository.
func (s *Store) Quality() Quality { return Quality{s} }

// List returns every object of a table, sorted by key.
func (r Quality) List(ctx context.Context, t QualityTable) ([]QualityObject, error) {
	rows, err := r.s.R().QueryContext(ctx, "SELECT key, name, doc, created_at, updated_at FROM "+string(t)+" ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QualityObject
	for rows.Next() {
		var o QualityObject
		var doc string
		var created, updated int64
		if err := rows.Scan(&o.Key, &o.Name, &doc, &created, &updated); err != nil {
			return nil, err
		}
		o.Doc, o.CreatedAt, o.UpdatedAt = json.RawMessage(doc), fromMS(created), fromMS(updated)
		out = append(out, o)
	}
	return out, rows.Err()
}

// Save inserts or replaces an object, keeping its creation time.
func (r Quality) Save(ctx context.Context, t QualityTable, o QualityObject, now time.Time) error {
	return r.s.exec(ctx, "INSERT INTO "+string(t)+` (key, name, doc, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET name = excluded.name, doc = excluded.doc, updated_at = excluded.updated_at`,
		o.Key, o.Name, string(o.Doc), ms(now), ms(now))
}

// Delete removes an object; false when it did not exist.
func (r Quality) Delete(ctx context.Context, t QualityTable, key string) (bool, error) {
	var n int64
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "DELETE FROM "+string(t)+" WHERE key = ?", key)
		if err != nil {
			return err
		}
		n, err = res.RowsAffected()
		return err
	})
	return n > 0, err
}

// ProfileUse counts the series, movies and artists using a profile key.
func (r Quality) ProfileUse(ctx context.Context, key string) (int, error) {
	var n int
	err := r.s.R().QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM series WHERE profile_key = ?)
		+ (SELECT COUNT(*) FROM movies WHERE profile_key = ?) + (SELECT COUNT(*) FROM artists WHERE profile_key = ?)`,
		key, key, key).Scan(&n)
	return n, err
}
