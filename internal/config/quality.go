package config

import (
	"strings"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
)

// Config-as-code for custom formats and quality profiles: two live
// sections keyed by object key whose entries are the same documents the
// web UI and the API edit. The whole set is compiled at load, so a profile
// naming a missing format or a regex that does not compile fails the load
// with its location.

func conditionKindNames() []string {
	var out []string
	for _, k := range decide.ConditionKinds() {
		out = append(out, string(k.Kind))
	}
	return out
}

func qualityIDs() []string {
	var out []string
	for _, q := range append(decide.Ladder(), decide.AudioLadder()...) {
		out = append(out, string(q.ID))
	}
	return out
}

func numberField(name, doc string) Field {
	return Field{Name: name, Node: &Node{Kind: KindNumber, Check: nonNegative, Doc: doc}}
}

func boolField(name, def, doc string) Field {
	return Field{Name: name, Node: &Node{Kind: KindBool, Default: def, Doc: doc}}
}

func intField(name, def, doc string) Field {
	return Field{Name: name, Node: &Node{Kind: KindInt, Default: def, Doc: doc}}
}

func customFormatsNode() *Node {
	cond := obj("",
		Field{Name: "kind", Node: &Node{Kind: KindEnum, Required: true, Enum: conditionKindNames(),
			Doc: "What the condition looks at: title, group or edition (a pattern); source, resolution, modifier, codec, audio, channels, hdr, language, flag, releasetype, container, bitdepth or dvfallback (values); size in GB or year (min and max)."}},
		Field{Name: "pattern", Node: &Node{Kind: KindString,
			Doc: "Regular expression for title, group and edition conditions, matched without regard to case, for example \\bDTS\\b."}},
		Field{Name: "values", Node: &Node{Kind: KindList, Doc: "Values for list conditions; the condition holds when the release has any of them, for example [aac, ac3, eac3].",
			Elem: &Node{Kind: KindString, Doc: "One value, for example 2160 for a resolution or eac3 for an audio codec."}}},
		numberField("min", "Lower bound of a size (GB, exclusive) or year (inclusive) condition."),
		numberField("max", "Upper bound of a size (GB) or year condition; 0 means no upper bound."),
		boolField("negate", "false", "Turn the condition around: it holds when the test fails."),
		boolField("required", "false", "The format matches only when this condition holds; otherwise one condition of the same kind is enough."),
		boolField("except", "false", "Language conditions only: hold when the release has a language other than the listed ones."),
	)
	return &Node{Kind: KindMap, Doc: "Custom formats, one entry per key you choose (lower-case letters and digits); profiles score them.",
		Elem: obj("",
			Field{Name: "name", Node: &Node{Kind: KindString, Required: true, Doc: "Name shown in the web UI and in decision reasons."}},
			Field{Name: "conditions", Node: &Node{Kind: KindList, Required: true, Elem: cond,
				Doc: "The tests a release must pass; conditions of one kind need one hit, conditions marked required must all hold."}},
		)}
}

func qualityProfilesNode() *Node {
	return &Node{Kind: KindMap, Doc: "Custom quality profiles, one entry per key you choose (not a built-in key such as hd1080p); series, movies and artists name them by key.",
		Elem: obj("",
			Field{Name: "name", Node: &Node{Kind: KindString, Required: true, Doc: "Name shown in the web UI."}},
			Field{Name: "kinds", Node: &Node{Kind: KindList, Required: true, Doc: "What the profile is for: tv, movie, both, or music on its own.",
				Elem: &Node{Kind: KindEnum, Enum: []string{string(domain.KindTV), string(domain.KindMovie), string(domain.KindMusic)}, Doc: "A media kind the profile serves."}}},
			Field{Name: "groups", Node: &Node{Kind: KindList, Required: true, Doc: "Allowed qualities from worst to best; qualities in one group count the same.",
				Elem: obj("",
					Field{Name: "name", Node: &Node{Kind: KindString, Doc: "Name of a group of several qualities, for example WEB 1080p."}},
					Field{Name: "qualities", Node: &Node{Kind: KindList, Required: true, Doc: "Quality ids in this group, for example [webdl-1080p, webrip-1080p].",
						Elem: &Node{Kind: KindEnum, Enum: qualityIDs(), Doc: "A quality id from the ladder."}}},
				)}},
			Field{Name: "cutoff", Node: &Node{Kind: KindEnum, Required: true, Enum: qualityIDs(), Doc: "Upgrades stop once a file has this quality (or one ranked the same or higher)."}},
			boolField("upgrades", "true", "Replace files with better releases until the cutoff and the cutoff format score are met."),
			Field{Name: "formats", Node: &Node{Kind: KindList, Doc: "Scores for custom formats; a release scores the sum of the formats it matches.",
				Elem: obj("",
					Field{Name: "format", Node: &Node{Kind: KindString, Required: true, Doc: "Key of a custom format."}},
					intField("score", "0", "Points a match adds (negative to penalise)."),
					boolField("reject", "false", "Reject every release matching this format, whatever the scores."),
				)}},
			intField("minFormatScore", "0", "Releases scoring below this are rejected."),
			intField("cutoffFormatScore", "0", "Upgrades continue until a file scores at least this (and has the cutoff quality)."),
			intField("minUpgradeFormatScore", "1", "Smallest score gain that counts as an upgrade at the same quality."),
			Field{Name: "sizes", Node: &Node{Kind: KindList, Doc: "Size limits of this profile for some qualities, in MB per minute of runtime; other qualities use the global limits.",
				Elem: obj("",
					Field{Name: "quality", Node: &Node{Kind: KindEnum, Required: true, Enum: qualityIDs(), Doc: "An allowed quality id."}},
					numberField("min", "Smallest size in MB per minute."),
					numberField("preferred", "Size aimed for in MB per minute; 0 uses the global preferred size."),
					numberField("max", "Largest size in MB per minute; must be above min."),
				)}},
			boolField("allowDiscImage", "false", "Allow full disc images (ISO, BDMV)."),
			boolField("allow3D", "false", "Allow 3D releases."),
		)}
}

// formatDocs reads the customFormats section into documents, by key.
func formatDocs(t tree) []decide.FormatDoc {
	sec, _ := t["customFormats"].(tree)
	out := make([]decide.FormatDoc, 0, len(sec))
	for _, key := range sortedTreeKeys(sec) {
		f, _ := sec[key].(tree)
		d := decide.FormatDoc{Key: key, Name: strOf(f["name"])}
		for _, c := range listOf(f["conditions"]) {
			ct, _ := c.(tree)
			d.Conditions = append(d.Conditions, decide.ConditionDoc{Kind: decide.ConditionKind(strOf(ct["kind"])), Pattern: strOf(ct["pattern"]),
				Values: stringList(ct["values"]), Min: numOf(ct["min"]), Max: numOf(ct["max"]), Negate: boolOf(ct["negate"]),
				Required: boolOf(ct["required"]), Except: boolOf(ct["except"])})
		}
		out = append(out, d)
	}
	return out
}

// profileDocs reads the qualityProfiles section into documents, by key.
func profileDocs(t tree) []decide.ProfileDoc {
	sec, _ := t["qualityProfiles"].(tree)
	out := make([]decide.ProfileDoc, 0, len(sec))
	for _, key := range sortedTreeKeys(sec) {
		p, _ := sec[key].(tree)
		d := decide.ProfileDoc{Key: key, Name: strOf(p["name"]), Cutoff: decide.QualityID(strOf(p["cutoff"])), Upgrades: boolOf(p["upgrades"]),
			MinFormatScore: intOf(p["minFormatScore"]), CutoffFormatScore: intOf(p["cutoffFormatScore"]),
			MinUpgradeFormatScore: intOf(p["minUpgradeFormatScore"]), AllowDiscImage: boolOf(p["allowDiscImage"]), Allow3D: boolOf(p["allow3D"])}
		for _, k := range stringList(p["kinds"]) {
			d.Kinds = append(d.Kinds, domain.MediaKind(k))
		}
		for _, g := range listOf(p["groups"]) {
			gt, _ := g.(tree)
			gd := decide.GroupDoc{Name: strOf(gt["name"])}
			for _, q := range stringList(gt["qualities"]) {
				gd.Qualities = append(gd.Qualities, decide.QualityID(q))
			}
			d.Groups = append(d.Groups, gd)
		}
		for _, f := range listOf(p["formats"]) {
			ft, _ := f.(tree)
			d.Formats = append(d.Formats, decide.ScoreDoc{Format: strOf(ft["format"]), Score: intOf(ft["score"]), Reject: boolOf(ft["reject"])})
		}
		for _, s := range listOf(p["sizes"]) {
			st, _ := s.(tree)
			d.Sizes = append(d.Sizes, decide.SizeDoc{Quality: decide.QualityID(strOf(st["quality"])), Min: numOf(st["min"]),
				Preferred: numOf(st["preferred"]), Max: numOf(st["max"])})
		}
		out = append(out, d)
	}
	return out
}

// checkQuality compiles the config's formats and profiles and reports every
// problem at its key path and source location.
func (c *Config) checkQuality() Errors {
	_, problems := decide.BuildCatalog(c.CustomFormats, c.QualityProfiles)
	var out Errors
	for _, p := range problems {
		section := map[string]string{"format": "customFormats", "profile": "qualityProfiles"}[p.Object]
		path := section + "." + p.Key
		if p.Path != "" {
			path += "." + p.Path
		}
		out = append(out, FieldError{Path: path, Loc: c.nearestLoc(path), Msg: p.Msg})
	}
	return out
}

// nearestLoc is where a path, or its closest set parent, came from.
func (c *Config) nearestLoc(path string) Location {
	for p := path; p != ""; p = parentPath(p) {
		if l := c.locs[p]; l.Kind != "" {
			return l
		}
	}
	return Location{}
}

// parentPath drops the last segment of a dotted path ("" at the top).
func parentPath(p string) string {
	i := strings.LastIndex(p, ".")
	if i < 0 {
		return ""
	}
	return p[:i]
}

func listOf(v any) []any { l, _ := v.([]any); return l }

func strOf(v any) string { s, _ := v.(string); return s }

func boolOf(v any) bool { b, _ := v.(bool); return b }

func numOf(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	}
	return 0
}
