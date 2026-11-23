package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// ── history ──────────────────────────────────────────────────────────────────

// HistoryEntry is one persisted event with its cursor id.
type HistoryEntry struct {
	ID    int64        `json:"id"`
	Event domain.Event `json:"event"`
}

// History persists events marked H in the catalogue.
type History struct{ s *Store }

// History returns the history repository.
func (s *Store) History() History { return History{s} }

// Append stores an event; appending the same EventID twice is a no-op.
func (r History) Append(ctx context.Context, ev domain.Event) error {
	env, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO history (event_id, type, occurred_at, actor, media_kind, imported, envelope)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, ev.EventID, string(ev.Type), ms(ev.OccurredAt), ev.Actor, string(ev.Kind), ev.Imported, string(env))
		return err
	})
}

// List returns up to limit entries older than cursor (0 = newest first) and
// the cursor for the next page (0 when there is none).
func (r History) List(ctx context.Context, cursor int64, limit int) ([]HistoryEntry, int64, error) {
	if cursor <= 0 {
		cursor = 1<<63 - 1
	}
	rows, err := r.s.R().QueryContext(ctx, "SELECT id, envelope FROM history WHERE id < ? ORDER BY id DESC LIMIT ?", cursor, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []HistoryEntry{}
	for rows.Next() {
		var e HistoryEntry
		var env string
		if err := rows.Scan(&e.ID, &env); err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal([]byte(env), &e.Event); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	var next int64
	if len(out) == limit && limit > 0 {
		next = out[len(out)-1].ID
	}
	return out, next, rows.Err()
}

// Prune deletes entries that occurred before t (retention).
func (r History) Prune(ctx context.Context, before time.Time) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM history WHERE occurred_at < ?", ms(before))
		return err
	})
}

// ── jobs ─────────────────────────────────────────────────────────────────────

// Jobs persists scheduler state (last run, result, backoff).
type Jobs struct{ s *Store }

// Jobs returns the jobs repository.
func (s *Store) Jobs() Jobs { return Jobs{s} }

// Save upserts a job's persisted fields.
func (r Jobs) Save(ctx context.Context, j domain.JobState) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO jobs (name, enabled, interval_ms, last_start, last_finish, last_result, last_error, failures, next_run)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (name) DO UPDATE SET enabled = excluded.enabled, interval_ms = excluded.interval_ms,
			last_start = excluded.last_start, last_finish = excluded.last_finish, last_result = excluded.last_result,
			last_error = excluded.last_error, failures = excluded.failures, next_run = excluded.next_run`,
			j.Name, j.Enabled, j.Interval.Milliseconds(), nullMS(j.LastStart), nullMS(j.LastFinish),
			string(j.LastResult), j.LastError, j.Failures, nullMS(j.NextRun))
		return err
	})
}

// List returns every persisted job state.
func (r Jobs) List(ctx context.Context) ([]domain.JobState, error) {
	rows, err := r.s.R().QueryContext(ctx, `SELECT name, enabled, interval_ms, last_start, last_finish, last_result, last_error, failures, next_run FROM jobs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.JobState
	for rows.Next() {
		var j domain.JobState
		var iv int64
		var start, finish, next sql.NullInt64
		var res string
		if err := rows.Scan(&j.Name, &j.Enabled, &iv, &start, &finish, &res, &j.LastError, &j.Failures, &next); err != nil {
			return nil, err
		}
		j.Interval, j.LastResult = time.Duration(iv)*time.Millisecond, domain.JobResult(res)
		j.LastStart, j.LastFinish, j.NextRun = ptrMS(start), ptrMS(finish), ptrMS(next)
		out = append(out, j)
	}
	return out, rows.Err()
}

// ── integration health ───────────────────────────────────────────────────────

// Health persists IntegrationHealth records.
type Health struct{ s *Store }

// Health returns the health repository.
func (s *Store) Health() Health { return Health{s} }

// Save upserts one record.
func (r Health) Save(ctx context.Context, h domain.IntegrationHealth) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO integration_health (type, key, status, since, last_ok_at, last_error_class,
			last_error_message, consecutive_failures, next_retry_at, fix_hint) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (type, key) DO UPDATE SET status = excluded.status, since = excluded.since, last_ok_at = excluded.last_ok_at,
			last_error_class = excluded.last_error_class, last_error_message = excluded.last_error_message,
			consecutive_failures = excluded.consecutive_failures, next_retry_at = excluded.next_retry_at, fix_hint = excluded.fix_hint`,
			h.Ref.Type, string(h.Ref.Key), string(h.Status), ms(h.Since), nullMS(h.LastOKAt), h.LastErrorClass,
			h.LastErrorMessage, h.ConsecutiveFailures, nullMS(h.NextRetryAt), h.FixHint)
		return err
	})
}

// List returns all records ordered by type and key.
func (r Health) List(ctx context.Context) ([]domain.IntegrationHealth, error) {
	rows, err := r.s.R().QueryContext(ctx, `SELECT type, key, status, since, last_ok_at, last_error_class, last_error_message,
		consecutive_failures, next_retry_at, fix_hint FROM integration_health ORDER BY type, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.IntegrationHealth
	for rows.Next() {
		var h domain.IntegrationHealth
		var key, status string
		var since int64
		var ok, retry sql.NullInt64
		if err := rows.Scan(&h.Ref.Type, &key, &status, &since, &ok, &h.LastErrorClass, &h.LastErrorMessage,
			&h.ConsecutiveFailures, &retry, &h.FixHint); err != nil {
			return nil, err
		}
		h.Ref.Key, h.Status, h.Since = domain.ObjectKey(key), domain.HealthStatus(status), fromMS(since)
		h.LastOKAt, h.NextRetryAt = ptrMS(ok), ptrMS(retry)
		out = append(out, h)
	}
	return out, rows.Err()
}

// ── root folders ─────────────────────────────────────────────────────────────

// RootFolder is a library root with its last check.
type RootFolder = domain.RootFolder

// RootFolders stores root folders keyed by path.
type RootFolders struct{ s *Store }

// RootFolders returns the root folder repository.
func (s *Store) RootFolders() RootFolders { return RootFolders{s} }

const upsertRoot = `INSERT INTO root_folders (path, kind, free_space, accessible, writable, problem, managed, last_checked)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (path) DO UPDATE SET kind = excluded.kind, free_space = excluded.free_space,
	accessible = excluded.accessible, writable = excluded.writable, problem = excluded.problem, managed = excluded.managed,
	last_checked = excluded.last_checked`

func putRoot(ctx context.Context, tx *sql.Tx, f RootFolder) error {
	_, err := tx.ExecContext(ctx, upsertRoot, f.Path, string(f.Kind), f.FreeSpace, f.Accessible, f.Writable, string(f.Problem),
		f.Managed, nullMS(f.LastChecked))
	return err
}

// Upsert creates or updates a root folder by path.
func (r RootFolders) Upsert(ctx context.Context, f RootFolder) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error { return putRoot(ctx, tx, f) })
}

// Replace makes the managed (config) folders exactly list in one transaction: managed folders not
// in it are dropped, the others upserted (idempotent).
func (r RootFolders) Replace(ctx context.Context, list []RootFolder) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM root_folders WHERE managed = 1"); err != nil {
			return err
		}
		for _, f := range list {
			if err := putRoot(ctx, tx, f); err != nil {
				return err
			}
		}
		return nil
	})
}

// List returns all root folders by path.
func (r RootFolders) List(ctx context.Context) ([]RootFolder, error) {
	rows, err := r.s.R().QueryContext(ctx, `SELECT path, kind, free_space, accessible, writable, problem, managed, last_checked
		FROM root_folders ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RootFolder{}
	for rows.Next() {
		var f RootFolder
		var kind, problem string
		var checked sql.NullInt64
		if err := rows.Scan(&f.Path, &kind, &f.FreeSpace, &f.Accessible, &f.Writable, &problem, &f.Managed, &checked); err != nil {
			return nil, err
		}
		f.Kind, f.Problem, f.LastChecked = domain.MediaKind(kind), domain.RootFolderProblem(problem), ptrMS(checked)
		out = append(out, f)
	}
	return out, rows.Err()
}

// Delete removes a root folder by path; deleting twice is fine.
func (r RootFolders) Delete(ctx context.Context, path string) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM root_folders WHERE path = ?", path)
		return err
	})
}
