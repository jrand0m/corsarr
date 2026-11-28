package automation

import (
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
)

func decided(id string, accepted bool, key string) Decided {
	return Decided{Release: domain.Release{ID: id}, Decision: domain.Decision{ReleaseID: id, Accepted: accepted, RankKey: key}}
}

func TestOutcomes(t *testing.T) {
	ds := []Decided{
		decided("low-quality", true, "005.500000.01"),
		decided("pick", true, "009.500000.01"),
		decided("fewer-formats", true, "009.499950.01"),
		decided("rejected", false, "010.500000.01"),
	}
	v := Outcomes(ds, domain.ModeActive)
	if v.Pick != "pick" || v.Mode != domain.ModeActive || len(v.Outcomes) != 4 {
		t.Fatalf("verdict %+v", v)
	}
	want := map[string]OutcomeStatus{"pick": OutcomeGrab, "low-quality": OutcomeOutranked, "fewer-formats": OutcomeOutranked, "rejected": OutcomeDeclined}
	for id, st := range want {
		if v.Outcomes[id].Status != st {
			t.Errorf("%s: %s, want %s", id, v.Outcomes[id].Status, st)
		}
	}
	if c := v.Outcomes["low-quality"].LostTo; c == nil || c.Criterion != "quality" {
		t.Errorf("low-quality lost to %+v", c)
	}
	if c := v.Outcomes["fewer-formats"].LostTo; c == nil || c.Criterion != "format_score" {
		t.Errorf("fewer-formats lost to %+v", c)
	}
	if v.Outcomes["rejected"].LostTo != nil || v.Outcomes["pick"].LostTo != nil {
		t.Error("only outranked releases name a criterion")
	}
}

func TestOutcomesShadowAndNothingAccepted(t *testing.T) {
	ds := []Decided{decided("a", true, "001"), decided("b", false, "002")}
	if v := Outcomes(ds, domain.ModeShadow); v.Outcomes["a"].Status != OutcomeShadow || v.Pick != "a" {
		t.Fatalf("shadow %+v", v)
	}
	v := Outcomes([]Decided{decided("b", false, "002")}, domain.ModeActive)
	if v.Pick != "" || v.Outcomes["b"].Status != OutcomeDeclined {
		t.Fatalf("nothing accepted %+v", v)
	}
}
