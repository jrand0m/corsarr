package migrate

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
)

// Pure mappings of source custom formats and quality profiles onto
// corsarr's documents. Every value that cannot be carried over exactly
// leaves a note; nothing is dropped silently.

// norm lower-cases and drops everything but letters and digits.
func norm(s string) string { return decide.KeyOf(s) }

var sonarrSources = map[string][]string{"television": {"hdtv"}, "televisionraw": {"hdtv"}, "web": {"webdl"}, "webrip": {"webrip"},
	"dvd": {"dvd"}, "bluray": {"bluray"}, "blurayraw": {"remux", "disc"}}

var radarrSources = map[string][]string{"cam": {"cam"}, "telesync": {"telesync"}, "telecine": {"telecine"}, "dvd": {"dvd"},
	"tv": {"hdtv"}, "webdl": {"webdl"}, "webrip": {"webrip"}, "bluray": {"bluray", "remux", "disc"}}

// Documented numeric values, used when a select field arrives without its options.
var (
	sonarrSourceIDs   = map[string]string{"1": "television", "2": "televisionraw", "3": "web", "4": "webrip", "5": "dvd", "6": "bluray", "7": "blurayraw"}
	radarrSourceIDs   = map[string]string{"1": "cam", "2": "telesync", "3": "telecine", "4": "workprint", "5": "dvd", "6": "tv", "7": "webdl", "8": "webrip", "9": "bluray"}
	modifierIDs       = map[string]string{"1": "regional", "2": "screener", "3": "rawhd", "4": "brdisk", "5": "remux"}
	releaseTypeIDs    = map[string]string{"1": "singleepisode", "2": "multiepisode", "3": "seasonpack"}
	sonarrFlagBits    = map[string]string{"1": "freeleech", "2": "halfleech", "4": "doubleupload", "8": "internal", "16": "scene", "32": "freeleech75", "64": "freeleech25", "128": "nuked"}
	radarrFlagBits    = map[string]string{"1": "freeleech", "2": "halfleech", "4": "doubleupload", "8": "golden", "16": "approved", "32": "internal", "64": "internal", "128": "scene", "256": "freeleech75", "512": "freeleech25", "1024": "userrelease", "2048": "nuked"}
	languageIDs       = map[string]string{"-2": "original", "-1": "any", "1": "english", "2": "french", "3": "spanish", "4": "german", "5": "italian", "6": "danish", "7": "dutch", "8": "japanese", "9": "icelandic", "10": "chinese", "11": "russian", "12": "polish", "13": "vietnamese", "14": "swedish", "15": "norwegian", "16": "finnish", "17": "turkish", "18": "portuguese", "19": "flemish", "20": "greek", "21": "korean", "22": "hungarian", "23": "hebrew", "24": "lithuanian", "25": "czech", "26": "hindi", "27": "romanian", "28": "thai", "29": "bulgarian", "30": "portuguesebrazil", "31": "arabic", "32": "ukrainian", "33": "persian", "34": "bengali"}
	languageISO       = map[string]string{"english": "en", "french": "fr", "spanish": "es", "german": "de", "italian": "it", "danish": "da", "dutch": "nl", "japanese": "ja", "icelandic": "is", "chinese": "zh", "russian": "ru", "polish": "pl", "vietnamese": "vi", "swedish": "sv", "norwegian": "no", "finnish": "fi", "turkish": "tr", "portuguese": "pt", "greek": "el", "korean": "ko", "hungarian": "hu", "hebrew": "he", "lithuanian": "lt", "czech": "cs", "hindi": "hi", "romanian": "ro", "thai": "th", "bulgarian": "bg", "arabic": "ar", "ukrainian": "uk", "persian": "fa", "bengali": "bn", "slovak": "sk", "latvian": "lv", "catalan": "ca", "croatian": "hr", "serbian": "sr", "bosnian": "bs", "estonian": "et", "tamil": "ta", "indonesian": "id", "telugu": "te", "macedonian": "mk", "slovenian": "sl", "malayalam": "ml", "kannada": "kn", "albanian": "sq", "afrikaans": "af", "marathi": "mr", "tagalog": "tl", "urdu": "ur", "mongolian": "mn", "georgian": "ka"}
	languageNearISO   = map[string]string{"flemish": "nl", "portuguesebrazil": "pt", "spanishlatino": "es"}
	unreportedFlags   = map[string]bool{"internal": true, "scene": true, "nuked": true}
	resolutionNearest = map[string]string{"360": "480", "540": "576"}
)

// choiceOf reads a select field by option name when the source sent its
// options, else through the documented ids.
func choiceOf(sp SrcSpec, field string, ids map[string]string) string {
	if n := sp.Options[field]; n != "" {
		return norm(n)
	}
	return ids[sp.Values[field]]
}

// mappedCondition is one source condition on corsarr's side; ok=false when
// nothing of it could be kept.
type mappedCondition struct {
	doc   decide.ConditionDoc
	notes []string
	ok    bool
}

// mapCondition maps one source condition (Sonarr or Radarr).
func mapCondition(app App, sp SrcSpec) mappedCondition {
	base := decide.ConditionDoc{Negate: sp.Negate, Required: sp.Required}
	pattern := func(k decide.ConditionKind) mappedCondition {
		base.Kind, base.Pattern = k, sp.Values["value"]
		return mappedCondition{doc: base, ok: base.Pattern != ""}
	}
	switch strings.TrimSuffix(sp.Impl, "Specification") {
	case "ReleaseTitle":
		return pattern(decide.CondTitle)
	case "ReleaseGroup":
		return pattern(decide.CondGroup)
	case "Edition":
		return pattern(decide.CondEdition)
	case "Source":
		return mapSource(app, sp, base)
	case "Resolution":
		return mapResolution(sp, base)
	case "QualityModifier":
		return mapModifier(sp, base)
	case "Language":
		return mapLanguage(sp, base)
	case "IndexerFlag":
		return mapFlag(app, sp, base)
	case "Size":
		return mapRange(decide.CondSize, sp, base)
	case "Year":
		return mapRange(decide.CondYear, sp, base)
	case "ReleaseType":
		return mapReleaseType(sp, base)
	}
	return mappedCondition{notes: []string{fmt.Sprintf("condition %q (%s) has no corsarr equivalent and was left out", sp.Name, sp.Impl)}}
}

func values(base decide.ConditionDoc, k decide.ConditionKind, vs ...string) mappedCondition {
	base.Kind, base.Values = k, vs
	return mappedCondition{doc: base, ok: len(vs) > 0}
}

func dropped(sp SrcSpec, why string) mappedCondition {
	return mappedCondition{notes: []string{fmt.Sprintf("condition %q left out: %s", sp.Name, why)}}
}

func mapSource(app App, sp SrcSpec, base decide.ConditionDoc) mappedCondition {
	table, ids := sonarrSources, sonarrSourceIDs
	if app == Radarr {
		table, ids = radarrSources, radarrSourceIDs
	}
	src := choiceOf(sp, "value", ids)
	vs, ok := table[src]
	if !ok {
		return dropped(sp, fmt.Sprintf("source %q is not recognised by corsarr", src))
	}
	m := values(base, decide.CondSource, vs...)
	if src == "televisionraw" {
		m.notes = append(m.notes, fmt.Sprintf("condition %q: raw HDTV counts as TV in corsarr", sp.Name))
	}
	return m
}

func mapResolution(sp SrcSpec, base decide.ConditionDoc) mappedCondition {
	r := strings.TrimSuffix(strings.ToLower(sp.Options["value"]), "p")
	if r == "" || r == "r"+sp.Values["value"] {
		r = sp.Values["value"]
	}
	r = strings.TrimPrefix(r, "r")
	if near, ok := resolutionNearest[r]; ok {
		m := values(base, decide.CondResolution, near)
		m.notes = []string{fmt.Sprintf("condition %q: %sp counts as %sp in corsarr", sp.Name, r, near)}
		return m
	}
	if _, ok := map[string]bool{"480": true, "576": true, "720": true, "1080": true, "2160": true}[r]; !ok {
		return dropped(sp, fmt.Sprintf("resolution %q is not on corsarr's ladder", r))
	}
	return values(base, decide.CondResolution, r)
}

func mapModifier(sp SrcSpec, base decide.ConditionDoc) mappedCondition {
	switch m := choiceOf(sp, "value", modifierIDs); m {
	case "regional", "screener", "rawhd", "remux":
		return values(base, decide.CondModifier, m)
	case "brdisk":
		return values(base, decide.CondModifier, "disc")
	default:
		return dropped(sp, fmt.Sprintf("quality modifier %q is not recognised by corsarr", m))
	}
}

func mapLanguage(sp SrcSpec, base decide.ConditionDoc) mappedCondition {
	lang := choiceOf(sp, "value", languageIDs)
	base.Except = sp.Values["exceptLanguage"] == "true"
	switch lang {
	case "original":
		return dropped(sp, "the item's original language is not known to custom formats")
	case "any":
		// any language: every release has one (English when the title names none)
		base.Except = !base.Except
		return values(base, decide.CondLanguage, "und")
	}
	if iso, ok := languageISO[lang]; ok {
		return values(base, decide.CondLanguage, iso)
	}
	if iso, ok := languageNearISO[lang]; ok {
		m := values(base, decide.CondLanguage, iso)
		m.notes = []string{fmt.Sprintf("condition %q: %s counts as %s", sp.Name, lang, iso)}
		return m
	}
	return dropped(sp, fmt.Sprintf("language %q is not recognised by corsarr", lang))
}

func mapFlag(app App, sp SrcSpec, base decide.ConditionDoc) mappedCondition {
	bits := sonarrFlagBits
	if app == Radarr {
		bits = radarrFlagBits
	}
	flag := bits[sp.Values["value"]]
	if n := sp.Options["value"]; n != "" {
		flag = norm(n[strings.LastIndex(n, "_")+1:])
	}
	switch {
	case flag == "freeleech" || flag == "halfleech" || flag == "doubleupload" || flag == "freeleech75" || flag == "freeleech25":
		return values(base, decide.CondFlag, flag)
	case unreportedFlags[flag]:
		m := values(base, decide.CondFlag, flag)
		m.notes = []string{fmt.Sprintf("condition %q: corsarr's indexers do not report the %s flag yet, so it never matches", sp.Name, flag)}
		return m
	}
	return dropped(sp, fmt.Sprintf("indexer flag %q is specific to one tracker", flag))
}

func mapRange(k decide.ConditionKind, sp SrcSpec, base decide.ConditionDoc) mappedCondition {
	lo, err1 := strconv.ParseFloat(sp.Values["min"], 64)
	hi, err2 := strconv.ParseFloat(sp.Values["max"], 64)
	if err1 != nil && err2 != nil {
		return dropped(sp, "it has no bounds")
	}
	base.Kind, base.Min, base.Max = k, math.Max(lo, 0), math.Max(hi, 0)
	return mappedCondition{doc: base, ok: true}
}

func mapReleaseType(sp SrcSpec, base decide.ConditionDoc) mappedCondition {
	switch choiceOf(sp, "value", releaseTypeIDs) {
	case "singleepisode":
		return values(base, decide.CondReleaseType, "episode")
	case "multiepisode":
		return values(base, decide.CondReleaseType, "multiepisode")
	case "seasonpack":
		return values(base, decide.CondReleaseType, "seasonpack")
	}
	return dropped(sp, "unknown release type")
}

// mapFormat maps a source format; ok=false when no condition survived. The
// document has no key yet.
func mapFormat(app App, f SrcFormat) (decide.FormatDoc, []string, bool) {
	doc := decide.FormatDoc{Name: strings.TrimSpace(f.Name)}
	var notes []string
	for _, sp := range f.Specs {
		m := mapCondition(app, sp)
		notes = append(notes, m.notes...)
		if m.ok {
			doc.Conditions = append(doc.Conditions, m.doc)
		}
	}
	return doc, notes, len(doc.Conditions) > 0
}

// qualityAliases are source quality names whose rung has another name here.
var qualityAliases = map[string]decide.QualityID{"dvdscr": "screener", "dvd-r": "dvd", "br-disk": "disc-1080p"}

// profileQuality maps a source quality name; ok=false when it has no rung.
func profileQuality(name string) (decide.QualityID, bool) {
	if id, ok := qualityAliases[strings.ToLower(strings.TrimSpace(name))]; ok {
		return id, true
	}
	return qualityID(name)
}

// mapCustomProfile maps a Sonarr or Radarr profile onto a document (no key);
// formatKeys maps source format ids onto corsarr format keys.
func mapCustomProfile(p SrcProfile, kind domain.MediaKind, formatKeys map[int]string, sizes map[string]SrcSize) (decide.ProfileDoc, []string) {
	doc := decide.ProfileDoc{Name: strings.TrimSpace(p.Name), Kinds: []domain.MediaKind{kind}, Upgrades: p.Upgrades,
		MinFormatScore: p.MinScore, CutoffFormatScore: p.CutoffScore, MinUpgradeFormatScore: max(p.StepScore, 1)}
	var notes []string
	cutoff := map[int]decide.QualityID{}
	for _, it := range p.Items {
		g, n := mapItem(it)
		notes = append(notes, n...)
		if len(g.Qualities) > 0 {
			cutoff[it.ID] = g.Qualities[0]
			if it.Allowed {
				doc.Groups = append(doc.Groups, g)
			}
		}
	}
	doc.Cutoff, notes = profileCutoff(doc, cutoff, p, notes)
	doc.Formats, notes = profileScores(p.Scores, formatKeys, notes)
	doc.Sizes, notes = profileSizes(doc.Groups, sizes, notes)
	return doc, notes
}

func mapItem(it SrcQualityItem) (decide.GroupDoc, []string) {
	g := decide.GroupDoc{}
	if it.Group {
		g.Name = it.Name
	}
	var notes []string
	for _, name := range it.Members {
		id, ok := profileQuality(name)
		switch {
		case !ok && it.Allowed:
			notes = append(notes, fmt.Sprintf("quality %s has no corsarr equivalent and was left out", name))
		case ok && decide.BelowFloor(id) && it.Allowed:
			notes = append(notes, fmt.Sprintf("quality %s is below corsarr's quality floor and was left out", name))
		case ok && !decide.BelowFloor(id):
			g.Qualities = append(g.Qualities, id)
		}
	}
	return g, notes
}

func profileCutoff(doc decide.ProfileDoc, byItem map[int]decide.QualityID, p SrcProfile, notes []string) (decide.QualityID, []string) {
	allowed := map[decide.QualityID]bool{}
	var best decide.QualityID
	for _, g := range doc.Groups {
		for _, q := range g.Qualities {
			allowed[q] = true
			best = q
		}
	}
	if q, ok := byItem[p.CutoffID]; ok && allowed[q] {
		return q, notes
	}
	if best != "" {
		notes = append(notes, fmt.Sprintf("cutoff %q has no corsarr equivalent; the best allowed quality is the cutoff", p.Cutoff))
	}
	return best, notes
}

func profileScores(scores []SrcScore, formatKeys map[int]string, notes []string) ([]decide.ScoreDoc, []string) {
	var out []decide.ScoreDoc
	for _, s := range scores {
		if s.Score == 0 {
			continue // a zero score changes nothing
		}
		key, ok := formatKeys[s.Format]
		if !ok {
			notes = append(notes, fmt.Sprintf("score %+d for custom format %q was left out: the format could not be carried over", s.Score, s.Name))
			continue
		}
		out = append(out, decide.ScoreDoc{Format: key, Score: s.Score})
	}
	return out, notes
}

// profileSizes copies the source quality definitions of the allowed
// qualities that differ from corsarr's defaults.
func profileSizes(groups []decide.GroupDoc, sizes map[string]SrcSize, notes []string) ([]decide.SizeDoc, []string) {
	byID := map[decide.QualityID]SrcSize{}
	for name, s := range sizes {
		if id, ok := profileQuality(name); ok {
			byID[id] = s
		}
	}
	var out []decide.SizeDoc
	for _, g := range groups {
		for _, q := range g.Qualities {
			src, ok := byID[q]
			if !ok {
				continue
			}
			doc, n := sizeDoc(q, src)
			notes = append(notes, n...)
			if doc != nil {
				out = append(out, *doc)
			}
		}
	}
	return out, notes
}

func sizeDoc(q decide.QualityID, s SrcSize) (*decide.SizeDoc, []string) {
	def := decide.DefaultDefinitions().For(q)
	lo, pref, hi := def.Min(), def.Preferred(), def.Max()
	var notes []string
	if s.Min != nil {
		lo = *s.Min
	}
	if s.Preferred != nil {
		pref = *s.Preferred
	}
	switch {
	case s.Max == nil || *s.Max <= 0:
		notes = append(notes, fmt.Sprintf("%s had no size limit; corsarr keeps a limit of %.0f MB per minute", q, hi))
	case *s.Max > decide.MaxMBPerMinute:
		hi = decide.MaxMBPerMinute
		notes = append(notes, fmt.Sprintf("%s limit %.0f MB per minute is above corsarr's largest allowed %.0f", q, *s.Max, hi))
	default:
		hi = *s.Max
	}
	if hi <= lo {
		hi = math.Min(decide.MaxMBPerMinute, math.Max(lo*2, lo+1))
	}
	pref = math.Min(math.Max(pref, lo), hi)
	if lo == def.Min() && pref == def.Preferred() && hi == def.Max() {
		return nil, notes
	}
	return &decide.SizeDoc{Quality: q, Min: lo, Preferred: pref, Max: hi}, notes
}
