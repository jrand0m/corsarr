package decide

import (
	"math"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Desirability weights: points lost per quality step from the cutoff, the
// most points size can cost (reached 100 % away from the preferred size), the
// most a format-score shortfall can cost and how many score points cost one.
const (
	qualityStepPoints = 25
	sizeMaxPoints     = 40
	formatMaxPoints   = 20
	formatPointsPer   = 5
	// sizeBand is how far from the preferred size still counts as on target.
	sizeBand = 0.10
)

// target is what a profile aims for, for one release.
type target struct {
	cutoffRank   int
	cutoff       string
	preferred    int64
	formatTarget int
	hasFormats   bool
}

func targetOf(f facts) target {
	spec := f.in.Item.Profile.spec
	rank, _ := f.in.Item.Profile.Rank(spec.Cutoff)
	q, _ := LookupQuality(spec.Cutoff)
	return target{cutoffRank: rank, cutoff: q.Name, preferred: f.bounds.Preferred,
		formatTarget: spec.CutoffFormatScore, hasFormats: len(spec.Formats) > 0}
}

// desirabilityOf measures a release against the profile's target; nil when
// its quality is outside the profile.
func desirabilityOf(f facts) *domain.Desirability {
	if !f.allowed {
		return nil
	}
	t := targetOf(f)
	d := &domain.Desirability{QualitySteps: f.rank - t.cutoffRank, Cutoff: t.cutoff}
	if t.hasFormats {
		d.FormatTarget, d.FormatShortfall = t.formatTarget, max(0, t.formatTarget-f.score)
	}
	if f.in.Release.Size > 0 && t.preferred > 0 {
		delta := float64(f.in.Release.Size-t.preferred) / float64(t.preferred)
		d.SizeDelta, d.PreferredSize = &delta, t.preferred
	}
	d.Score = clamp(100-qualityPoints(d.QualitySteps)-sizePoints(d.SizeDelta)-formatPoints(d.FormatShortfall), 0, 100)
	d.Position = positionOf(d)
	return d
}

func qualityPoints(steps int) int { return qualityStepPoints * abs(steps) }

func sizePoints(delta *float64) int {
	if delta == nil {
		return 0
	}
	off := math.Abs(*delta)
	if off <= sizeBand {
		return 0
	}
	return int(math.Round(math.Min(off, 1) * sizeMaxPoints))
}

func formatPoints(shortfall int) int {
	return min(formatMaxPoints, (shortfall+formatPointsPer-1)/formatPointsPer)
}

// positionOf: quality decides first, then size outside the band, then a
// format-score shortfall.
func positionOf(d *domain.Desirability) domain.Position {
	switch {
	case d.QualitySteps != 0:
		return sideOf(d.QualitySteps > 0)
	case d.SizeDelta != nil && math.Abs(*d.SizeDelta) > sizeBand:
		return sideOf(*d.SizeDelta > 0)
	case d.FormatShortfall > 0:
		return domain.PositionBelow
	}
	return domain.PositionOnTarget
}

func sideOf(above bool) domain.Position {
	if above {
		return domain.PositionAbove
	}
	return domain.PositionBelow
}

func abs(v int) int { return max(v, -v) }
