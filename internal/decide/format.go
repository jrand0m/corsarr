package decide

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
)

// ConditionKind names what a custom-format condition looks at.
type ConditionKind string

const (
	CondTitle       ConditionKind = "title"
	CondGroup       ConditionKind = "group"
	CondEdition     ConditionKind = "edition"
	CondSource      ConditionKind = "source"
	CondResolution  ConditionKind = "resolution"
	CondModifier    ConditionKind = "modifier"
	CondCodec       ConditionKind = "codec"
	CondAudio       ConditionKind = "audio"
	CondChannels    ConditionKind = "channels"
	CondHDR         ConditionKind = "hdr"
	CondLanguage    ConditionKind = "language"
	CondFlag        ConditionKind = "flag"
	CondSize        ConditionKind = "size"
	CondReleaseType ConditionKind = "releasetype"
	CondYear        ConditionKind = "year"
	CondContainer   ConditionKind = "container"
	CondBitDepth    ConditionKind = "bitdepth"
	CondDVFallback  ConditionKind = "dvfallback"
)

// ConditionInput is how a condition kind takes its value.
type ConditionInput string

const (
	InputPattern ConditionInput = "pattern" // a case-insensitive regular expression
	InputValues  ConditionInput = "values"  // one or more values from a fixed list
	InputRange   ConditionInput = "range"   // a minimum and a maximum
)

// Choice is one value a condition may hold, with its label.
type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// KindInfo describes one condition kind for editors and validation.
type KindInfo struct {
	Kind   ConditionKind  `json:"kind"`
	Label  string         `json:"label"`
	Input  ConditionInput `json:"input"`
	Unit   string         `json:"unit,omitempty"`
	Values []Choice       `json:"values,omitempty"`
	// Open means values outside the list are accepted too (languages).
	Open bool `json:"open,omitempty"`
}

func choices(pairs ...string) []Choice {
	out := make([]Choice, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, Choice{Value: pairs[i], Label: pairs[i+1]})
	}
	return out
}

var conditionKinds = []KindInfo{
	{Kind: CondTitle, Label: "Release title", Input: InputPattern},
	{Kind: CondGroup, Label: "Release group", Input: InputPattern},
	{Kind: CondEdition, Label: "Edition", Input: InputPattern},
	{Kind: CondSource, Label: "Source", Input: InputValues, Values: choices("cam", "CAM", "telesync", "Telesync", "telecine", "Telecine",
		"screener", "Screener", "dvd", "DVD", "hdtv", "TV", "webrip", "WEBRip", "webdl", "WEB-DL", "bluray", "Blu-ray",
		"remux", "Remux", "disc", "Full disc")},
	{Kind: CondResolution, Label: "Resolution", Input: InputValues, Values: choices("480", "480p", "576", "576p", "720", "720p",
		"1080", "1080p", "2160", "2160p")},
	{Kind: CondModifier, Label: "Quality modifier", Input: InputValues, Values: choices("remux", "Remux", "disc", "Full disc",
		"screener", "Screener", "regional", "Regional", "rawhd", "Raw HD")},
	{Kind: CondCodec, Label: "Video codec", Input: InputValues, Values: choices(string(parse.CodecH264), "H.264", string(parse.CodecH265), "H.265 (HEVC)",
		string(parse.CodecAV1), "AV1", string(parse.CodecVP9), "VP9", string(parse.CodecXviD), "XviD", string(parse.CodecVC1), "VC-1",
		string(parse.CodecMPEG2), "MPEG-2")},
	{Kind: CondAudio, Label: "Audio codec", Input: InputValues, Values: choices(string(parse.AudioAAC), "AAC", string(parse.AudioAC3), "AC3 (Dolby Digital)",
		string(parse.AudioEAC3), "EAC3 (Dolby Digital Plus)", string(parse.AudioDTS), "DTS", string(parse.AudioDTSHD), "DTS-HD",
		string(parse.AudioDTSX), "DTS:X", string(parse.AudioTrueHD), "TrueHD", string(parse.AudioFLAC), "FLAC",
		string(parse.AudioOpus), "Opus", string(parse.AudioMP3), "MP3", string(parse.AudioLPCM), "LPCM")},
	{Kind: CondChannels, Label: "Audio channels", Input: InputValues, Values: choices("1.0", "1.0 (mono)", "2.0", "2.0 (stereo)", "2.1", "2.1",
		"5.1", "5.1", "6.1", "6.1", "7.1", "7.1")},
	{Kind: CondHDR, Label: "HDR format", Input: InputValues, Values: choices(string(parse.HDR10), "HDR10", string(parse.HDR10Plus), "HDR10+",
		string(parse.HDRDV), "Dolby Vision", string(parse.HDRHLG), "HLG", "sdr", "SDR (no HDR)")},
	{Kind: CondLanguage, Label: "Language", Input: InputValues, Open: true, Values: choices("en", "English", "fr", "French", "de", "German",
		"es", "Spanish", "it", "Italian", "pt", "Portuguese", "nl", "Dutch", "sv", "Swedish", "no", "Norwegian", "da", "Danish",
		"fi", "Finnish", "pl", "Polish", "ru", "Russian", "uk", "Ukrainian", "cs", "Czech", "hu", "Hungarian", "tr", "Turkish",
		"el", "Greek", "he", "Hebrew", "ar", "Arabic", "hi", "Hindi", "ja", "Japanese", "ko", "Korean", "zh", "Chinese",
		"multi", "Multiple languages")},
	{Kind: CondFlag, Label: "Indexer flag", Input: InputValues, Values: choices("freeleech", "Freeleech", "halfleech", "Halfleech",
		"doubleupload", "Double upload", "freeleech75", "Freeleech 75 %", "freeleech25", "Freeleech 25 %", "internal", "Internal",
		"scene", "Scene", "nuked", "Nuked")},
	{Kind: CondSize, Label: "Size", Input: InputRange, Unit: "GB"},
	{Kind: CondReleaseType, Label: "Release type", Input: InputValues, Values: choices("episode", "Single episode",
		"multiepisode", "Multiple episodes", "seasonpack", "Season pack")},
	{Kind: CondYear, Label: "Year", Input: InputRange},
	{Kind: CondContainer, Label: "Container", Input: InputValues, Values: choices("mkv", "MKV", "mp4", "MP4", "m4v", "M4V",
		"avi", "AVI", "ts", "MPEG-TS", "m2ts", "M2TS", "wmv", "WMV", "iso", "ISO image")},
	{Kind: CondBitDepth, Label: "Video bit depth", Input: InputValues, Values: choices("8", "8-bit", "10", "10-bit", "12", "12-bit")},
	{Kind: CondDVFallback, Label: "Dolby Vision fallback", Input: InputValues, Values: choices("none", "Dolby Vision without HDR10 fallback (profile 5)",
		"hdr10", "Dolby Vision with an HDR10 base (profile 7 or 8)")},
}

// ConditionKinds lists every condition kind in editor order.
func ConditionKinds() []KindInfo { return slices.Clone(conditionKinds) }

// KindInfoOf finds a condition kind.
func KindInfoOf(k ConditionKind) (KindInfo, bool) {
	for _, info := range conditionKinds {
		if info.Kind == k {
			return info, true
		}
	}
	return KindInfo{}, false
}

// Subject is what conditions test, derived once per release. Empty strings,
// zero numbers and nil slices mean the release does not say.
type Subject struct {
	Title       string
	Group       string
	Edition     string
	Source      string
	Resolution  int
	Modifiers   []string
	Codec       string
	Audio       string
	Channels    string
	HDR         []string // "sdr" when the title names no HDR format
	Languages   []string // "en" when the title names no language
	Flags       []string
	Size        int64 // bytes
	ReleaseType string
	Year        int
	Container   string // from the largest listed file, else a title marker
	BitDepth    int    // from a title marker (10bit, Hi10P, …)
	// DVFallback is "none" for Dolby Vision without an HDR10 base (profile 5),
	// "hdr10" with one (profiles 7 and 8, or DV plus HDR10 named together).
	DVFallback string
}

// SubjectOf reads the condition facts of a release once.
func SubjectOf(r domain.Release, p parse.ParsedRelease) Subject {
	s := Subject{Title: r.Title, Group: p.Group.Value, Edition: p.Edition.Value, Codec: string(p.Codec.Value),
		Audio: string(p.Audio.Value), Channels: p.Channels.Value, Flags: r.Flags, Size: r.Size, Year: p.Year.Value,
		Languages: p.Languages, ReleaseType: releaseType(p.Episode), Container: containerOf(r), BitDepth: bitDepthOf(r.Title),
		DVFallback: dvFallbackOf(p)}
	if p.Resolution.Known {
		s.Resolution = int(p.Resolution.Value)
	}
	if p.Source.Known {
		s.Source = string(p.Source.Value)
	}
	if p.Disc.Known {
		s.Source = string(SourceDisc)
		s.Modifiers = append(s.Modifiers, "disc")
	}
	switch p.Source.Value {
	case parse.SourceRemux:
		s.Modifiers = append(s.Modifiers, "remux")
	case parse.SourceScreener:
		s.Modifiers = append(s.Modifiers, "screener")
	}
	for _, h := range p.HDR {
		s.HDR = append(s.HDR, string(h))
	}
	if len(s.HDR) == 0 {
		s.HDR = []string{"sdr"}
	}
	if len(s.Languages) == 0 {
		s.Languages = []string{"en"}
	}
	return s
}

// TitleSubject is the subject of a bare title (no size or indexer flags).
func TitleSubject(title string) Subject {
	return SubjectOf(domain.Release{Title: title}, parse.ParseRelease(title))
}

var (
	containerMarker = regexp.MustCompile(`(?i)[. \[(-](mkv|mp4|m4v|avi|m2ts|ts|wmv|iso)(?:$|[\]). -])`)
	bitDepthMarker  = regexp.MustCompile(`(?i)\b(?:(8|10|12)[ ._-]?bits?|hi(10)p)\b`)
	videoFiles      = map[string]bool{"mkv": true, "mp4": true, "m4v": true, "avi": true, "ts": true, "m2ts": true, "wmv": true, "iso": true}
)

// containerOf reads the container from the indexer's file list (the first
// video file), else from a marker in the title; "" when neither says.
func containerOf(r domain.Release) string {
	for _, f := range r.FileList {
		if i := strings.LastIndex(f, "."); i >= 0 {
			if ext := strings.ToLower(f[i+1:]); videoFiles[ext] {
				return ext
			}
		}
	}
	if m := containerMarker.FindStringSubmatch(r.Title); m != nil {
		return strings.ToLower(m[1])
	}
	return ""
}

// bitDepthOf reads a video bit-depth marker; 0 when the title has none.
func bitDepthOf(title string) int {
	m := bitDepthMarker.FindStringSubmatch(title)
	switch {
	case m == nil:
		return 0
	case m[2] != "":
		return 10
	}
	n := 0
	_, _ = fmt.Sscan(m[1], &n)
	return n
}

// dvFallbackOf tells Dolby Vision with an HDR10 base from DV alone; "" when
// the release has no DV or does not say which.
func dvFallbackOf(p parse.ParsedRelease) string {
	if !slices.Contains(p.HDR, parse.HDRDV) {
		return ""
	}
	switch {
	case p.DVProfile.Known && p.DVProfile.Value == 5:
		return "none"
	case p.DVProfile.Known && (p.DVProfile.Value == 7 || p.DVProfile.Value == 8):
		return "hdr10"
	case slices.Contains(p.HDR, parse.HDR10) || slices.Contains(p.HDR, parse.HDR10Plus):
		return "hdr10"
	}
	return ""
}

func releaseType(e parse.Episode) string {
	switch e.Kind {
	case parse.EpisodeSeason:
		return "seasonpack"
	case parse.EpisodeStandard:
		if len(e.Episodes) > 1 {
			return "multiepisode"
		}
		return "episode"
	}
	return ""
}

// Condition is one compiled test of a custom format; build it from a
// ConditionDoc (Compile) or with NewCondition for a title regex.
type Condition struct {
	kind     ConditionKind
	re       *regexp.Regexp
	values   []string
	min, max float64
	negate   bool
	required bool
	except   bool
}

// NewCondition compiles a case-insensitive release-title regex condition.
func NewCondition(name, pattern string, negate, required bool) (Condition, error) {
	re, err := compilePattern(pattern)
	if err != nil {
		return Condition{}, fmt.Errorf("condition %q: %w", name, err)
	}
	return Condition{kind: CondTitle, re: re, negate: negate, required: required}, nil
}

func compilePattern(pattern string) (*regexp.Regexp, error) {
	if strings.TrimSpace(pattern) == "" {
		return nil, errors.New("pattern is empty")
	}
	return regexp.Compile(`(?i)` + pattern)
}

// Kind is what the condition looks at.
func (c Condition) Kind() ConditionKind { return c.kind }

// Required reports whether the condition must match for its format to match.
func (c Condition) Required() bool { return c.required }

// Matches reports whether the condition holds for a subject (negation applied).
// A value the release does not state never matches before negation.
func (c Condition) Matches(s Subject) bool { return c.test(s) != c.negate }

func (c Condition) test(s Subject) bool {
	switch c.kind {
	case CondTitle:
		return c.re.MatchString(s.Title)
	case CondGroup:
		return s.Group != "" && c.re.MatchString(s.Group)
	case CondEdition:
		return s.Edition != "" && c.re.MatchString(s.Edition)
	case CondSize:
		return s.Size > 0 && float64(s.Size) > c.min*gb && (c.max <= 0 || float64(s.Size) <= c.max*gb)
	case CondYear:
		return s.Year > 0 && float64(s.Year) >= c.min && (c.max <= 0 || float64(s.Year) <= c.max)
	case CondLanguage:
		return c.testLanguage(s.Languages)
	}
	return c.anyValue(c.subjectValues(s))
}

func (c Condition) subjectValues(s Subject) []string {
	switch c.kind {
	case CondSource:
		return nonEmpty(s.Source)
	case CondResolution:
		if s.Resolution == 0 {
			return nil
		}
		return []string{fmt.Sprint(s.Resolution)}
	case CondModifier:
		return s.Modifiers
	case CondCodec:
		return nonEmpty(s.Codec)
	case CondAudio:
		return nonEmpty(s.Audio)
	case CondChannels:
		return nonEmpty(s.Channels)
	case CondHDR:
		return s.HDR
	case CondFlag:
		return s.Flags
	case CondContainer:
		return nonEmpty(s.Container)
	case CondBitDepth:
		if s.BitDepth == 0 {
			return nil
		}
		return []string{fmt.Sprint(s.BitDepth)}
	case CondDVFallback:
		return nonEmpty(s.DVFallback)
	case CondReleaseType:
		return nonEmpty(s.ReleaseType)
	}
	return nil
}

func nonEmpty(v string) []string {
	if v == "" {
		return nil
	}
	return []string{v}
}

func (c Condition) anyValue(have []string) bool {
	for _, h := range have {
		if slices.Contains(c.values, strings.ToLower(h)) {
			return true
		}
	}
	return false
}

// testLanguage: the release has one of the values, or with except, a
// language outside them.
func (c Condition) testLanguage(langs []string) bool {
	if !c.except {
		return c.anyValue(langs)
	}
	for _, l := range langs {
		if !slices.Contains(c.values, strings.ToLower(l)) {
			return true
		}
	}
	return false
}

// CustomFormat is a named, compiled condition set.
type CustomFormat struct {
	key        string
	name       string
	conditions []Condition
}

// NewCustomFormat refuses a format without a name or conditions.
func NewCustomFormat(name string, conditions ...Condition) (CustomFormat, error) {
	if name == "" || len(conditions) == 0 {
		return CustomFormat{}, errors.New("a custom format needs a name and at least one condition")
	}
	return CustomFormat{key: KeyOf(name), name: name, conditions: slices.Clone(conditions)}, nil
}

func (f CustomFormat) Name() string { return f.name }
func (f CustomFormat) Key() string  { return f.key }

// Matches groups the conditions by kind: a group passes when all its
// required conditions match and, if it has others, at least one of those
// matches; the format matches when every group passes.
func (f CustomFormat) Matches(s Subject) bool {
	return formatMatches(f.conditions, f.Explain(s))
}

// Explain is each condition's result for a subject, in order.
func (f CustomFormat) Explain(s Subject) []bool {
	out := make([]bool, len(f.conditions))
	for i, c := range f.conditions {
		out[i] = c.Matches(s)
	}
	return out
}

type groupState struct{ requiredFailed, hasOptional, optionalHit bool }

func formatMatches(conds []Condition, hits []bool) bool {
	groups := map[ConditionKind]*groupState{}
	for i, c := range conds {
		g := groups[c.kind]
		if g == nil {
			g = &groupState{}
			groups[c.kind] = g
		}
		if c.required {
			g.requiredFailed = g.requiredFailed || !hits[i]
			continue
		}
		g.hasOptional, g.optionalHit = true, g.optionalHit || hits[i]
	}
	for _, g := range groups {
		if g.requiredFailed || (g.hasOptional && !g.optionalHit) {
			return false
		}
	}
	return len(groups) > 0
}

// KeyOf derives a stable object key from a name: lower-case letters and
// digits only ("HD-1080p" → "hd1080p"); a name without any becomes "x".
func KeyOf(name string) string {
	k := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, strings.ToLower(name))
	if k == "" || k[0] < 'a' {
		k = "x" + k
	}
	if len(k) > 48 {
		k = k[:48]
	}
	return k
}
