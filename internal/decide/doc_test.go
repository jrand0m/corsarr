package decide

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
)

func fieldsOf(es []DocError) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Path)
	}
	return out
}

func TestCompileFormat_Refuses(t *testing.T) {
	cases := map[string]struct {
		doc  FormatDoc
		path string
	}{
		"bad key":        {FormatDoc{Key: "Bad Key", Name: "n", Conditions: []ConditionDoc{{Kind: CondTitle, Pattern: "x"}}}, "key"},
		"no name":        {FormatDoc{Key: "k", Conditions: []ConditionDoc{{Kind: CondTitle, Pattern: "x"}}}, "name"},
		"no conditions":  {FormatDoc{Key: "k", Name: "n"}, "conditions"},
		"unknown kind":   {FormatDoc{Key: "k", Name: "n", Conditions: []ConditionDoc{{Kind: "colour"}}}, "conditions.0.kind"},
		"bad regex":      {FormatDoc{Key: "k", Name: "n", Conditions: []ConditionDoc{{Kind: CondTitle, Pattern: "ok"}, {Kind: CondTitle, Pattern: "(x264"}}}, "conditions.1.pattern"},
		"empty regex":    {FormatDoc{Key: "k", Name: "n", Conditions: []ConditionDoc{{Kind: CondGroup}}}, "conditions.0.pattern"},
		"no values":      {FormatDoc{Key: "k", Name: "n", Conditions: []ConditionDoc{{Kind: CondSource}}}, "conditions.0.values"},
		"bad value":      {FormatDoc{Key: "k", Name: "n", Conditions: []ConditionDoc{{Kind: CondResolution, Values: []string{"1080", "999"}}}}, "conditions.0.values.1"},
		"except misused": {FormatDoc{Key: "k", Name: "n", Conditions: []ConditionDoc{{Kind: CondAudio, Values: []string{"aac"}, Except: true}}}, "conditions.0.except"},
		"size range":     {FormatDoc{Key: "k", Name: "n", Conditions: []ConditionDoc{{Kind: CondSize, Min: 5, Max: 2}}}, "conditions.0.max"},
		"negative":       {FormatDoc{Key: "k", Name: "n", Conditions: []ConditionDoc{{Kind: CondYear, Min: -1}}}, "conditions.0.min"},
	}
	for name, c := range cases {
		_, es := CompileFormat(c.doc)
		if len(es) == 0 || es[0].Path != c.path {
			t.Errorf("%s: errors %v", name, es)
		}
	}
	_, es := CompileFormat(FormatDoc{Key: "k", Name: "n", Conditions: []ConditionDoc{{Kind: CondTitle, Pattern: "ab[x"}}})
	if !strings.Contains(es[0].Msg, "position 3") {
		t.Fatalf("position: %v", es)
	}
}

func TestFormatDoc_RoundTrip(t *testing.T) {
	doc := FormatDoc{Key: "ps5", Name: "PS5 audio", Conditions: []ConditionDoc{
		{Kind: CondAudio, Values: []string{"aac", "eac3"}, Required: true},
		{Kind: CondTitle, Pattern: `\bdts\b`, Negate: true},
		{Kind: CondLanguage, Values: []string{"en"}, Except: true},
		{Kind: CondSize, Min: 1, Max: 8}}}
	f, es := CompileFormat(doc)
	if len(es) > 0 {
		t.Fatal(es)
	}
	if got := f.Doc(); !reflect.DeepEqual(got, doc) {
		t.Fatalf("round trip\n%+v\n%+v", got, doc)
	}
}

func validProfile() ProfileDoc {
	return ProfileDoc{Key: "ps5high", Name: "High - PS5", Kinds: []domain.MediaKind{domain.KindTV, domain.KindMovie},
		Groups: []GroupDoc{{Qualities: []QualityID{"webdl-720p"}}, {Name: "WEB 1080p", Qualities: []QualityID{"webrip-1080p", "webdl-1080p"}}},
		Cutoff: "webrip-1080p", Upgrades: true, Formats: []ScoreDoc{{Format: "ps5", Score: 100}, {Format: "bad", Score: 0, Reject: true}},
		MinFormatScore: 0, CutoffFormatScore: 100, MinUpgradeFormatScore: 10,
		Sizes: []SizeDoc{{Quality: "webdl-720p", Min: 2, Preferred: 10, Max: 20}}}
}

func testFormats(t *testing.T) map[string]CustomFormat {
	ps5, _ := CompileFormat(FormatDoc{Key: "ps5", Name: "PS5", Conditions: []ConditionDoc{{Kind: CondAudio, Values: []string{"aac", "eac3"}}}})
	bad, _ := CompileFormat(FormatDoc{Key: "bad", Name: "Bad", Conditions: []ConditionDoc{{Kind: CondGroup, Pattern: "^bad$"}}})
	return map[string]CustomFormat{"ps5": ps5, "bad": bad}
}

func TestCompileProfile_AndRoundTrip(t *testing.T) {
	doc := validProfile()
	p, es := CompileProfile(doc, testFormats(t))
	if len(es) > 0 {
		t.Fatal(es)
	}
	if !p.Serves(domain.KindTV) || !p.Serves(domain.KindMovie) || p.Serves(domain.KindMusic) || p.Key() != "ps5high" {
		t.Fatal("kinds/key")
	}
	if d := p.Definition(nil, "webdl-720p"); d.Max() != 20 || d.Min() != 2 {
		t.Fatalf("size overlay %+v", d)
	}
	if d := p.Definition(nil, "webdl-1080p"); d.Max() != 70 {
		t.Fatalf("global definition %+v", d)
	}
	if got := p.Doc(); !reflect.DeepEqual(got, doc) {
		t.Fatalf("round trip\n%+v\n%+v", got, doc)
	}
}

func TestCompileProfile_Refuses(t *testing.T) {
	mut := func(f func(*ProfileDoc)) ProfileDoc { d := validProfile(); f(&d); return d }
	cases := map[string]struct {
		doc  ProfileDoc
		path string
	}{
		"builtin key":      {mut(func(d *ProfileDoc) { d.Key = "hd1080p" }), "key"},
		"no kinds":         {mut(func(d *ProfileDoc) { d.Kinds = nil }), "kinds"},
		"music with tv":    {mut(func(d *ProfileDoc) { d.Kinds = []domain.MediaKind{domain.KindMusic, domain.KindTV} }), "kinds"},
		"book":             {mut(func(d *ProfileDoc) { d.Kinds = []domain.MediaKind{domain.KindBook} }), "kinds.0"},
		"no groups":        {mut(func(d *ProfileDoc) { d.Groups = nil }), "groups"},
		"unknown quality":  {mut(func(d *ProfileDoc) { d.Groups[0].Qualities = []QualityID{"webdl-9000p"} }), "groups.0.qualities.0"},
		"audio on video":   {mut(func(d *ProfileDoc) { d.Groups[0].Qualities = []QualityID{"flac"} }), "groups.0.qualities.0"},
		"below floor":      {mut(func(d *ProfileDoc) { d.Groups[0].Qualities = []QualityID{"unknown"} }), "groups.0.qualities.0"},
		"twice":            {mut(func(d *ProfileDoc) { d.Groups[0].Qualities = []QualityID{"webdl-1080p"} }), "groups.1.qualities.1"},
		"cutoff":           {mut(func(d *ProfileDoc) { d.Cutoff = "bluray-1080p" }), "cutoff"},
		"missing format":   {mut(func(d *ProfileDoc) { d.Formats[0].Format = "nope" }), "formats.0.format"},
		"format twice":     {mut(func(d *ProfileDoc) { d.Formats[1].Format = "ps5" }), "formats.1.format"},
		"size not allowed": {mut(func(d *ProfileDoc) { d.Sizes[0].Quality = "bluray-1080p" }), "sizes.0.quality"},
		"size range":       {mut(func(d *ProfileDoc) { d.Sizes[0].Max = 1 }), "sizes.0.max"},
		"negative step":    {mut(func(d *ProfileDoc) { d.MinUpgradeFormatScore = -1 }), "minUpgradeFormatScore"},
		"music below floor": {mut(func(d *ProfileDoc) {
			d.Kinds = []domain.MediaKind{domain.KindMusic}
			d.Groups = []GroupDoc{{Qualities: []QualityID{"mp3-96"}}}
			d.Cutoff = "mp3-96"
			d.Sizes = nil
		}), "groups.0.qualities.0"},
	}
	for name, c := range cases {
		_, es := CompileProfile(c.doc, testFormats(t))
		if len(es) == 0 || !contains(fieldsOf(es), c.path) {
			t.Errorf("%s: errors %v", name, es)
		}
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func TestSizeDoc_PreferredDefaultsInsideRange(t *testing.T) {
	d := validProfile()
	d.Sizes = []SizeDoc{{Quality: "webdl-720p", Min: 1, Max: 5}} // shipped preferred 17 is above max
	p, es := CompileProfile(d, testFormats(t))
	if len(es) > 0 {
		t.Fatal(es)
	}
	if got := p.Definition(nil, "webdl-720p").Preferred(); got != 5 {
		t.Fatalf("preferred %v", got)
	}
}

func TestBuildCatalog(t *testing.T) {
	formats := []FormatDoc{
		{Key: "ps5", Name: "PS5", Conditions: []ConditionDoc{{Kind: CondAudio, Values: []string{"aac"}}}},
		{Key: "bad", Name: "Bad", Conditions: []ConditionDoc{{Kind: CondGroup, Pattern: "^bad$"}}},
		{Key: "ps5", Name: "Again", Conditions: []ConditionDoc{{Kind: CondAudio, Values: []string{"aac"}}}},
		{Key: "broken", Name: "Broken", Conditions: []ConditionDoc{{Kind: CondTitle, Pattern: "("}}},
	}
	music := ProfileDoc{Key: "hires", Name: "Hi-res", Kinds: []domain.MediaKind{domain.KindMusic},
		Groups: []GroupDoc{{Qualities: []QualityID{"flac-24"}}}, Cutoff: "flac-24"}
	c, errs := BuildCatalog(formats, []ProfileDoc{validProfile(), music})
	if len(errs) != 2 || errs[0].Key != "ps5" || errs[1].Key != "broken" {
		t.Fatalf("errors %v", errs)
	}
	if len(c.Formats()) != 2 || c.Formats()[0].Name() != "Bad" {
		t.Fatal("formats sorted by name")
	}
	if _, ok := c.Lookup(domain.KindTV, "ps5high"); !ok {
		t.Fatal("custom tv")
	}
	if _, ok := c.Lookup(domain.KindMusic, "ps5high"); ok {
		t.Fatal("video profile served music")
	}
	if p, ok := c.Lookup(domain.KindMovie, "hd1080p"); !ok || p.Spec().Kind != domain.KindMovie {
		t.Fatal("built-in movie")
	}
	if n := len(c.Profiles(domain.KindMusic)); n != 4 {
		t.Fatalf("music profiles %d", n)
	}
	if u := c.UsedBy("bad"); len(u) != 1 || u[0].Key() != "ps5high" {
		t.Fatalf("used by %v", u)
	}
	restore := WithCatalog(c)
	if _, ok := Lookup(domain.KindTV, "ps5high"); !ok {
		t.Fatal("current catalog")
	}
	restore()
	if _, ok := Lookup(domain.KindTV, "ps5high"); ok {
		t.Fatal("restore")
	}
	if !IsBuiltinKey("lossless") || IsBuiltinKey("ps5high") {
		t.Fatal("builtin keys")
	}
}

func TestCompileFormat_Limits(t *testing.T) {
	title := ConditionDoc{Kind: CondTitle, Pattern: "x"}
	if _, es := CompileFormat(FormatDoc{Key: "k", Name: strings.Repeat("é", maxNameLen), Conditions: []ConditionDoc{title}}); len(es) > 0 {
		t.Fatalf("longest name refused: %v", es)
	}
	if _, es := CompileFormat(FormatDoc{Key: "k", Name: strings.Repeat("é", maxNameLen+1), Conditions: []ConditionDoc{title}}); !contains(fieldsOf(es), "name") {
		t.Fatalf("name too long: %v", es)
	}
	most := make([]ConditionDoc, maxConditions)
	for i := range most {
		most[i] = title
	}
	if f, es := CompileFormat(FormatDoc{Key: "k", Name: "n", Conditions: most}); len(es) > 0 || len(f.Doc().Conditions) != maxConditions {
		t.Fatalf("most conditions refused: %v", es)
	}
	if _, es := CompileFormat(FormatDoc{Key: "k", Name: "n", Conditions: append(most, title)}); !contains(fieldsOf(es), "conditions") {
		t.Fatalf("too many conditions: %v", es)
	}
}

func TestCompileFormat_PatternErrorPosition(t *testing.T) {
	msg := func(pattern string) string {
		_, es := CompileFormat(FormatDoc{Key: "k", Name: "n", Conditions: []ConditionDoc{{Kind: CondTitle, Pattern: pattern}}})
		if len(es) != 1 {
			t.Fatalf("%q: errors %v", pattern, es)
		}
		return es[0].Msg
	}
	if m := msg("*abc"); !strings.HasSuffix(m, "at position 1") {
		t.Errorf("error at the start: %q", m)
	}
	if m := msg("(x264"); strings.Contains(m, "position") || !strings.Contains(m, "missing closing )") {
		t.Errorf("error spanning the whole pattern: %q", m)
	}
}

func TestCompileFormat_ValuesAndRanges(t *testing.T) {
	cases := []struct {
		name string
		c    ConditionDoc
		path string // "" = accepted
	}{
		{"language outside the list", ConditionDoc{Kind: CondLanguage, Values: []string{"xyz"}}, ""},
		{"language-shaped source", ConditionDoc{Kind: CondSource, Values: []string{"xyz"}}, "conditions.0.values.0"},
		{"year of one year", ConditionDoc{Kind: CondYear, Min: 2020, Max: 2020}, ""},
		{"year below min", ConditionDoc{Kind: CondYear, Min: 2020, Max: 2019}, "conditions.0.max"},
		{"size max under one", ConditionDoc{Kind: CondSize, Min: 1, Max: 0.5}, "conditions.0.max"},
		{"size max equals min", ConditionDoc{Kind: CondSize, Min: 0.5, Max: 0.5}, "conditions.0.max"},
		{"size without bounds", ConditionDoc{Kind: CondSize}, ""},
		{"size above min", ConditionDoc{Kind: CondSize, Min: 0.5, Max: 0.6}, ""},
	}
	for _, c := range cases {
		_, es := CompileFormat(FormatDoc{Key: "k", Name: "n", Conditions: []ConditionDoc{c.c}})
		switch {
		case c.path == "" && len(es) > 0:
			t.Errorf("%s: refused %v", c.name, es)
		case c.path != "" && (len(es) != 1 || es[0].Path != c.path):
			t.Errorf("%s: errors %v", c.name, es)
		}
	}
}

func TestCompileProfile_ReportsOnlyTheRealProblem(t *testing.T) {
	mut := func(f func(*ProfileDoc)) ProfileDoc { d := validProfile(); f(&d); return d }
	cases := map[string]struct {
		doc   ProfileDoc
		paths []string
	}{
		"no kinds":  {mut(func(d *ProfileDoc) { d.Kinds = nil }), []string{"kinds"}},
		"no groups": {mut(func(d *ProfileDoc) { d.Groups, d.Sizes = nil, nil }), []string{"groups"}},
		"cutoff outside one quality": {mut(func(d *ProfileDoc) {
			d.Groups, d.Sizes = []GroupDoc{{Qualities: []QualityID{"webdl-720p"}}}, nil
		}), []string{"cutoff"}},
		"empty group":     {mut(func(d *ProfileDoc) { d.Groups = append(d.Groups, GroupDoc{}) }), []string{"groups.2.qualities"}},
		"two size limits": {mut(func(d *ProfileDoc) { d.Sizes = append(d.Sizes, d.Sizes[0]) }), []string{"sizes.1.quality"}},
	}
	for name, c := range cases {
		_, es := CompileProfile(c.doc, testFormats(t))
		if !reflect.DeepEqual(fieldsOf(es), c.paths) {
			t.Errorf("%s: errors %v", name, es)
		}
	}
}

func TestCompileProfile_MusicFloorIsAllowed(t *testing.T) {
	d := ProfileDoc{Key: "floor", Name: "Floor", Kinds: []domain.MediaKind{domain.KindMusic},
		Groups: []GroupDoc{{Qualities: []QualityID{musicFloor}}}, Cutoff: musicFloor}
	if _, es := CompileProfile(d, nil); len(es) > 0 {
		t.Fatalf("floor refused: %v", es)
	}
	if BelowFloor(musicFloor) || !BelowFloor("mp3-96") {
		t.Fatal("floor boundary")
	}
}

func TestBuildCatalog_LeavesOutProfilesWithOneProblem(t *testing.T) {
	broken := validProfile()
	broken.Key, broken.Cutoff = "broken", "bluray-1080p"
	other := validProfile()
	other.Key, other.Name, other.Formats = "other", "Other", []ScoreDoc{{Format: "ps5", Score: 1}}
	formats := []FormatDoc{
		{Key: "ps5", Name: "Same", Conditions: []ConditionDoc{{Kind: CondAudio, Values: []string{"aac"}}}},
		{Key: "bad", Name: "same", Conditions: []ConditionDoc{{Kind: CondGroup, Pattern: "^bad$"}}},
	}
	c, errs := BuildCatalog(formats, []ProfileDoc{broken, validProfile(), other})
	if len(errs) != 1 || errs[0].Key != "broken" || errs[0].Path != "cutoff" {
		t.Fatalf("errors %v", errs)
	}
	if got := errs[0].Error(); got != "profile broken: cutoff: must be one of the allowed qualities" {
		t.Fatalf("message %q", got)
	}
	if n := len(c.Customs()); n != 2 {
		t.Fatalf("customs %d", n)
	}
	if fs := c.Formats(); fs[0].Key() != "ps5" || fs[1].Key() != "bad" {
		t.Fatal("formats with the same name keep their order")
	}
	if u := c.UsedBy("bad"); len(u) != 1 || u[0].Key() != "ps5high" {
		t.Fatalf("used by %v", u)
	}
}

func TestDocError_Message(t *testing.T) {
	if got := (DocError{Path: "name", Msg: "too long"}).Error(); got != "name: too long" {
		t.Fatal(got)
	}
	if got := (DocError{Msg: "bad"}).Error(); got != "bad" {
		t.Fatal(got)
	}
}
