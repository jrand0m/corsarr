package definition

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var sizePattern = regexp.MustCompile(`(?i)^([0-9][0-9.,]*)\s*([kmgtp]?i?b?|bytes?)$`)

var sizeUnits = map[byte]float64{'k': 1 << 10, 'm': 1 << 20, 'g': 1 << 30, 't': 1 << 40, 'p': 1 << 50}

// ParseSize reads "1.5 GiB", "700 MB", "1,234 KB" or a byte count. Decimal
// and binary prefixes are both read as powers of 1024, as trackers mean them.
func ParseSize(s string) int64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, " ", " "))
	m := sizePattern.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	num := normalizeNumber(m[1])
	f, err := strconv.ParseFloat(num, 64)
	if err != nil || f < 0 {
		return 0
	}
	unit := strings.ToLower(m[2])
	if unit != "" {
		if mult, ok := sizeUnits[unit[0]]; ok {
			f *= mult
		}
	}
	if f > math.MaxInt64/2 {
		return 0
	}
	return int64(f)
}

// normalizeNumber turns "1,234.5" or "1.234,5" or "1,5" into "1234.5" / "1.5".
func normalizeNumber(s string) string {
	dot, comma := strings.LastIndex(s, "."), strings.LastIndex(s, ",")
	switch {
	case comma < 0:
		return s
	case dot < 0 && len(s)-comma-1 != 3:
		return strings.Replace(s, ",", ".", 1)
	case dot > comma || dot < 0:
		return strings.ReplaceAll(s, ",", "")
	}
	return strings.Replace(strings.ReplaceAll(s, ".", ""), ",", ".", 1)
}

// ParseInt reads an integer, ignoring thousands separators; 0 when absent.
func ParseInt(s string) int {
	s = strings.TrimSpace(strings.NewReplacer(",", "", " ", "", " ", "").Replace(s))
	if f, err := strconv.ParseFloat(s, 64); err == nil && f >= 0 && f < math.MaxInt32 {
		return int(f)
	}
	return 0
}

// ParseFloat reads a decimal (download/upload factors); def when absent.
func ParseFloat(s string, def float64) float64 {
	if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
		return f
	}
	return def
}

var dateLayouts = []string{
	time.RFC3339Nano, time.RFC3339, time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822,
	"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02", "02.01.2006 15:04", "02.01.2006",
	"Jan 2, 2006", "2 Jan 2006", "Jan. 2, 2006",
}

var relPattern = regexp.MustCompile(`(?i)^(\d+|an?)\s*(sec|second|min|minute|hour|hr|day|week|month|year)s?\s+ago$`)

var relUnits = map[string]time.Duration{
	"sec": time.Second, "second": time.Second, "min": time.Minute, "minute": time.Minute,
	"hour": time.Hour, "hr": time.Hour, "day": 24 * time.Hour, "week": 7 * 24 * time.Hour,
	"month": 30 * 24 * time.Hour, "year": 365 * 24 * time.Hour,
}

// ParseDate reads a publish date: the definition's own layouts first, then
// unix seconds or milliseconds, common layouts, "N units ago", "today" and
// "yesterday". The zero time means unknown.
func ParseDate(s string, layouts []string, now time.Time) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, l := range append(append([]string{}, layouts...), dateLayouts...) {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC()
		}
	}
	if t, ok := unixDate(s); ok {
		return t
	}
	return relativeDate(strings.ToLower(s), now)
}

func unixDate(s string) (time.Time, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	switch {
	case err != nil || n <= 0:
		return time.Time{}, false
	case len(s) >= 13:
		return time.UnixMilli(n).UTC(), true
	}
	return time.Unix(n, 0).UTC(), true
}

func relativeDate(s string, now time.Time) time.Time {
	day := now.UTC().Truncate(24 * time.Hour)
	switch {
	case strings.HasPrefix(s, "today") || s == "just now" || s == "now":
		return withClock(day, s)
	case strings.HasPrefix(s, "yesterday"):
		return withClock(day.Add(-24*time.Hour), s)
	}
	m := relPattern.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}
	}
	n := 1
	if v, err := strconv.Atoi(m[1]); err == nil {
		n = v
	}
	// Truncating to the unit keeps the date stable between reads, so a page
	// saying "2 hours ago" does not look newer on every feed read.
	unit := relUnits[strings.ToLower(m[2])]
	return now.UTC().Truncate(min(unit, 24*time.Hour)).Add(-time.Duration(n) * unit)
}

var clockPattern = regexp.MustCompile(`(\d{1,2}):(\d{2})`)

func withClock(day time.Time, s string) time.Time {
	m := clockPattern.FindStringSubmatch(s)
	if m == nil {
		return day
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	return day.Add(time.Duration(h)*time.Hour + time.Duration(mi)*time.Minute)
}
