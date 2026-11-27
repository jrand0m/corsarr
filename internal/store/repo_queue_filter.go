package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// QueueFilter narrows a queue listing; zero fields match everything. Without
// states, finished downloads (imported, removed) are left out.
type QueueFilter struct {
	Downloader domain.ObjectKey
	Kind       domain.MediaKind
	States     []domain.DownloadState
}

// ListFiltered pages the queue newest first under a filter.
func (r Queue) ListFiltered(ctx context.Context, f QueueFilter, cursor int64, limit int) ([]StoredDownload, int64, error) {
	if cursor <= 0 {
		cursor = 1<<63 - 1
	}
	where, args := f.where()
	out, err := r.many(ctx, "SELECT "+downloadCols+" FROM downloads WHERE id < ? AND "+where+" ORDER BY id DESC LIMIT ?",
		append(append([]any{cursor}, args...), limit)...)
	var next int64
	if len(out) == limit && limit > 0 {
		next = out[len(out)-1].ID
	}
	return out, next, err
}

// where renders the filter as SQL conditions and their arguments.
func (f QueueFilter) where() (string, []any) {
	conds, args := []string{}, []any{}
	add := func(cond, val string) {
		if val != "" {
			conds, args = append(conds, cond), append(args, val)
		}
	}
	add("downloader = ?", string(f.Downloader))
	add("kind = ?", string(f.Kind))
	if len(f.States) == 0 {
		return strings.Join(append(conds, "state NOT IN ('imported', 'removed')"), " AND "), args
	}
	for _, s := range f.States {
		args = append(args, string(s))
	}
	return strings.Join(append(conds, "state IN ("+placeholders(len(f.States))+")"), " AND "), args
}

// Transition is one recorded state change of a download.
type Transition struct {
	From   domain.DownloadState `json:"from"`
	To     domain.DownloadState `json:"to"`
	Actor  domain.Actor         `json:"requestedBy"`
	Reason string               `json:"reason,omitempty"`
	At     time.Time            `json:"at"`
}

// AddTransition records one state change of a download.
func (r Queue) AddTransition(ctx context.Context, downloadID int64, t Transition) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO download_transitions (download_id, from_state, to_state, actor, reason, at)
			VALUES (?, ?, ?, ?, ?, ?)`, downloadID, string(t.From), string(t.To), string(t.Actor), t.Reason, ms(t.At))
		return err
	})
}

// Transitions returns a download's state history, oldest first.
func (r Queue) Transitions(ctx context.Context, downloadID int64) ([]Transition, error) {
	rows, err := r.s.R().QueryContext(ctx, `SELECT from_state, to_state, actor, reason, at FROM download_transitions
		WHERE download_id = ? ORDER BY id`, downloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Transition{}
	for rows.Next() {
		var t Transition
		var from, to, actor string
		var at int64
		if err := rows.Scan(&from, &to, &actor, &t.Reason, &at); err != nil {
			return nil, err
		}
		t.From, t.To, t.Actor, t.At = domain.DownloadState(from), domain.DownloadState(to), domain.Actor(actor), fromMS(at)
		out = append(out, t)
	}
	return out, rows.Err()
}
