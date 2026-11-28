package httpx

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// advert is a limit a service announces in its response headers.
type advert struct {
	limit     float64       // requests per window; 0 = none announced
	window    time.Duration // the policy window, or a guess from the reset
	guessed   bool          // window inferred, not stated
	remaining int           // -1 = not announced
	reset     time.Time     // when the current window resets; zero = unknown
}

// parseAdvert reads RateLimit-* (both header drafts) and X-RateLimit-*.
func parseAdvert(h http.Header, now time.Time) advert {
	limit := firstHeader(h, "RateLimit-Limit", "X-RateLimit-Limit")
	policy := firstHeader(h, "RateLimit-Policy", "X-RateLimit-Policy")
	a := advert{remaining: -1, reset: parseReset(firstHeader(h, "RateLimit-Reset", "X-RateLimit-Reset"), now)}
	a.limit = firstPositive(param(policy, "q"), leadingNumber(limit), leadingNumber(policy))
	if v, err := strconv.Atoi(strings.TrimSpace(firstHeader(h, "RateLimit-Remaining", "X-RateLimit-Remaining"))); err == nil {
		a.remaining = v
	}
	a.window, a.guessed = advertWindow(firstPositive(param(policy, "w"), param(limit, "w")), a.reset, now)
	return a
}

func advertWindow(seconds float64, reset, now time.Time) (time.Duration, bool) {
	switch {
	case seconds > 0:
		return time.Duration(seconds * float64(time.Second)), false
	case !reset.IsZero():
		return windowFor(reset.Sub(now)), true
	}
	return time.Hour, true // nothing said: assume the slowest window
}

func firstHeader(h http.Header, names ...string) string {
	for _, n := range names {
		if v := h.Get(n); v != "" {
			return v
		}
	}
	return ""
}

func firstPositive(vs ...float64) float64 {
	for _, v := range vs {
		if v > 0 {
			return v
		}
	}
	return 0
}

// leadingNumber is the number a header value starts with ("100, 100;w=60").
func leadingNumber(v string) float64 {
	end := strings.IndexAny(v, ",;")
	if end >= 0 {
		v = v[:end]
	}
	return number(v)
}

// param finds name=value among the ;- and ,-separated parts of v.
func param(v, name string) float64 {
	for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == ';' || r == ',' }) {
		if k, val, ok := strings.Cut(strings.TrimSpace(part), "="); ok && k == name {
			return number(val)
		}
	}
	return 0
}

func number(v string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0
	}
	return f
}

// parseReset accepts seconds until the reset or a Unix time.
func parseReset(v string, now time.Time) time.Time {
	f := number(v)
	switch {
	case f == 0:
		return time.Time{}
	case f > 1e9:
		return time.Unix(int64(f), 0)
	}
	return now.Add(time.Duration(f * float64(time.Second)))
}
