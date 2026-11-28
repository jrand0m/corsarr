package migrate

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
)

func spec(impl string, values map[string]string, options map[string]string) SrcSpec {
	if options == nil {
		options = map[string]string{}
	}
	return SrcSpec{Name: impl, Impl: impl + "Specification", Values: values, Options: options}
}

func TestMapCondition_EveryKind(t *testing.T) {
	cases := []struct {
		app  App
		sp   SrcSpec
		want string // kind:values|pattern|min-max, "" = dropped
		note bool
	}{
		{Sonarr, spec("ReleaseTitle", map[string]string{"value": `\bDTS\b`}, nil), `title:\bDTS\b`, false},
		{Radarr, spec("ReleaseGroup", map[string]string{"value": `^FLUX$`}, nil), `group:^FLUX$`, false},
		{Radarr, spec("Edition", map[string]string{"value": `director`}, nil), `edition:director`, false},
		{Sonarr, spec("Source", map[string]string{"value": "7"}, nil), "source:remux,disc", false},
		{Sonarr, spec("Source", map[string]string{"value": "3"}, map[string]string{"value": "Web"}), "source:webdl", false},
		{Sonarr, spec("Source", map[string]string{"value": "2"}, nil), "source:hdtv", true},
		{Radarr, spec("Source", map[string]string{"value": "9"}, nil), "source:bluray,remux,disc", false},
		{Radarr, spec("Source", map[string]string{"value": "4"}, nil), "", true},
		{Sonarr, spec("Resolution", map[string]string{"value": "2160"}, map[string]string{"value": "R2160p"}), "resolution:2160", false},
		{Sonarr, spec("Resolution", map[string]string{"value": "540"}, nil), "resolution:576", true},
		{Radarr, spec("QualityModifier", map[string]string{"value": "4"}, nil), "modifier:disc", false},
		{Radarr, spec("Language", map[string]string{"value": "2"}, map[string]string{"value": "French"}), "language:fr", false},
		{Radarr, spec("Language", map[string]string{"value": "1", "exceptLanguage": "true"}, nil), "language!:en", false},
		{Radarr, spec("Language", map[string]string{"value": "-1"}, nil), "language!:und", false},
		{Radarr, spec("Language", map[string]string{"value": "-2"}, nil), "", true},
		{Sonarr, spec("IndexerFlag", map[string]string{"value": "1"}, nil), "flag:freeleech", false},
		{Radarr, spec("IndexerFlag", map[string]string{"value": "8"}, map[string]string{"value": "PTP_Golden"}), "", true},
		{Radarr, spec("IndexerFlag", map[string]string{"value": "2048"}, nil), "flag:nuked", true},
		{Sonarr, spec("IndexerFlag", map[string]string{"value": "1"}, map[string]string{"value": "Freeleech"}), "flag:freeleech", false},
		{Sonarr, spec("IndexerFlag", map[string]string{"value": "1"}, map[string]string{"value": "G_Freeleech"}), "flag:freeleech", false},
		{Sonarr, spec("Size", map[string]string{"min": "1", "max": "10"}, nil), "size:1-10", false},
		{Sonarr, spec("Size", map[string]string{"min": "5"}, nil), "size:5-0", false},
		{Sonarr, spec("Size", map[string]string{"max": "10"}, nil), "size:0-10", false},
		{Radarr, spec("Year", map[string]string{"min": "-5", "max": "-1"}, nil), "year:0-0", false},
		{Sonarr, spec("Size", map[string]string{}, nil), "", true},
		{Radarr, spec("Year", map[string]string{"min": "1990", "max": "1999"}, nil), "year:1990-1999", false},
		{Sonarr, spec("ReleaseType", map[string]string{"value": "3"}, nil), "releasetype:seasonpack", false},
		{Sonarr, spec("Mystery", nil, nil), "", true},
	}
	for _, c := range cases {
		m := mapCondition(c.app, c.sp)
		got := ""
		if m.ok {
			got = describe(m.doc)
		}
		if got != c.want || (len(m.notes) > 0) != c.note {
			t.Errorf("%s %s: got %q notes %v", c.app, c.sp.Impl, got, m.notes)
		}
	}
}

func describe(d decide.ConditionDoc) string {
	k := string(d.Kind)
	if d.Except {
		k += "!"
	}
	switch {
	case d.Pattern != "":
		return k + ":" + d.Pattern
	case len(d.Values) > 0:
		return k + ":" + strings.Join(d.Values, ",")
	}
	return fmt.Sprintf("%s:%g-%g", k, d.Min, d.Max)
}

func TestMapFormat_DropsUnsupportedRegexWithNote(t *testing.T) {
	f := SrcFormat{ID: 1, Name: "Not DTS", Specs: []SrcSpec{
		spec("ReleaseTitle", map[string]string{"value": `^(?!.*DTS).*$`}, nil),
		spec("Resolution", map[string]string{"value": "1080"}, nil)}}
	doc, notes, ok := mapFormat(Sonarr, f)
	doc, notes = keepCompilable(doc, notes)
	if !ok || len(doc.Conditions) != 1 || len(notes) != 1 || !strings.Contains(notes[0], "title condition was left out") {
		t.Fatalf("doc %+v notes %v", doc, notes)
	}
}

func f64p(v float64) *float64 { return &v }

// ownerShaped builds n identical device formats and m tiered profiles, as
// the owner's Sonarr and Radarr hold them.
func ownerShaped(app App, n, m int) Snapshot {
	s := Snapshot{Source: SourceInfo{App: app}, Profiles: map[int]SrcProfile{},
		Sizes: map[string]SrcSize{"WEBDL-1080p": {Min: f64p(5), Preferred: f64p(20), Max: f64p(40)}, "WEBDL-2160p": {Min: f64p(10), Max: nil}}}
	for i := 1; i <= n; i++ {
		s.CustomFormats = append(s.CustomFormats, SrcFormat{ID: 100 + i, Name: fmt.Sprintf("Device - %d Compatible", i), Specs: []SrcSpec{
			spec("ReleaseTitle", map[string]string{"value": fmt.Sprintf(`\bcodec%d\b`, i)}, nil),
			{Name: "Not 2160", Impl: "ResolutionSpecification", Negate: true, Required: true, Values: map[string]string{"value": "2160"}}}})
	}
	for i := 1; i <= m; i++ {
		s.Profiles[i] = SrcProfile{ID: i, Name: fmt.Sprintf("Tier %d - PS5", i), Upgrades: true, CutoffID: 1001, Cutoff: "WEB 1080p",
			MinScore: -100, CutoffScore: 200, StepScore: 10,
			Items: []SrcQualityItem{
				{ID: 1, Name: "Unknown", Members: []string{"Unknown"}},
				{ID: 5, Name: "WEBDL-720p", Allowed: true, Members: []string{"WEBDL-720p"}},
				{ID: 1001, Name: "WEB 1080p", Group: true, Allowed: true, Members: []string{"WEBRip-1080p", "WEBDL-1080p"}},
				{ID: 18, Name: "WEBDL-2160p", Allowed: i%2 == 0, Members: []string{"WEBDL-2160p"}},
			},
			Scores: []SrcScore{{Format: 100 + (i % n) + 1, Name: "Device", Score: 100 + i}, {Format: 999, Name: "Gone", Score: 5}, {Format: 101, Score: 0}}}
	}
	return s
}

func TestPlan_OwnerShapedProfilesAndFormatsDeduplicate(t *testing.T) {
	tv, movies := ownerShaped(Sonarr, 20, 30), ownerShaped(Radarr, 20, 30)
	tv.Series = []SrcSeries{{ID: 1, TMDB: 11, Title: "Show", ProfileID: 7, Path: "/tv/show", Root: "/tv"}}
	rep := BuildPlan([]Snapshot{tv, movies}, State{Quality: QualityState{Formats: map[string]string{}, Profiles: map[string]string{}}}, Options{})
	targets := map[string]map[App]Outcome{}
	for _, e := range rep.Entries {
		if e.Category != CatProfiles {
			continue
		}
		if targets[e.Target] == nil {
			targets[e.Target] = map[App]Outcome{}
		}
		targets[e.Target][e.Source] = e.Outcome
	}
	formats, profiles := 0, 0
	for target, outs := range targets {
		switch {
		case strings.HasPrefix(target, "customformat:"):
			formats++
		case strings.HasPrefix(target, "profile:"):
			profiles++
		}
		if len(outs) != 2 || outs[Radarr] != OutUnchanged {
			t.Errorf("%s: %v", target, outs)
		}
	}
	if formats != 20 || profiles != 30 {
		t.Fatalf("formats %d profiles %d", formats, profiles)
	}
	e := entry(t, rep, "sonarr:profile:2")
	if e.Outcome != OutApproximate || !strings.Contains(e.Reason, `"Gone"`) || !strings.Contains(e.Reason, "webdl-2160p had no size limit") {
		t.Fatalf("profile 2: %s %s", e.Outcome, e.Reason)
	}
	op := e.op.(qualityOp)
	p := op.profile
	if p.Key != "tier2ps5" || len(p.Kinds) != 2 || p.Cutoff != "webrip-1080p" || len(p.Groups) != 3 || p.Groups[1].Name != "WEB 1080p" ||
		p.MinFormatScore != -100 || p.CutoffFormatScore != 200 || p.MinUpgradeFormatScore != 10 || len(p.Formats) != 1 || p.Formats[0].Score != 102 {
		t.Fatalf("profile doc %+v", p)
	}
	if len(op.formats) != 1 || op.formats[0].Key != p.Formats[0].Format {
		t.Fatalf("the profile write carries its formats: %+v", op.formats)
	}
	sizes := map[decide.QualityID]decide.SizeDoc{}
	for _, s := range p.Sizes {
		sizes[s.Quality] = s
	}
	if s := sizes["webdl-1080p"]; s.Min != 5 || s.Preferred != 20 || s.Max != 40 {
		t.Fatalf("sizes %+v", p.Sizes)
	}
	if s := sizes["webdl-2160p"]; s.Min != 10 || s.Max != 250 {
		t.Fatalf("unlimited max keeps the default: %+v", p.Sizes)
	}
	if _, errs := decide.CompileProfile(*p, map[string]decide.CustomFormat{}); len(errs) == 0 {
		t.Fatal("without its formats the profile must not compile")
	}
}

func TestPlan_ItemsGetTheMappedProfileKey(t *testing.T) {
	tv := ownerShaped(Sonarr, 2, 3)
	snaps := []Snapshot{tv}
	b := newBuilder(snaps, State{}, Options{})
	b.planQuality(snaps)
	if k := profileKey(snaps[0], 3, domain.KindTV); k != "tier3ps5" {
		t.Fatalf("key %q", k)
	}
	if k := profileKey(snaps[0], 99, domain.KindTV); k != "hd1080p" {
		t.Fatalf("unknown id falls back: %q", k)
	}
}

func TestPlan_SameNameDifferentContentIsDisambiguated(t *testing.T) {
	tv, movies := ownerShaped(Sonarr, 1, 1), ownerShaped(Radarr, 1, 1)
	movies.CustomFormats[0].Specs[0].Values["value"] = `\bother\b`
	p := movies.Profiles[1]
	p.Upgrades = false
	movies.Profiles[1] = p
	rep := BuildPlan([]Snapshot{tv, movies}, State{}, Options{})
	f := entry(t, rep, "radarr:customformat:101")
	if f.Target != "customformat:device1compatibleradarr" || f.Outcome != OutApproximate {
		t.Fatalf("format %s %s %s", f.Target, f.Outcome, f.Reason)
	}
	if pe := entry(t, rep, "radarr:profile:1"); pe.Target != "profile:tier1ps5radarr" {
		t.Fatalf("profile %s", pe.Target)
	}
}

func TestQualityOutcome(t *testing.T) {
	stored := map[string]string{"same": "h1", "diff": "h1"}
	cases := map[string]Outcome{"same": OutUnchanged, "diff": OutUpdate, "new": OutCreate, "cfg": OutManaged}
	for key, want := range cases {
		fp := "h1"
		if key == "diff" {
			fp = "h2"
		}
		if got, _ := qualityOutcome(stored, map[string]bool{"cfg": true}, key, fp, "profile"); got != want {
			t.Errorf("%s: %s", key, got)
		}
	}
}

func TestReadQuality_WireShapes(t *testing.T) {
	cases := map[string]string{`"x"`: "x", `1080`: "1080", `1.5`: "1.5", `true`: "true", `null`: "", ``: ""}
	for raw, want := range cases {
		if got := rawText([]byte(raw)); got != want {
			t.Errorf("%s: %q", raw, got)
		}
	}
	f := srcFormat(wCustomFormat{ID: 3, Name: "HDR", Specifications: []wSpec{{Name: "src", Implementation: "SourceSpecification", Required: true,
		Fields: []wSpecField{{Name: "value", Value: []byte("6"), SelectOptions: []wSpecOption{{Value: 5, Name: "DVD"}, {Value: 6, Name: "Bluray"}}}}}}})
	if sp := f.Specs[0]; sp.Values["value"] != "6" || sp.Options["value"] != "Bluray" || !sp.Required {
		t.Fatalf("spec %+v", sp)
	}
	items := srcItems([]wProfileItem{{ID: 1001, Name: "WEB", Allowed: true, Items: []wProfileItem{{Quality: &struct {
		ID   int
		Name string
	}{3, "WEBDL-1080p"}}}}})
	if len(items) != 1 || !items[0].Group || items[0].Members[0] != "WEBDL-1080p" {
		t.Fatalf("items %+v", items)
	}
}

func TestSizeDoc(t *testing.T) {
	if d, notes := sizeDoc("webdl-1080p", SrcSize{Min: f64p(6), Preferred: f64p(35), Max: f64p(70)}); d != nil || notes != nil {
		t.Fatalf("defaults need no limit: %+v %v", d, notes)
	}
	d, notes := sizeDoc("webdl-1080p", SrcSize{Min: f64p(10), Max: f64p(5000)})
	if d == nil || d.Max != decide.MaxMBPerMinute || len(notes) != 1 {
		t.Fatalf("capped: %+v %v", d, notes)
	}
	d, _ = sizeDoc("webdl-720p", SrcSize{Min: f64p(50), Max: f64p(40)})
	if d.Max <= d.Min || d.Preferred < d.Min || d.Preferred > d.Max {
		t.Fatalf("kept valid: %+v", d)
	}
}

func TestConfigQuality(t *testing.T) {
	if q := configQuality(nil); len(q.ConfigFormats) != 0 {
		t.Fatal("nil config")
	}
	q := configQuality(&config.Config{CustomFormats: []decide.FormatDoc{{Key: "a"}}, QualityProfiles: []decide.ProfileDoc{{Key: "b"}}})
	if !q.ConfigFormats["a"] || !q.ConfigProfiles["b"] {
		t.Fatalf("%+v", q)
	}
}

func TestCustomProfileKeepsTheBluray576pRung(t *testing.T) {
	p := SrcProfile{ID: 1, Name: "SD", CutoffID: 22, Items: []SrcQualityItem{{ID: 22, Name: "Bluray-576p", Allowed: true, Members: []string{"Bluray-576p"}}}}
	doc, notes := mapCustomProfile(p, domain.KindMovie, nil, map[string]SrcSize{"Bluray-576p": {Min: f64p(2), Preferred: f64p(8), Max: f64p(20)}})
	if len(notes) != 0 || doc.Cutoff != "bluray-576p" || len(doc.Sizes) != 1 || doc.Sizes[0].Quality != "bluray-576p" || doc.Sizes[0].Max != 20 {
		t.Fatalf("doc %+v notes %v", doc, notes)
	}
	if _, errs := decide.CompileProfile(decide.ProfileDoc{Key: "sd", Name: doc.Name, Kinds: doc.Kinds, Groups: doc.Groups, Cutoff: doc.Cutoff, Sizes: doc.Sizes}, nil); len(errs) > 0 {
		t.Fatal(errs)
	}
}

func TestMapFormat_OkOnlyWhenAConditionSurvives(t *testing.T) {
	one := SrcFormat{ID: 1, Name: "One", Specs: []SrcSpec{spec("Resolution", map[string]string{"value": "1080"}, nil)}}
	if doc, _, ok := mapFormat(Sonarr, one); !ok || len(doc.Conditions) != 1 {
		t.Fatalf("one condition: %+v %v", doc, ok)
	}
	none := SrcFormat{ID: 2, Name: "None", Specs: []SrcSpec{spec("Mystery", nil, nil)}}
	if doc, notes, ok := mapFormat(Sonarr, none); ok || len(doc.Conditions) != 0 || len(notes) != 1 {
		t.Fatalf("no condition: %+v %v %v", doc, notes, ok)
	}
}

func TestCustomProfileCutoffAndUpgradeStep(t *testing.T) {
	p := SrcProfile{ID: 1, Name: "Mixed", CutoffID: 2, Cutoff: "WEBDL-1080p", Items: []SrcQualityItem{
		{ID: 1, Name: "WEBDL-720p", Allowed: true, Members: []string{"WEBDL-720p"}},
		{ID: 3, Name: "BR-DISK", Allowed: true, Members: []string{"BR-DISK"}},
		{ID: 2, Name: "WEBDL-1080p", Members: []string{"WEBDL-1080p"}},
	}}
	doc, notes := mapCustomProfile(p, domain.KindMovie, nil, nil)
	if doc.Cutoff != "disc-1080p" || len(doc.Groups) != 2 || doc.Groups[1].Qualities[0] != "disc-1080p" {
		t.Fatalf("a cutoff that is not allowed falls back to the best allowed: %+v", doc)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], `cutoff "WEBDL-1080p" has no corsarr equivalent`) {
		t.Fatalf("notes %v", notes)
	}
	if doc.MinUpgradeFormatScore != 1 {
		t.Fatalf("a zero upgrade step becomes 1: %d", doc.MinUpgradeFormatScore)
	}
	p.Items = p.Items[2:]
	if doc, notes := mapCustomProfile(p, domain.KindMovie, nil, nil); doc.Cutoff != "" || len(notes) != 0 {
		t.Fatalf("nothing allowed: no cutoff and no cutoff note: %+v %v", doc, notes)
	}
}

func TestSizeDocWidensAndBoundsTheMaximum(t *testing.T) {
	def := decide.DefaultDefinitions().For("webdl-1080p")
	cases := []struct {
		name     string
		min, max float64
		wantMax  float64
		notes    int
	}{
		{"maximum equal to the minimum doubles it", 10, 10, 20, 0},
		{"a tiny minimum adds one", 0.5, 0.2, 1.5, 0},
		{"no limit keeps the default", 1, 0, def.Max(), 1},
		{"a limit of one is kept", 0.5, 1, 1, 0},
		{"the largest allowed limit is kept as is", 1, decide.MaxMBPerMinute, decide.MaxMBPerMinute, 0},
	}
	for _, c := range cases {
		d, notes := sizeDoc("webdl-1080p", SrcSize{Min: f64p(c.min), Max: f64p(c.max)})
		if d == nil || d.Max != c.wantMax || len(notes) != c.notes {
			t.Errorf("%s: %+v %v", c.name, d, notes)
		}
	}
}
