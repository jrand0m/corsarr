package store

import (
	"context"
	"database/sql"
	"time"
)

// RateLimits keeps the outbound client's learned limits, one opaque
// document per service.
type RateLimits struct{ s *Store }

// RateLimits returns the learned rate limit repository.
func (s *Store) RateLimits() RateLimits { return RateLimits{s} }

// LoadRateLimits returns every stored document by service.
func (r RateLimits) LoadRateLimits(ctx context.Context) (map[string][]byte, error) {
	rows, err := r.s.R().QueryContext(ctx, "SELECT key, doc FROM rate_limits")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]byte{}
	for rows.Next() {
		var key, doc string
		if err := rows.Scan(&key, &doc); err != nil {
			return nil, err
		}
		out[key] = []byte(doc)
	}
	return out, rows.Err()
}

// SaveRateLimit upserts one service's document.
func (r RateLimits) SaveRateLimit(ctx context.Context, key string, doc []byte) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO rate_limits (key, doc, updated_at) VALUES (?, ?, ?)
			ON CONFLICT (key) DO UPDATE SET doc = excluded.doc, updated_at = excluded.updated_at`, key, string(doc), ms(time.Now()))
		return err
	})
}
