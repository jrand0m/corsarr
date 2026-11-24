package music

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
)

// AutoThreshold is the share of payload audio files that must map to a
// track before an import goes ahead without a person.
const AutoThreshold = 0.85

// Placement is one audio file mapped to one track.
type Placement struct {
	File  string // payload-relative path
	Track domain.Track
	By    string // "number" or "title": what decided the mapping
}

// FileMatch maps a payload's audio files onto an album's tracks.
type FileMatch struct {
	Placements []Placement `json:"placements"`
	Unmatched  []string    `json:"unmatched"` // audio files no track took
	Missing    []int64     `json:"missing"`   // track ids no file filled
	Score      float64     `json:"score"`     // matched share of the audio files
}

// Auto reports whether the match is good enough to import unattended.
func (m FileMatch) Auto() bool { return len(m.Placements) > 0 && m.Score >= AutoThreshold }

var (
	discTrackRe = regexp.MustCompile(`^(\d)[-._ ](\d{1,2})\b`)
	trackRe     = regexp.MustCompile(`^(\d{1,3})\b`)
	innerNumRe  = regexp.MustCompile(`(?:^|[ ._-])(\d{1,2})[ ._-]+\D`)
	discDirRe   = regexp.MustCompile(`(?i)\b(?:cd|disc|disk|vinyl|side)[ ._-]?(\d{1,2})\b`)
)

// fileKey is what a file name says about its position.
type fileKey struct {
	disc, number int // 0 = unknown
	name         string
}

// keyOf reads disc and track numbers from a payload path: "1-03 Title",
// "03 - Title", "Artist - Album - 03 - Title", and disc folders "CD2".
func keyOf(rel string) fileKey {
	base := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	k := fileKey{name: movies.Normalize(base)}
	if m := discDirRe.FindStringSubmatch(filepath.Dir(rel)); m != nil {
		k.disc, _ = strconv.Atoi(m[1])
	}
	switch m := discTrackRe.FindStringSubmatch(base); {
	case m != nil && k.disc == 0:
		k.disc, _ = strconv.Atoi(m[1])
		k.number, _ = strconv.Atoi(m[2])
	default:
		k.number = leadingNumber(base)
	}
	return k
}

func leadingNumber(base string) int {
	if m := trackRe.FindStringSubmatch(base); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n % 100 // "103" on a multi-disc rip is disc 1 track 3
	}
	if m := innerNumRe.FindStringSubmatch(base); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// MatchFiles maps audio files to tracks (pure): first by disc and track
// number (a number alone when the edition has one medium or the number is
// unique), then by the track title appearing in the file name. Each track
// takes at most one file.
func MatchFiles(files []string, tracks []domain.Track) FileMatch {
	taken := map[int64]bool{}
	var m FileMatch
	var rest []string
	for _, f := range files {
		if t, ok := byNumber(keyOf(f), tracks, taken); ok {
			taken[t.ID] = true
			m.Placements = append(m.Placements, Placement{File: f, Track: t, By: "number"})
			continue
		}
		rest = append(rest, f)
	}
	for _, f := range rest {
		if t, ok := byTitle(keyOf(f), tracks, taken); ok {
			taken[t.ID] = true
			m.Placements = append(m.Placements, Placement{File: f, Track: t, By: "title"})
			continue
		}
		m.Unmatched = append(m.Unmatched, f)
	}
	for _, t := range tracks {
		if !taken[t.ID] {
			m.Missing = append(m.Missing, t.ID)
		}
	}
	if len(files) > 0 {
		m.Score = float64(len(m.Placements)) / float64(len(files))
	}
	return m
}

func byNumber(k fileKey, tracks []domain.Track, taken map[int64]bool) (domain.Track, bool) {
	if k.number == 0 {
		return domain.Track{}, false
	}
	var hits []domain.Track
	for _, t := range tracks {
		if t.Number == k.number && (k.disc == 0 || t.Medium == k.disc) && !taken[t.ID] {
			hits = append(hits, t)
		}
	}
	if len(hits) != 1 {
		return domain.Track{}, false // unknown, or the same number on several discs
	}
	return hits[0], true
}

func byTitle(k fileKey, tracks []domain.Track, taken map[int64]bool) (domain.Track, bool) {
	var best domain.Track
	for _, t := range tracks {
		title := movies.Normalize(t.Title)
		if title != "" && !taken[t.ID] && strings.Contains(k.name, title) && len(title) > len(movies.Normalize(best.Title)) {
			best = t
		}
	}
	return best, best.ID != 0
}
