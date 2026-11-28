package decide

import (
	"fmt"
	"path"
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
	formats   []MatchedFormat
	bounds    SizeBounds
	payload   PayloadVerdict
}

func derive(in Input) facts {
	f := facts{in: in}
	f.quality, f.confident = qualityFor(in)
	f.rank, f.allowed = in.Item.Profile.Rank(f.quality.ID)
	f.score, f.formats = in.Item.Profile.Score(SubjectOf(in.Release, in.Parsed))
	f.bounds = Bounds(in.Item.Profile.Definition(in.Context.Definitions, f.quality.ID), in.Item.Ref.Kind, in.Item.RuntimeMinutes)
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
	ruleMaxResolution, ruleRejectFormat, ruleMinFormatScore, ruleUpgrade, ruleMonitored,
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
		Formats:     formatsOrNil(f.formats),
		RankKey:     rankKey(f),
		Override:    domain.OverrideFor(reasons),

		Desirability: desirabilityOf(f),
	}
}

func rulePayloadDangerous(f facts) *domain.Reason {
	if f.payload.Status != PayloadDangerous {
		return nil
	}
	return brief(reason("payload.dangerous", domain.ReasonSafety, map[string]any{"files": f.payload.Files},
		"executable or script in payload: %s", strings.Join(f.payload.Files, ", ")), "Unsafe file inside: %s", firstPlus(f.payload.Files))
}

func rulePayloadNotVideo(f facts) *domain.Reason {
	if f.payload.Status != PayloadNotVideo {
		return nil
	}
	return brief(reason("payload.not_video", domain.ReasonSafety, map[string]any{"files": f.payload.Files},
		"largest file %s is not a video file", f.payload.Files[0]), "Main file is not a video")
}

func ruleBlocklisted(f facts) *domain.Reason {
	if !f.in.Context.Blocklisted {
		return nil
	}
	return brief(reason("blocklist", domain.ReasonPermanent, nil, "release is blocklisted for this item"), "Blocklisted")
}

func ruleGrabbedBefore(f facts) *domain.Reason {
	if !f.in.Context.GrabbedBefore {
		return nil
	}
	return brief(reason("history.grabbed", domain.ReasonPermanent, nil, "release was grabbed before for this item"), "Grabbed before")
}

func ruleProfileKind(f facts) *domain.Reason {
	pk := f.in.Item.Profile.spec.Kind
	if f.in.Item.Profile.Serves(f.in.Item.Ref.Kind) {
		return nil
	}
	return brief(reason("profile.kind", domain.ReasonPermanent, map[string]any{"profileKind": pk, "itemKind": f.in.Item.Ref.Kind},
		"profile %q is for %s, item is %s", f.in.Item.Profile.spec.Name, pk, f.in.Item.Ref.Kind), "Profile is for %s", pk)
}

func ruleDiscImage(f facts) *domain.Reason {
	if !f.in.Parsed.Disc.Known || f.in.Item.Profile.spec.AllowDiscImage {
		return nil
	}
	return brief(reason("disc_image", domain.ReasonPermanent, map[string]any{"disc": f.in.Parsed.Disc.Value},
		"disc image (%s) not allowed by profile", f.in.Parsed.Disc.Value), "Disc image, not allowed")
}

func ruleThreeD(f facts) *domain.Reason {
	if !f.in.Parsed.ThreeD || f.in.Item.Profile.spec.Allow3D {
		return nil
	}
	return brief(reason("three_d", domain.ReasonPermanent, nil, "3D release not allowed by profile"), "3D, not allowed")
}

func ruleQualityAllowed(f facts) *domain.Reason {
	if f.allowed {
		return nil
	}
	return brief(reason("quality.not_allowed", domain.ReasonPermanent,
		map[string]any{"quality": f.quality.Name, "confident": f.confident},
		"quality %s is not allowed by profile %q", f.quality.Name, f.in.Item.Profile.spec.Name), "%s is not in the profile", f.quality.Name)
}

// brief adds the short line a result card shows.
func brief(r *domain.Reason, format string, args ...any) *domain.Reason {
	r.Summary = fmt.Sprintf(format, args...)
	return r
}

// firstPlus names the first item and how many more there are.
func firstPlus(xs []string) string {
	if len(xs) > 1 {
		return fmt.Sprintf("%s (+%d more)", path.Base(strings.ReplaceAll(xs[0], `\`, "/")), len(xs)-1)
	}
	return path.Base(strings.ReplaceAll(xs[0], `\`, "/"))
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
	return brief(reason("size.max", domain.ReasonPermanent, sizeValues(f, f.bounds.Max),
		"size %s exceeds max %s for %s at %s", gbStr(f.in.Release.Size), gbStr(f.bounds.Max), f.quality.Name, runtimeNote(f.bounds)), "Too large: %s, max %s", gbStr(f.in.Release.Size), gbStr(f.bounds.Max))
}

func ruleSizeMin(f facts) *domain.Reason {
	if f.in.Release.Size <= 0 || f.in.Release.Size >= f.bounds.Min {
		return nil
	}
	return brief(reason("size.min", domain.ReasonPermanent, sizeValues(f, f.bounds.Min),
		"size %s is below min %s for %s at %s", gbStr(f.in.Release.Size), gbStr(f.bounds.Min), f.quality.Name, runtimeNote(f.bounds)), "Too small: %s, min %s", gbStr(f.in.Release.Size), gbStr(f.bounds.Min))
}

func ruleMaxResolution(f facts) *domain.Reason {
	limit, res := f.in.Context.MaxResolution, int(f.in.Parsed.Resolution.Value)
	if limit <= 0 || !f.in.Parsed.Resolution.Known || res <= limit {
		return nil
	}
	return brief(reason("search.max_resolution", domain.ReasonPreference, map[string]any{"resolution": res, "cap": limit},
		"resolution %dp is above this search's cap of %dp", res, limit), "Above this search's %dp cap", limit)
}

func ruleMinFormatScore(f facts) *domain.Reason {
	min := f.in.Item.Profile.spec.MinFormatScore
	if f.score >= min {
		return nil
	}
	return brief(reason("format.min_score", domain.ReasonPreference, map[string]any{"score": f.score, "min": min, "formats": formatNames(f.formats)},
		"format score %d is below the profile minimum %d%s", f.score, min, scoredList(f.formats)), "Format score %d, needs %d", f.score, min)
}

func ruleRejectFormat(f facts) *domain.Reason {
	var names []string
	for _, m := range f.formats {
		if m.Reject {
			names = append(names, m.Name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	return brief(reason("format.reject", domain.ReasonPermanent, map[string]any{"formats": names},
		"matches %s, which this profile rejects", strings.Join(names, ", ")), "Rejected format: %s", firstPlus(names))
}

func formatNames(ms []MatchedFormat) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name
	}
	return out
}

// scoredList is " (Name +50, Other −10)" or "" when nothing matched.
func scoredList(ms []MatchedFormat) string {
	if len(ms) == 0 {
		return ""
	}
	parts := make([]string, len(ms))
	for i, m := range ms {
		parts[i] = fmt.Sprintf("%s %+d", m.Name, m.Score)
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

func formatsOrNil(ms []MatchedFormat) []MatchedFormat {
	if len(ms) == 0 {
		return nil
	}
	return ms
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
	return brief(reason("upgrade.not_better", domain.ReasonPreference,
		map[string]any{"current": cur.Quality, "currentScore": cur.FormatScore, "candidate": f.quality.ID, "candidateScore": f.score},
		"not an upgrade: %s", why), "Not an upgrade")
}

func candidateOf(f facts) Candidate {
	return Candidate{Quality: f.quality.ID, FormatScore: f.score, Group: f.in.Parsed.Group.Value,
		Revision: f.in.Parsed.Revision(), Proper: f.in.Parsed.Proper}
}

func ruleMonitored(f facts) *domain.Reason {
	if f.in.Item.Monitored {
		return nil
	}
	return brief(reason("item.unmonitored", domain.ReasonPreference, nil, "item is not monitored"), "Not monitored")
}

func ruleAvailable(f facts) *domain.Reason {
	if f.in.Item.Available {
		return nil
	}
	return brief(reason("item.not_available", domain.ReasonTemporary, nil, "item is not available yet"), "Not released yet")
}

func ruleQueued(f facts) *domain.Reason {
	if !f.in.Context.QueuedEqualOrBetter {
		return nil
	}
	return brief(reason("queue.equal_or_better", domain.ReasonTemporary, nil, "an equal or better release is already queued"), "Equal or better already queued")
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
	return brief(reason("release.too_new", domain.ReasonTemporary, map[string]any{"age": age.String(), "minAge": c.MinAge.String()},
		"release is %s old, waiting until %s", age.Round(time.Minute), c.MinAge), "Too new: waiting %s", c.MinAge)
}

func ruleSeeders(f facts) *domain.Reason {
	r, min := f.in.Release, f.in.Context.MinSeeders
	if r.Protocol != domain.ProtoTorrent || r.Seeders >= min {
		return nil
	}
	return brief(reason("seeders.min", domain.ReasonTemporary, map[string]any{"seeders": r.Seeders, "min": min},
		"%d seeders, need at least %d", r.Seeders, min), "Too few seeders: %d, needs %d", r.Seeders, min)
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
