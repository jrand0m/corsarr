package automation

import (
	"context"
	"sort"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// TopReasons is how many rejection rules a search result keeps.
const TopReasons = 5

// Rejections counts the rules that rejected releases in one search, most
// frequent first, at most TopReasons of them.
func Rejections(ds []Decided) []domain.ReasonCount {
	byRule := map[string]*domain.ReasonCount{}
	for _, d := range ds {
		if d.Decision.Accepted {
			continue
		}
		for _, r := range d.Decision.Reasons {
			rc, ok := byRule[r.RuleID]
			if !ok {
				rc = &domain.ReasonCount{RuleID: r.RuleID, Category: r.Category, Message: r.Message}
				byRule[r.RuleID] = rc
			}
			rc.Count++
		}
	}
	out := make([]domain.ReasonCount, 0, len(byRule))
	for _, rc := range byRule {
		out = append(out, *rc)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].RuleID < out[j].RuleID
	})
	return out[:min(len(out), TopReasons)]
}

// StateStore keeps per-item search state (store.Searches).
type StateStore interface {
	State(ctx context.Context, item domain.ItemRef) (domain.SearchState, error)
	SaveState(ctx context.Context, st domain.SearchState) error
}

// RecordSearch stores one search's result for an item: when it ran, what
// it found and why releases were rejected; a search that grabbed nothing
// counts as a miss and pushes the next eligible search out.
func RecordSearch(ctx context.Context, st StateStore, ref domain.ItemRef, at time.Time, res domain.SearchEventData) error {
	cur, err := st.State(ctx, ref)
	if err != nil {
		return err
	}
	cur.Item = ref
	cur.Misses++
	if res.Grabbed != "" {
		cur.Misses = 0
	}
	next := NextEligible(at, cur.Misses)
	cur.LastSearched, cur.NextEligible, cur.LastResult = &at, &next, &res
	return st.SaveState(ctx, cur)
}
