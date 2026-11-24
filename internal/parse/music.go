package parse

import (
	"regexp"
	"strconv"
	"strings"
)

// AudioFormat is the audio container/codec family a music release names.
type AudioFormat string

const (
	FormatFLAC    AudioFormat = "flac"
	FormatALAC    AudioFormat = "alac"
	FormatAPE     AudioFormat = "ape"
	FormatWavPack AudioFormat = "wavpack"
	FormatWAV     AudioFormat = "wav"
	FormatMP3     AudioFormat = "mp3"
	FormatAAC     AudioFormat = "aac"
	FormatVorbis  AudioFormat = "vorbis"
	FormatOpus    AudioFormat = "opus"
	FormatWMA     AudioFormat = "wma"
)

// Lossless reports whether the format keeps every sample.
func (f AudioFormat) Lossless() bool {
	switch f {
	case FormatFLAC, FormatALAC, FormatAPE, FormatWavPack, FormatWAV:
		return true
	}
	return false
}

// MusicRelease is what a music release title says. Like ParsedRelease,
// every attribute is either stated (Known) or unknown.
type MusicRelease struct {
	Artist      string             `json:"artist"`
	Album       string             `json:"album"`
	Year        Field[int]         `json:"year"`
	Format      Field[AudioFormat] `json:"format"`
	Bitrate     Field[int]         `json:"bitrate"`  // kbps; VBR presets map to their typical rate
	VBR         Field[string]      `json:"vbr"`      // "v0", "v2"
	BitDepth    Field[int]         `json:"bitDepth"` // 16 or 24
	Discography bool               `json:"discography"`
	Group       Field[string]      `json:"group"`
}

var (
	formatRules = []struct {
		re *regexp.Regexp
		f  AudioFormat
	}{
		{regexp.MustCompile(`(?i)\b(flac)\b`), FormatFLAC},
		{regexp.MustCompile(`(?i)\b(alac)\b`), FormatALAC},
		{regexp.MustCompile(`(?i)\b(ape)\b`), FormatAPE},
		{regexp.MustCompile(`(?i)\b(wavpack|wv)\b`), FormatWavPack},
		{regexp.MustCompile(`(?i)\b(wav)\b`), FormatWAV},
		{regexp.MustCompile(`(?i)\b(mp3)\b`), FormatMP3},
		{regexp.MustCompile(`(?i)\b(aac|m4a)\b`), FormatAAC},
		{regexp.MustCompile(`(?i)\b(ogg|vorbis)\b`), FormatVorbis},
		{regexp.MustCompile(`(?i)\b(opus)\b`), FormatOpus},
		{regexp.MustCompile(`(?i)\b(wma)\b`), FormatWMA},
	}
	kbpsRe     = regexp.MustCompile(`(?i)\b(\d{2,3})\s?(?:kbps|kbit|k)\b`)
	bareRateRe = regexp.MustCompile(`(?i)\b(?:mp3|aac)[\s._-]*\[?(\d{2,3})\]?\b`)
	vbrRe      = regexp.MustCompile(`(?i)\b(v0|v2)\b`)
	depthRe    = regexp.MustCompile(`(?i)\b(16|24)[\s._-]?bits?\b|\b(16|24)[-/](44|48|88|96|176|192)\b|\bhi[\s._-]?res\b`)
	yearRe     = regexp.MustCompile(`\b(19[5-9]\d|20[0-4]\d)\b`)
	rangeRe    = regexp.MustCompile(`\b(19|20)\d{2}\s?-\s?(19|20)\d{2}\b`)
	discogRe   = regexp.MustCompile(`(?i)\b(discography|complete albums|collection|anthology)\b`)
	sceneRe    = regexp.MustCompile(`-([A-Za-z0-9]+)$`)
	bracketRe  = regexp.MustCompile(`\s*[\[({].*$`)
)

var vbrRates = map[string]int{"v0": 245, "v2": 190}

// ParseMusic parses a music release title such as
// "Artist - Album (2020) [FLAC 24bit]" or "Artist-Album-WEB-2020-GROUP".
// It never fails: what it cannot read stays unknown.
func ParseMusic(title string) MusicRelease {
	m := MusicRelease{Discography: discogRe.MatchString(title) || rangeRe.MatchString(title)}
	fillAudioFormat(&m, title)
	if y := yearRe.FindString(title); y != "" {
		n, _ := strconv.Atoi(y)
		m.Year = known(n)
	}
	m.Artist, m.Album, m.Group = names(strings.TrimSpace(title))
	return m
}

func fillAudioFormat(m *MusicRelease, title string) {
	for _, r := range formatRules {
		if r.re.MatchString(title) {
			m.Format = known(r.f)
			break
		}
	}
	if v := vbrRe.FindString(title); v != "" {
		v = strings.ToLower(v)
		m.VBR, m.Bitrate = known(v), known(vbrRates[v])
	}
	if b := firstRate(title); b > 0 && !m.VBR.Known {
		m.Bitrate = known(b)
	}
	if d := depthRe.FindStringSubmatch(title); d != nil {
		m.BitDepth = known(depthOf(d))
	}
}

func firstRate(title string) int {
	for _, re := range []*regexp.Regexp{kbpsRe, bareRateRe} {
		if s := re.FindStringSubmatch(title); s != nil {
			n, _ := strconv.Atoi(s[1])
			if n >= 32 && n <= 512 {
				return n
			}
		}
	}
	return 0
}

func depthOf(d []string) int {
	switch {
	case d[1] != "":
		return atoi(d[1])
	case d[2] != "":
		return atoi(d[2])
	}
	return 24 // "hi-res"
}

// names splits artist, album and (scene titles) group. "Artist - Album …"
// splits on the first spaced dash; dotted or underscored scene titles split
// on dashes and keep the trailing group.
func names(title string) (artist, album string, group Field[string]) {
	if a, b, ok := strings.Cut(title, " - "); ok {
		return clean(a), cleanAlbum(b), group
	}
	s := strings.NewReplacer("_", " ", ".", " ").Replace(title)
	if g := sceneRe.FindStringSubmatch(s); g != nil && strings.Count(s, "-") >= 2 {
		group = known(g[1])
		s = s[:len(s)-len(g[0])]
	}
	parts := strings.Split(s, "-")
	if len(parts) < 2 {
		return "", cleanAlbum(s), group
	}
	return clean(parts[0]), cleanAlbum(parts[1]), group
}

func clean(s string) string { return strings.Join(strings.Fields(s), " ") }

// cleanAlbum drops bracketed tags and everything from the year or format on.
func cleanAlbum(s string) string {
	s = bracketRe.ReplaceAllString(s, "")
	if loc := yearRe.FindStringIndex(s); loc != nil && loc[0] > 0 {
		s = s[:loc[0]]
	}
	for _, r := range formatRules {
		if loc := r.re.FindStringIndex(s); loc != nil && loc[0] > 0 {
			s = s[:loc[0]]
		}
	}
	return strings.Trim(clean(s), " -")
}
