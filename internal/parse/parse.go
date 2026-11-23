package parse

import (
	"regexp"
	"strings"
)

var (
	extRe        = regexp.MustCompile(`(?i)\.(mkv|mp4|avi|m4v|mov|wmv|webm|mpg|mpeg|m2ts|iso|img|nzb|torrent)$`)
	siteSuffixRe = regexp.MustCompile(`(?i)\s*\[(?:rarbg|eztv(?:\.re)?|ettv|tgx|rartv|publichd|[0-9a-f]{8})\]$`)
	sitePrefixRe = regexp.MustCompile(`(?i)^(?:www\.)?[a-z0-9-]+\.(?:com|org|net|to|re|mx|se)\s+-\s+`)
	leadGroupRe  = regexp.MustCompile(`^\[([^\]]+)\]\s*`)
	dashGroupRe  = regexp.MustCompile(`-([A-Za-z0-9_]+)$`)
	tailGroupRe  = regexp.MustCompile(`\s*\[([^\]]+)\]$`)
	digitsRe     = regexp.MustCompile(`\d+`)
	acronymRe    = regexp.MustCompile(`\b(?:[A-Za-z]\.){2,}[A-Za-z]?\b`)
	sepRe        = regexp.MustCompile(`[._\s]+`)
	akaRe        = regexp.MustCompile(`(?i)\s+aka\s+`)
)

// notGroups are dash suffixes that belong to a token (WEB-DL, DTS-HD, H-SBS).
var notGroups = map[string]bool{"dl": true, "hd": true, "ma": true, "rip": true, "sbs": true,
	"ou": true, "x": true, "es": true, "hdr": true, "ray": true, "ac3": true, "3": true}

// ParseRelease parses a release title. It never fails: whatever cannot be
// read stays unknown.
func ParseRelease(title string) ParsedRelease {
	s, ext, lead := strip(title)
	group, groupStart := findGroup(s, lead)
	ep, epIdx := findEpisode(s[:groupStart])
	titleEnd, year := locateTitle(s[:groupStart], epIdx)
	if titleEnd == groupStart && groupStart < len(s) {
		// Nothing marked the end of the title, so the "group" is part of it (Spider-Man).
		group, groupStart, titleEnd = Field[string]{}, len(s), len(s)
	}
	p := ParsedRelease{Raw: title, Year: year, Group: group, Episode: ep}
	p.Titles = cleanTitles(s[:titleEnd])
	if len(p.Titles) > 0 {
		p.Title = p.Titles[0]
	}
	tail := s[titleEnd:groupStart]
	fillVideo(&p, tail, ext)
	fillAudio(&p, tail)
	fillMeta(&p, tail)
	return p
}

// strip removes the file extension, site tags and a leading [group].
func strip(raw string) (s, ext, lead string) {
	s = strings.TrimSpace(raw)
	if m := extRe.FindStringSubmatch(s); m != nil {
		ext = strings.ToLower(m[1])
		s = s[:len(s)-len(m[0])]
	}
	s = siteSuffixRe.ReplaceAllString(s, "")
	s = siteSuffixRe.ReplaceAllString(s, "")
	s = sitePrefixRe.ReplaceAllString(s, "")
	if m := leadGroupRe.FindStringSubmatch(s); m != nil {
		lead = m[1]
		s = s[len(m[0]):]
	}
	return strings.TrimSpace(s), ext, lead
}

// findGroup returns the release group and where it starts in s.
func findGroup(s, lead string) (Field[string], int) {
	if m := dashGroupRe.FindStringSubmatchIndex(s); m != nil && isGroupName(s[m[2]:m[3]]) {
		return known(s[m[2]:m[3]]), m[0]
	}
	if m := tailGroupRe.FindStringSubmatchIndex(s); m != nil && isGroupName(s[m[2]:m[3]]) {
		return known(s[m[2]:m[3]]), m[0]
	}
	if lead != "" {
		return known(lead), len(s)
	}
	return Field[string]{}, len(s)
}

func isGroupName(g string) bool {
	if notGroups[strings.ToLower(g)] || digitsRe.FindString(g) == g {
		return false
	}
	padded := " " + g + " "
	return earliest(markerRules, padded) < 0 && earliest(softMarkerRules, padded) < 0 && !channelsRe.MatchString(padded)
}

// locateTitle finds where the title ends and the year, if any. The year is
// the last plausible year before the first strong marker, so "2001 A Space
// Odyssey 1968" and "Blade Runner 2049 2017" keep the numbers in the title.
func locateTitle(s string, epIdx int) (int, Field[int]) {
	limit := minPos(epIdx, earliest(markerRules, s))
	if idx, y := findYear(s, limit); idx > 0 {
		return idx, known(y)
	}
	end := minPos(limit, earliest(softMarkerRules, s))
	if end < 0 {
		end = len(s)
	}
	return end, Field[int]{}
}

// minPos is the smaller of two indexes where -1 means "absent".
func minPos(a, b int) int {
	if a < 0 || (b >= 0 && b < a) {
		return b
	}
	return a
}

func findYear(s string, limit int) (int, int) {
	idx, year := -1, 0
	for _, m := range digitsRe.FindAllStringIndex(s, -1) {
		if m[0] == 0 || (limit >= 0 && m[0] >= limit) || !isYearToken(s, m[0], m[1]) {
			continue
		}
		idx, year = m[0], atoi(s[m[0]:m[1]])
	}
	return idx, year
}

func isYearToken(s string, from, to int) bool {
	y := atoi(s[from:to])
	return to-from == 4 && y >= 1900 && y <= 2099 && !isAlnumAt(s, from-1) && !isAlnumAt(s, to)
}

func isAlnumAt(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	c := s[i] | 0x20
	return (c >= 'a' && c <= 'z') || (s[i] >= '0' && s[i] <= '9')
}

// cleanTitles turns the title part into readable titles, split on AKA.
// Dotted acronyms (S.W.A.T, E.T.) keep their dots.
func cleanTitles(seg string) []string {
	seg = acronymRe.ReplaceAllStringFunc(seg, protectAcronym)
	seg = sepRe.ReplaceAllString(seg, " ")
	seg = strings.ReplaceAll(seg, "\x00", ".")
	var out []string
	for _, t := range akaRe.Split(seg, -1) {
		if t = strings.Trim(t, " -([{"); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// protectAcronym hides an acronym's dots from separator handling; a trailing
// dot (E.T.) also ended the word, so a space follows it.
func protectAcronym(a string) string {
	out := strings.ReplaceAll(a, ".", "\x00")
	if strings.HasSuffix(a, ".") {
		out += " "
	}
	return out
}

func fillVideo(p *ParsedRelease, tail, ext string) {
	p.Resolution, _ = first(resolutionRules, tail)
	p.Source, _ = first(sourceRules, tail)
	p.Codec, _ = first(codecRules, tail)
	p.HDR = all(hdrRules, tail)
	if m := dvProfileRe.FindStringSubmatch(tail); m != nil {
		p.DVProfile = known(atoi(m[1]))
	}
	p.Disc, _ = first(discRules, tail)
	if ext == "iso" || ext == "img" {
		p.Disc = known(DiscFormat(ext))
	}
	p.ThreeD = len(all(threeDRule, tail)) > 0
}

func fillAudio(p *ParsedRelease, tail string) {
	p.Audio, _ = first(audioRules, tail)
	p.Atmos = len(all(atmosRule, tail)) > 0
	if m := channelsRe.FindStringSubmatch(tail); m != nil {
		p.Channels = known(m[1] + "." + m[2])
	}
}

func fillMeta(p *ParsedRelease, tail string) {
	p.Proper = len(all(properRule, tail)) > 0
	p.Repack = len(all(repackRule, tail)) > 0
	if m := versionRe.FindStringSubmatch(tail); m != nil {
		p.Version = known(atoi(m[1]))
	}
	p.Edition, _ = first(editionRules, tail)
	p.Languages = all(languageRules, tail)
}
