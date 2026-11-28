package httpx

import (
	"hash/fnv"
	"sync"
	"time"
)

// How a ceiling was found.
const (
	SourceAdvertised = "advertised" // the service announced it in its headers
	SourceDeclared   = "declared"   // the plugin knows the service's documented limit
	SourceThrottle   = "throttle"   // the service throttled at this many requests
	SourceProbe      = "probe"      // a careful check found the limit here
)

// ProbeMode says how far a check may step past a service's observed ceiling.
type ProbeMode int

const (
	ProbeNormal ProbeMode = iota // up to +30 %
	ProbeGentle                  // up to +10 %, for services that punish harder
	ProbeOff                     // never checked; only throttles teach it
)

const (
	probeStep     = 0.10
	minStepHold   = 10 * time.Second
	fillShare     = 0.9 // a step counts only when demand really filled it
	recoverySteps = 20  // successes to climb back from a halved pace
	minPaceShare  = 1.0 / 64
)

func (m ProbeMode) maxLevel() float64 {
	switch m {
	case ProbeGentle:
		return 1 + probeStep
	case ProbeOff:
		return 1
	}
	return 1 + 3*probeStep
}

func (m ProbeMode) String() string {
	return [...]string{"normal", "gentle", "off"}[m]
}

// Ceiling is where a service starts throttling: Limit requests per Window.
type Ceiling struct {
	Window time.Duration `json:"window"`
	Limit  float64       `json:"limit"`
	Source string        `json:"source"`
	At     time.Time     `json:"at"`
}

func (c Ceiling) rate() float64 { return c.Limit / c.Window.Seconds() }

func (c Ceiling) authoritative() bool {
	return c.Source == SourceAdvertised || c.Source == SourceDeclared
}

// learnCfg is the resolved Learning.
type learnCfg struct {
	headroom   float64
	probeEvery time.Duration
}

// probe is a running check of one window's ceiling.
type probe struct {
	window   time.Duration
	base     float64 // the ceiling when the check started
	level    float64 // current multiple of base allowed
	passed   float64 // highest level that held without a throttle
	stepEnds time.Time
	hold     time.Duration
}

// learner is one service's view of its rate limit: what it has counted,
// where throttling starts, and the pace it keeps below that.
type learner struct {
	mu           sync.Mutex
	key          string
	counts       []*slidingCount // parallel to countWindows
	ceil         []Ceiling
	caps         map[time.Duration]float64
	pace         float64 // requests per second; 0 = not paced
	next, hold   time.Time
	lastThrottle time.Time
	checkedAt    time.Time // last time the ceiling was found or confirmed
	mode         ProbeMode
	challenged   bool
	probe        *probe
}

func newLearner(key string) *learner {
	l := &learner{key: key, caps: map[time.Duration]float64{}}
	for _, w := range countWindows {
		l.counts = append(l.counts, newSliding(w))
	}
	return l
}

func (l *learner) count(w time.Duration, now time.Time) int {
	for i, cw := range countWindows {
		if cw == w {
			return l.counts[i].count(now)
		}
	}
	return 0
}

func (l *learner) ceiling(w time.Duration) (Ceiling, bool) {
	for _, c := range l.ceil {
		if c.Window == w {
			return c, true
		}
	}
	return Ceiling{}, false
}

func (l *learner) setCeiling(c Ceiling) {
	if limit, ok := l.caps[c.Window]; ok {
		c.Limit = min(c.Limit, limit)
	}
	for i := range l.ceil {
		if l.ceil[i].Window == c.Window {
			l.ceil[i] = c
			return
		}
	}
	l.ceil = append(l.ceil, c)
}

func (l *learner) authoritative() bool {
	for _, c := range l.ceil {
		if c.authoritative() {
			return true
		}
	}
	return false
}

// tightest is the ceiling that allows the fewest requests per second.
func (l *learner) tightest() (Ceiling, bool) {
	var best Ceiling
	for _, c := range l.ceil {
		if best.Window == 0 || c.rate() < best.rate() {
			best = c
		}
	}
	return best, best.Window != 0
}

// maxPace is the fastest pace allowed now: headroom below every ceiling, or
// the probe's level on the window being checked. 0 means nothing is known.
func (l *learner) maxPace(headroom float64) float64 {
	r := 0.0
	for _, c := range l.ceil {
		f := headroom
		if l.probe != nil && c.Window == l.probe.window {
			f = l.probe.level
		}
		if v := c.rate() * f; r == 0 || v < r {
			r = v
		}
	}
	return r
}

func (l *learner) currentPace(cfg learnCfg) float64 {
	m := l.maxPace(cfg.headroom)
	if l.probe != nil {
		return m
	}
	if l.pace == 0 || l.pace > m {
		l.pace = m
	}
	return l.pace
}

// reserve books the next send slot and returns how long to wait for it.
// A request that has to queue is real demand, the only time a check starts.
func (l *learner) reserve(now time.Time, cfg learnCfg) time.Duration {
	start := latest(now, l.hold, l.next)
	if start.After(now) && l.probeDue(now, cfg) {
		l.beginProbe(now)
	}
	if pace := l.currentPace(cfg); pace > 0 {
		l.next = start.Add(time.Duration(float64(time.Second) / pace))
	}
	return start.Sub(now)
}

func latest(ts ...time.Time) time.Time {
	out := ts[0]
	for _, t := range ts[1:] {
		if t.After(out) {
			out = t
		}
	}
	return out
}

// accepted records a request the service answered without throttling and
// lets the pace creep back up. It reports whether a ceiling changed.
func (l *learner) accepted(now time.Time, cfg learnCfg) bool {
	for _, c := range l.counts {
		c.add(now)
	}
	if m := l.maxPace(cfg.headroom); m > 0 && l.probe == nil {
		l.pace = min(m, l.pace+m/recoverySteps)
	}
	return l.stepProbe(now)
}

// throttled handles a throttle signal: wait out Retry-After, learn the
// ceiling unless the service announced it, and halve the pace.
func (l *learner) throttled(now time.Time, retry time.Duration, hasRetry bool, cfg learnCfg) {
	l.lastThrottle = now
	if hasRetry {
		l.hold = latest(l.hold, now.Add(retry))
	}
	switch {
	case l.probe != nil:
		l.endProbe(now, true)
	case !l.authoritative():
		l.learnFromThrottle(now, retry, hasRetry)
	}
	m := l.maxPace(cfg.headroom)
	base := l.pace
	if base == 0 || base > m {
		base = m
	}
	l.pace = max(base/2, m*minPaceShare)
}

func (l *learner) learnFromThrottle(now time.Time, retry time.Duration, hasRetry bool) {
	w := l.throttleWindow(now, retry, hasRetry)
	if n := l.count(w, now); n > 0 {
		l.setCeiling(Ceiling{Window: w, Limit: float64(n), Source: SourceThrottle, At: now})
		l.checkedAt = now
	}
}

// throttleWindow picks the window that ran out: the one Retry-After points
// at, else the shortest one the traffic was not already kept below.
func (l *learner) throttleWindow(now time.Time, retry time.Duration, hasRetry bool) time.Duration {
	if hasRetry {
		return windowFor(retry)
	}
	for _, w := range countWindows {
		if c, ok := l.ceiling(w); !ok || float64(l.count(w, now)) >= c.Limit {
			return w
		}
	}
	return countWindows[len(countWindows)-1]
}

// advertised takes an announced limit as the ceiling and holds sending
// while the service says nothing is left. It reports whether it changed.
func (l *learner) advertised(a advert, now time.Time) bool {
	if a.remaining == 0 && a.reset.After(now) {
		l.hold = latest(l.hold, a.reset)
	}
	if a.limit <= 0 {
		return false
	}
	w := l.advertWindow(a)
	if old, ok := l.advertisedCeiling(); ok && old.Window == w && old.Limit == a.limit {
		return false
	}
	l.keepDeclared()
	l.setCeiling(Ceiling{Window: w, Limit: a.limit, Source: SourceAdvertised, At: now})
	l.checkedAt = now
	return true
}

// advertWindow keeps the longest window seen when it is only guessed from
// a reset, since a reset bounds the window from below.
func (l *learner) advertWindow(a advert) time.Duration {
	if old, ok := l.advertisedCeiling(); ok && a.guessed && old.Window > a.window {
		return old.Window
	}
	return a.window
}

// keepDeclared drops what was learned by observation once a service says
// its limit itself.
func (l *learner) keepDeclared() {
	keep := l.ceil[:0]
	for _, c := range l.ceil {
		if c.Source == SourceDeclared {
			keep = append(keep, c)
		}
	}
	l.ceil, l.probe = keep, nil
}

func (l *learner) advertisedCeiling() (Ceiling, bool) {
	for _, c := range l.ceil {
		if c.Source == SourceAdvertised {
			return c, true
		}
	}
	return Ceiling{}, false
}

// nextProbe is when the observed ceiling is next checked (zero = never).
func (l *learner) nextProbe(cfg learnCfg) time.Time {
	if cfg.probeEvery <= 0 || l.mode.maxLevel() <= 1 || l.challenged || l.authoritative() || len(l.ceil) == 0 {
		return time.Time{}
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(l.key))
	spread := int64(cfg.probeEvery / 10)
	return l.checkedAt.Add(cfg.probeEvery + time.Duration(int64(h.Sum32())%max(spread, 1)))
}

func (l *learner) probeDue(now time.Time, cfg learnCfg) bool {
	next := l.nextProbe(cfg)
	return l.probe == nil && !next.IsZero() && !now.Before(next)
}

func (l *learner) beginProbe(now time.Time) {
	c, _ := l.tightest()
	hold := max(c.Window, minStepHold)
	l.probe = &probe{window: c.Window, base: c.Limit, level: 1, stepEnds: now.Add(hold), hold: hold}
}

// probeTop is the highest level a check may reach: the mode's bound, and
// never past an owner's cap on that window.
func (l *learner) probeTop(p *probe) float64 {
	top := l.mode.maxLevel()
	if limit, ok := l.caps[p.window]; ok {
		top = min(top, limit/p.base)
	}
	return top
}

// stepProbe moves a check on once a step has been held for its window: up
// one step when demand filled it, otherwise the check ends where it is.
func (l *learner) stepProbe(now time.Time) bool {
	p := l.probe
	if p == nil || now.Before(p.stepEnds) {
		return false
	}
	if float64(l.count(p.window, now)) < p.base*p.level*fillShare {
		l.endProbe(now, false) // demand dried up: nothing more to learn now
		return true
	}
	p.passed = p.level
	top := l.probeTop(p)
	if p.level+probeStep > top+1e-9 {
		l.endProbe(now, false)
		return true
	}
	p.level += probeStep
	p.stepEnds = now.Add(p.hold)
	return false
}

// endProbe settles a check: the ceiling moves to the last level that held,
// or down to what was accepted when even the old ceiling was throttled.
func (l *learner) endProbe(now time.Time, throttled bool) {
	p := l.probe
	l.probe, l.checkedAt = nil, now
	switch {
	case p.passed > 1:
		l.setCeiling(Ceiling{Window: p.window, Limit: p.base * p.passed, Source: SourceProbe, At: now})
	case throttled && p.passed == 0:
		if n := float64(l.count(p.window, now)); n > 0 && n < p.base {
			l.setCeiling(Ceiling{Window: p.window, Limit: n, Source: SourceProbe, At: now})
		}
	}
}
