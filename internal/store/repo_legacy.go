package store

import (
	"context"
	"database/sql"

	"github.com/jrand0m/corsarr/internal/domain"
)

// ── compatibility API ids ────────────────────────────────────────────────────

// LegacyIDs maps string keys to stable integer ids, one numbering per space
// (profile, rootfolder, tag, indexer).
type LegacyIDs struct{ s *Store }

// LegacyIDs returns the id map repository.
func (s *Store) LegacyIDs() LegacyIDs { return LegacyIDs{s} }

// ID returns the key's id, assigning the next free one on first use.
func (r LegacyIDs) ID(ctx context.Context, space, key string) (int64, error) {
	var id int64
	err := r.s.R().QueryRowContext(ctx, "SELECT id FROM legacy_ids WHERE space = ? AND key = ?", space, key).Scan(&id)
	if ok, err := found(err); ok || err != nil {
		return id, err
	}
	err = r.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO legacy_ids (space, key, id)
			SELECT ?, ?, COALESCE(MAX(id), 0) + 1 FROM legacy_ids WHERE space = ?`, space, key, space); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, "SELECT id FROM legacy_ids WHERE space = ? AND key = ?", space, key).Scan(&id)
	})
	return id, err
}

// Key returns the key behind an id.
func (r LegacyIDs) Key(ctx context.Context, space string, id int64) (string, bool, error) {
	var key string
	err := r.s.R().QueryRowContext(ctx, "SELECT key FROM legacy_ids WHERE space = ? AND id = ?", space, id).Scan(&key)
	ok, err := found(err)
	return key, ok, err
}

// Keys lists a space's keys in id order.
func (r LegacyIDs) Keys(ctx context.Context, space string) ([]string, error) {
	rows, err := r.s.R().QueryContext(ctx, "SELECT key FROM legacy_ids WHERE space = ? ORDER BY id", space)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// IndexerGrabs counts the grabs of one indexer's releases.
type IndexerGrabs struct {
	Grabs  int
	Failed int // whose download failed
}

// GrabsByIndexer counts grabs per indexer that offered the release.
func (r Queue) GrabsByIndexer(ctx context.Context) (map[domain.ObjectKey]IndexerGrabs, error) {
	rows, err := r.s.R().QueryContext(ctx, `SELECT json_extract(ix.value, '$.key') AS k, COUNT(*),
		SUM(CASE WHEN d.state = 'failed' THEN 1 ELSE 0 END)
		FROM grabs g JOIN json_each(g.release, '$.indexers') ix LEFT JOIN downloads d ON d.grab_id = g.id
		WHERE k IS NOT NULL GROUP BY k`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[domain.ObjectKey]IndexerGrabs{}
	for rows.Next() {
		var k string
		var g IndexerGrabs
		if err := rows.Scan(&k, &g.Grabs, &g.Failed); err != nil {
			return nil, err
		}
		out[domain.ObjectKey(k)] = g
	}
	return out, rows.Err()
}
