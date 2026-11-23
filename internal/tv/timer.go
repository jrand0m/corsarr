package tv

import (
	"context"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// maxTimer caps one timer wait so clock jumps (suspend, NTP) self-correct.
const maxTimer = 24 * time.Hour

// Start arms the availability timer: item.available is published once per
// episode at its availability time from a timer, never by polling.
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

// Kick re-evaluates pending episodes now (after add, edit or refresh).
func (s *Service) Kick() {
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	next, err := s.CheckAvailability(ctx)
	s.logErr(err)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if next.IsZero() || s.ctx.Err() != nil {
		return
	}
	s.timer = time.AfterFunc(min(max(next.Sub(s.d.Now()), 0), maxTimer), s.Kick)
}

// CheckAvailability publishes item.available (reason "aired") for every
// pending episode whose time has come and returns the next one (zero when
// none).
func (s *Service) CheckAvailability(ctx context.Context) (time.Time, error) {
	s.checkMu.Lock() // the timer and Kick from edits may race: publish once
	defer s.checkMu.Unlock()
	now := s.d.Now()
	var due []domain.Episode
	var next time.Time
	err := s.d.Repo.PendingAvailability(ctx, func(e domain.Episode) error {
		at, _ := AvailableAt(e) // pending episodes have an air date
		switch {
		case !at.After(now):
			due = append(due, e)
		case next.IsZero() || at.Before(next):
			next = at
		}
		return nil
	})
	for _, e := range due {
		if err := s.d.Repo.SetAvailablePublished(ctx, e.ID, true); err != nil {
			return next, err
		}
		s.publish(domain.EvItemAvailable, "system", []domain.EventItem{s.EpisodeItem(ctx, e.ID)},
			domain.ItemEventData{Reason: "aired", Monitored: true})
	}
	return next, err
}
