package store

import (
	"context"
	"database/sql"
	"time"
)

// OutboxEntry is one pending or finished notification delivery.
type OutboxEntry struct {
	ID         int64
	EventID    string
	Connection string
	EventType  string
	Title      string
	Body       string
	Envelope   string // the event envelope as JSON
	BatchKey   string // deliveries with one key and connection go out as one message ("" = alone)
	Status     string // pending | sent | failed
	Attempts   int
	NextAt     time.Time
	LastError  string
}

// Outbox stores notification deliveries (writer: notify).
type Outbox struct{ s *Store }

// Outbox returns the outbox repository.
func (s *Store) Outbox() Outbox { return Outbox{s} }

// Enqueue adds a delivery; (event id, connection) twice is a no-op (idempotent).
func (r Outbox) Enqueue(ctx context.Context, e OutboxEntry) (bool, error) {
	var created bool
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO notify_outbox (event_id, connection, event_type, title, body,
			envelope, batch_key, next_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, e.EventID, e.Connection, e.EventType, e.Title,
			e.Body, e.Envelope, e.BatchKey, ms(e.NextAt), ms(now()))
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		created = n == 1
		return nil
	})
	return created, err
}

const outboxCols = "id, event_id, connection, event_type, title, body, envelope, batch_key, status, attempts, next_at, last_error"

// Due returns pending deliveries due at t, oldest first.
func (r Outbox) Due(ctx context.Context, t time.Time, limit int) ([]OutboxEntry, error) {
	return r.list(ctx, "SELECT "+outboxCols+" FROM notify_outbox WHERE status = 'pending' AND next_at <= ? ORDER BY next_at, id LIMIT ?", ms(t), limit)
}

// Batch lists the pending deliveries of one connection and batch key, oldest first.
func (r Outbox) Batch(ctx context.Context, connection, key string) ([]OutboxEntry, error) {
	return r.list(ctx, "SELECT "+outboxCols+" FROM notify_outbox WHERE status = 'pending' AND connection = ? AND batch_key = ? ORDER BY id",
		connection, key)
}

// ByStatus lists deliveries in a status (tests and diagnostics).
func (r Outbox) ByStatus(ctx context.Context, status string) ([]OutboxEntry, error) {
	return r.list(ctx, "SELECT "+outboxCols+" FROM notify_outbox WHERE status = ? ORDER BY id", status)
}

func (r Outbox) list(ctx context.Context, q string, args ...any) ([]OutboxEntry, error) {
	rows, err := r.s.R().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutboxEntry
	for rows.Next() {
		var e OutboxEntry
		var next int64
		if err := rows.Scan(&e.ID, &e.EventID, &e.Connection, &e.EventType, &e.Title, &e.Body, &e.Envelope, &e.BatchKey, &e.Status,
			&e.Attempts, &next, &e.LastError); err != nil {
			return nil, err
		}
		e.NextAt = fromMS(next)
		out = append(out, e)
	}
	return out, rows.Err()
}

// NextDue returns when the next pending delivery is due (ok=false when none).
func (r Outbox) NextDue(ctx context.Context) (time.Time, bool, error) {
	var v sql.NullInt64
	err := r.s.R().QueryRowContext(ctx, "SELECT MIN(next_at) FROM notify_outbox WHERE status = 'pending'").Scan(&v)
	return fromMS(v.Int64), v.Valid, err
}

// Finish stores the outcome of one attempt.
func (r Outbox) Finish(ctx context.Context, e OutboxEntry) error {
	var sent sql.NullInt64
	if e.Status == "sent" {
		sent = sql.NullInt64{Int64: ms(now()), Valid: true}
	}
	return r.s.exec(ctx, "UPDATE notify_outbox SET status = ?, attempts = ?, next_at = ?, last_error = ?, sent_at = ? WHERE id = ?",
		e.Status, e.Attempts, ms(e.NextAt), e.LastError, sent, e.ID)
}

// Refreshes stores pending media-server refreshes (writer: notify).
type Refreshes struct{ s *Store }

// Refreshes returns the refresh repository.
func (s *Store) Refreshes() Refreshes { return Refreshes{s} }

// Add queues (server, path); a pending duplicate is merged.
func (r Refreshes) Add(ctx context.Context, server, path string, due time.Time) error {
	return r.s.exec(ctx, "INSERT OR IGNORE INTO refresh_requests (server, path, next_at, created_at) VALUES (?, ?, ?, ?)",
		server, path, ms(due), ms(now()))
}

// Due returns the paths due at t grouped by server.
func (r Refreshes) Due(ctx context.Context, t time.Time) (map[string][]string, error) {
	rows, err := r.s.R().QueryContext(ctx, "SELECT server, path FROM refresh_requests WHERE next_at <= ? ORDER BY server, path", ms(t))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var server, path string
		if err := rows.Scan(&server, &path); err != nil {
			return nil, err
		}
		out[server] = append(out[server], path)
	}
	return out, rows.Err()
}

// NextDue returns when the next refresh is due (ok=false when none).
func (r Refreshes) NextDue(ctx context.Context) (time.Time, bool, error) {
	var v sql.NullInt64
	err := r.s.R().QueryRowContext(ctx, "SELECT MIN(next_at) FROM refresh_requests").Scan(&v)
	return fromMS(v.Int64), v.Valid, err
}

// Done removes refreshed paths.
func (r Refreshes) Done(ctx context.Context, server string, paths []string) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		for _, p := range paths {
			if _, err := tx.ExecContext(ctx, "DELETE FROM refresh_requests WHERE server = ? AND path = ?", server, p); err != nil {
				return err
			}
		}
		return nil
	})
}

// Attempts is the most failed attempts among the server's paths.
func (r Refreshes) Attempts(ctx context.Context, server string, paths []string) (int, error) {
	most := 0
	for _, p := range paths {
		var n int
		ok, err := found(r.s.R().QueryRowContext(ctx, "SELECT attempts FROM refresh_requests WHERE server = ? AND path = ?", server, p).Scan(&n))
		if err != nil {
			return most, err
		}
		if ok {
			most = max(most, n)
		}
	}
	return most, nil
}

// Retry postpones paths after a failed refresh.
func (r Refreshes) Retry(ctx context.Context, server string, paths []string, next time.Time) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		for _, p := range paths {
			if _, err := tx.ExecContext(ctx, "UPDATE refresh_requests SET attempts = attempts + 1, next_at = ? WHERE server = ? AND path = ?",
				ms(next), server, p); err != nil {
				return err
			}
		}
		return nil
	})
}
