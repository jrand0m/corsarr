package movies

import (
	"context"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// maxTimer caps one timer wait so clock jumps (suspend, NTP) self-correct.
const maxTimer = 24 * time.Hour

// Start arms the availability timer: item.available is published once per
// movie from a timer at the exact date, never by polling.
func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()
	s.Kick()
}

// Stop disarms the timer.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

// Kick re-evaluates pending movies now (after add or edit).
func (s *Service) Kick() {
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	next, err := s.CheckAvailability(ctx)
	s.logErr(err)
	s.arm(next)
}

func (s *Service) arm(next time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if next.IsZero() || s.ctx.Err() != nil {
		return
	}
	wait := min(max(next.Sub(s.d.Now()), 0), maxTimer)
	s.timer = time.AfterFunc(wait, s.Kick)
}

// CheckAvailability publishes item.available for every pending movie whose
// time has come and returns the next future availability (zero when none).
func (s *Service) CheckAvailability(ctx context.Context) (time.Time, error) {
	s.checkMu.Lock() // the timer and Kick from add/edit may race: publish once
	defer s.checkMu.Unlock()
	now := s.d.Now()
	var due []domain.Movie
	var next time.Time
	err := s.d.Repo.PendingAvailability(ctx, func(m domain.Movie) error {
		at, ok := AvailableAt(m)
		switch {
		case !ok:
		case !at.After(now):
			due = append(due, m)
		case next.IsZero() || at.Before(next):
			next = at
		}
		return nil
	})
	for _, m := range due {
		if err := s.d.Repo.SetAvailablePublished(ctx, m.ID, true); err != nil {
			return next, err
		}
		_, _ = s.d.Pub.Publish(domain.Event{Type: domain.EvItemAvailable, Actor: "system", Kind: domain.KindMovie,
			Items: []domain.EventItem{m.EventItem()}, Data: domain.ItemEventData{Reason: "min_availability", Monitored: true}})
	}
	return next, err
}
