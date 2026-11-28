package decide

import (
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
)

func withFormats(t *testing.T, cutoffScore int) QualityProfile {
	t.Helper()
	spec := profile(t, "Any").Spec()
	c, err := NewCondition("x265", `x265`, false, false)
	if err != nil {
		t.Fatal(err)
	}
	cf, _ := NewCustomFormat("x265", c)
	spec.Formats, spec.CutoffFormatScore = []FormatScore{{Format: cf, Score: 50}}, cutoffScore
	p, err := NewQualityProfile(spec)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDesirability(t *testing.T) {
	// "Any" profile: cutoff WEBDL-1080p; preferred size at the default runtime.
	pref := Decide(input(t, "Film.2020.1080p.WEB-DL-GRP", 5*gb)).Desirability.PreferredSize
	cases := []struct {
		name, title string
		size        int64
		profile     *QualityProfile
		score       int
		pos         domain.Position
		steps       int
	}{
		{"on target", "Film.2020.1080p.WEB-DL-GRP", pref, nil, 100, domain.PositionOnTarget, 0},
		{"inside the size band", "Film.2020.1080p.WEB-DL-GRP", pref + pref/20, nil, 100, domain.PositionOnTarget, 0},
		{"one step above", "Film.2020.1080p.BluRay.x264-GRP", pref, nil, 75, domain.PositionAbove, 1},
		{"one step below", "Film.2020.1080p.HDTV.x264-GRP", pref, nil, 75, domain.PositionBelow, -1},
		{"much larger", "Film.2020.1080p.WEB-DL-GRP", pref * 3, nil, 60, domain.PositionAbove, 0},
		{"half size", "Film.2020.1080p.WEB-DL-GRP", pref / 2, nil, 80, domain.PositionBelow, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := input(t, c.title, c.size)
			if c.profile != nil {
				in.Item.Profile = *c.profile
			}
			d := Decide(in).Desirability
			if d == nil || d.Score != c.score || d.Position != c.pos || d.QualitySteps != c.steps || d.Cutoff != "WEBDL-1080p" {
				t.Fatalf("got %+v", d)
			}
		})
	}
}

func TestDesirabilityFormatShortfallAndUnknowns(t *testing.T) {
	in := input(t, "Film.2020.1080p.WEB-DL-GRP", 0)
	in.Item.Profile = withFormats(t, 100)
	d := Decide(in).Desirability
	if d.SizeDelta != nil || d.FormatShortfall != 100 || d.FormatTarget != 100 || d.Score != 80 || d.Position != domain.PositionBelow {
		t.Fatalf("shortfall: %+v", d)
	}
	in = input(t, "Film.2020.1080p.WEB-DL.x265-GRP", 0)
	in.Item.Profile = withFormats(t, 40)
	if d := Decide(in).Desirability; d.FormatShortfall != 0 || d.Score != 100 || d.Position != domain.PositionOnTarget {
		t.Fatalf("surplus is not penalised: %+v", d)
	}
	if d := Decide(input(t, "Film.2020.1080p.BluRay.ISO", gb)).Desirability; d != nil {
		t.Fatalf("outside the profile: %+v", d)
	}
}

func TestDesirabilityClampsAtZero(t *testing.T) {
	d := Decide(input(t, "Film.2020.DVDRip.XviD-GRP", 100*gb)).Desirability
	if d == nil || d.Score != 0 || d.Position != domain.PositionBelow {
		t.Fatalf("got %+v", d)
	}
}
