package automation

import (
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
)

// OutcomeStatus is what automatic search does with one release.
type OutcomeStatus string

const (
	OutcomeGrab      OutcomeStatus = "grab"      // the single pick
	OutcomeShadow    OutcomeStatus = "shadow"    // the pick, recorded only (shadow mode)
	OutcomeOutranked OutcomeStatus = "outranked" // accepted, another release ranks higher
	OutcomeDeclined  OutcomeStatus = "declined"  // rejected by at least one rule
)

// Outcome is one release's automation outcome; LostTo is set for outranked releases.
type Outcome struct {
	Status OutcomeStatus         `json:"status"`
	LostTo *decide.RankCriterion `json:"lostTo,omitempty"`
}

// Verdict is what automatic search would do with a whole result set.
type Verdict struct {
	Mode     domain.KindMode    `json:"mode"`
	Pick     string             `json:"pick,omitempty"`
	Outcomes map[string]Outcome `json:"outcomes"`
}

// Outcomes applies the same choice automatic search makes (the best accepted
// release after ranking) to a result set; in shadow mode the pick is only
// recorded.
func Outcomes(ds []Decided, mode domain.KindMode) Verdict {
	v := Verdict{Mode: mode, Outcomes: make(map[string]Outcome, len(ds))}
	top, _, ok := best(ds)
	if ok {
		v.Pick = top.Decision.ReleaseID
	}
	for _, d := range ds {
		v.Outcomes[d.Decision.ReleaseID] = outcomeOf(d.Decision, top.Decision, ok, mode)
	}
	return v
}

func outcomeOf(d, pick domain.Decision, picked bool, mode domain.KindMode) Outcome {
	switch {
	case !d.Accepted:
		return Outcome{Status: OutcomeDeclined}
	case picked && d.ReleaseID == pick.ReleaseID && mode == domain.ModeShadow:
		return Outcome{Status: OutcomeShadow}
	case picked && d.ReleaseID == pick.ReleaseID:
		return Outcome{Status: OutcomeGrab}
	}
	lost := decide.LostTo(d, pick)
	return Outcome{Status: OutcomeOutranked, LostTo: &lost}
}
