// Package health is the IntegrationHealth registry: one record per integration, a pure transition rule,
// `health.changed` on every status change, persistence on change only.
package health

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Failure classes of an integration's health.
const (
	ClassAuth             = "auth"
	ClassRateLimited      = "rate_limited"
	ClassTimeout          = "timeout"
	ClassUnreachable      = "unreachable"
	ClassParse            = "parse"
	ClassChallenge        = "challenge"
	ClassServerError      = "server_error"
	ClassIdentityMismatch = "identity_mismatch"
	ClassPath             = "path"
	ClassDiskFull         = "disk_full"
	ClassConfig           = "config"
	ClassSessionExpired   = "session_expired"
)

// actionNeeded classes stop blind retries until the owner acts.
var actionNeeded = map[string]bool{ClassAuth: true, ClassChallenge: true, ClassSessionExpired: true, ClassIdentityMismatch: true}

var hints = map[string]string{
	ClassAuth:             "Check the credentials or API key in the integration settings.",
	ClassChallenge:        "The site is behind anti-bot protection (such as Cloudflare or DDoS-Guard), not rejecting the credentials; open it in a browser, wait for the protection to relax or configure a solver.",
	ClassSessionExpired:   "Sign in again from the integration settings.",
	ClassIdentityMismatch: "The server answering is not the one configured; check the URL.",
	ClassRateLimited:      "The service is rate limiting; corsarr retries after the indicated time.",
	ClassUnreachable:      "Check the URL, DNS and that the service is running.",
	ClassTimeout:          "The service is slow or overloaded; corsarr will retry.",
	ClassDiskFull:         "Free disk space on the affected path.",
	ClassPath:             "Check that the path exists and corsarr may access it.",
	ClassConfig:           "Fix the configuration error shown and reload.",
}

// FailingAfter is the consecutive-failure count at which degraded → failing.
const FailingAfter = 3

// Observation is one result reported by an integration.
type Observation struct {
	OK        bool
	Disabled  bool
	Class     string
	Message   string
	NextRetry *time.Time
}

// Next is the pure transition rule.
func Next(cur domain.IntegrationHealth, o Observation, now time.Time) domain.IntegrationHealth {
	next := cur
	switch {
	case o.Disabled:
		next.Status, next.NextRetryAt = domain.HealthDisabled, nil
	case o.OK:
		next.Status, next.ConsecutiveFailures, next.NextRetryAt = domain.HealthOK, 0, nil
		next.LastErrorClass, next.LastErrorMessage, next.FixHint = "", "", ""
		next.LastOKAt = &now
	default:
		next.ConsecutiveFailures++
		next.LastErrorClass, next.LastErrorMessage, next.NextRetryAt = o.Class, o.Message, o.NextRetry
		next.FixHint = hints[o.Class]
		next.Status = failStatus(o.Class, next.ConsecutiveFailures)
	}
	if next.Status != cur.Status || cur.Since.IsZero() {
		next.Since = now
	}
	return next
}

func failStatus(class string, failures int) domain.HealthStatus {
	switch {
	case actionNeeded[class]:
		return domain.HealthActionNeeded
	case class == ClassRateLimited:
		return domain.HealthBackedOff
	case failures >= FailingAfter:
		return domain.HealthFailing
	}
	return domain.HealthDegraded
}

// Publisher is the event bus.
type Publisher interface {
	Publish(domain.Event) (domain.Event, error)
}

// Saver persists records (store.Health implements it).
type Saver interface {
	Save(ctx context.Context, h domain.IntegrationHealth) error
}

// Registry is the single writer of integration health.
type Registry struct {
	mu   sync.Mutex
	m    map[domain.IntegrationRef]domain.IntegrationHealth
	pub  Publisher
	save Saver
	now  func() time.Time
}

// New builds a registry; pub and save may be nil (tests).
func New(pub Publisher, save Saver, now func() time.Time) *Registry {
	if now == nil {
		now = time.Now
	}
	return &Registry{m: map[domain.IntegrationRef]domain.IntegrationHealth{}, pub: pub, save: save, now: now}
}

// Load seeds records persisted by a previous run.
func (r *Registry) Load(list []domain.IntegrationHealth) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, h := range list {
		r.m[h.Ref] = h
	}
}

// Report applies an observation and returns the new record. A status change
// publishes health.changed; any change except a refreshed LastOKAt is saved.
func (r *Registry) Report(ref domain.IntegrationRef, o Observation) domain.IntegrationHealth {
	r.mu.Lock()
	cur, known := r.m[ref]
	if !known {
		cur.Ref = ref
	}
	next := Next(cur, o, r.now().UTC())
	r.m[ref] = next
	r.mu.Unlock()
	changed := next.Status != cur.Status || !known
	if changed && r.pub != nil {
		_, _ = r.pub.Publish(domain.Event{Type: domain.EvHealthChanged, Actor: "system", Data: map[string]any{
			"ref": ref, "from": cur.Status, "to": next.Status, "class": next.LastErrorClass, "message": next.LastErrorMessage,
		}})
	}
	if r.save != nil && (changed || next.ConsecutiveFailures != cur.ConsecutiveFailures) {
		_ = r.save.Save(context.Background(), next)
	}
	return next
}

// OK reports a success.
func (r *Registry) OK(ref domain.IntegrationRef) { r.Report(ref, Observation{OK: true}) }

// Fail reports a failure of a class.
func (r *Registry) Fail(ref domain.IntegrationRef, class, msg string) {
	r.Report(ref, Observation{Class: class, Message: msg})
}

// Get returns one record.
func (r *Registry) Get(ref domain.IntegrationRef) (domain.IntegrationHealth, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.m[ref]
	return h, ok
}

// Blocked reports whether calls should not be retried blindly (action needed).
func (r *Registry) Blocked(ref domain.IntegrationRef) bool {
	h, _ := r.Get(ref)
	return h.Status == domain.HealthActionNeeded
}

// List returns every record sorted by type and key.
func (r *Registry) List() []domain.IntegrationHealth {
	r.mu.Lock()
	out := make([]domain.IntegrationHealth, 0, len(r.m))
	for _, h := range r.m {
		out = append(out, h)
	}
	r.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ref.Type != out[j].Ref.Type {
			return out[i].Ref.Type < out[j].Ref.Type
		}
		return out[i].Ref.Key < out[j].Ref.Key
	})
	return out
}
