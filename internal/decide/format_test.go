package decide

import (
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
)

func compileFormat(t *testing.T, conds ...ConditionDoc) CustomFormat {
	t.Helper()
	f, errs := CompileFormat(FormatDoc{Key: "f", Name: "F", Conditions: conds})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	return f
}

func subject(title string, size int64, flags ...string) Subject {
	return SubjectOf(domain.Release{Title: title, Size: size, Flags: flags}, parse.ParseRelease(title))
}

func TestSubjectOf(t *testing.T) {
	s := subject("Movie.2020.2160p.UHD.BluRay.REMUX.HDR10.TrueHD.7.1.Atmos.HEVC-GRP", 40e9, "freeleech")
	if s.Resolution != 2160 || s.Source != "remux" || s.Codec != "h265" || s.Audio != "truehd" || s.Group != "GRP" || s.Year != 2020 {
		t.Fatalf("subject %+v", s)
	}
	if len(s.HDR) != 1 || s.HDR[0] != "hdr10" || len(s.Modifiers) != 1 || s.Modifiers[0] != "remux" {
		t.Fatalf("hdr %v modifiers %v", s.HDR, s.Modifiers)
	}
	plain := subject("Show.S01E01.720p.WEB-DL.AAC2.0.H.264-GRP", 0)
	if plain.HDR[0] != "sdr" || plain.Languages[0] != "en" || plain.ReleaseType != "episode" {
		t.Fatalf("defaults %+v", plain)
	}
	if subject("Show.S01.1080p.WEB-DL-GRP", 0).ReleaseType != "seasonpack" ||
		subject("Show.S01E01E02.1080p.WEB-DL-GRP", 0).ReleaseType != "multiepisode" {
		t.Fatal("release types")
	}
	disc := subject("Movie.2020.1080p.BluRay.ISO-GRP", 0)
	if disc.Source != "disc" || disc.Modifiers[0] != "disc" {
		t.Fatalf("disc %+v", disc)
	}
}

func TestCondition_EveryKind(t *testing.T) {
	uhd := subject("Movie.2020.2160p.WEB-DL.DV.HDR10.DDP5.1.H.265-GRP", 15e9, "freeleech", "internal")
	cases := []struct {
		name string
		c    ConditionDoc
		s    Subject
		want bool
	}{
		{"title", ConditionDoc{Kind: CondTitle, Pattern: `\bweb-?dl\b`}, uhd, true},
		{"group", ConditionDoc{Kind: CondGroup, Pattern: `^grp$`}, uhd, true},
		{"group unknown", ConditionDoc{Kind: CondGroup, Pattern: `.*`}, subject("Movie 2020 1080p", 0), false},
		{"group unknown negated", ConditionDoc{Kind: CondGroup, Pattern: `.*`, Negate: true}, subject("Movie 2020 1080p", 0), true},
		{"edition", ConditionDoc{Kind: CondEdition, Pattern: `director`}, subject("Movie.2020.Directors.Cut.1080p.BluRay.x264-GRP", 0), true},
		{"source", ConditionDoc{Kind: CondSource, Values: []string{"webdl", "webrip"}}, uhd, true},
		{"source miss", ConditionDoc{Kind: CondSource, Values: []string{"bluray"}}, uhd, false},
		{"resolution", ConditionDoc{Kind: CondResolution, Values: []string{"2160"}}, uhd, true},
		{"modifier", ConditionDoc{Kind: CondModifier, Values: []string{"remux"}}, uhd, false},
		{"codec", ConditionDoc{Kind: CondCodec, Values: []string{"h265"}}, uhd, true},
		{"audio", ConditionDoc{Kind: CondAudio, Values: []string{"aac", "ac3", "eac3"}}, uhd, true},
		{"channels", ConditionDoc{Kind: CondChannels, Values: []string{"5.1"}}, uhd, true},
		{"hdr any", ConditionDoc{Kind: CondHDR, Values: []string{"dv"}}, uhd, true},
		{"hdr sdr", ConditionDoc{Kind: CondHDR, Values: []string{"sdr"}}, uhd, false},
		{"language default english", ConditionDoc{Kind: CondLanguage, Values: []string{"en"}}, uhd, true},
		{"language except", ConditionDoc{Kind: CondLanguage, Values: []string{"en"}, Except: true}, subject("Movie.2020.FRENCH.1080p.WEB-DL-GRP", 0), true},
		{"language except own", ConditionDoc{Kind: CondLanguage, Values: []string{"en"}, Except: true}, uhd, false},
		{"flag", ConditionDoc{Kind: CondFlag, Values: []string{"internal"}}, uhd, true},
		{"size in", ConditionDoc{Kind: CondSize, Min: 1, Max: 20}, uhd, true},
		{"size above", ConditionDoc{Kind: CondSize, Min: 1, Max: 10}, uhd, false},
		{"size open", ConditionDoc{Kind: CondSize, Min: 10}, uhd, true},
		{"size unknown", ConditionDoc{Kind: CondSize, Min: 0, Max: 10}, subject("Movie 2020", 0), false},
		{"release type", ConditionDoc{Kind: CondReleaseType, Values: []string{"seasonpack"}}, subject("Show.S02.1080p.WEB-DL-GRP", 0), true},
		{"year", ConditionDoc{Kind: CondYear, Min: 2019, Max: 2021}, uhd, true},
		{"year out", ConditionDoc{Kind: CondYear, Min: 2021}, uhd, false},
	}
	for _, c := range cases {
		f := compileFormat(t, c.c)
		if got := f.conditions[0].Matches(c.s); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestFormat_GroupedMatching(t *testing.T) {
	// required 2160p, HDR10 or Dolby Vision
	f := compileFormat(t,
		ConditionDoc{Kind: CondResolution, Values: []string{"2160"}, Required: true},
		ConditionDoc{Kind: CondHDR, Values: []string{"hdr10"}},
		ConditionDoc{Kind: CondHDR, Values: []string{"dv"}})
	cases := map[string]bool{
		"Movie.2020.2160p.WEB-DL.HDR10.H.265-GRP": true,
		"Movie.2020.2160p.WEB-DL.DV.H.265-GRP":    true,
		"Movie.2020.2160p.WEB-DL.H.265-GRP":       false,
		"Movie.2020.1080p.WEB-DL.HDR10.H.265-GRP": false,
	}
	for title, want := range cases {
		if got := f.Matches(subject(title, 0)); got != want {
			t.Errorf("%s: got %v", title, got)
		}
	}
	notBad := compileFormat(t, ConditionDoc{Kind: CondGroup, Pattern: `^bad$`, Negate: true})
	if !notBad.Matches(subject("Movie.2020.1080p.WEB-DL-GOOD", 0)) || notBad.Matches(subject("Movie.2020.1080p.WEB-DL-BAD", 0)) {
		t.Fatal("negated group")
	}
	if got := f.Explain(subject("Movie.2020.2160p.WEB-DL.DV.H.265-GRP", 0)); got[0] != true || got[1] != false || got[2] != true {
		t.Fatalf("explain %v", got)
	}
}

func TestKeyOf(t *testing.T) {
	cases := map[string]string{"HD-1080p": "hd1080p", "Low ~1.5GB/h - PS5": "low15gbhps5", "4K": "x4k", "—": "x"}
	for in, want := range cases {
		if got := KeyOf(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}

func TestCondition_ContainerBitDepthAndDolbyVisionFallback(t *testing.T) {
	withFiles := SubjectOf(domain.Release{Title: "Movie.2020.1080p.WEB-DL.H.264-GRP", FileList: []string{"Movie/sample.txt", "Movie/movie.MP4"}},
		parse.ParseRelease("Movie.2020.1080p.WEB-DL.H.264-GRP"))
	cases := []struct {
		name string
		c    ConditionDoc
		s    Subject
		want bool
	}{
		{"container from files", ConditionDoc{Kind: CondContainer, Values: []string{"mp4"}}, withFiles, true},
		{"container from title", ConditionDoc{Kind: CondContainer, Values: []string{"avi"}}, subject("Movie.2004.DVDRip.XviD.avi", 0), true},
		{"container unknown", ConditionDoc{Kind: CondContainer, Values: []string{"mkv"}}, subject("Movie.2020.1080p.WEB-DL-GRP", 0), false},
		{"container unknown negated", ConditionDoc{Kind: CondContainer, Values: []string{"mkv"}, Negate: true}, subject("Movie.2020.1080p.WEB-DL-GRP", 0), true},
		{"10bit", ConditionDoc{Kind: CondBitDepth, Values: []string{"10"}}, subject("Show.S01E01.1080p.WEB.H.264.10bit-GRP", 0), true},
		{"hi10p", ConditionDoc{Kind: CondBitDepth, Values: []string{"10"}}, subject("[Grp] Show - 01 [1080p Hi10P]", 0), true},
		{"bit depth unknown", ConditionDoc{Kind: CondBitDepth, Values: []string{"8"}}, subject("Show.S01E01.1080p.WEB.H.264-GRP", 0), false},
		{"dv with hdr10", ConditionDoc{Kind: CondDVFallback, Values: []string{"hdr10"}}, subject("Movie.2020.2160p.WEB-DL.DV.HDR10.H.265-GRP", 0), true},
		{"dv alone unknown", ConditionDoc{Kind: CondDVFallback, Values: []string{"none", "hdr10"}}, subject("Movie.2020.2160p.WEB-DL.DV.H.265-GRP", 0), false},
		{"no dv", ConditionDoc{Kind: CondDVFallback, Values: []string{"none"}}, subject("Movie.2020.2160p.WEB-DL.HDR10.H.265-GRP", 0), false},
	}
	for _, c := range cases {
		if got := compileFormat(t, c.c).conditions[0].Matches(c.s); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
	p5 := parse.ParseRelease("Movie.2020.2160p.WEB-DL.DV.H.265-GRP")
	p5.DVProfile = parse.Field[int]{Value: 5, Known: true}
	if dvFallbackOf(p5) != "none" {
		t.Fatal("profile 5 has no fallback")
	}
	p5.DVProfile.Value = 8
	if dvFallbackOf(p5) != "hdr10" {
		t.Fatal("profile 8 has an HDR10 base")
	}
}
