package store

// Repository pattern (copy this for new tables):
//
//   - One small struct per aggregate holding *Store: `type Foos struct{ s *Store }`
//     plus a constructor `func (s *Store) Foos() Foos`.
//   - Reads use s.R() (concurrent, query_only). Writes use s.Write(ctx, func(tx)…),
//     which runs on the single writer goroutine in one transaction.
//   - Writes are idempotent: upsert (`INSERT … ON CONFLICT DO UPDATE`) or
//     `INSERT OR IGNORE` keyed by a natural id, so a retry after a crash ends in
//     the same state.
//   - Times are stored as unix milliseconds UTC (ms/fromMS); secret fields are
//     stored sealed (s.Encrypt / s.Decrypt), never in clear text.
//   - Consumers depend on a narrow interface declared in their own package, not
//     on this package's concrete types.

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func ms(t time.Time) int64 { return t.UTC().UnixMilli() }

func fromMS(v int64) time.Time { return time.UnixMilli(v).UTC() }

func nullMS(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: ms(*t), Valid: true}
}

func ptrMS(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := fromMS(v.Int64)
	return &t
}

// found turns sql.ErrNoRows into (false, nil).
func found(err error) (bool, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// ── settings ─────────────────────────────────────────────────────────────────

// Settings is a key/value store for small kernel state (e.g. first-run flags).
type Settings struct{ s *Store }

// Settings returns the settings repository.
func (s *Store) Settings() Settings { return Settings{s} }

// Get returns the value and whether it exists.
func (r Settings) Get(ctx context.Context, key string) (string, bool, error) {
	var v string
	ok, err := found(r.s.R().QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&v))
	return v, ok, err
}

// Set upserts a value.
func (r Settings) Set(ctx context.Context, key, value string) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
			ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, ms(time.Now()))
		return err
	})
}

// ── admin user, sessions, api key ────────────────────────────────────────────

// AdminUser is the single admin login.
type AdminUser struct {
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

// Auth stores the admin user, sessions and the API key.
type Auth struct{ s *Store }

// Auth returns the auth repository.
func (s *Store) Auth() Auth { return Auth{s} }

// Admin returns the admin user and whether one is set (first run = false).
func (r Auth) Admin(ctx context.Context) (AdminUser, bool, error) {
	var u AdminUser
	var created int64
	ok, err := found(r.s.R().QueryRowContext(ctx, "SELECT username, password_hash, created_at FROM admin_user WHERE id = 1").
		Scan(&u.Username, &u.PasswordHash, &created))
	u.CreatedAt = fromMS(created)
	return u, ok, err
}

// SetAdmin creates the admin user once; ErrExists when one is already set
// (first run is a one-time transition).
func (r Auth) SetAdmin(ctx context.Context, username, hash string) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		now := ms(time.Now())
		res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO admin_user (id, username, password_hash, created_at, updated_at)
			VALUES (1, ?, ?, ?, ?)`, username, hash, now, now)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrExists
		}
		return nil
	})
}

// ErrExists is returned when a create-once row already exists.
var ErrExists = errors.New("already exists")

// CreateSession stores a session by the hash of its token.
func (r Auth) CreateSession(ctx context.Context, tokenHash string, expires time.Time) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		now := ms(time.Now())
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO sessions (token_hash, created_at, expires_at, last_seen_at) VALUES (?, ?, ?, ?)`,
			tokenHash, now, ms(expires), now)
		return err
	})
}

// SessionValid reports whether a session exists and has not expired at now.
func (r Auth) SessionValid(ctx context.Context, tokenHash string, now time.Time) (bool, error) {
	var one int
	return found(r.s.R().QueryRowContext(ctx, "SELECT 1 FROM sessions WHERE token_hash = ? AND expires_at > ?", tokenHash, ms(now)).Scan(&one))
}

// DeleteSession removes a session (logout); deleting twice is fine.
func (r Auth) DeleteSession(ctx context.Context, tokenHash string) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", tokenHash)
		return err
	})
}

// PruneSessions deletes sessions expired before now.
func (r Auth) PruneSessions(ctx context.Context, now time.Time) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", ms(now))
		return err
	})
}

// APIKey returns the decrypted API key and whether one exists.
func (r Auth) APIKey(ctx context.Context) (string, bool, error) {
	var sealed string
	ok, err := found(r.s.R().QueryRowContext(ctx, "SELECT key_sealed FROM api_key WHERE id = 1").Scan(&sealed))
	if !ok {
		return "", false, err
	}
	key, err := r.s.Decrypt(sealed)
	return key, err == nil, err
}

// SetAPIKey stores (or rotates) the API key, sealed with secret.key.
func (r Auth) SetAPIKey(ctx context.Context, key string) error {
	sealed, err := r.s.Encrypt(key)
	if err != nil {
		return err
	}
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO api_key (id, key_sealed, created_at) VALUES (1, ?, ?)
			ON CONFLICT (id) DO UPDATE SET key_sealed = excluded.key_sealed, created_at = excluded.created_at`, sealed, ms(time.Now()))
		return err
	})
}

// ReadOnlyAPIKey returns the decrypted read-only key and whether one exists.
func (r Auth) ReadOnlyAPIKey(ctx context.Context) (string, bool, error) {
	var sealed string
	ok, err := found(r.s.R().QueryRowContext(ctx, "SELECT key_sealed FROM api_key_readonly WHERE id = 1").Scan(&sealed))
	if !ok {
		return "", false, err
	}
	key, err := r.s.Decrypt(sealed)
	return key, err == nil, err
}

// SetReadOnlyAPIKey stores (or rotates) the read-only key; "" revokes it.
func (r Auth) SetReadOnlyAPIKey(ctx context.Context, key string) error {
	if key == "" {
		return r.s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, "DELETE FROM api_key_readonly")
			return err
		})
	}
	sealed, err := r.s.Encrypt(key)
	if err != nil {
		return err
	}
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO api_key_readonly (id, key_sealed, created_at) VALUES (1, ?, ?)
			ON CONFLICT (id) DO UPDATE SET key_sealed = excluded.key_sealed, created_at = excluded.created_at`, sealed, ms(time.Now()))
		return err
	})
}
