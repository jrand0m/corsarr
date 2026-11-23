package decide

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
)

// CurrentFile is what the item already has on disk.
type CurrentFile struct {
	Quality     QualityID
	FormatScore int
	Group       string
	Revision    int // 1 = original; see parse.ParsedRelease.Revision
}

// Item is the target's state as the media-kind adapter reports it.
type Item struct {
	Ref            domain.ItemRef
	Monitored      bool
	Available      bool
	RuntimeMinutes int          // 0 = unknown, the kind default applies
	Current        *CurrentFile // nil = no file yet
	Profile        QualityProfile
}

// Context carries the lookup verdicts and settings for one decision.
type Context struct {
	Now                 time.Time
	Blocklisted         bool
	QueuedEqualOrBetter bool
	GrabbedBefore       bool
	// MaxResolution is a one-off resolution cap: the highest
	// resolution allowed (inclusive) for this search only (0 = none). "Replace
	// with lower resolution" sets it one step below the current file.
	MaxResolution     int
	MinAge            time.Duration // releases younger than this wait (delay)
	MinSeeders        int           // torrents below this wait
	PreferredProtocol domain.Protocol
	Definitions       Definitions   // nil = shipped defaults
	Payload           PayloadPolicy // zero = shipped list
}

// Input is everything Decide needs; it holds no I/O handles.
type Input struct {
	Release domain.Release
	Parsed  parse.ParsedRelease
	Item    Item
	Files   []PayloadFile // nil = Release.FileList without sizes
	Context Context
}

// facts are derived once and shared by every rule.
type facts struct {
	in        Input
	quality   Quality
	confident bool
	rank      int
	allowed   bool
	score     int
	formats   []string
	bounds    SizeBounds
	payload   PayloadVerdict
}

func derive(in Input) facts {
	f := facts{in: in}
	f.quality, f.confident = QualityOf(in.Parsed)
	f.rank, f.allowed = in.Item.Profile.Rank(f.quality.ID)
	f.score, f.formats = in.Item.Profile.Score(in.Release.Title)
	f.bounds = Bounds(in.Context.Definitions.For(f.quality.ID), in.Item.Ref.Kind, in.Item.RuntimeMinutes)
	files := in.Files
	if files == nil {
		files = FilesFromNames(in.Release.FileList)
	}
	f.payload = in.Context.Payload.Verdict(files, isVideoKind(in.Item.Ref.Kind))
	return f
}

func isVideoKind(k domain.MediaKind) bool { return k == domain.KindMovie || k == domain.KindTV }

type rule func(f facts) *domain.Reason

func reason(id string, cat domain.ReasonCategory, values map[string]any, format string, args ...any) *domain.Reason {
	return &domain.Reason{RuleID: id, Category: cat, Message: fmt.Sprintf(format, args...), Values: values}
}

// rules run in this order and all of them run.
var rules = []rule{
	rulePayloadDangerous, rulePayloadNotVideo,
	ruleBlocklisted, ruleGrabbedBefore, ruleProfileKind, ruleDiscImage, ruleThreeD,
	ruleQualityAllowed, ruleSizeMax, ruleSizeMin,
	ruleMaxResolution, ruleMinFormatScore, ruleUpgrade, ruleMonitored,
	ruleAvailable, ruleQueued, ruleTooNew, ruleSeeders,
}

// Decide evaluates every rule and returns the full verdict.
func Decide(in Input) domain.Decision {
	f := derive(in)
	reasons := []domain.Reason{}
	for _, r := range rules {
		if why := r(f); why != nil {
			reasons = append(reasons, *why)
		}
	}
	return domain.Decision{
		ReleaseID:   in.Release.ID,
		ItemRefs:    []domain.ItemRef{in.Item.Ref},
		Accepted:    len(reasons) == 0,
		Reasons:     reasons,
		Quality:     f.quality.Name,
		Resolution:  int(in.Parsed.Resolution.Value),
		FormatScore: f.score,
		RankKey:     rankKey(f),
		Override:    domain.OverrideFor(reasons),
	}
}

func rulePayloadDangerous(f facts) *domain.Reason {
	if f.payload.Status != PayloadDangerous {
		return nil
	}
	return reason("payload.dangerous", domain.ReasonSafety, map[string]any{"files": f.payload.Files},
		"executable or script in payload: %s", strings.Join(f.payload.Files, ", "))
}

func rulePayloadNotVideo(f facts) *domain.Reason {
	if f.payload.Status != PayloadNotVideo {
		return nil
	}
	return reason("payload.not_video", domain.ReasonSafety, map[string]any{"files": f.payload.Files},
		"largest file %s is not a video file", f.payload.Files[0])
}

func ruleBlocklisted(f facts) *domain.Reason {
	if !f.in.Context.Blocklisted {
		return nil
	}
	return reason("blocklist", domain.ReasonPermanent, nil, "release is blocklisted for this item")
}

func ruleGrabbedBefore(f facts) *domain.Reason {
	if !f.in.Context.GrabbedBefore {
		return nil
	}
	return reason("history.grabbed", domain.ReasonPermanent, nil, "release was grabbed before for this item")
}

func ruleProfileKind(f facts) *domain.Reason {
	pk := f.in.Item.Profile.spec.Kind
	if pk == f.in.Item.Ref.Kind {
		return nil
	}
	return reason("profile.kind", domain.ReasonPermanent, map[string]any{"profileKind": pk, "itemKind": f.in.Item.Ref.Kind},
		"profile %q is for %s, item is %s", f.in.Item.Profile.spec.Name, pk, f.in.Item.Ref.Kind)
}

func ruleDiscImage(f facts) *domain.Reason {
	if !f.in.Parsed.Disc.Known || f.in.Item.Profile.spec.AllowDiscImage {
		return nil
	}
	return reason("disc_image", domain.ReasonPermanent, map[string]any{"disc": f.in.Parsed.Disc.Value},
		"disc image (%s) not allowed by profile", f.in.Parsed.Disc.Value)
}

func ruleThreeD(f facts) *domain.Reason {
	if !f.in.Parsed.ThreeD || f.in.Item.Profile.spec.Allow3D {
		return nil
	}
	return reason("three_d", domain.ReasonPermanent, nil, "3D release not allowed by profile")
}

func ruleQualityAllowed(f facts) *domain.Reason {
	if f.allowed {
		return nil
	}
	return reason("quality.not_allowed", domain.ReasonPermanent,
		map[string]any{"quality": f.quality.Name, "confident": f.confident},
		"quality %s is not allowed by profile %q", f.quality.Name, f.in.Item.Profile.spec.Name)
}

func gbStr(b int64) string { return fmt.Sprintf("%.1f GB", float64(b)/gb) }

func runtimeNote(b SizeBounds) string {
	if b.DefaultRuntime {
		return fmt.Sprintf("%d min (default runtime, item runtime unknown)", b.Runtime)
	}
	return fmt.Sprintf("%d min", b.Runtime)
}

func sizeValues(f facts, limit int64) map[string]any {
	return map[string]any{"size": f.in.Release.Size, "limit": limit, "quality": f.quality.Name,
		"runtime": f.bounds.Runtime, "defaultRuntime": f.bounds.DefaultRuntime, "ceiling": f.bounds.Ceiling}
}

func ruleSizeMax(f facts) *domain.Reason {
	if f.in.Release.Size <= f.bounds.Max {
		return nil
	}
	return reason("size.max", domain.ReasonPermanent, sizeValues(f, f.bounds.Max),
		"size %s exceeds max %s for %s at %s", gbStr(f.in.Release.Size), gbStr(f.bounds.Max), f.quality.Name, runtimeNote(f.bounds))
}

func ruleSizeMin(f facts) *domain.Reason {
	if f.in.Release.Size <= 0 || f.in.Release.Size >= f.bounds.Min {
		return nil
	}
	return reason("size.min", domain.ReasonPermanent, sizeValues(f, f.bounds.Min),
		"size %s is below min %s for %s at %s", gbStr(f.in.Release.Size), gbStr(f.bounds.Min), f.quality.Name, runtimeNote(f.bounds))
}

func ruleMaxResolution(f facts) *domain.Reason {
	limit, res := f.in.Context.MaxResolution, int(f.in.Parsed.Resolution.Value)
	if limit <= 0 || !f.in.Parsed.Resolution.Known || res <= limit {
		return nil
	}
	return reason("search.max_resolution", domain.ReasonPreference, map[string]any{"resolution": res, "cap": limit},
		"resolution %dp is above this search's cap of %dp", res, limit)
}

func ruleMinFormatScore(f facts) *domain.Reason {
	min := f.in.Item.Profile.spec.MinFormatScore
	if f.score >= min {
		return nil
	}
	return reason("format.min_score", domain.ReasonPreference, map[string]any{"score": f.score, "min": min, "formats": f.formats},
		"format score %d is below the profile minimum %d", f.score, min)
}

func ruleUpgrade(f facts) *domain.Reason {
	cur := f.in.Item.Current
	if cur == nil || !f.allowed {
		return nil
	}
	ok, why := IsUpgrade(f.in.Item.Profile, *cur, candidateOf(f))
	if ok {
		return nil
	}
	return reason("upgrade.not_better", domain.ReasonPreference,
		map[string]any{"current": cur.Quality, "currentScore": cur.FormatScore, "candidate": f.quality.ID, "candidateScore": f.score},
		"not an upgrade: %s", why)
}

func candidateOf(f facts) Candidate {
	return Candidate{Quality: f.quality.ID, FormatScore: f.score, Group: f.in.Parsed.Group.Value,
		Revision: f.in.Parsed.Revision(), Proper: f.in.Parsed.Proper}
}

func ruleMonitored(f facts) *domain.Reason {
	if f.in.Item.Monitored {
		return nil
	}
	return reason("item.unmonitored", domain.ReasonPreference, nil, "item is not monitored")
}

func ruleAvailable(f facts) *domain.Reason {
	if f.in.Item.Available {
		return nil
	}
	return reason("item.not_available", domain.ReasonTemporary, nil, "item is not available yet")
}

func ruleQueued(f facts) *domain.Reason {
	if !f.in.Context.QueuedEqualOrBetter {
		return nil
	}
	return reason("queue.equal_or_better", domain.ReasonTemporary, nil, "an equal or better release is already queued")
}

func ruleTooNew(f facts) *domain.Reason {
	c, pub := f.in.Context, f.in.Release.PublishedAt
	if c.MinAge <= 0 || pub.IsZero() {
		return nil
	}
	age := c.Now.Sub(pub)
	if age >= c.MinAge {
		return nil
	}
	return reason("release.too_new", domain.ReasonTemporary, map[string]any{"age": age.String(), "minAge": c.MinAge.String()},
		"release is %s old, waiting until %s", age.Round(time.Minute), c.MinAge)
}

func ruleSeeders(f facts) *domain.Reason {
	r, min := f.in.Release, f.in.Context.MinSeeders
	if r.Protocol != domain.ProtoTorrent || r.Seeders >= min {
		return nil
	}
	return reason("seeders.min", domain.ReasonTemporary, map[string]any{"seeders": r.Seeders, "min": min},
		"%d seeders, need at least %d", r.Seeders, min)
}

// Rank orders decisions: accepted first, then by rank key (best first), then
// by release id so the order is total and stable. The input is not modified.
func Rank(ds []domain.Decision) []domain.Decision {
	out := append([]domain.Decision(nil), ds...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Accepted != b.Accepted {
			return a.Accepted
		}
		if a.RankKey != b.RankKey {
			return a.RankKey > b.RankKey
		}
		return a.ReleaseID < b.ReleaseID
	})
	return out
}
