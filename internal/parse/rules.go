package parse

import "regexp"

// A token rule matches one naming token between separators. Separators are
// space . _ - and brackets; the leading boundary is non-capturing so that the
// token itself is submatch 1 and its index is exact.
const (
	lb = `(?:^|[\s._\-\[\(\{,/])`
	rb = `(?:$|[\s._\-\]\)\},/])`
	// audio tokens may carry the channel layout glued on: DDP5.1, AAC2.0
	ch = `(?:[ ._-]?[124-8][ ._][01])?`
)

func tok(p string) *regexp.Regexp { return regexp.MustCompile(`(?i)` + lb + `(` + p + `)` + rb) }

type rule[T comparable] struct {
	re  *regexp.Regexp
	val T
}

// first returns the value of the first rule (in priority order) that matches
// s, and the index where that token starts.
func first[T comparable](rules []rule[T], s string) (Field[T], int) {
	for _, r := range rules {
		if m := r.re.FindStringSubmatchIndex(s); m != nil {
			return known(r.val), m[2]
		}
	}
	return Field[T]{}, -1
}

// all returns every distinct rule value that matches s, in rule order.
func all[T comparable](rules []rule[T], s string) []T {
	var out []T
	seen := map[T]bool{}
	for _, r := range rules {
		if !seen[r.val] && r.re.MatchString(s) {
			seen[r.val] = true
			out = append(out, r.val)
		}
	}
	return out
}

// earliest is the smallest token index > 0 of any rule, or -1.
func earliest[T comparable](rules []rule[T], s string) int {
	best := -1
	for _, r := range rules {
		for _, m := range r.re.FindAllStringSubmatchIndex(s, -1) {
			if m[2] > 0 && (best < 0 || m[2] < best) {
				best = m[2]
			}
		}
	}
	return best
}

var resolutionRules = []rule[Resolution]{
	{tok(`2160[pi]|4k|uhd|3840x2160`), R2160},
	{tok(`1080[pi]|1920x1080|fhd`), R1080},
	{tok(`720p|1280x720`), R720},
	{tok(`576[pi]`), R576},
	{tok(`480[pi]|640x480|720x480`), R480},
}

// Order matters: the more specific source wins.
var sourceRules = []rule[Source]{
	{tok(`(?:bd|uhd|dvd)?remux`), SourceRemux},
	{tok(`dvdscr|bdscr|webscr|scr|screener`), SourceScreener},
	{tok(`hdtc|tc|telecine`), SourceTelecine},
	{tok(`hdts|ts|telesync|pdvd`), SourceTelesync},
	{tok(`hdcam|cam-?rip|cam`), SourceCam},
	{tok(`(?:uhd[ ._-]?)?blu-?ray|bdrip|brrip|bd25|bd50|bd66|bd100|bdmv`), SourceBluray},
	{tok(`web-?rip`), SourceWEBRip},
	{tok(`web-?dl|web|amzn|nf|dsnp|hmax|atvp|hulu|pcok|itunes`), SourceWEBDL},
	{tok(`hdtv(?:rip)?|pdtv|sdtv|dsr(?:ip)?|tvrip`), SourceHDTV},
	{tok(`dvd-?rip|dvd(?:r|5|9)?`), SourceDVD},
}

var codecRules = []rule[Codec]{
	{tok(`x\.?265|h\.?265|hevc(?:10)?`), CodecH265},
	{tok(`x\.?264|h\.?264|avc`), CodecH264},
	{tok(`av1`), CodecAV1},
	{tok(`vp9`), CodecVP9},
	{tok(`xvid|divx`), CodecXviD},
	{tok(`vc-?1`), CodecVC1},
	{tok(`mpeg-?2`), CodecMPEG2},
}

var hdrRules = []rule[HDR]{
	{tok(`dv|dovi|dolby[ ._-]?vision|(?:dv|dovi)[ ._-]?p[5-9]`), HDRDV},
	{tok(`hdr10(?:\+|plus|p)`), HDR10Plus},
	{tok(`hdr10|hdr`), HDR10},
	{tok(`hlg`), HDRHLG},
}

var dvProfileRe = regexp.MustCompile(`(?i)(?:dv|dovi|dolby[ ._-]?vision)[ ._-]?(?:p|profile[ ._-]?)([5-9])`)

var discRules = []rule[DiscFormat]{
	{tok(`iso`), DiscISO},
	{tok(`img`), DiscIMG},
	{tok(`bdmv|br-?disk|complete[ ._-](?:uhd[ ._-])?blu-?ray|bd25|bd50|bd66|bd100`), DiscBDMV},
}

var threeDRule = []rule[bool]{{tok(`3d|h-?sbs|half-?sbs|sbs|h-?ou|half-?ou|mvc`), true}}

var audioRules = []rule[Audio]{
	{tok(`true-?hd` + ch), AudioTrueHD},
	{tok(`dts[ ._-]?x` + ch), AudioDTSX},
	{tok(`dts[ ._-]?hd(?:[ ._-]?(?:ma|hra?))?` + ch + `|dts[ ._-]?ma` + ch), AudioDTSHD},
	{tok(`dts(?:[ ._-]?es)?` + ch), AudioDTS},
	{tok(`e-?ac-?3` + ch + `|ddp` + ch + `|dd\+` + ch), AudioEAC3},
	{tok(`ac-?3` + ch + `|dd` + ch + `|dolby[ ._-]?digital`), AudioAC3},
	{tok(`(?:he-?)?aac(?:[ ._-]?lc)?` + ch), AudioAAC},
	{tok(`flac` + ch), AudioFLAC},
	{tok(`opus` + ch), AudioOpus},
	{tok(`mp3`), AudioMP3},
	{tok(`l?pcm` + ch), AudioLPCM},
}

var atmosRule = []rule[bool]{{tok(`atmos`), true}}

var channelsRe = regexp.MustCompile(`(?:^|[^\d])([124-8])[ .]([01])(?:$|[^\d])`)

var properRule = []rule[bool]{{tok(`proper|real[ ._-]proper`), true}}
var repackRule = []rule[bool]{{tok(`repack\d?|rerip`), true}}
var versionRe = regexp.MustCompile(`(?i)(?:^|[\s._\-\[\(]|e\d{1,3}|^\d{1,3})v([2-9])(?:$|[\s._\-\]\)])`)

var editionRules = []rule[string]{
	{tok(`director'?s?[ ._-]?cut|dc`), "Director's Cut"},
	{tok(`extended(?:[ ._-](?:cut|edition))?`), "Extended"},
	{tok(`theatrical(?:[ ._-](?:cut|edition))?`), "Theatrical"},
	{tok(`unrated`), "Unrated"},
	{tok(`uncut`), "Uncut"},
	{tok(`final[ ._-]cut`), "Final Cut"},
	{tok(`ultimate[ ._-](?:cut|edition)`), "Ultimate"},
	{tok(`special[ ._-]edition`), "Special Edition"},
	{tok(`collector'?s?[ ._-]edition`), "Collector's Edition"},
	{tok(`criterion(?:[ ._-]collection)?`), "Criterion"},
	{tok(`\d{2,3}th[ ._-]anniversary(?:[ ._-]edition)?|anniversary[ ._-]edition`), "Anniversary"},
	{tok(`imax(?:[ ._-]enhanced)?`), "IMAX"},
	{tok(`open[ ._-]matte`), "Open Matte"},
	{tok(`remastered|4k[ ._-]remaster`), "Remastered"},
}

var languageRules = []rule[string]{
	{tok(`multi(?:[ ._-]?subs?)?|dual(?:[ ._-]audio)?`), "multi"},
	{tok(`english|eng`), "en"},
	{tok(`truefrench|french|vff|vfq|vf2|vostfr`), "fr"},
	{tok(`german|deutsch|ger`), "de"},
	{tok(`spanish|castellano|latino|esp`), "es"},
	{tok(`italian|ita`), "it"},
	{tok(`russian|rus`), "ru"},
	{tok(`ukrainian|ukr`), "uk"},
	{tok(`japanese|jpn`), "ja"},
	{tok(`korean|kor`), "ko"},
	{tok(`chinese|mandarin|chs|cht`), "zh"},
	{tok(`hindi`), "hi"},
	{tok(`polish`), "pl"},
	{tok(`dutch`), "nl"},
	{tok(`portuguese|por`), "pt"},
	{tok(`swedish`), "sv"},
	{tok(`danish`), "da"},
	{tok(`norwegian`), "no"},
	{tok(`finnish`), "fi"},
	{tok(`turkish`), "tr"},
}

// Strong markers end the title: once one appears, the title is over.
var markerRules = []rule[bool]{
	{tok(`(?:2160|1080|720|576|480)[pi]|4k|uhd`), true},
	{tok(`x\.?26[45]|h\.?26[45]|hevc|avc|av1|xvid|divx`), true},
	{tok(`blu-?ray|web-?dl|web-?rip|webdl|hdtv|remux|dvd-?rip|bdrip|brrip|hdcam|telesync|hdts|dvdscr`), true},
	{tok(`hdr10\+?|dolby[ ._-]?vision|dovi`), true},
	{tok(`bdmv|br-?disk|complete[ ._-](?:uhd[ ._-])?blu-?ray`), true},
}

// Soft markers end the title only when no year was found.
var softMarkerRules = []rule[bool]{
	{tok(`proper|repack|rerip|web|internal|limited`), true},
	{tok(`3d|h-?sbs|half-?sbs|sbs`), true},
	{tok(`extended|unrated|uncut|remastered|imax|director'?s?[ ._-]?cut|theatrical`), true},
	{tok(`multi|truefrench|french|german|spanish|italian|russian|ukrainian|dual`), true},
	{tok(`aac|ac3|eac3|ddp|dts|truehd|flac|atmos`), true},
}
