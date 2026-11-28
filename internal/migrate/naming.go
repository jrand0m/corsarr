package migrate

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/naming"
)

// SrcNaming is a source's naming settings: whether it renames files, its
// colon replacement (by name) and its formats by setting name.
type SrcNaming struct {
	Rename  *bool
	Colon   string
	Formats map[string]string
}

// TokenMapping is how one source naming token lands in corsarr's templates.
type TokenMapping struct {
	Source App    `json:"source"`
	Field  string `json:"field"`
	Token  string `json:"token"`
	Target string `json:"target,omitempty"`
	Status string `json:"status"` // exact | approximate | none
	Note   string `json:"note,omitempty"`
}

// Token mapping statuses.
const (
	TokenExact       = "exact"
	TokenApproximate = "approximate"
	TokenNone        = "none"
)

// NamingMapping is a source's naming mapped onto one kind's naming.
type NamingMapping struct {
	Kind   domain.MediaKind
	Scheme naming.Scheme
	Tokens []TokenMapping
	Notes  []string
}

// namingFields are corsarr's naming settings of each kind, in the order of
// the scheme's folder, inner folder and file templates.
var namingFields = map[domain.MediaKind][]string{
	domain.KindTV:    {"seriesFolder", "seasonFolder", "episodeFile"},
	domain.KindMovie: {"movieFolder", "", "movieFile"},
	domain.KindMusic: {"artistFolder", "albumFolder", "trackFile"},
}

// sourceFormats names the source settings that feed the folder, inner
// folder and file templates of each app.
var sourceFormats = map[App][3]string{
	Sonarr: {"seriesFolderFormat", "seasonFolderFormat", "standardEpisodeFormat"},
	Radarr: {"movieFolderFormat", "", "standardMovieFormat"},
	Lidarr: {"artistFolderFormat", "", "standardTrackFormat"},
}

// unmappedFormats are source formats corsarr has no template for; they are
// listed when set.
var unmappedFormats = map[App][]string{
	Sonarr: {"dailyEpisodeFormat", "animeEpisodeFormat"},
	Lidarr: {"multiDiscTrackFormat"},
}

type tokenRule struct {
	targets []string
	exact   bool
	note    string
	padded  bool // a number corsarr always writes with two digits
}

const (
	noteClean  = "written as the plain value (corsarr does not strip punctuation)"
	noteThe    = "written as the plain value (corsarr does not move a leading \"The\")"
	noteYear   = "written as the title followed by the year"
	notePadded = "corsarr always writes two digits"
)

// tokenRules maps a source token name (lower case, separators removed) onto
// corsarr tokens.
var tokenRules = map[string]tokenRule{
	"seriestitle": {targets: []string{naming.TokenSeriesTitle}, exact: true}, "seriescleantitle": {targets: []string{naming.TokenSeriesTitle}, note: noteClean},
	"seriestitlethe":       {targets: []string{naming.TokenSeriesTitle}, note: noteThe},
	"seriestitleyear":      {targets: []string{naming.TokenSeriesTitle, naming.TokenReleaseYear}, note: noteYear},
	"seriescleantitleyear": {targets: []string{naming.TokenSeriesTitle, naming.TokenReleaseYear}, note: noteYear},
	"season":               {targets: []string{naming.TokenSeason}, exact: true, padded: true}, "episode": {targets: []string{naming.TokenEpisode}, exact: true, padded: true},
	"episodetitle": {targets: []string{naming.TokenEpisodeTitle}, exact: true}, "episodecleantitle": {targets: []string{naming.TokenEpisodeTitle}, note: noteClean},
	"airdate":      {targets: []string{naming.TokenAirDate}, exact: true},
	"qualitytitle": {targets: []string{naming.TokenQualityFull}, exact: true},
	"qualityfull":  {targets: []string{naming.TokenQualityFull}, note: "written without the Proper/Repack mark"},
	"releasegroup": {targets: []string{naming.TokenReleaseGroup}, exact: true}, "releaseyear": {targets: []string{naming.TokenReleaseYear}, exact: true},
	"movietitle": {targets: []string{naming.TokenMovieTitle}, exact: true}, "moviecleantitle": {targets: []string{naming.TokenMovieTitle}, note: noteClean},
	"movietitlethe":       {targets: []string{naming.TokenMovieTitle}, note: noteThe},
	"movietitleyear":      {targets: []string{naming.TokenMovieTitle, naming.TokenReleaseYear}, note: noteYear},
	"moviecleantitleyear": {targets: []string{naming.TokenMovieTitle, naming.TokenReleaseYear}, note: noteYear},
	"editiontags":         {targets: []string{naming.TokenEdition}, note: "the edition corsarr reads from the release name"},
	"imdbid":              {targets: []string{naming.TokenIMDbID}, exact: true},
	"artistname":          {targets: []string{naming.TokenArtistName}, exact: true}, "artistcleanname": {targets: []string{naming.TokenArtistName}, note: noteClean},
	"artistnamethe": {targets: []string{naming.TokenArtistName}, note: noteThe},
	"albumtitle":    {targets: []string{naming.TokenAlbumTitle}, exact: true}, "albumcleantitle": {targets: []string{naming.TokenAlbumTitle}, note: noteClean},
	"tracktitle": {targets: []string{naming.TokenTrackTitle}, exact: true}, "trackcleantitle": {targets: []string{naming.TokenTrackTitle}, note: noteClean},
	"track": {targets: []string{naming.TokenTrack}, exact: true, padded: true}, "medium": {targets: []string{naming.TokenMedium}, exact: true, padded: true},
	"mediumformat": {targets: []string{naming.TokenMediumFormat}, exact: true},
}

// colonModes maps a source colon replacement onto corsarr's, with a note
// when it differs.
var colonModes = map[string]struct {
	mode naming.ColonMode
	note string
}{
	"delete": {naming.ColonDelete, ""}, "dash": {naming.ColonDash, ""}, "smart": {naming.ColonSmart, ""},
	"spacedashspace": {naming.ColonSpaceDash, ""},
	"spacedash":      {naming.ColonSpaceDash, "colons become \" - \" (a space on both sides)"},
	"custom":         {naming.ColonSmart, "a custom colon replacement became smart"},
}

// colonNames are the numeric forms of the colon replacement setting.
var colonNames = []string{"delete", "dash", "spaceDash", "spaceDashSpace", "smart", "custom"}

// colonName reads the colon replacement as a number or a name.
func colonName(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		if n >= 0 && n < len(colonNames) {
			return colonNames[n]
		}
		return ""
	}
	var s string
	_ = json.Unmarshal(raw, &s) // anything else reads as unset
	return s
}

// defaultScheme is the built-in naming of a kind.
func defaultScheme(k domain.MediaKind) naming.Scheme {
	switch k {
	case domain.KindTV:
		return naming.DefaultTV()
	case domain.KindMusic:
		return naming.DefaultMusic()
	}
	return naming.DefaultMovie()
}

// mapNaming maps a source's naming onto its kind's naming, token by token.
func mapNaming(a App, n SrcNaming) NamingMapping {
	m := NamingMapping{Kind: a.Kind(), Scheme: defaultScheme(a.Kind())}
	if n.Rename != nil {
		m.Scheme.Rename = *n.Rename
	}
	m.colon(n.Colon)
	formats := sourceFormats[a]
	folder, sub, file := n.Formats[formats[0]], "", n.Formats[formats[2]]
	if formats[1] != "" {
		sub = n.Formats[formats[1]]
	}
	if a == Lidarr {
		sub, file = m.splitTrack(file)
	}
	slots := []*string{&m.Scheme.Folder, &m.Scheme.Sub, &m.Scheme.File}
	for i, src := range []string{folder, sub, file} {
		field := formats[i]
		if a == Lidarr && i == 1 {
			field = formats[2]
		}
		if src != "" {
			*slots[i] = m.template(a, field, src, *slots[i])
		}
	}
	m.unmapped(a, n)
	return m
}

func (m *NamingMapping) colon(name string) {
	c, ok := colonModes[strings.ToLower(name)]
	switch {
	case name == "":
	case !ok:
		m.Notes = append(m.Notes, fmt.Sprintf("colon replacement %q became smart", name))
	default:
		m.Scheme.Colon = c.mode
		if c.note != "" {
			m.Notes = append(m.Notes, c.note)
		}
	}
}

// splitTrack splits a music track format that carries the album folder
// ("Album (Year)/Artist - Track") into the album folder and the file.
func (m *NamingMapping) splitTrack(format string) (sub, file string) {
	i := strings.LastIndex(format, "/")
	if i < 0 {
		if format != "" {
			m.Notes = append(m.Notes, "the track format has no album folder; corsarr keeps its own album folder")
		}
		return "", format
	}
	return format[:i], format[i+1:]
}

func (m *NamingMapping) unmapped(a App, n SrcNaming) {
	for _, f := range unmappedFormats[a] {
		if n.Formats[f] != "" {
			m.Notes = append(m.Notes, f+" is not carried over: corsarr uses one template for every "+map[App]string{Sonarr: "episode", Lidarr: "track"}[a])
		}
	}
	if sp := n.Formats["specialsFolderFormat"]; sp != "" && sp != naming.DefaultSpecialFolder {
		m.Notes = append(m.Notes, fmt.Sprintf("specials folder %q is not carried over: corsarr uses %q", sp, naming.DefaultSpecialFolder))
	}
}

// template converts one source format; when the result is empty or does not
// parse, def is kept and the reason noted.
func (m *NamingMapping) template(a App, field, src, def string) string {
	out, tokens := convertFormat(a, field, src)
	m.Tokens = append(m.Tokens, tokens...)
	if strings.Trim(out, " -_./") == "" {
		m.Notes = append(m.Notes, field+" has no token corsarr knows; the built-in template is kept")
		return def
	}
	if _, err := naming.ParseTemplate(out); err != nil {
		m.Notes = append(m.Notes, field+" could not be converted ("+err.Error()+"); the built-in template is kept")
		return def
	}
	return out
}

// convertFormat rewrites a source format into corsarr's template syntax and
// reports every token on the way.
func convertFormat(a App, field, src string) (string, []TokenMapping) {
	var b strings.Builder
	var tokens []TokenMapping
	for src != "" {
		open := strings.IndexByte(src, '{')
		end := strings.IndexByte(src, '}')
		if open < 0 || end < open {
			b.WriteString(strings.NewReplacer("{", "", "}", "").Replace(src))
			break
		}
		b.WriteString(strings.ReplaceAll(src[:open], "}", ""))
		text, tm, rest := convertBraces(src[open+1:end], src[end+1:])
		tm.Source, tm.Field = a, field
		b.WriteString(text)
		tokens = append(tokens, tm)
		src = rest
	}
	return b.String(), tokens
}

// convertBraces converts one brace group whose body runs up to the first
// closing brace. A nested group ("{imdb-{ImdbId}}") keeps the outer text as
// affixes of the inner token; rest is what follows the group.
func convertBraces(body, after string) (string, TokenMapping, string) {
	inner := strings.IndexByte(body, '{')
	if inner < 0 {
		text, tm := convertToken(body)
		return text, tm, after
	}
	outerSuffix, rest, _ := strings.Cut(after, "}")
	text, tm := convertToken(body[inner+1:])
	tm.Token = "{" + body + "}" + outerSuffix + "}"
	if text != "" {
		text = "{" + body[:inner] + text[1:len(text)-1] + outerSuffix + "}"
	}
	return text, tm, rest
}

// convertToken maps one brace body ("[Quality Full]", "season:00"): the text
// before the first letter and after the last letter or digit is kept as
// optional affixes around the corsarr token.
func convertToken(body string) (string, TokenMapping) {
	tm := TokenMapping{Token: "{" + body + "}", Status: TokenNone, Note: "no corsarr equivalent; left out"}
	first := strings.IndexFunc(body, unicode.IsLetter)
	last := strings.LastIndexFunc(body, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) })
	if first < 0 || last < first {
		return "", tm
	}
	prefix, core, suffix := body[:first], body[first:last+1], body[last+1:]
	name, format, _ := strings.Cut(core, ":")
	rule, ok := tokenRules[normalToken(name)]
	if !ok {
		return "", tm
	}
	tm.Status, tm.Note, tm.Target = TokenApproximate, rule.note, strings.Join(rule.targets, " + ")
	switch {
	case rule.padded && format != "00":
		tm.Note = notePadded
	case strings.ContainsAny(name, "._") && rule.exact:
		tm.Note = "written with spaces (the source separates words with " + string(name[strings.IndexAny(name, "._")]) + ")"
	case rule.exact:
		tm.Status = TokenExact
	}
	if len(rule.targets) == 1 {
		return "{" + prefix + rule.targets[0] + suffix + "}", tm
	}
	return "{" + prefix + rule.targets[0] + "}{ (" + rule.targets[1] + ")" + suffix + "}", tm
}

// normalToken lower-cases a token name and drops its word separators.
func normalToken(name string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '.' || r == '_' || r == '-' {
			return -1
		}
		return unicode.ToLower(r)
	}, name)
}

// namingSummary is the plan reason of a mapped naming.
func namingSummary(m NamingMapping) string {
	count := map[string]int{}
	var none []string
	for _, t := range m.Tokens {
		count[t.Status]++
		if t.Status == TokenNone {
			none = append(none, t.Token)
		}
	}
	rename := map[bool]string{true: "on", false: "off"}[m.Scheme.Rename]
	out := fmt.Sprintf("add naming.%s to your config file (see the snippet); renaming %s; tokens: %d exact, %d approximate, %d without an equivalent",
		m.Kind, rename, count[TokenExact], count[TokenApproximate], count[TokenNone])
	if len(none) > 0 {
		out += " (" + strings.Join(none, ", ") + ")"
	}
	if len(m.Notes) > 0 {
		out += "; " + strings.Join(m.Notes, "; ")
	}
	return out
}
