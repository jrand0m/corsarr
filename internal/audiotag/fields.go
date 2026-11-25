package audiotag

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// setter stores one tag value. Every setter keeps a value already set, so
// the first source read wins and alternate key spellings act as fallbacks.
type setter func(t *Tags, v string)

// fieldKeys maps a normalised key (upper case, no spaces, underscores or
// dashes) to its setter. The same table serves Vorbis comments, APE items,
// ID3 user text frames and MP4 freeform atoms, which all use these names.
var fieldKeys = map[string]setter{
	"TITLE":       text(func(t *Tags) *string { return &t.Title }),
	"ARTIST":      text(func(t *Tags) *string { return &t.Artist }),
	"ALBUMARTIST": text(func(t *Tags) *string { return &t.AlbumArtist }),
	"ALBUM":       text(func(t *Tags) *string { return &t.Album }),

	"TRACKNUMBER": numbered(func(t *Tags) (*int, *int) { return &t.Track, &t.TrackTotal }),
	"TRACK":       numbered(func(t *Tags) (*int, *int) { return &t.Track, &t.TrackTotal }),
	"TRACKTOTAL":  count(func(t *Tags) *int { return &t.TrackTotal }),
	"TOTALTRACKS": count(func(t *Tags) *int { return &t.TrackTotal }),
	"DISCNUMBER":  numbered(func(t *Tags) (*int, *int) { return &t.Disc, &t.DiscTotal }),
	"DISC":        numbered(func(t *Tags) (*int, *int) { return &t.Disc, &t.DiscTotal }),
	"DISCTOTAL":   count(func(t *Tags) *int { return &t.DiscTotal }),
	"TOTALDISCS":  count(func(t *Tags) *int { return &t.DiscTotal }),

	"DATE":         date(func(t *Tags) *string { return &t.Date }),
	"YEAR":         date(func(t *Tags) *string { return &t.Date }),
	"ORIGINALDATE": date(func(t *Tags) *string { return &t.OriginalDate }),
	"ORIGINALYEAR": date(func(t *Tags) *string { return &t.OriginalDate }),

	"LABEL":                  text(func(t *Tags) *string { return &t.Label }),
	"ORGANIZATION":           text(func(t *Tags) *string { return &t.Label }),
	"PUBLISHER":              text(func(t *Tags) *string { return &t.Label }),
	"RELEASESTATUS":          text(func(t *Tags) *string { return &t.ReleaseStatus }),
	"MUSICBRAINZALBUMSTATUS": text(func(t *Tags) *string { return &t.ReleaseStatus }),
	"RELEASETYPE":            text(func(t *Tags) *string { return &t.ReleaseType }),
	"MUSICBRAINZALBUMTYPE":   text(func(t *Tags) *string { return &t.ReleaseType }),

	"MUSICBRAINZALBUMID":        id(func(t *Tags) *string { return &t.MusicBrainz.Album }),
	"MUSICBRAINZRELEASEGROUPID": id(func(t *Tags) *string { return &t.MusicBrainz.ReleaseGroup }),
	"MUSICBRAINZARTISTID":       id(func(t *Tags) *string { return &t.MusicBrainz.Artist }),
	"MUSICBRAINZALBUMARTISTID":  id(func(t *Tags) *string { return &t.MusicBrainz.AlbumArtist }),
	"MUSICBRAINZTRACKID":        id(func(t *Tags) *string { return &t.MusicBrainz.Recording }),
	"MUSICBRAINZRELEASETRACKID": id(func(t *Tags) *string { return &t.MusicBrainz.Track }),
}

var keyNoise = strings.NewReplacer(" ", "", "_", "", "-", "")

// set stores value under a tagger key such as "ALBUMARTIST",
// "Album Artist" or "MusicBrainz Album Id"; unknown keys are ignored.
func (t *Tags) set(key, value string) {
	value = strings.TrimSpace(value)
	if f, ok := fieldKeys[keyNoise.Replace(strings.ToUpper(key))]; ok && value != "" {
		f(t, value)
	}
}

// setLength stores a tagged length in milliseconds as the fallback duration.
func (t *Tags) setLength(ms string) {
	if n, err := strconv.Atoi(strings.TrimSpace(ms)); err == nil && n > 0 && t.Duration == 0 {
		t.Duration = time.Duration(n) * time.Millisecond
	}
}

func text(sel func(*Tags) *string) setter {
	return func(t *Tags, v string) {
		if p := sel(t); *p == "" {
			*p = v
		}
	}
}

// id keeps the first identifier of a multi-valued entry.
func id(sel func(*Tags) *string) setter {
	return text(sel).first()
}

func (f setter) first() setter {
	return func(t *Tags, v string) {
		v, _, _ = strings.Cut(strings.NewReplacer(";", "/", ",", "/").Replace(v), "/")
		if v = strings.TrimSpace(v); v != "" {
			f(t, v)
		}
	}
}

func count(sel func(*Tags) *int) setter {
	return func(t *Tags, v string) {
		if p := sel(t); *p == 0 {
			*p = atoi(v)
		}
	}
}

// numbered reads "N" or "N/Total".
func numbered(sel func(*Tags) (*int, *int)) setter {
	return func(t *Tags, v string) {
		n, total, _ := strings.Cut(v, "/")
		pn, pt := sel(t)
		if *pn == 0 {
			*pn = atoi(n)
		}
		if *pt == 0 {
			*pt = atoi(total)
		}
	}
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

var dateRe = regexp.MustCompile(`^(\d{4})(?:[-./](\d{2})(?:[-./](\d{2}))?)?`)

// date stores a plausible date normalised to YYYY, YYYY-MM or YYYY-MM-DD.
func date(sel func(*Tags) *string) setter {
	return text(sel).normalised()
}

func (f setter) normalised() setter {
	return func(t *Tags, v string) {
		if d := normaliseDate(v); d != "" {
			f(t, d)
		}
	}
}

// normaliseDate keeps the date part of a tagged timestamp. Years outside
// the span of recorded music (before 1860 or after next year) are rejected
// as garbage.
func normaliseDate(v string) string {
	m := dateRe.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil || !plausibleYear(atoi(m[1])) {
		return ""
	}
	return strings.Join(nonEmpty(m[1:]), "-")
}

func plausibleYear(y int) bool { return y >= 1860 && y <= time.Now().Year()+1 }

func nonEmpty(parts []string) []string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func yearOf(d string) int {
	if len(d) < 4 {
		return 0
	}
	return atoi(d[:4])
}

// readComments parses a Vorbis comment block (as used by FLAC, Ogg Vorbis
// and Opus): a vendor string, then KEY=value entries.
func readComments(b []byte, t *Tags) {
	p := 4 + int(le32(b, 0))
	n := int(le32(b, p))
	p += 4
	for i := 0; i < n; i++ {
		l := int(le32(b, p))
		entry := slice(b, p+4, l)
		if entry == nil {
			return
		}
		if k, v, ok := strings.Cut(string(entry), "="); ok {
			t.set(k, v)
		}
		p += 4 + l
	}
}
