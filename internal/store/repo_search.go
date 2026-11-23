package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Searches stores per-item search state and the per-indexer query log
// (writer: automation).
type Searches struct{ s *Store }

// Searches returns the search repository.
func (s *Store) Searches() Searches { return Searches{s} }

// SaveState upserts an item's search state.
func (r Searches) SaveState(ctx context.Context, st domain.SearchState) error {
	result := ""
	if st.LastResult != nil {
		result = jsonText(st.LastResult)
	}
	return r.s.exec(ctx, `INSERT INTO search_state (item_kind, item_id, last_searched, next_eligible, misses, last_result)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (item_kind, item_id) DO UPDATE SET last_searched = excluded.last_searched,
		next_eligible = excluded.next_eligible, misses = excluded.misses, last_result = excluded.last_result`,
		string(st.Item.Kind), st.Item.ID, nullMS(st.LastSearched), nullMS(st.NextEligible), st.Misses, result)
}

// State returns an item's search state (zero state with the ref when never searched).
func (r Searches) State(ctx context.Context, item domain.ItemRef) (domain.SearchState, error) {
	st, err := scanState(r.s.R().QueryRowContext(ctx, `SELECT item_kind, item_id, last_searched, next_eligible, misses, last_result
		FROM search_state WHERE item_kind = ? AND item_id = ?`, string(item.Kind), item.ID))
	if ok, err := found(err); !ok {
		return domain.SearchState{Item: item}, err
	}
	return st, nil
}

// States returns the search state of every searched item of a kind by id.
func (r Searches) States(ctx context.Context, kind domain.MediaKind) (map[int64]domain.SearchState, error) {
	rows, err := r.s.R().QueryContext(ctx, `SELECT item_kind, item_id, last_searched, next_eligible, misses, last_result
		FROM search_state WHERE item_kind = ?`, string(kind))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]domain.SearchState{}
	for rows.Next() {
		st, err := scanState(rows)
		if err != nil {
			return nil, err
		}
		out[st.Item.ID] = st
	}
	return out, rows.Err()
}

func scanState(sc scanner) (domain.SearchState, error) {
	var st domain.SearchState
	var kind, result string
	var last, next sql.NullInt64
	if err := sc.Scan(&kind, &st.Item.ID, &last, &next, &st.Misses, &result); err != nil {
		return st, err
	}
	st.Item.Kind, st.LastSearched, st.NextEligible = domain.MediaKind(kind), ptrMS(last), ptrMS(next)
	if result != "" {
		var res domain.SearchEventData
		if json.Unmarshal([]byte(result), &res) == nil {
			st.LastResult = &res
		}
	}
	return st, nil
}

// RecordQuery logs one query sent to an indexer.
func (r Searches) RecordQuery(ctx context.Context, indexer domain.ObjectKey, at time.Time) error {
	return r.s.exec(ctx, "INSERT INTO indexer_queries (indexer, at) VALUES (?, ?)", string(indexer), ms(at))
}

// Queries counts an indexer's queries since t and returns the oldest of them
// (zero when none), from which the window frees up again.
func (r Searches) Queries(ctx context.Context, indexer domain.ObjectKey, since time.Time) (int, time.Time, error) {
	var n int
	var oldest sql.NullInt64
	err := r.s.R().QueryRowContext(ctx, "SELECT COUNT(*), MIN(at) FROM indexer_queries WHERE indexer = ? AND at >= ?",
		string(indexer), ms(since)).Scan(&n, &oldest)
	if !oldest.Valid {
		return n, time.Time{}, err
	}
	return n, fromMS(oldest.Int64), err
}

// PruneQueries drops query log rows older than t.
func (r Searches) PruneQueries(ctx context.Context, before time.Time) error {
	return r.s.exec(ctx, "DELETE FROM indexer_queries WHERE at < ?", ms(before))
}
