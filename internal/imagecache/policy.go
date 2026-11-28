package imagecache

import (
	"math"
	"sort"

	"github.com/jrand0m/corsarr/internal/store"
)

// Settings are the cache's configuration.
type Settings struct {
	Enabled          bool
	Dir              string
	MaxSize          int64
	Quality          int
	MaxWidth         int
	BackdropMaxWidth int
	Degrade          Degrade
}

// Degrade are the shrink rules applied near the size cap.
type Degrade struct {
	QualityFloor int
	QualityStep  int
	WidthFloor   int
	WidthStep    float64
	HighWater    float64
	LowWater     float64
}

// maxWidth is the stored width cap of a role.
func (s Settings) maxWidth(r Role) int {
	if r == RoleBackdrop {
		return s.BackdropMaxWidth
	}
	return s.MaxWidth
}

// widthFloor is the narrowest a role is shrunk to; backdrops keep the same
// proportion to their cap as posters.
func (s Settings) widthFloor(r Role) int {
	if r == RoleBackdrop && s.MaxWidth > 0 {
		return s.Degrade.WidthFloor * s.BackdropMaxWidth / s.MaxWidth
	}
	return s.Degrade.WidthFloor
}

func (s Settings) high() int64 { return int64(math.Floor(float64(s.MaxSize) * s.Degrade.HighWater)) }
func (s Settings) low() int64  { return int64(math.Floor(float64(s.MaxSize) * s.Degrade.LowWater)) }

// nextQuality is the quality of the next re-encode step; equal to q when
// the floor is reached.
func (d Degrade) nextQuality(q int) int {
	if q <= d.QualityFloor || d.QualityStep <= 0 {
		return q
	}
	return max(q-d.QualityStep, d.QualityFloor)
}

// nextWidth is the width of the next shrink step; equal to w when the floor
// is reached.
func nextWidth(w, floor int, step float64) int {
	if w <= floor || step <= 0 || step >= 1 {
		return w
	}
	return max(int(math.Round(float64(w)*step)), floor)
}

// Eviction tiers, removed in this order.
const (
	tierVariant = iota
	tierOther
	tierLibrary
)

func tier(e store.ImageEntry, library map[string]bool) int {
	switch {
	case e.Variant > 0:
		return tierVariant
	case library[e.Origin]:
		return tierLibrary
	}
	return tierOther
}

// evictionOrder sorts stored entries by tier, then least recently used
// first (pure; entries arrive least recently used first).
func evictionOrder(entries []store.ImageEntry, library map[string]bool) []store.ImageEntry {
	out := append([]store.ImageEntry(nil), entries...)
	sort.SliceStable(out, func(i, j int) bool { return tier(out[i], library) < tier(out[j], library) })
	return out
}
