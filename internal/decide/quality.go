// Package decide turns a parsed release into an explained accept/reject
// Decision. Everything here is pure: callers supply item state,
// lookups (blocklist, queue, history) and the clock as plain values.
package decide

import (
	"fmt"
	"math"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
)

// QualityID is the stable key of a rung on the video ladder ("webdl-1080p").
type QualityID string

// Tier is the resolution bucket of a quality; TierUnknown sorts lowest.
type Tier int

const (
	TierUnknown Tier = 0
	TierSD      Tier = 480
	Tier720     Tier = 720
	Tier1080    Tier = 1080
	Tier2160    Tier = 2160
)

// SourceDisc is the ladder source for full-disc images (ISO/BDMV/IMG); the
// parser reports those as a separate flag, the ladder ranks them as a source.
const SourceDisc parse.Source = "disc"

// Quality is one rung of the video ladder: source × resolution tier.
type Quality struct {
	ID     QualityID    `json:"id"`
	Name   string       `json:"name"`
	Source parse.Source `json:"source"`
	Tier   Tier         `json:"tier"`
	Rank   int          `json:"rank"` // position on the ladder, 0 = Unknown
}

// ladder lists every video quality from worst to best.
var ladder = func() []Quality {
	rows := []struct {
		id, name string
		src      parse.Source
		tier     Tier
	}{
		{"unknown", "Unknown", "", TierUnknown},
		{"cam", "CAM", parse.SourceCam, TierSD},
		{"telesync", "TELESYNC", parse.SourceTelesync, TierSD},
		{"telecine", "TELECINE", parse.SourceTelecine, TierSD},
		{"screener", "SCREENER", parse.SourceScreener, TierSD},
		{"dvd", "DVD", parse.SourceDVD, TierSD},
		{"sdtv", "SDTV", parse.SourceHDTV, TierSD},
		{"webrip-480p", "WEBRip-480p", parse.SourceWEBRip, TierSD},
		{"webdl-480p", "WEBDL-480p", parse.SourceWEBDL, TierSD},
		{"bluray-480p", "Bluray-480p", parse.SourceBluray, TierSD},
		{"bluray-576p", "Bluray-576p", parse.SourceBluray, TierSD},
		{"hdtv-720p", "HDTV-720p", parse.SourceHDTV, Tier720},
		{"webrip-720p", "WEBRip-720p", parse.SourceWEBRip, Tier720},
		{"webdl-720p", "WEBDL-720p", parse.SourceWEBDL, Tier720},
		{"bluray-720p", "Bluray-720p", parse.SourceBluray, Tier720},
		{"hdtv-1080p", "HDTV-1080p", parse.SourceHDTV, Tier1080},
		{"webrip-1080p", "WEBRip-1080p", parse.SourceWEBRip, Tier1080},
		{"webdl-1080p", "WEBDL-1080p", parse.SourceWEBDL, Tier1080},
		{"bluray-1080p", "Bluray-1080p", parse.SourceBluray, Tier1080},
		{"remux-1080p", "Remux-1080p", parse.SourceRemux, Tier1080},
		{"disc-1080p", "Disc-1080p", SourceDisc, Tier1080},
		{"hdtv-2160p", "HDTV-2160p", parse.SourceHDTV, Tier2160},
		{"webrip-2160p", "WEBRip-2160p", parse.SourceWEBRip, Tier2160},
		{"webdl-2160p", "WEBDL-2160p", parse.SourceWEBDL, Tier2160},
		{"bluray-2160p", "Bluray-2160p", parse.SourceBluray, Tier2160},
		{"remux-2160p", "Remux-2160p", parse.SourceRemux, Tier2160},
		{"disc-2160p", "Disc-2160p", SourceDisc, Tier2160},
	}
	out := make([]Quality, len(rows))
	for i, r := range rows {
		out[i] = Quality{ID: QualityID(r.id), Name: r.name, Source: r.src, Tier: r.tier, Rank: i}
	}
	return out
}()

type srcTier struct {
	src  parse.Source
	tier Tier
}

// exactRes names rungs that sit inside a tier and are picked only when the
// release states that exact resolution.
var exactRes = map[srcRes]QualityID{{parse.SourceBluray, parse.R576}: "bluray-576p"}

type srcRes struct {
	src parse.Source
	res parse.Resolution
}

var (
	byID       = map[QualityID]Quality{}
	bySrcTier  = map[srcTier]Quality{}
	lowestTier = map[parse.Source]Quality{}
)

func init() {
	for _, q := range ladder {
		byID[q.ID] = q
		if _, taken := bySrcTier[srcTier{q.Source, q.Tier}]; !taken {
			bySrcTier[srcTier{q.Source, q.Tier}] = q
		}
		if _, ok := lowestTier[q.Source]; !ok {
			lowestTier[q.Source] = q
		}
	}
}

// Ladder returns the video ladder, worst first.
func Ladder() []Quality { return append([]Quality(nil), ladder...) }

// LookupQuality finds a ladder rung by id.
func LookupQuality(id QualityID) (Quality, bool) { q, ok := byID[id]; return q, ok }

// QualityUnknown is the bottom rung.
var QualityUnknown = ladder[0]

func tierOf(r parse.Field[parse.Resolution]) Tier {
	switch {
	case !r.Known:
		return TierUnknown
	case r.Value >= parse.R2160:
		return Tier2160
	case r.Value >= parse.R1080:
		return Tier1080
	case r.Value >= parse.R720:
		return Tier720
	}
	return TierSD
}

// QualityOf maps a parsed release to a ladder rung. When source or resolution
// is missing it picks the most conservative rung and reports confident=false.
func QualityOf(p parse.ParsedRelease) (Quality, bool) {
	tier := tierOf(p.Resolution)
	src, srcKnown := p.Source.Value, p.Source.Known
	if p.Disc.Known {
		src, srcKnown = SourceDisc, true
	}
	if !srcKnown {
		if tier == TierUnknown {
			return QualityUnknown, false
		}
		src = parse.SourceHDTV // the lowest source that exists at every tier
	}
	if id, ok := exactRes[srcRes{src, p.Resolution.Value}]; ok && srcKnown && p.Resolution.Known {
		return byID[id], true
	}
	if q, ok := bySrcTier[srcTier{src, tier}]; ok {
		return q, srcKnown && tier != TierUnknown
	}
	return lowestTier[src], false
}

// MaxMBPerMinute is the largest maximum a definition may hold (60 GB/hour).
const MaxMBPerMinute = 1000.0

// QualityDefinition holds size bounds in MB (10^6 bytes) per minute of
// runtime. The maximum is always finite: the constructor is the only way in.
type QualityDefinition struct {
	min, preferred, max float64
}

// NewQualityDefinition validates 0 <= min <= preferred <= max <= MaxMBPerMinute and min < max.
func NewQualityDefinition(min, preferred, max float64) (QualityDefinition, error) {
	for _, v := range []float64{min, preferred, max} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return QualityDefinition{}, fmt.Errorf("sizes must be finite and >= 0 (largest allowed max is %.0f MB/min)", MaxMBPerMinute)
		}
	}
	if max > MaxMBPerMinute {
		return QualityDefinition{}, fmt.Errorf("max %.1f MB/min is above the largest allowed value %.0f MB/min", max, MaxMBPerMinute)
	}
	if max <= min || preferred < min || preferred > max {
		return QualityDefinition{}, fmt.Errorf("need min %.1f < max %.1f and preferred %.1f between them", min, max, preferred)
	}
	return QualityDefinition{min: min, preferred: preferred, max: max}, nil
}

func (d QualityDefinition) Min() float64       { return d.min }
func (d QualityDefinition) Preferred() float64 { return d.preferred }
func (d QualityDefinition) Max() float64       { return d.max }

// Definitions maps qualities to their size bounds; a missing entry (or the
// zero value) falls back to the shipped default, so no quality is ever unbounded.
type Definitions map[QualityID]QualityDefinition

var tierDefaults = map[Tier]QualityDefinition{
	TierUnknown: {min: 0, preferred: 10, max: 25},
	TierSD:      {min: 1, preferred: 10, max: 25},
	Tier720:     {min: 3, preferred: 17, max: 35},
	Tier1080:    {min: 6, preferred: 35, max: 70},
	Tier2160:    {min: 15, preferred: 120, max: 250},
}

// DefaultDefinitions returns the shipped bounds: max 2160p 250, 1080p 70,
// 720p 35, SD 25 MB/min; Remux and disc images are capped like their tier;
// lossy audio 3, lossless 12 and hi-res audio 60 MB/min.
func DefaultDefinitions() Definitions {
	d := Definitions{}
	for _, q := range append(Ladder(), audioLadder...) {
		d[q.ID] = shippedDefinition(q.ID)
	}
	return d
}

// For returns the definition for a quality, falling back to the default.
func (d Definitions) For(id QualityID) QualityDefinition {
	if def, ok := d[id]; ok {
		return def
	}
	return shippedDefinition(id)
}

// KindLimits are the per-kind size safety nets.
type KindLimits struct {
	CeilingBytes   int64 // hard per-item maximum, applied even when runtime is wrong
	DefaultRuntime int   // minutes, used when the item's runtime is unknown
}

const gb = 1_000_000_000

var kindLimits = map[domain.MediaKind]KindLimits{
	domain.KindTV:    {CeilingBytes: 15 * gb, DefaultRuntime: 45},
	domain.KindMovie: {CeilingBytes: 50 * gb, DefaultRuntime: 120},
	domain.KindMusic: {CeilingBytes: 3 * gb, DefaultRuntime: 45},
	domain.KindBook:  {CeilingBytes: 2 * gb, DefaultRuntime: 600},
}

// LimitsFor returns the kind's ceiling and default runtime (movie limits for an unknown kind).
func LimitsFor(k domain.MediaKind) KindLimits {
	if l, ok := kindLimits[k]; ok {
		return l
	}
	return kindLimits[domain.KindMovie]
}

// SizeBounds is a definition applied to one item's runtime, in bytes.
type SizeBounds struct {
	Min, Preferred, Max int64
	Runtime             int  // minutes used
	DefaultRuntime      bool // true when the item's runtime was unknown
	Ceiling             bool // true when the kind ceiling, not the definition, set Max
}

// Bounds applies a definition to a runtime (0 = unknown) for a kind.
func Bounds(def QualityDefinition, kind domain.MediaKind, runtime int) SizeBounds {
	lim := LimitsFor(kind)
	b := SizeBounds{Runtime: runtime}
	if runtime <= 0 {
		b.Runtime, b.DefaultRuntime = lim.DefaultRuntime, true
	}
	mb := func(perMin float64) int64 { return int64(perMin * float64(b.Runtime) * 1e6) }
	b.Min, b.Preferred, b.Max = mb(def.min), mb(def.preferred), mb(def.max)
	if b.Max > lim.CeilingBytes {
		b.Max, b.Ceiling = lim.CeilingBytes, true
	}
	b.Preferred = min(b.Preferred, b.Max)
	return b
}
