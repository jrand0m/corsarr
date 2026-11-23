package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// RuleAction is one persisted rule action.
type RuleAction struct {
	ID             int64          `json:"id"`
	RunID          int64          `json:"runId"`
	RuleID         string         `json:"ruleId"`
	Mode           string         `json:"mode"`
	Candidate      string         `json:"candidate"`
	Classification string         `json:"classification"`
	Evidence       map[string]any `json:"evidence,omitempty"`
	Ops            []string       `json:"ops"`
	Undo           string         `json:"-"` // opaque undo payload of the rule
	UndoAvailable  bool           `json:"undoAvailable"`
	Status         string         `json:"status"` // planned | acted | failed | undone | would-act | deferred | aborted
	Error          string         `json:"error,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
	ActedAt        *time.Time     `json:"actedAt,omitempty"`
	UndoneAt       *time.Time     `json:"undoneAt,omitempty"`
}

// Rules stores RuleRun and RuleAction (owner: the kernel rule runner).
type Rules struct{ s *Store }

// Rules returns the rule repository.
func (s *Store) Rules() Rules { return Rules{s} }

// StartRun opens a run and returns its id.
func (r Rules) StartRun(ctx context.Context, ruleID, mode string, at time.Time) (int64, error) {
	var id int64
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "INSERT INTO rule_runs (rule_id, mode, started_at) VALUES (?, ?, ?) RETURNING id",
			ruleID, mode, ms(at)).Scan(&id)
	})
	return id, err
}

// FinishRun closes a run with its counts per classification.
func (r Rules) FinishRun(ctx context.Context, id int64, counts map[string]int, at time.Time) error {
	return r.s.exec(ctx, "UPDATE rule_runs SET finished_at = ?, counts = ? WHERE id = ?", ms(at), jsonText(counts), id)
}

// AddAction persists an action before anything is mutated and returns its id.
func (r Rules) AddAction(ctx context.Context, a RuleAction) (int64, error) {
	var id int64
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `INSERT INTO rule_actions (run_id, rule_id, mode, candidate, classification, evidence,
			ops, undo, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`, a.RunID, a.RuleID, a.Mode,
			a.Candidate, a.Classification, jsonText(a.Evidence), jsonText(a.Ops), a.Undo, a.Status, ms(a.CreatedAt)).Scan(&id)
	})
	return id, err
}

// FinishAction records the outcome of acting (or undoing).
func (r Rules) FinishAction(ctx context.Context, a RuleAction) error {
	return r.s.exec(ctx, `UPDATE rule_actions SET classification = ?, undo = ?, status = ?, error = ?, acted_at = ?,
		undone_at = ? WHERE id = ?`, a.Classification, a.Undo, a.Status, a.Error, nullMS(a.ActedAt), nullMS(a.UndoneAt), a.ID)
}

const actionCols = `id, run_id, rule_id, mode, candidate, classification, evidence, ops, undo, status, error, created_at,
	acted_at, undone_at`

func scanAction(sc scanner) (RuleAction, error) {
	var a RuleAction
	var ev, ops string
	var created int64
	var acted, undone sql.NullInt64
	err := sc.Scan(&a.ID, &a.RunID, &a.RuleID, &a.Mode, &a.Candidate, &a.Classification, &ev, &ops, &a.Undo, &a.Status,
		&a.Error, &created, &acted, &undone)
	_ = json.Unmarshal([]byte(ev), &a.Evidence) // written by this repository
	_ = json.Unmarshal([]byte(ops), &a.Ops)
	a.CreatedAt, a.ActedAt, a.UndoneAt = fromMS(created), ptrMS(acted), ptrMS(undone)
	a.UndoAvailable = a.Undo != "" && a.Status == "acted"
	return a, err
}

// Action returns one action.
func (r Rules) Action(ctx context.Context, id int64) (RuleAction, bool, error) {
	a, err := scanAction(r.s.R().QueryRowContext(ctx, "SELECT "+actionCols+" FROM rule_actions WHERE id = ?", id))
	ok, err := found(err)
	return a, ok, err
}

// Actions pages actions newest first.
func (r Rules) Actions(ctx context.Context, cursor int64, limit int) ([]RuleAction, int64, error) {
	if cursor <= 0 {
		cursor = 1<<63 - 1
	}
	rows, err := r.s.R().QueryContext(ctx, "SELECT "+actionCols+" FROM rule_actions WHERE id < ? ORDER BY id DESC LIMIT ?", cursor, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []RuleAction{}
	for rows.Next() {
		a, err := scanAction(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	var next int64
	if len(out) == limit && limit > 0 {
		next = out[len(out)-1].ID
	}
	return out, next, rows.Err()
}

// ActedBefore streams acted actions of a rule created before t (e.g. recycle
// entries old enough to purge).
func (r Rules) ActedBefore(ctx context.Context, ruleID string, t time.Time, fn func(RuleAction) error) error {
	rows, err := r.s.R().QueryContext(ctx, "SELECT "+actionCols+` FROM rule_actions WHERE rule_id = ? AND status = 'acted'
		AND created_at < ? ORDER BY id`, ruleID, ms(t))
	if err != nil {
		return err
	}
	var list []RuleAction
	for rows.Next() {
		a, err := scanAction(rows)
		if err != nil {
			rows.Close()
			return err
		}
		list = append(list, a)
	}
	rows.Close()
	for _, a := range list {
		if err := fn(a); err != nil {
			return err
		}
	}
	return rows.Err()
}
