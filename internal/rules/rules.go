// Package rules is the kernel rule runner: every automatic destructive action is a named Rule; the runner
// persists a RuleAction (evidence, planned ops) before acting, re-checks live
// state right before mutating, enforces the per-run cap, honours dry-run,
// stores the undo payload and publishes rule.acted / rule.run_completed /
// rule.undone. Safety actions are not rules and never come here.
package rules

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

// Mode is a rule's mode.
type Mode string

const (
	ModeOff    Mode = "off"
	ModeDryRun Mode = "dry-run"
	ModeLive   Mode = "live"
)

// Classifications of a candidate.
const (
	ClassWouldAct = "would-act"
	ClassActed    = "acted"
	ClassDeferred = "deferred"
	ClassAmbig    = "ambiguous"
)

// UndoWindow is how long an acted action can be undone.
const UndoWindow = 30 * 24 * time.Hour

// Candidate is one thing a rule may act on.
type Candidate struct {
	Key      string
	Evidence map[string]any
	Ops      []string
}

// Rule is a named destructive action.
type Rule struct {
	ID          string
	Description string
	Mode        Mode
	Cap         int // per run; 0 = unlimited
	Find        func(ctx context.Context) ([]Candidate, error)
	// Recheck re-derives live state right before acting; false aborts with
	// "state changed". nil means the candidate cannot go stale.
	Recheck func(ctx context.Context, c Candidate) (bool, error)
	// Act mutates and returns the undo payload ("" = cannot be undone).
	Act  func(ctx context.Context, c Candidate) (string, error)
	Undo func(ctx context.Context, payload string) error
}

// Store is the persistence the runner owns (store.Rules implements it).
type Store interface {
	StartRun(ctx context.Context, ruleID, mode string, at time.Time) (int64, error)
	FinishRun(ctx context.Context, id int64, counts map[string]int, at time.Time) error
	AddAction(ctx context.Context, a store.RuleAction) (int64, error)
	FinishAction(ctx context.Context, a store.RuleAction) error
	Action(ctx context.Context, id int64) (store.RuleAction, bool, error)
	Actions(ctx context.Context, cursor int64, limit int) ([]store.RuleAction, int64, error)
	ActedBefore(ctx context.Context, ruleID string, t time.Time, fn func(store.RuleAction) error) error
}

// Publisher is the event bus.
type Publisher interface {
	Publish(domain.Event) (domain.Event, error)
}

// Runner executes registered rules.
type Runner struct {
	st    Store
	pub   Publisher
	now   func() time.Time
	mu    sync.Mutex
	rules map[string]Rule
	modes map[string]Mode // configured modes overriding each rule's default
}

// New returns a runner; now may be nil.
func New(st Store, pub Publisher, now func() time.Time) *Runner {
	if now == nil {
		now = time.Now
	}
	return &Runner{st: st, pub: pub, now: now, rules: map[string]Rule{}}
}

// ErrUnknownRule and friends are the runner's refusals.
var (
	ErrUnknownRule = errors.New("unknown rule")
	ErrNoUndo      = errors.New("action cannot be undone")
)

// Register adds a rule; ids are unique.
func (r *Runner) Register(rule Rule) error {
	if rule.ID == "" || rule.Act == nil {
		return fmt.Errorf("rule needs an id and an action")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.rules[rule.ID]; dup {
		return fmt.Errorf("rule %s registered twice", rule.ID)
	}
	r.rules[rule.ID] = rule
	return nil
}

func (r *Runner) rule(id string) (Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rule, ok := r.rules[id]
	if !ok {
		return Rule{}, fmt.Errorf("%w: %s", ErrUnknownRule, id)
	}
	if m, ok := r.modes[id]; ok {
		rule.Mode = m
	}
	return rule, nil
}

// SetModes replaces the configured mode overrides (rule id → mode); rules
// not named run in their default mode. It returns the ids that name no
// registered rule. A run already started keeps the mode it began with.
func (r *Runner) SetModes(modes map[string]Mode) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modes = map[string]Mode{}
	var unknown []string
	for id, m := range modes {
		if _, ok := r.rules[id]; !ok {
			unknown = append(unknown, id)
			continue
		}
		r.modes[id] = m
	}
	sort.Strings(unknown)
	return unknown
}

// IDs lists the registered rule ids, sorted.
func (r *Runner) IDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.rules))
	for id := range r.rules {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Mode returns the mode a rule runs in now.
func (r *Runner) Mode(id string) (Mode, error) {
	rule, err := r.rule(id)
	return rule.Mode, err
}

// Run evaluates a rule's candidates and acts on them per its mode and cap.
// It returns the counts per classification.
func (r *Runner) Run(ctx context.Context, id string) (map[string]int, error) {
	rule, err := r.rule(id)
	if err != nil || rule.Mode == ModeOff || rule.Find == nil {
		return map[string]int{}, err
	}
	cands, err := rule.Find(ctx)
	if err != nil {
		return nil, err
	}
	return r.execute(ctx, rule, cands)
}

// Apply runs a rule on one candidate found by an event (e.g. an upgrade's
// replaced file) and returns the stored action.
func (r *Runner) Apply(ctx context.Context, id string, c Candidate) (store.RuleAction, error) {
	rule, err := r.rule(id)
	if err != nil {
		return store.RuleAction{}, err
	}
	var last store.RuleAction
	_, err = r.run(ctx, rule, []Candidate{c}, func(a store.RuleAction) { last = a })
	return last, err
}

func (r *Runner) execute(ctx context.Context, rule Rule, cands []Candidate) (map[string]int, error) {
	return r.run(ctx, rule, cands, func(store.RuleAction) {})
}

func (r *Runner) run(ctx context.Context, rule Rule, cands []Candidate, seen func(store.RuleAction)) (map[string]int, error) {
	runID, err := r.st.StartRun(ctx, rule.ID, string(rule.Mode), r.now())
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, c := range cands {
		a, err := r.one(ctx, rule, runID, c, counts[ClassActed])
		if err != nil {
			return counts, err
		}
		counts[a.Classification]++
		seen(a)
	}
	if err := r.st.FinishRun(ctx, runID, counts, r.now()); err != nil {
		return counts, err
	}
	r.publish(domain.EvRuleRunCompleted, domain.RuleEventData{RuleID: rule.ID, Mode: string(rule.Mode), Counts: counts})
	return counts, nil
}

// one persists the action first, then acts (live) or records would-act.
func (r *Runner) one(ctx context.Context, rule Rule, runID int64, c Candidate, acted int) (store.RuleAction, error) {
	a := store.RuleAction{RunID: runID, RuleID: rule.ID, Mode: string(rule.Mode), Candidate: c.Key, Evidence: c.Evidence,
		Ops: c.Ops, Classification: classify(rule, acted), CreatedAt: r.now()}
	a.Status = a.Classification
	if a.Classification == ClassActed {
		a.Status = "planned"
	}
	id, err := r.st.AddAction(ctx, a)
	if err != nil {
		return a, err
	}
	a.ID = id
	if a.Classification != ClassActed {
		return a, nil
	}
	return r.act(ctx, rule, a, c)
}

func classify(rule Rule, acted int) string {
	switch {
	case rule.Mode != ModeLive:
		return ClassWouldAct
	case rule.Cap > 0 && acted >= rule.Cap:
		return ClassDeferred
	}
	return ClassActed
}

func (r *Runner) act(ctx context.Context, rule Rule, a store.RuleAction, c Candidate) (store.RuleAction, error) {
	if rule.Recheck != nil {
		if still, err := rule.Recheck(ctx, c); err != nil || !still {
			a.Classification, a.Status, a.Error = ClassAmbig, "aborted", "state changed"
			return a, r.st.FinishAction(ctx, a)
		}
	}
	undo, err := rule.Act(ctx, c)
	at := r.now()
	a.ActedAt = &at
	if err != nil {
		a.Status, a.Error = "failed", err.Error()
		return a, r.st.FinishAction(ctx, a)
	}
	a.Status, a.Undo, a.UndoAvailable = "acted", undo, undo != "" && rule.Undo != nil
	if err := r.st.FinishAction(ctx, a); err != nil {
		return a, err
	}
	r.publish(domain.EvRuleActed, domain.RuleEventData{RuleID: rule.ID, Mode: string(rule.Mode), ActionID: a.ID,
		Classification: ClassActed, Evidence: c.Evidence, Ops: c.Ops, UndoAvailable: a.UndoAvailable})
	return a, nil
}

// Undo reverses an acted action within the undo window.
func (r *Runner) Undo(ctx context.Context, actionID int64) (store.RuleAction, error) {
	a, ok, err := r.st.Action(ctx, actionID)
	if err != nil || !ok {
		return a, errors.Join(fmt.Errorf("%w: no action %d", ErrUnknownRule, actionID), err)
	}
	rule, err := r.rule(a.RuleID)
	if err != nil {
		return a, err
	}
	if !r.undoable(a, rule) {
		return a, ErrNoUndo
	}
	if err := rule.Undo(ctx, a.Undo); err != nil {
		return a, err
	}
	at := r.now()
	a.Status, a.UndoneAt, a.UndoAvailable = "undone", &at, false
	if err := r.st.FinishAction(ctx, a); err != nil {
		return a, err
	}
	r.publish(domain.EvRuleUndone, domain.RuleEventData{RuleID: a.RuleID, Mode: a.Mode, ActionID: a.ID})
	return a, nil
}

func (r *Runner) undoable(a store.RuleAction, rule Rule) bool {
	if !a.UndoAvailable || rule.Undo == nil || a.ActedAt == nil {
		return false
	}
	return r.now().Sub(*a.ActedAt) <= UndoWindow
}

// Actions pages stored actions newest first.
func (r *Runner) Actions(ctx context.Context, cursor int64, limit int) ([]store.RuleAction, int64, error) {
	return r.st.Actions(ctx, cursor, limit)
}

func (r *Runner) publish(t domain.EventType, data domain.RuleEventData) {
	if r.pub != nil {
		_, _ = r.pub.Publish(domain.Event{Type: t, Actor: "automation", Data: data})
	}
}
