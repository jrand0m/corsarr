package decide

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
)

func mustCond(t *testing.T, name, pattern string, negate, required bool) Condition {
	t.Helper()
	c, err := NewCondition(name, pattern, negate, required)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mustFormat(t *testing.T, name string, conds ...Condition) CustomFormat {
	t.Helper()
	f, err := NewCustomFormat(name, conds...)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestNewCondition_RefusesInvalidRegex(t *testing.T) {
	if _, err := NewCondition("broken", `(x264`, false, false); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("want error naming the condition, got %v", err)
	}
}

func TestNewCustomFormat_RefusesEmpty(t *testing.T) {
	c := mustCond(t, "c", "x", false, false)
	if _, err := NewCustomFormat("", c); err == nil {
		t.Error("empty name accepted")
	}
	if _, err := NewCustomFormat("f"); err == nil {
		t.Error("no conditions accepted")
	}
}

func TestCustomFormat_Matches(t *testing.T) {
	compatible := mustFormat(t, "Direct play codec",
		mustCond(t, "h264 or hevc", `\b(x|h)\.?26[45]\b|hevc`, false, true),
		mustCond(t, "no AV1", `\bav1\b`, true, true))
	safeAudio := mustFormat(t, "Safe audio",
		mustCond(t, "aac", `\baac`, false, false),
		mustCond(t, "ac3", `\b(e-?)?ac-?3|ddp?`, false, false))
	notAudioDTS := mustFormat(t, "Not DTS", mustCond(t, "dts", `\bdts`, true, false))
	cases := []struct {
		f     CustomFormat
		title string
		want  bool
	}{
		{compatible, "Movie.2020.1080p.WEB.H264-GRP", true},
		{compatible, "Movie.2020.1080p.WEB.HEVC-GRP", true},
		{compatible, "Movie.2020.1080p.WEB.AV1.HEVC-GRP", false}, // required negation fails
		{compatible, "Movie.2020.1080p.WEB.XviD-GRP", false},     // required condition fails
		{safeAudio, "Movie.2020.1080p.WEB.DDP5.1.H264-GRP", true},
		{safeAudio, "Movie.2020.1080p.WEB.AAC.H264-GRP", true},
		{safeAudio, "Movie.2020.1080p.BluRay.DTS.x264-GRP", false},
		{notAudioDTS, "Movie.2020.1080p.BluRay.DTS.x264-GRP", false},
		{notAudioDTS, "Movie.2020.1080p.BluRay.AC3.x264-GRP", true},
	}
	for _, c := range cases {
		if got := c.f.Matches(TitleSubject(c.title)); got != c.want {
			t.Errorf("%s on %s: got %v", c.f.Name(), c.title, got)
		}
	}
}

func TestNewQualityProfile_RefusesInvalid(t *testing.T) {
	ok := []QualityGroup{{Qualities: []QualityID{"webdl-1080p"}}}
	cases := map[string]ProfileSpec{
		"no name":            {Kind: domain.KindMovie, Groups: ok, Cutoff: "webdl-1080p"},
		"bad kind":           {Name: "p", Kind: "comic", Groups: ok, Cutoff: "webdl-1080p"},
		"empty allowed":      {Name: "p", Kind: domain.KindMovie, Cutoff: "webdl-1080p"},
		"empty group":        {Name: "p", Kind: domain.KindMovie, Groups: []QualityGroup{{Name: "g"}}, Cutoff: "webdl-1080p"},
		"unknown quality":    {Name: "p", Kind: domain.KindMovie, Groups: single("webdl-9000p"), Cutoff: "webdl-9000p"},
		"duplicate quality":  {Name: "p", Kind: domain.KindMovie, Groups: single("webdl-1080p", "webdl-1080p"), Cutoff: "webdl-1080p"},
		"cutoff not allowed": {Name: "p", Kind: domain.KindMovie, Groups: ok, Cutoff: "bluray-1080p"},
		"negative increment": {Name: "p", Kind: domain.KindMovie, Groups: ok, Cutoff: "webdl-1080p", MinUpgradeIncrement: -1},
		"size not allowed":   {Name: "p", Kind: domain.KindMovie, Groups: ok, Cutoff: "webdl-1080p", Sizes: map[QualityID]QualityDefinition{"bluray-1080p": {}}},
		"music shared":       {Name: "p", Kinds: []domain.MediaKind{domain.KindMusic, domain.KindTV}, Groups: ok, Cutoff: "webdl-1080p"},
		"kind twice":         {Name: "p", Kinds: []domain.MediaKind{domain.KindTV, domain.KindTV}, Groups: ok, Cutoff: "webdl-1080p"},
		"audio on video":     {Name: "p", Kind: domain.KindMovie, Groups: single("flac"), Cutoff: "flac"},
	}
	for name, spec := range cases {
		if _, err := NewQualityProfile(spec); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestNewQualityProfile_OnlyTVAndMoviesShare(t *testing.T) {
	ok := []QualityGroup{{Qualities: []QualityID{"webdl-1080p"}}}
	for _, other := range []domain.MediaKind{domain.KindBook, domain.KindMusic} {
		_, err := NewQualityProfile(ProfileSpec{Name: "p", Kinds: []domain.MediaKind{domain.KindTV, other}, Groups: ok, Cutoff: "webdl-1080p"})
		if err == nil || !strings.Contains(err.Error(), "only TV and movies can share a profile") {
			t.Errorf("tv with %s: %v", other, err)
		}
	}
}

func TestNewQualityProfile_GroupsRankEqualAndDefaultIncrement(t *testing.T) {
	p, err := NewQualityProfile(ProfileSpec{Name: "p", Kind: domain.KindTV,
		Groups: []QualityGroup{{Qualities: []QualityID{"hdtv-720p"}}, group("WEB", "webrip-720p", "webdl-720p")},
		Cutoff: "webdl-720p"})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := p.Rank("webrip-720p")
	b, _ := p.Rank("webdl-720p")
	c, _ := p.Rank("hdtv-720p")
	if a != b || a <= c {
		t.Fatalf("ranks webrip %d webdl %d hdtv %d", a, b, c)
	}
	if _, ok := p.Rank("bluray-720p"); ok {
		t.Fatal("bluray not allowed")
	}
	if p.Spec().MinUpgradeIncrement != 1 {
		t.Fatal("default increment should be 1")
	}
}

func TestDefaultProfiles(t *testing.T) {
	for _, kind := range []domain.MediaKind{domain.KindMovie, domain.KindTV} {
		ps := DefaultProfiles(kind)
		var names []string
		for _, p := range ps {
			names = append(names, p.Spec().Name)
			for _, banned := range []QualityID{"unknown", "cam", "telesync", "remux-1080p", "remux-2160p", "disc-1080p", "disc-2160p"} {
				if _, ok := p.Rank(banned); ok {
					t.Errorf("%s allows %s", p.Spec().Name, banned)
				}
			}
			if p.Spec().AllowDiscImage || p.Spec().Allow3D || p.Spec().Kind != kind {
				t.Errorf("%s: unsafe defaults", p.Spec().Name)
			}
		}
		if !reflect.DeepEqual(names, []string{"Any", "HD-1080p", "Ultra-HD"}) {
			t.Fatalf("names %v", names)
		}
		if _, ok := ps[0].Rank("webdl-2160p"); ok {
			t.Error("Any stops at 1080p")
		}
	}
	if DefaultProfiles(domain.KindBook) != nil {
		t.Fatal("no shipped profiles for books")
	}
}

func TestProfileScore(t *testing.T) {
	free := mustFormat(t, "Freeleech", mustCond(t, "fl", `freeleech`, false, false))
	bad := mustFormat(t, "Bad group", mustCond(t, "grp", `-BADGRP$`, false, false))
	p, err := NewQualityProfile(ProfileSpec{Name: "p", Kind: domain.KindMovie, Groups: single("webdl-1080p"),
		Cutoff: "webdl-1080p", Formats: []FormatScore{{Format: free, Score: 50}, {Format: bad, Score: -1000}}})
	if err != nil {
		t.Fatal(err)
	}
	if free.Name() != "Freeleech" {
		t.Fatalf("name %q", free.Name())
	}
	if s, n := p.ScoreTitle("Movie.2020.1080p.WEB.FREELEECH-BADGRP"); s != -950 || len(n) != 2 {
		t.Fatalf("score %d %v", s, n)
	}
	if s, n := p.ScoreTitle("Movie.2020.1080p.WEB-GOOD"); s != 0 || len(n) != 0 {
		t.Fatalf("score %d %v", s, n)
	}
}
