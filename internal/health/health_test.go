package health

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func TestNextTransitions(t *testing.T) {
	h := domain.IntegrationHealth{}
	steps := []struct {
		o    Observation
		want domain.HealthStatus
	}{
		{Observation{OK: true}, domain.HealthOK},
		{Observation{Class: ClassTimeout}, domain.HealthDegraded},
		{Observation{Class: ClassTimeout}, domain.HealthDegraded},
		{Observation{Class: ClassServerError}, domain.HealthFailing},
		{Observation{Class: ClassRateLimited}, domain.HealthBackedOff},
		{Observation{OK: true}, domain.HealthOK},
		{Observation{Class: ClassAuth}, domain.HealthActionNeeded},
		{Observation{Class: ClassChallenge}, domain.HealthActionNeeded},
		{Observation{Class: ClassSessionExpired}, domain.HealthActionNeeded},
		{Observation{Class: ClassIdentityMismatch}, domain.HealthActionNeeded},
		{Observation{Disabled: true}, domain.HealthDisabled},
	}
	for i, s := range steps {
		now := t0.Add(time.Duration(i) * time.Minute)
		prev := h
		h = Next(h, s.o, now)
		if h.Status != s.want {
			t.Fatalf("step %d: %s want %s", i, h.Status, s.want)
		}
		if (prev.Status != h.Status) != h.Since.Equal(now) {
			t.Fatalf("step %d: since %v", i, h.Since)
		}
	}
	h = Next(domain.IntegrationHealth{}, Observation{Class: ClassAuth, Message: "401"}, t0)
	if h.FixHint == "" || h.LastErrorMessage != "401" || h.ConsecutiveFailures != 1 {
		t.Fatalf("%+v", h)
	}
	h = Next(h, Observation{OK: true}, t0)
	if h.FixHint != "" || h.LastErrorClass != "" || h.ConsecutiveFailures != 0 || !h.LastOKAt.Equal(t0) {
		t.Fatalf("ok resets: %+v", h)
	}
}

type pubRec struct {
	mu  sync.Mutex
	evs []domain.Event
}

func (p *pubRec) Publish(ev domain.Event) (domain.Event, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.evs = append(p.evs, ev)
	return ev, nil
}

type saveRec struct{ n int }

func (s *saveRec) Save(context.Context, domain.IntegrationHealth) error { s.n++; return nil }

func TestRegistryPublishesOnStatusChangeAndSavesOnChange(t *testing.T) {
	pub, sv := &pubRec{}, &saveRec{}
	r := New(pub, sv, func() time.Time { return t0 })
	ref := domain.IntegrationRef{Type: "indexer", Key: "knaben"}
	r.OK(ref)
	r.OK(ref) // unchanged: no event, no write
	r.Fail(ref, ClassTimeout, "slow")
	r.Fail(ref, ClassTimeout, "slow") // same status, failure count changed: write only
	r.Fail(ref, ClassAuth, "401")
	if len(pub.evs) != 3 || pub.evs[0].Type != domain.EvHealthChanged {
		t.Fatalf("events: %d", len(pub.evs))
	}
	if sv.n != 4 {
		t.Fatalf("saves: %d", sv.n)
	}
	if !r.Blocked(ref) {
		t.Fatal("auth failure must block blind retries")
	}
	d := pub.evs[2].Data.(map[string]any)
	if d["from"] != domain.HealthDegraded || d["to"] != domain.HealthActionNeeded {
		t.Fatalf("data: %v", d)
	}
}

func TestRegistryLoadGetList(t *testing.T) {
	r := New(nil, nil, nil)
	a := domain.IntegrationHealth{Ref: domain.IntegrationRef{Type: "notifier", Key: "ntfy"}, Status: domain.HealthOK}
	b := domain.IntegrationHealth{Ref: domain.IntegrationRef{Type: "indexer", Key: "z"}, Status: domain.HealthFailing}
	c := domain.IntegrationHealth{Ref: domain.IntegrationRef{Type: "indexer", Key: "a"}, Status: domain.HealthOK}
	r.Load([]domain.IntegrationHealth{a, b, c})
	l := r.List()
	if len(l) != 3 || l[0].Ref.Key != "a" || l[1].Ref.Key != "z" || l[2].Ref.Type != "notifier" {
		t.Fatalf("%v", l)
	}
	if h, ok := r.Get(b.Ref); !ok || h.Status != domain.HealthFailing {
		t.Fatal("get")
	}
	r.Report(b.Ref, Observation{Disabled: true})
	if h, _ := r.Get(b.Ref); h.Status != domain.HealthDisabled {
		t.Fatal("disabled")
	}
}
