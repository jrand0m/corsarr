package decide

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Condition is one title-regex test of a custom format. It is compiled once,
// at construction; an invalid pattern can never reach a search.
type Condition struct {
	name     string
	re       *regexp.Regexp
	negate   bool
	required bool
}

// NewCondition compiles a case-insensitive title regex.
func NewCondition(name, pattern string, negate, required bool) (Condition, error) {
	re, err := regexp.Compile(`(?i)` + pattern)
	if err != nil {
		return Condition{}, fmt.Errorf("condition %q: %w", name, err)
	}
	return Condition{name: name, re: re, negate: negate, required: required}, nil
}

// Matches reports whether the condition holds for a title (negation applied).
func (c Condition) Matches(title string) bool { return c.re.MatchString(title) != c.negate }

// CustomFormat is a named condition set (title regex only for the MVP).
type CustomFormat struct {
	name       string
	conditions []Condition
}

// NewCustomFormat refuses a format without a name or conditions.
func NewCustomFormat(name string, conditions ...Condition) (CustomFormat, error) {
	if name == "" || len(conditions) == 0 {
		return CustomFormat{}, errors.New("a custom format needs a name and at least one condition")
	}
	return CustomFormat{name: name, conditions: append([]Condition(nil), conditions...)}, nil
}

func (f CustomFormat) Name() string { return f.name }

// Matches is true when every required condition holds and at least one condition holds.
func (f CustomFormat) Matches(title string) bool {
	anyHit := false
	for _, c := range f.conditions {
		ok := c.Matches(title)
		if c.required && !ok {
			return false
		}
		anyHit = anyHit || ok
	}
	return anyHit
}

// FormatScore is a profile's score for one format.
type FormatScore struct {
	Format CustomFormat
	Score  int
}

// QualityGroup is a set of qualities a profile treats as equal. A group of
// one needs no name.
type QualityGroup struct {
	Name      string
	Qualities []QualityID
}

// ProfileSpec is the editable shape of a QualityProfile.
type ProfileSpec struct {
	Name                string
	Kind                domain.MediaKind
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
	if _, err := domain.ParseMediaKind(string(spec.Kind)); err != nil {
		return QualityProfile{}, fmt.Errorf("profile %q: %w", spec.Name, err)
	}
	rank, err := rankGroups(spec.Groups)
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
	spec.Groups = append([]QualityGroup(nil), spec.Groups...)
	spec.Formats = append([]FormatScore(nil), spec.Formats...)
	return QualityProfile{spec: spec, rank: rank}, nil
}

func rankGroups(groups []QualityGroup) (map[QualityID]int, error) {
	rank := map[QualityID]int{}
	for i, g := range groups {
		for _, id := range g.Qualities {
			if _, ok := byID[id]; !ok {
				return nil, fmt.Errorf("unknown quality %q", id)
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

// Score sums the scores of the formats a title matches and names them.
func (p QualityProfile) Score(title string) (int, []string) {
	total, names := 0, []string{}
	for _, fs := range p.spec.Formats {
		if fs.Format.Matches(title) {
			total += fs.Score
			names = append(names, fs.Format.name)
		}
	}
	return total, names
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

// DefaultProfiles returns the shipped video profiles (Any, HD-1080p, Ultra-HD)
// for TV or movies; none of them allows Unknown, pre-release sources, Remux,
// disc images or 3D. Other kinds get none.
func DefaultProfiles(kind domain.MediaKind) []QualityProfile {
	if kind != domain.KindMovie && kind != domain.KindTV {
		return nil
	}
	var out []QualityProfile
	for _, s := range defaultSpecs(kind) {
		p, err := NewQualityProfile(s)
		if err != nil {
			panic(err) // shipped data; covered by tests
		}
		out = append(out, p)
	}
	return out
}
