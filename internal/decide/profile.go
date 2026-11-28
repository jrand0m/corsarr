package decide

import (
	"errors"
	"fmt"

	"github.com/jrand0m/corsarr/internal/domain"
)

// FormatScore is a profile's score for one format; Reject makes a match a
// permanent rejection whatever the scores.
type FormatScore struct {
	Format CustomFormat
	Score  int
	Reject bool
}

// MatchedFormat is a format a release matched, with the profile's score.
type MatchedFormat = domain.FormatMatch

// QualityGroup is a set of qualities a profile treats as equal. A group of
// one needs no name.
type QualityGroup struct {
	Name      string
	Qualities []QualityID
}

// ProfileSpec is the editable shape of a QualityProfile.
type ProfileSpec struct {
	Key                 string // "" = derived from Name
	Name                string
	Kind                domain.MediaKind   // the first kind served
	Kinds               []domain.MediaKind // nil = Kind only; tv and movie may share one profile
	Sizes               map[QualityID]QualityDefinition
	Groups              []QualityGroup // allowed qualities, worst group first
	Cutoff              QualityID
	Upgrades            bool
	Formats             []FormatScore
	MinFormatScore      int
	CutoffFormatScore   int
	MinUpgradeIncrement int // 0 means the default of 1
	AllowDiscImage      bool
	Allow3D             bool
}

// QualityProfile is a validated ProfileSpec; build it with NewQualityProfile.
type QualityProfile struct {
	spec ProfileSpec
	rank map[QualityID]int
}

// NewQualityProfile refuses a profile that could not decide anything sensibly.
func NewQualityProfile(spec ProfileSpec) (QualityProfile, error) {
	if spec.Name == "" {
		return QualityProfile{}, errors.New("profile needs a name")
	}
	if err := normaliseKinds(&spec); err != nil {
		return QualityProfile{}, fmt.Errorf("profile %q: %w", spec.Name, err)
	}
	if spec.Key == "" {
		spec.Key = KeyOf(spec.Name)
	}
	rank, err := rankGroups(spec.Groups, spec.Kind == domain.KindMusic)
	if err != nil {
		return QualityProfile{}, fmt.Errorf("profile %q: %w", spec.Name, err)
	}
	if _, ok := rank[spec.Cutoff]; !ok {
		return QualityProfile{}, fmt.Errorf("profile %q: cutoff %q is not an allowed quality", spec.Name, spec.Cutoff)
	}
	if spec.MinUpgradeIncrement < 0 {
		return QualityProfile{}, fmt.Errorf("profile %q: minimum upgrade increment must be >= 0", spec.Name)
	}
	if spec.MinUpgradeIncrement == 0 {
		spec.MinUpgradeIncrement = 1
	}
	for id := range spec.Sizes {
		if _, ok := rank[id]; !ok {
			return QualityProfile{}, fmt.Errorf("profile %q: size limits for %q, which is not allowed", spec.Name, id)
		}
	}
	spec.Groups = append([]QualityGroup(nil), spec.Groups...)
	spec.Formats = append([]FormatScore(nil), spec.Formats...)
	spec.Kinds = append([]domain.MediaKind(nil), spec.Kinds...)
	return QualityProfile{spec: spec, rank: rank}, nil
}

// normaliseKinds checks the served kinds: one kind, or TV and movies together
// (they share the video ladder); music never shares.
func normaliseKinds(spec *ProfileSpec) error {
	if len(spec.Kinds) == 0 {
		spec.Kinds = []domain.MediaKind{spec.Kind}
	}
	seen := map[domain.MediaKind]bool{}
	for _, k := range spec.Kinds {
		if _, err := domain.ParseMediaKind(string(k)); err != nil {
			return err
		}
		if seen[k] {
			return fmt.Errorf("kind %s is listed twice", k)
		}
		seen[k] = true
	}
	if len(spec.Kinds) > 1 && (seen[domain.KindMusic] || seen[domain.KindBook]) {
		return errors.New("only TV and movies can share a profile")
	}
	spec.Kind = spec.Kinds[0]
	return nil
}

func rankGroups(groups []QualityGroup, audio bool) (map[QualityID]int, error) {
	rank := map[QualityID]int{}
	for i, g := range groups {
		for _, id := range g.Qualities {
			if _, ok := byID[id]; !ok {
				return nil, fmt.Errorf("unknown quality %q", id)
			}
			if _, isAudio := audioTiers[id]; isAudio != audio {
				return nil, fmt.Errorf("quality %q is not on this kind's ladder", id)
			}
			if _, dup := rank[id]; dup {
				return nil, fmt.Errorf("quality %q is listed twice", id)
			}
			rank[id] = i
		}
	}
	if len(rank) == 0 {
		return nil, errors.New("no allowed qualities")
	}
	return rank, nil
}

// Spec returns a copy of the profile's settings.
func (p QualityProfile) Spec() ProfileSpec { return p.spec }

// Rank is the group index of an allowed quality (higher is better).
func (p QualityProfile) Rank(id QualityID) (int, bool) { r, ok := p.rank[id]; return r, ok }

// Key is the profile's stable key.
func (p QualityProfile) Key() string { return p.spec.Key }

// Serves reports whether items of a kind may use the profile.
func (p QualityProfile) Serves(k domain.MediaKind) bool {
	for _, s := range p.spec.Kinds {
		if s == k {
			return true
		}
	}
	return false
}

// Score sums the scores of the formats a release matches and lists them in
// profile order; Reject marks formats that reject on a match.
func (p QualityProfile) Score(s Subject) (int, []MatchedFormat) {
	total, matched := 0, []MatchedFormat{}
	for _, fs := range p.spec.Formats {
		if fs.Format.Matches(s) {
			total += fs.Score
			matched = append(matched, MatchedFormat{Key: fs.Format.key, Name: fs.Format.name, Score: fs.Score, Reject: fs.Reject})
		}
	}
	return total, matched
}

// ScoreTitle scores a bare title (no size or indexer flags) and names the
// matched formats.
func (p QualityProfile) ScoreTitle(title string) (int, []string) {
	total, matched := p.Score(TitleSubject(title))
	names := make([]string, len(matched))
	for i, m := range matched {
		names[i] = m.Name
	}
	return total, names
}

// Definition is the size bound of a quality under this profile: its own
// limit when it sets one, else the global definition.
func (p QualityProfile) Definition(defs Definitions, id QualityID) QualityDefinition {
	if d, ok := p.spec.Sizes[id]; ok {
		return d
	}
	return defs.For(id)
}

func single(ids ...QualityID) []QualityGroup {
	out := make([]QualityGroup, len(ids))
	for i, id := range ids {
		out[i] = QualityGroup{Qualities: []QualityID{id}}
	}
	return out
}

func group(name string, ids ...QualityID) QualityGroup {
	return QualityGroup{Name: name, Qualities: ids}
}

func defaultSpecs(kind domain.MediaKind) []ProfileSpec {
	sd := single("dvd", "sdtv", "webrip-480p", "webdl-480p", "bluray-480p")
	anyGroups := append(sd,
		QualityGroup{Qualities: []QualityID{"hdtv-720p"}}, group("WEB 720p", "webrip-720p", "webdl-720p"),
		QualityGroup{Qualities: []QualityID{"bluray-720p"}},
		QualityGroup{Qualities: []QualityID{"hdtv-1080p"}}, group("WEB 1080p", "webrip-1080p", "webdl-1080p"),
		QualityGroup{Qualities: []QualityID{"bluray-1080p"}})
	hd := []QualityGroup{{Qualities: []QualityID{"hdtv-1080p"}}, group("WEB 1080p", "webrip-1080p", "webdl-1080p"), {Qualities: []QualityID{"bluray-1080p"}}}
	uhd := []QualityGroup{{Qualities: []QualityID{"hdtv-2160p"}}, group("WEB 2160p", "webrip-2160p", "webdl-2160p"), {Qualities: []QualityID{"bluray-2160p"}}}
	return []ProfileSpec{
		{Name: "Any", Kind: kind, Groups: anyGroups, Cutoff: "webdl-1080p", Upgrades: true},
		{Name: "HD-1080p", Kind: kind, Groups: hd, Cutoff: "bluray-1080p", Upgrades: true},
		{Name: "Ultra-HD", Kind: kind, Groups: uhd, Cutoff: "bluray-2160p", Upgrades: true},
	}
}

// DefaultProfiles returns the shipped profiles of a kind: video (Any,
// HD-1080p, Ultra-HD) for TV or movies, none of which allows Unknown,
// pre-release sources, Remux, disc images or 3D; music (Any, Standard,
// Lossless), none of which allows anything below MP3-128. Books get none.
func DefaultProfiles(kind domain.MediaKind) []QualityProfile {
	specs := map[domain.MediaKind][]ProfileSpec{domain.KindMovie: defaultSpecs(kind), domain.KindTV: defaultSpecs(kind),
		domain.KindMusic: musicSpecs()}[kind]
	var out []QualityProfile
	for _, s := range specs {
		p, err := NewQualityProfile(s)
		if err != nil {
			panic(err) // shipped data; covered by tests
		}
		out = append(out, p)
	}
	return out
}
