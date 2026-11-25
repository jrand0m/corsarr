package music

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/audiotag"
	"github.com/jrand0m/corsarr/internal/domain"
)

// Matching thresholds. A distance runs from 0 (certainly the same track)
// to 1 (certainly not).
const (
	// AutoThreshold is the share of payload audio files that must map to a
	// track before an import goes ahead without a person.
	AutoThreshold = 0.85
	// TrackFloor is the distance from which a file and a track are no match.
	TrackFloor = 0.4
	// AlbumFloor is the mean distance of the placed files above which the
	// import waits for a person instead of guessing.
	AlbumFloor = 0.3
)

// AudioFile is one payload audio file with the tags read from it. Tags is
// zero when the file could not be read; matching then relies on its name.
type AudioFile struct {
	Rel  string // payload-relative path
	Tags audiotag.Tags
}

// Named wraps payload paths whose tags are unknown.
func Named(rels ...string) []AudioFile {
	out := make([]AudioFile, len(rels))
	for i, r := range rels {
		out[i] = AudioFile{Rel: r}
	}
	return out
}

// Placement is one audio file mapped to one track.
type Placement struct {
	File     string // payload-relative path
	Track    domain.Track
	By       string  // the strongest agreeing evidence: "recording", "number" or "title"
	Distance float64 // how far apart file and track are, below TrackFloor
}

// FileMatch maps a payload's audio files onto an album's tracks.
type FileMatch struct {
	Placements []Placement `json:"placements"`
	Unmatched  []string    `json:"unmatched"` // audio files no track took
	Missing    []int64     `json:"missing"`   // track ids no file filled
	Score      float64     `json:"score"`     // matched share of the audio files
	Distance   float64     `json:"distance"`  // mean distance of the placements
}

// Auto reports whether the match is good enough to import unattended:
// enough of the files are placed, and the placements are close on average.
func (m FileMatch) Auto() bool {
	return len(m.Placements) > 0 && m.Score >= AutoThreshold && m.Distance <= AlbumFloor
}

var (
	discTrackRe   = regexp.MustCompile(`^(\d)[-._ ](\d{1,2})\b`)
	trackRe       = regexp.MustCompile(`^(\d{1,3})\b`)
	innerNumRe    = regexp.MustCompile(`(?:^|[ ._-])(\d{1,2})[ ._-]+\D`)
	discDirRe     = regexp.MustCompile(`(?i)\b(?:cd|disc|disk|vinyl|side)[ ._-]?(\d{1,2})\b`)
	leadingNumsRe = regexp.MustCompile(`^[\d\s._-]+`)
)

// fileKey is what a file name says about its position.
type fileKey struct {
	disc, number int    // 0 = unknown
	name         string // the base name without a leading track number
}

// keyOf reads disc and track numbers from a payload path: "1-03 Title",
// "03 - Title", "Artist - Album - 03 - Title", and disc folders "CD2".
func keyOf(rel string) fileKey {
	base := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	k := fileKey{name: leadingNumsRe.ReplaceAllString(base, "")}
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

// MatchFiles maps audio files to tracks (pure). Every file-track pair gets
// a distance from the title, the disc and track number, the length and the
// recording id, using the tags where present and the file name otherwise.
// Files and tracks are then paired so the summed distance is the smallest
// possible; a pair at TrackFloor or farther is left unmatched.
func MatchFiles(files []AudioFile, tracks []domain.Track) FileMatch {
	ev, cand := make([]evidence, len(files)), newCandidates(tracks)
	for i, f := range files {
		ev[i] = evidenceOf(f)
	}
	cost := pairCosts(ev, cand)
	m := FileMatch{Placements: []Placement{}, Unmatched: []string{}, Missing: []int64{}}
	taken := make([]bool, len(tracks))
	cols := assign(cost)
	for i, f := range files {
		j := cols[i]
		if j >= len(tracks) || cost[i][j] >= TrackFloor {
			m.Unmatched = append(m.Unmatched, f.Rel)
			continue
		}
		taken[j] = true
		m.Placements = append(m.Placements, Placement{File: f.Rel, Track: tracks[j], By: cand.by(ev[i], j), Distance: cost[i][j]})
	}
	m.Missing = missing(tracks, taken)
	m.Score, m.Distance = summary(m.Placements, len(files))
	return m
}

func missing(tracks []domain.Track, taken []bool) []int64 {
	out := []int64{}
	for j, t := range tracks {
		if !taken[j] {
			out = append(out, t.ID)
		}
	}
	return out
}

// summary is the placed share of the files and the placements' mean distance.
func summary(ps []Placement, files int) (score, distance float64) {
	if files == 0 || len(ps) == 0 {
		return 0, 0
	}
	for _, p := range ps {
		distance += p.Distance
	}
	return float64(len(ps)) / float64(files), distance / float64(len(ps))
}

// pairCosts is the square distance matrix of files to tracks. Distances
// are capped at TrackFloor, the cost of leaving a file or track unpaired,
// which is also what the padding rows and columns cost.
func pairCosts(ev []evidence, cand candidates) [][]float64 {
	n := max(len(ev), len(cand.tracks))
	cost := make([][]float64, n)
	for i := range cost {
		cost[i] = make([]float64, n)
		for j := range cost[i] {
			cost[i][j] = TrackFloor
			if i < len(ev) && j < len(cand.tracks) {
				cost[i][j] = min(cand.distance(ev[i], j), TrackFloor)
			}
		}
	}
	return cost
}

// evidence is what one file says about the track it holds.
type evidence struct {
	disc, number int    // 0 = unknown
	title        string // the tagged title; "" = none
	name         string // the file name without its track number
	duration     time.Duration
	recording    string
}

// evidenceOf prefers the tags and falls back to the file name for every
// field the tags leave empty.
func evidenceOf(f AudioFile) evidence {
	k := keyOf(f.Rel)
	return evidence{disc: or(f.Tags.Disc, k.disc), number: or(f.Tags.Track, k.number), title: f.Tags.Title, name: k.name,
		duration: f.Tags.Duration, recording: strings.ToLower(f.Tags.MusicBrainz.Recording)}
}

func or(a, b int) int {
	if a > 0 {
		return a
	}
	return b
}

// Weights of the parts of a pair distance. A recording id is the strongest
// evidence; a title read from a file name is the weakest.
const (
	weightRecording = 5
	weightNumber    = 3
	weightTitle     = 3
	weightName      = 1
	weightDuration  = 2
)

// part is one weighed comparison; unknown parts are left out.
type part struct {
	weight, distance float64
	known            bool
}

// candidates are an album's tracks with what number matching needs.
type candidates struct {
	tracks   []domain.Track
	absolute []int       // 1-based position over every medium
	hits     map[int]int // how many tracks a bare number could name
}

func newCandidates(tracks []domain.Track) candidates {
	c := candidates{tracks: tracks, absolute: make([]int, len(tracks)), hits: map[int]int{}}
	for j, t := range tracks {
		c.absolute[j] = j + 1
		c.hits[t.Number]++
		if c.absolute[j] != t.Number {
			c.hits[c.absolute[j]]++
		}
	}
	return c
}

// distance is the weighted mean of every known part; 1 when nothing is known.
func (c candidates) distance(e evidence, j int) float64 {
	var sum, weights float64
	for _, p := range c.parts(e, j) {
		sum += p.weight * p.distance
		weights += p.weight
	}
	if weights == 0 {
		return 1
	}
	return sum / weights
}

func (c candidates) parts(e evidence, j int) []part {
	t := c.tracks[j]
	rd, rok := recordingDistance(e.recording, t.RecordingID)
	nd, nok := c.numberDistance(e, j)
	tw, td, tok := textPart(e, t.Title)
	dd, dok := durationDistance(e.duration, t.DurationSec)
	all := []part{{weightRecording, rd, rok}, {weightNumber, nd, nok}, {tw, td, tok}, {weightDuration, dd, dok}}
	out := all[:0]
	for _, p := range all {
		if p.known {
			out = append(out, p)
		}
	}
	return out
}

func recordingDistance(file, track string) (float64, bool) {
	if file == "" || track == "" {
		return 0, false
	}
	if strings.EqualFold(file, track) {
		return 0, true
	}
	return 1, true
}

// numberAmbiguous is the distance of a bare track number that several
// tracks carry (the same number on several discs).
const numberAmbiguous = 0.35

// numberDistance compares disc and track numbers. Without a disc the
// number may count within a disc or over the whole album.
func (c candidates) numberDistance(e evidence, j int) (float64, bool) {
	t := c.tracks[j]
	switch {
	case e.number == 0 || t.Number == 0:
		return 0, false
	case e.disc > 0 && t.Medium > 0:
		return boolDistance(e.disc == t.Medium && e.number == t.Number), true
	case e.number != t.Number && e.number != c.absolute[j]:
		return 1, true
	case c.hits[e.number] > 1:
		return numberAmbiguous, true
	}
	return 0, true
}

func boolDistance(same bool) float64 {
	if same {
		return 0
	}
	return 1
}

// textPart compares the tagged title, or else the file name, with the
// track title.
func textPart(e evidence, track string) (float64, float64, bool) {
	if e.title != "" {
		return weightTitle, titleDistance(e.title, track), true
	}
	d, ok := nameDistance(e.name, track)
	return weightName, d, ok
}

// titleContained is the distance of a title that holds the other whole,
// as in "Song (Remastered)" for "Song".
const titleContained = 0.1

// minContained is the shortest reduced title that may count as contained.
const minContained = 3

// titleDistance compares two titles once reduced for comparison.
func titleDistance(a, b string) float64 {
	ka, kb := matchKey(a), matchKey(b)
	switch {
	case ka == kb:
		return 0
	case contains(ka, kb) || contains(kb, ka):
		return titleContained
	}
	return textDistance(ka, kb)
}

// nameDistance compares a file name with a track title: a name that holds
// the whole title is a match ("Artist - Album - Title").
func nameDistance(name, title string) (float64, bool) {
	kn, kt := matchKey(name), matchKey(title)
	switch {
	case kn == "" || kt == "":
		return 0, false
	case kn == kt || contains(kn, kt):
		return 0, true
	}
	return textDistance(kn, kt), true
}

// textFloor is the similarity at or below which two texts share nothing
// worth counting; unrelated words still share a few letters.
const textFloor = 0.5

// textDistance spreads the similarity above textFloor over the whole
// distance range, so a typo stays close and unrelated titles are far.
func textDistance(a, b string) float64 {
	return min(1, (1-similarity(a, b))/(1-textFloor))
}

func contains(long, short string) bool {
	return len([]rune(short)) >= minContained && strings.Contains(long, short)
}

// Lengths within durationSlack count as equal; the distance then grows to
// 1 over durationScale.
const (
	durationSlack = 10 * time.Second
	durationScale = 30 * time.Second
)

func durationDistance(file time.Duration, trackSec int) (float64, bool) {
	if file <= 0 || trackSec <= 0 {
		return 0, false
	}
	delta := file - time.Duration(trackSec)*time.Second
	if delta < 0 {
		delta = -delta
	}
	return min(1, max(0, float64(delta-durationSlack)/float64(durationScale))), true
}

// by names the strongest evidence that agrees for a placed pair.
func (c candidates) by(e evidence, j int) string {
	if d, ok := recordingDistance(e.recording, c.tracks[j].RecordingID); ok && d == 0 {
		return "recording"
	}
	if d, ok := c.numberDistance(e, j); ok && d < 1 {
		return "number"
	}
	return "title"
}
