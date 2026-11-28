package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
)

const qualityYAML = `version: 1
customFormats:
  ps5audio:
    name: Audio - PS5 Safe
    conditions:
      - kind: audio
        values: [aac, ac3, eac3]
      - kind: title
        pattern: '\bdts\b'
        negate: true
        required: true
qualityProfiles:
  ps5high:
    name: High - PS5
    kinds: [tv, movie]
    groups:
      - qualities: [webdl-720p]
      - name: WEB 1080p
        qualities: [webrip-1080p, webdl-1080p]
    cutoff: webdl-1080p
    formats:
      - format: ps5audio
        score: 100
    cutoffFormatScore: 100
    sizes:
      - quality: webdl-1080p
        min: 5
        max: 40
`

func TestQualitySectionsLoadAsDocuments(t *testing.T) {
	c := mustLoad(t, inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": qualityYAML}))
	if len(c.CustomFormats) != 1 || len(c.QualityProfiles) != 1 {
		t.Fatalf("docs %+v %+v", c.CustomFormats, c.QualityProfiles)
	}
	f := c.CustomFormats[0]
	if f.Key != "ps5audio" || len(f.Conditions) != 2 || f.Conditions[1].Pattern != `\bdts\b` || !f.Conditions[1].Negate || !f.Conditions[1].Required {
		t.Fatalf("format %+v", f)
	}
	p := c.QualityProfiles[0]
	if p.Key != "ps5high" || len(p.Kinds) != 2 || p.Kinds[1] != domain.KindMovie || !p.Upgrades || p.MinUpgradeFormatScore != 1 ||
		len(p.Groups) != 2 || p.Groups[1].Name != "WEB 1080p" || p.Formats[0].Score != 100 || p.Sizes[0].Max != 40 {
		t.Fatalf("profile %+v", p)
	}
	y := string(c.ExportYAML(ExportOptions{}))
	if !strings.Contains(y, "ps5audio") || !strings.Contains(y, "webrip-1080p") {
		t.Fatalf("export misses the sections:\n%s", y)
	}
	again := mustLoad(t, inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": y}))
	if string(again.ExportYAML(ExportOptions{})) != y {
		t.Fatal("export does not round-trip")
	}
}

func TestQualitySectionsReportProblemsWithLocations(t *testing.T) {
	bad := strings.Replace(qualityYAML, "format: ps5audio", "format: missing", 1)
	bad = strings.Replace(bad, `'\bdts\b'`, `'(dts'`, 1)
	_, err := Load(inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": bad}), testRegistry(t))
	var es Errors
	if !errors.As(err, &es) {
		t.Fatalf("want Errors, got %v", err)
	}
	paths := map[string]FieldError{}
	for _, e := range es {
		paths[e.Path] = e
	}
	pat, ok := paths["customFormats.ps5audio.conditions.1.pattern"]
	if !ok || pat.Loc.File != "/c.yml" || pat.Loc.Line == 0 {
		t.Fatalf("pattern error %v in %v", pat, es)
	}
	if _, ok := paths["qualityProfiles.ps5high.formats.0.format"]; !ok {
		t.Fatalf("missing format error in %v", es)
	}
}
