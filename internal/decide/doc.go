package decide

import (
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Documents are the plain, editable shape of custom formats and profiles:
// the API body, the database row and the config file entry. Compile turns
// them into the validated types the engine uses.

// ConditionDoc is one condition of a format document.
type ConditionDoc struct {
	Kind     ConditionKind `json:"kind"`
	Pattern  string        `json:"pattern,omitempty"`
	Values   []string      `json:"values,omitempty"`
	Min      float64       `json:"min,omitempty"`
	Max      float64       `json:"max,omitempty"`
	Negate   bool          `json:"negate,omitempty"`
	Required bool          `json:"required,omitempty"`
	Except   bool          `json:"except,omitempty"`
}

// FormatDoc is a custom format document.
type FormatDoc struct {
	Key        string         `json:"key"`
	Name       string         `json:"name"`
	Conditions []ConditionDoc `json:"conditions"`
}

// GroupDoc is one rank of a profile's ladder; several qualities share it.
type GroupDoc struct {
	Name      string      `json:"name,omitempty"`
	Qualities []QualityID `json:"qualities"`
}

// ScoreDoc is a profile's score for one format.
type ScoreDoc struct {
	Format string `json:"format"`
	Score  int    `json:"score"`
	Reject bool   `json:"reject,omitempty"`
}

// SizeDoc is a profile's own size limit for one quality, in MB per minute.
// Preferred 0 takes the global preferred size, kept inside min..max.
type SizeDoc struct {
	Quality   QualityID `json:"quality"`
	Min       float64   `json:"min"`
	Preferred float64   `json:"preferred,omitempty"`
	Max       float64   `json:"max"`
}

// ProfileDoc is a custom quality profile document.
type ProfileDoc struct {
	Key                   string             `json:"key"`
	Name                  string             `json:"name"`
	Kinds                 []domain.MediaKind `json:"kinds"`
	Groups                []GroupDoc         `json:"groups"`
	Cutoff                QualityID          `json:"cutoff"`
	Upgrades              bool               `json:"upgrades"`
	Formats               []ScoreDoc         `json:"formats,omitempty"`
	MinFormatScore        int                `json:"minFormatScore"`
	CutoffFormatScore     int                `json:"cutoffFormatScore"`
	MinUpgradeFormatScore int                `json:"minUpgradeFormatScore,omitempty"`
	Sizes                 []SizeDoc          `json:"sizes,omitempty"`
	AllowDiscImage        bool               `json:"allowDiscImage,omitempty"`
	Allow3D               bool               `json:"allow3D,omitempty"`
}

// DocError is one validation failure at a path inside a document
// ("conditions.2.pattern").
type DocError struct {
	Path string `json:"field"`
	Msg  string `json:"message"`
}

func (e DocError) Error() string {
	if e.Path == "" {
		return e.Msg
	}
	return e.Path + ": " + e.Msg
}

// Limits on documents.
const (
	maxNameLen    = 80
	maxConditions = 50
)

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9]{0,47}$`)
var langPattern = regexp.MustCompile(`^([a-z]{2,3}|multi)$`)

type errs []DocError

func (e *errs) add(path, format string, a ...any) {
	*e = append(*e, DocError{Path: path, Msg: fmt.Sprintf(format, a...)})
}

func checkIdentity(e *errs, key, name string) {
	if !keyPattern.MatchString(key) {
		e.add("key", "must start with a lower-case letter and use only lower-case letters and digits (at most 48)")
	}
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > maxNameLen {
		e.add("name", "must be 1 to %d characters", maxNameLen)
	}
}

// CompileFormat validates a format document and compiles its conditions.
func CompileFormat(d FormatDoc) (CustomFormat, []DocError) {
	var e errs
	checkIdentity(&e, d.Key, d.Name)
	if len(d.Conditions) == 0 || len(d.Conditions) > maxConditions {
		e.add("conditions", "needs 1 to %d conditions", maxConditions)
	}
	conds := make([]Condition, 0, len(d.Conditions))
	for i, cd := range d.Conditions {
		c, ok := compileCondition(&e, fmt.Sprintf("conditions.%d", i), cd)
		if ok {
			conds = append(conds, c)
		}
	}
	if len(e) > 0 {
		return CustomFormat{}, e
	}
	return CustomFormat{key: d.Key, name: strings.TrimSpace(d.Name), conditions: conds}, nil
}

func compileCondition(e *errs, path string, d ConditionDoc) (Condition, bool) {
	info, ok := KindInfoOf(d.Kind)
	if !ok {
		e.add(path+".kind", "unknown condition kind %q", d.Kind)
		return Condition{}, false
	}
	if d.Except && d.Kind != CondLanguage {
		e.add(path+".except", "only language conditions can use except")
	}
	c := Condition{kind: d.Kind, negate: d.Negate, required: d.Required, except: d.Except}
	before := len(*e)
	switch info.Input {
	case InputPattern:
		c.re = checkPattern(e, path+".pattern", d.Pattern)
	case InputValues:
		c.values = checkValues(e, path+".values", info, d.Values)
	case InputRange:
		c.min, c.max = d.Min, d.Max
		checkRange(e, path, d)
	}
	return c, len(*e) == before
}

func checkPattern(e *errs, path, pattern string) *regexp.Regexp {
	re, err := compilePattern(pattern)
	if err == nil {
		return re
	}
	var se *syntax.Error
	if _, perr := syntax.Parse(pattern, syntax.Perl); errors.As(perr, &se) {
		if at := strings.Index(pattern, se.Expr); at >= 0 && se.Expr != pattern {
			e.add(path, "not a valid regular expression: %s at position %d", se.Code, at+1)
			return nil
		}
		e.add(path, "not a valid regular expression: %s", se.Code)
		return nil
	}
	e.add(path, "%v", err)
	return nil
}

func checkValues(e *errs, path string, info KindInfo, values []string) []string {
	if len(values) == 0 {
		e.add(path, "choose at least one value")
		return nil
	}
	out := make([]string, 0, len(values))
	for i, v := range values {
		v = strings.ToLower(strings.TrimSpace(v))
		if !valueAllowed(info, v) {
			e.add(fmt.Sprintf("%s.%d", path, i), "%q is not a %s value", v, strings.ToLower(info.Label))
			continue
		}
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func valueAllowed(info KindInfo, v string) bool {
	if info.Open && langPattern.MatchString(v) {
		return true
	}
	for _, c := range info.Values {
		if c.Value == v {
			return true
		}
	}
	return false
}

func checkRange(e *errs, path string, d ConditionDoc) {
	if d.Min < 0 || d.Max < 0 {
		e.add(path+".min", "must be 0 or more")
		return
	}
	if d.Max > 0 && d.Max < d.Min || (d.Kind == CondSize && d.Max > 0 && d.Max == d.Min) {
		e.add(path+".max", "must be above the minimum (0 means no upper limit)")
	}
}

// builtinKeys are the keys of every shipped profile (of any kind).
var builtinKeys = sync.OnceValue(func() map[string]bool {
	out := map[string]bool{}
	for _, k := range []domain.MediaKind{domain.KindTV, domain.KindMovie, domain.KindMusic} {
		for _, p := range DefaultProfiles(k) {
			out[p.Key()] = true
		}
	}
	return out
})

// IsBuiltinKey reports whether a key names a shipped profile.
func IsBuiltinKey(key string) bool { return builtinKeys()[key] }

// CompileProfile validates a profile document against the known formats.
func CompileProfile(d ProfileDoc, formats map[string]CustomFormat) (QualityProfile, []DocError) {
	var e errs
	checkIdentity(&e, d.Key, d.Name)
	if builtinKeys()[d.Key] {
		e.add("key", "%q is the key of a built-in profile; choose another", d.Key)
	}
	audio := checkKinds(&e, d.Kinds)
	groups, allowed := checkGroups(&e, d.Groups, audio)
	if _, ok := allowed[d.Cutoff]; !ok && len(allowed) > 0 {
		e.add("cutoff", "must be one of the allowed qualities")
	}
	scores := checkScores(&e, d.Formats, formats)
	sizes := checkSizes(&e, d.Sizes, allowed)
	if d.MinUpgradeFormatScore < 0 {
		e.add("minUpgradeFormatScore", "must be 1 or more")
	}
	if len(e) > 0 {
		return QualityProfile{}, e
	}
	p, err := NewQualityProfile(ProfileSpec{Key: d.Key, Name: strings.TrimSpace(d.Name), Kinds: d.Kinds, Groups: groups,
		Cutoff: d.Cutoff, Upgrades: d.Upgrades, Formats: scores, MinFormatScore: d.MinFormatScore,
		CutoffFormatScore: d.CutoffFormatScore, MinUpgradeIncrement: d.MinUpgradeFormatScore, Sizes: sizes,
		AllowDiscImage: d.AllowDiscImage, Allow3D: d.Allow3D})
	if err != nil {
		return QualityProfile{}, []DocError{{Msg: err.Error()}}
	}
	return p, nil
}

// checkKinds accepts one kind, or TV and movies together; it reports
// whether the profile is on the audio ladder.
func checkKinds(e *errs, kinds []domain.MediaKind) bool {
	if len(kinds) == 0 {
		e.add("kinds", "choose what the profile is for")
		return false
	}
	seen := map[domain.MediaKind]bool{}
	for i, k := range kinds {
		if k != domain.KindTV && k != domain.KindMovie && k != domain.KindMusic {
			e.add(fmt.Sprintf("kinds.%d", i), "must be tv, movie or music")
		}
		seen[k] = true
	}
	if seen[domain.KindMusic] && len(kinds) > 1 {
		e.add("kinds", "music profiles cannot also serve TV or movies")
	}
	return seen[domain.KindMusic]
}

func checkGroups(e *errs, docs []GroupDoc, audio bool) ([]QualityGroup, map[QualityID]bool) {
	allowed := map[QualityID]bool{}
	groups := make([]QualityGroup, 0, len(docs))
	for i, g := range docs {
		path := fmt.Sprintf("groups.%d", i)
		if len(g.Qualities) == 0 {
			e.add(path+".qualities", "a group needs at least one quality")
		}
		for j, id := range g.Qualities {
			if msg := qualityProblem(id, audio, allowed); msg != "" {
				e.add(fmt.Sprintf("%s.qualities.%d", path, j), "%s", msg)
				continue
			}
			allowed[id] = true
		}
		groups = append(groups, QualityGroup{Name: strings.TrimSpace(g.Name), Qualities: slices.Clone(g.Qualities)})
	}
	if len(docs) == 0 {
		e.add("groups", "allow at least one quality")
	}
	return groups, allowed
}

func qualityProblem(id QualityID, audio bool, seen map[QualityID]bool) string {
	q, ok := byID[id]
	_, isAudio := audioTiers[id]
	switch {
	case !ok:
		return fmt.Sprintf("unknown quality %q", id)
	case isAudio != audio:
		return fmt.Sprintf("%s is not on this kind's ladder", q.Name)
	case seen[id]:
		return fmt.Sprintf("%s is listed twice", q.Name)
	case BelowFloor(id):
		return fmt.Sprintf("%s is below the quality floor and can never be grabbed", q.Name)
	}
	return ""
}

// BelowFloor reports whether a quality sits below its kind's global floor:
// Unknown for video, anything under MP3-128 for music.
func BelowFloor(id QualityID) bool {
	if _, isAudio := audioTiers[id]; isAudio {
		return byID[id].Rank < byID[musicFloor].Rank
	}
	return id == QualityUnknown.ID
}

func checkScores(e *errs, docs []ScoreDoc, formats map[string]CustomFormat) []FormatScore {
	out := make([]FormatScore, 0, len(docs))
	seen := map[string]bool{}
	for i, s := range docs {
		path := fmt.Sprintf("formats.%d.format", i)
		f, ok := formats[s.Format]
		switch {
		case !ok:
			e.add(path, "no custom format with the key %q", s.Format)
			continue
		case seen[s.Format]:
			e.add(path, "%s is scored twice", f.name)
			continue
		}
		seen[s.Format] = true
		out = append(out, FormatScore{Format: f, Score: s.Score, Reject: s.Reject})
	}
	return out
}

func checkSizes(e *errs, docs []SizeDoc, allowed map[QualityID]bool) map[QualityID]QualityDefinition {
	out := map[QualityID]QualityDefinition{}
	for i, s := range docs {
		path := fmt.Sprintf("sizes.%d", i)
		if !allowed[s.Quality] {
			e.add(path+".quality", "%q is not an allowed quality", s.Quality)
			continue
		}
		if _, dup := out[s.Quality]; dup {
			e.add(path+".quality", "%q has two size limits", s.Quality)
			continue
		}
		pref := s.Preferred
		if pref == 0 {
			pref = min(max(shippedDefinition(s.Quality).preferred, s.Min), s.Max)
		}
		d, err := NewQualityDefinition(s.Min, pref, s.Max)
		if err != nil {
			e.add(path+".max", "%v", err)
			continue
		}
		out[s.Quality] = d
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ObjectError is a document error with the object it belongs to.
type ObjectError struct {
	Object string // "format" or "profile"
	Key    string
	DocError
}

func (e ObjectError) Error() string { return e.Object + " " + e.Key + ": " + e.DocError.Error() }

// BuildCatalog compiles format and profile documents next to the built-in
// profiles. Invalid documents are left out and reported; a profile is
// checked against the formats that compiled. Duplicate keys are reported.
func BuildCatalog(formats []FormatDoc, profiles []ProfileDoc) (*Catalog, []ObjectError) {
	var out []ObjectError
	report := func(obj, key string, es []DocError) {
		for _, e := range es {
			out = append(out, ObjectError{Object: obj, Key: key, DocError: e})
		}
	}
	c := newCatalog()
	for _, d := range formats {
		if _, dup := c.formatByKey[d.Key]; dup {
			report("format", d.Key, []DocError{{Path: "key", Msg: "another custom format has this key"}})
			continue
		}
		f, es := CompileFormat(d)
		if len(es) > 0 {
			report("format", d.Key, es)
			continue
		}
		c.addFormat(f)
	}
	for _, d := range profiles {
		if _, dup := c.customByKey[d.Key]; dup {
			report("profile", d.Key, []DocError{{Path: "key", Msg: "another profile has this key"}})
			continue
		}
		p, es := CompileProfile(d, c.formatByKey)
		if len(es) > 0 {
			report("profile", d.Key, es)
			continue
		}
		c.addProfile(p)
	}
	c.sort()
	return c, out
}

// Doc turns a compiled custom profile back into its document.
func (p QualityProfile) Doc() ProfileDoc {
	s := p.spec
	d := ProfileDoc{Key: s.Key, Name: s.Name, Kinds: slices.Clone(s.Kinds), Cutoff: s.Cutoff, Upgrades: s.Upgrades,
		MinFormatScore: s.MinFormatScore, CutoffFormatScore: s.CutoffFormatScore, MinUpgradeFormatScore: s.MinUpgradeIncrement,
		AllowDiscImage: s.AllowDiscImage, Allow3D: s.Allow3D}
	for _, g := range s.Groups {
		d.Groups = append(d.Groups, GroupDoc{Name: g.Name, Qualities: slices.Clone(g.Qualities)})
	}
	for _, f := range s.Formats {
		d.Formats = append(d.Formats, ScoreDoc{Format: f.Format.key, Score: f.Score, Reject: f.Reject})
	}
	ids := make([]QualityID, 0, len(s.Sizes))
	for id := range s.Sizes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return byID[ids[i]].Rank < byID[ids[j]].Rank })
	for _, id := range ids {
		q := s.Sizes[id]
		d.Sizes = append(d.Sizes, SizeDoc{Quality: id, Min: q.min, Preferred: q.preferred, Max: q.max})
	}
	return d
}

// Doc turns a compiled format back into its document.
func (f CustomFormat) Doc() FormatDoc {
	d := FormatDoc{Key: f.key, Name: f.name}
	for _, c := range f.conditions {
		cd := ConditionDoc{Kind: c.kind, Values: slices.Clone(c.values), Min: c.min, Max: c.max, Negate: c.negate,
			Required: c.required, Except: c.except}
		if c.re != nil {
			cd.Pattern = strings.TrimPrefix(c.re.String(), "(?i)")
		}
		d.Conditions = append(d.Conditions, cd)
	}
	return d
}
