package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// ManagedIndexer is an indexer instance added from the web UI. Secrets hold
// the plain values of secret settings; they are sealed at rest.
type ManagedIndexer struct {
	Key       domain.ObjectKey
	PluginID  string
	Name      string
	Enabled   bool
	Settings  map[string]any
	Secrets   map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Indexers is the repository of UI-managed indexer instances (writer: indexers).
type Indexers struct{ s *Store }

// Indexers returns the indexer instance repository.
func (s *Store) Indexers() Indexers { return Indexers{s} }

const indexerCols = "key, plugin_id, name, enabled, settings, secrets, created_at, updated_at"

// List returns every managed instance, sorted by key.
func (r Indexers) List(ctx context.Context) ([]ManagedIndexer, error) {
	rows, err := r.s.R().QueryContext(ctx, "SELECT "+indexerCols+" FROM indexer_instances ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManagedIndexer
	for rows.Next() {
		m, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Get returns one managed instance.
func (r Indexers) Get(ctx context.Context, key domain.ObjectKey) (ManagedIndexer, bool, error) {
	m, err := r.scan(r.s.R().QueryRowContext(ctx, "SELECT "+indexerCols+" FROM indexer_instances WHERE key = ?", key))
	if ok, err := found(err); !ok {
		return ManagedIndexer{}, false, err
	}
	return m, true, nil
}

func (r Indexers) scan(sc scanner) (ManagedIndexer, error) {
	var m ManagedIndexer
	var settings, secrets string
	var created, updated int64
	if err := sc.Scan(&m.Key, &m.PluginID, &m.Name, &m.Enabled, &settings, &secrets, &created, &updated); err != nil {
		return m, err
	}
	m.CreatedAt, m.UpdatedAt = fromMS(created), fromMS(updated)
	if err := json.Unmarshal([]byte(settings), &m.Settings); err != nil {
		return m, err
	}
	sealed := map[string]string{}
	if err := json.Unmarshal([]byte(secrets), &sealed); err != nil {
		return m, err
	}
	m.Secrets = map[string]string{}
	for k, v := range sealed {
		plain, err := r.s.Decrypt(v)
		if err != nil {
			return m, err
		}
		m.Secrets[k] = plain
	}
	return m, nil
}

// Save inserts or replaces an instance (idempotent); CreatedAt is kept on update.
func (r Indexers) Save(ctx context.Context, m ManagedIndexer, now time.Time) error {
	sealed := map[string]string{}
	for k, v := range m.Secrets {
		s, err := r.s.Encrypt(v)
		if err != nil {
			return err
		}
		sealed[k] = s
	}
	if m.Settings == nil {
		m.Settings = map[string]any{}
	}
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO indexer_instances (`+indexerCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (key) DO UPDATE SET plugin_id = excluded.plugin_id, name = excluded.name, enabled = excluded.enabled,
			settings = excluded.settings, secrets = excluded.secrets, updated_at = excluded.updated_at`,
			m.Key, m.PluginID, m.Name, m.Enabled, jsonText(m.Settings), jsonText(sealed), ms(now), ms(now))
		return err
	})
}

// Delete removes an instance; it reports whether one existed.
func (r Indexers) Delete(ctx context.Context, key domain.ObjectKey) (bool, error) {
	var n int64
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "DELETE FROM indexer_instances WHERE key = ?", key)
		if err == nil {
			n, err = res.RowsAffected()
		}
		return err
	})
	return n > 0, err
}
