// Package naming renders library folder and file names from templates.
// It is pure: it builds strings and never touches disk.
//
// Template syntax: literal text plus tokens in braces, e.g.
//
//	{Movie Title}{ (Release Year)}/{Movie Title}{ (Release Year)}{ [Quality Full]}{ - Release Group}
//
// Text inside the braces around the token name is optional: it is written
// only when the token has a value, so "{ (Release Year)}" disappears when
// the year is unknown. A "/" in the template separates path segments.
package naming

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Token names understood by movie templates.
const (
	TokenMovieTitle   = "Movie Title"
	TokenReleaseYear  = "Release Year"
	TokenQualityFull  = "Quality Full"
	TokenEdition      = "Edition"
	TokenReleaseGroup = "Release Group"
	TokenIMDbID       = "IMDb Id"
)

// Token names understood by episode templates; season and episode values
// arrive zero-padded, a multi-episode file as "01-E03".
const (
	TokenSeriesTitle  = "Series Title"
	TokenSeason       = "Season"
	TokenEpisodeTitle = "Episode Title"
	TokenEpisode      = "Episode"
	TokenAirDate      = "Air Date"
)

// Token names understood by music templates; track and medium values arrive
// zero-padded.
const (
	TokenArtistName   = "Artist Name"
	TokenAlbumTitle   = "Album Title"
	TokenTrackTitle   = "Track Title"
	TokenTrack        = "Track"
	TokenMediumFormat = "Medium Format"
	TokenMedium       = "Medium"
)

// Default music templates: the medium folder is used only when the edition
// has more than one medium.
const (
	DefaultArtistFolder = "{Artist Name}"
	DefaultAlbumFolder  = "{Album Title}{ (Release Year)}"
	DefaultMediumFolder = "{Medium Format} {Medium}"
	DefaultTrackFile    = "{Artist Name} - {Album Title} - {Track} - {Track Title}"
)

// Default movie templates.
const (
	DefaultMovieFolder = "{Movie Title}{ (Release Year)}"
	DefaultMovieFile   = "{Movie Title}{ (Release Year)}{ - Edition}{ [Quality Full]}{ - Release Group}"
)

// Default episode templates; the season folder is "Specials" for season 0.
const (
	DefaultSeriesFolder  = "{Series Title}{ (Release Year)}"
	DefaultSeasonFolder  = "Season {Season}"
	DefaultSpecialFolder = "Specials"
	DefaultEpisodeFile   = "{Series Title} - S{Season}E{Episode}{ - Episode Title}{ [Quality Full]}"
)

// Values are the token values for one movie file.
type Values struct {
	MovieTitle   string
	ReleaseYear  int // 0 = unknown
	QualityFull  string
	Edition      string
	ReleaseGroup string
	IMDbID       string
	SeriesTitle  string
	Season       string
	Episode      string
	EpisodeTitle string
	AirDate      string
	ArtistName   string
	AlbumTitle   string
	TrackTitle   string
	Track        string
	MediumFormat string
	Medium       string
}

func (v Values) byToken() map[string]string {
	year := ""
	if v.ReleaseYear > 0 {
		year = strconv.Itoa(v.ReleaseYear)
	}
	return map[string]string{
		TokenMovieTitle: v.MovieTitle, TokenReleaseYear: year, TokenQualityFull: v.QualityFull,
		TokenEdition: v.Edition, TokenReleaseGroup: v.ReleaseGroup, TokenIMDbID: v.IMDbID,
		TokenSeriesTitle: v.SeriesTitle, TokenSeason: v.Season, TokenEpisodeTitle: v.EpisodeTitle,
		TokenEpisode: v.Episode, TokenAirDate: v.AirDate,
		TokenArtistName: v.ArtistName, TokenAlbumTitle: v.AlbumTitle, TokenTrackTitle: v.TrackTitle, TokenTrack: v.Track,
		TokenMediumFormat: v.MediumFormat, TokenMedium: v.Medium,
	}
}

// tokenNames is matched in order: a name that contains another comes first.
var tokenNames = []string{TokenMovieTitle, TokenReleaseYear, TokenQualityFull, TokenEdition, TokenReleaseGroup, TokenIMDbID,
	TokenSeriesTitle, TokenEpisodeTitle, TokenEpisode, TokenSeason, TokenAirDate,
	TokenArtistName, TokenAlbumTitle, TokenTrackTitle, TokenTrack, TokenMediumFormat, TokenMedium}

// part is literal text (token == "") or a token with optional affixes.
type part struct {
	text           string
	token          string
	prefix, suffix string
}

// Template is a parsed template: one list of parts per path segment.
type Template struct {
	segments [][]part
}

// ParseTemplate validates a template once, where it enters.
func ParseTemplate(s string) (Template, error) {
	if strings.TrimSpace(s) == "" {
		return Template{}, errors.New("template is empty")
	}
	var t Template
	for _, seg := range strings.Split(s, "/") {
		parts, err := parseSegment(seg)
		if err != nil {
			return Template{}, err
		}
		t.segments = append(t.segments, parts)
	}
	return t, nil
}

func parseSegment(seg string) ([]part, error) {
	var parts []part
	for seg != "" {
		open := strings.IndexAny(seg, "{}")
		if open < 0 {
			return append(parts, part{text: seg}), nil
		}
		if seg[open] == '}' {
			return nil, fmt.Errorf("unexpected '}' in %q", seg)
		}
		end := strings.IndexByte(seg[open:], '}')
		if end < 0 {
			return nil, fmt.Errorf("unclosed '{' in %q", seg)
		}
		tok, err := parseToken(seg[open+1 : open+end])
		if err != nil {
			return nil, err
		}
		parts = append(parts, part{text: seg[:open]}, tok)
		seg = seg[open+end+1:]
	}
	return parts, nil
}

func parseToken(body string) (part, error) {
	lower := strings.ToLower(body)
	for _, name := range tokenNames {
		if i := strings.Index(lower, strings.ToLower(name)); i >= 0 {
			return part{token: name, prefix: body[:i], suffix: body[i+len(name):]}, nil
		}
	}
	return part{}, fmt.Errorf("unknown token {%s}", body)
}

// ColonMode says what replaces ":".
type ColonMode string

const (
	ColonDelete    ColonMode = "delete"    // "A: B" → "A B", "12:30" → "1230"
	ColonDash      ColonMode = "dash"      // "A: B" → "A- B"
	ColonSpaceDash ColonMode = "spaceDash" // "A: B" → "A - B"
	ColonSmart     ColonMode = "smart"     // "A: B" → "A - B", "12:30" → "1230"
)

var colonReplacers = map[ColonMode]*strings.Replacer{
	ColonDelete:    strings.NewReplacer(":", ""),
	ColonDash:      strings.NewReplacer(":", "-"),
	ColonSpaceDash: strings.NewReplacer(":", " - "),
	ColonSmart:     strings.NewReplacer(": ", " - ", ":", ""),
}

// IllegalMode says how characters unsafe on Linux/SMB shares are handled.
type IllegalMode string

const (
	IllegalReplace IllegalMode = "replace" // swap for a lookalike or dash
	IllegalRemove  IllegalMode = "remove"
)

var illegalReplacers = map[IllegalMode]*strings.Replacer{
	IllegalReplace: strings.NewReplacer(`\`, "-", "/", "-", "|", "-", "*", "-", "?", "", `"`, "'", "<", "", ">", ""),
	IllegalRemove:  strings.NewReplacer(`\`, "", "/", "", "|", "", "*", "", "?", "", `"`, "", "<", "", ">", ""),
}

// MaxSegmentBytes is the per-path-component limit of common filesystems.
const MaxSegmentBytes = 255

// Options control rendering. The zero value means smart colons and replace.
type Options struct {
	Colon     ColonMode
	Illegal   IllegalMode
	Extension string // appended to the last segment, kept when truncating ("mkv" or ".mkv")
}

// Warning explains a change the renderer had to make.
type Warning struct {
	Code    string `json:"code"` // template | truncated | empty_segment | reserved_name
	Segment int    `json:"segment"`
	Message string `json:"message"`
}

// Preview renders a template with the default options.
func Preview(template string, v Values) (string, []Warning) { return Options{}.Preview(template, v) }

// Preview parses and renders a template; a template error is returned as a
// warning with an empty path, so a preview box can show it inline.
func (o Options) Preview(template string, v Values) (string, []Warning) {
	t, err := ParseTemplate(template)
	if err != nil {
		return "", []Warning{{Code: "template", Message: err.Error()}}
	}
	return o.Render(t, v)
}

// Render builds the relative path for a parsed template.
func (o Options) Render(t Template, v Values) (string, []Warning) {
	o = o.withDefaults()
	var segs []string
	var warns []Warning
	for i, parts := range t.segments {
		ext := ""
		if i == len(t.segments)-1 {
			ext = o.Extension
		}
		s, w := o.segment(i, parts, v.byToken(), ext)
		segs, warns = append(segs, s), append(warns, w...)
	}
	return strings.Join(segs, "/"), warns
}

func (o Options) withDefaults() Options {
	if colonReplacers[o.Colon] == nil {
		o.Colon = ColonSmart
	}
	if illegalReplacers[o.Illegal] == nil {
		o.Illegal = IllegalReplace
	}
	if o.Extension != "" && !strings.HasPrefix(o.Extension, ".") {
		o.Extension = "." + o.Extension
	}
	return o
}

func join(parts []part, vals map[string]string) string {
	var b strings.Builder
	for _, p := range parts {
		if p.token == "" {
			b.WriteString(p.text)
		} else if val := vals[p.token]; val != "" {
			b.WriteString(p.prefix + val + p.suffix)
		}
	}
	return b.String()
}

// clean applies colon and illegal-character rules, drops control characters,
// collapses spaces and trims what filesystems or shells mishandle at the ends.
func (o Options) clean(s string) string {
	s = colonReplacers[o.Colon].Replace(s)
	s = illegalReplacers[o.Illegal].Replace(s)
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimLeft(strings.TrimRight(s, ". "), ". ")
}

func (o Options) segment(i int, parts []part, vals map[string]string, ext string) (string, []Warning) {
	var warns []Warning
	s := o.clean(join(parts, vals))
	if len(s)+len(ext) > MaxSegmentBytes {
		s = o.shorten(parts, vals, ext)
		warns = append(warns, Warning{Code: "truncated", Segment: i,
			Message: fmt.Sprintf("name longer than %d bytes was shortened", MaxSegmentBytes)})
	}
	if s == "" {
		s = "_"
		warns = append(warns, Warning{Code: "empty_segment", Segment: i, Message: "segment rendered empty; using \"_\""})
	}
	if isReserved(s) {
		s += "_"
		warns = append(warns, Warning{Code: "reserved_name", Segment: i, Message: "name is reserved on Windows/SMB; added \"_\""})
	}
	return s + ext, warns
}

// shorten truncates the longest token value first, then falls back to
// cutting the whole segment; the extension is always kept.
func (o Options) shorten(parts []part, vals map[string]string, ext string) string {
	budget := MaxSegmentBytes - len(ext)
	s := o.clean(join(parts, vals))
	for len(s) > budget {
		name := longestToken(parts, vals)
		if name == "" {
			return strings.TrimRight(truncBytes(s, budget), ". ")
		}
		vals[name] = strings.TrimSpace(truncBytes(vals[name], len(vals[name])-(len(s)-budget)))
		s = o.clean(join(parts, vals))
	}
	return s
}

func longestToken(parts []part, vals map[string]string) string {
	best := ""
	for _, p := range parts {
		if p.token != "" && len(vals[p.token]) > len(vals[best]) {
			best = p.token
		}
	}
	return best
}

// truncBytes cuts s to at most n bytes without splitting a UTF-8 sequence.
func truncBytes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

var reserved = map[string]bool{"con": true, "prn": true, "aux": true, "nul": true}

func isReserved(s string) bool {
	base := strings.ToLower(strings.SplitN(s, ".", 2)[0])
	if reserved[base] {
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) && base[3] >= '1' && base[3] <= '9'
}
