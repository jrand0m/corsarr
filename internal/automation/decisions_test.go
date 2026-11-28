package automation

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func rejected(rules ...string) Decided {
	var rs []domain.Reason
	for _, r := range rules {
		rs = append(rs, domain.Reason{RuleID: r, Category: domain.ReasonPreference, Message: r + " failed"})
	}
	return Decided{Decision: domain.Decision{Reasons: rs}}
}

func TestRejectionsCountsTheTopRules(t *testing.T) {
	ds := []Decided{rejected("size", "quality"), rejected("size"), rejected("lang"), {Decision: domain.Decision{Accepted: true,
		Reasons: []domain.Reason{{RuleID: "ignored"}}}}}
	for i := range TopReasons + 2 {
		ds = append(ds, rejected(fmt.Sprintf("r%d", i)))
	}
	got := Rejections(ds)
	if len(got) != TopReasons || got[0].RuleID != "size" || got[0].Count != 2 || got[0].Message != "size failed" || got[1].Count != 1 {
		t.Fatalf("%+v", got)
	}
	for _, rc := range got {
		if rc.RuleID == "ignored" {
			t.Fatal("accepted decisions are not counted")
		}
	}
	if len(Rejections(nil)) != 0 {
		t.Fatal("no decisions, no reasons")
	}
}

type memStates struct {
	m   map[domain.ItemRef]domain.SearchState
	err error
}

func (s *memStates) State(_ context.Context, ref domain.ItemRef) (domain.SearchState, error) {
	return s.m[ref], s.err
}

func (s *memStates) SaveState(_ context.Context, st domain.SearchState) error {
	s.m[st.Item] = st
	return nil
}

func TestRecordSearch(t *testing.T) {
	st := &memStates{m: map[domain.ItemRef]domain.SearchState{}}
	ref := domain.ItemRef{Kind: domain.KindTV, ID: 3}
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	res := domain.SearchEventData{Releases: 2, Rejections: []domain.ReasonCount{{RuleID: "size", Count: 2}}}
	for range 2 {
		if err := RecordSearch(context.Background(), st, ref, at, res); err != nil {
			t.Fatal(err)
		}
	}
	got := st.m[ref]
	if got.Misses != 2 || !got.LastSearched.Equal(at) || got.LastResult.Rejections[0].RuleID != "size" || !got.NextEligible.After(at) {
		t.Fatalf("%+v", got)
	}
	res.Grabbed = "X"
	_ = RecordSearch(context.Background(), st, ref, at, res)
	if st.m[ref].Misses != 0 {
		t.Fatal("a grab resets the misses")
	}
	st.err = errors.New("db down")
	if RecordSearch(context.Background(), st, ref, at, res) == nil {
		t.Fatal("a read failure is reported")
	}
}
