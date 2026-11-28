package decide

import (
	"math"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
)

func TestQualityOf(t *testing.T) {
	cases := []struct {
		title     string
		want      QualityID
		confident bool
	}{
		{"Movie.2020.1080p.WEB-DL.H264-GRP", "webdl-1080p", true},
		{"Movie.2020.1080p.WEBRip.x264-GRP", "webrip-1080p", true},
		{"Movie.2020.2160p.UHD.BluRay.x265-GRP", "bluray-2160p", true},
		{"Movie.2020.1080p.BluRay.REMUX.AVC-GRP", "remux-1080p", true},
		{"Movie.2020.720p.HDTV.x264-GRP", "hdtv-720p", true},
		{"Movie.2020.576p.BluRay.x264-GRP", "bluray-576p", true},
		{"Movie.2020.480p.WEB.x264-GRP", "webdl-480p", true},
		{"Movie.2020.DVDRip.XviD-GRP", "dvd", false},
		{"Movie.2020.HDCAM.x264-GRP", "cam", false},
		{"Movie.2020.1080p.3D.BluRay.AVC.ISO", "disc-1080p", true},
		{"Movie.2020.COMPLETE.UHD.BLURAY-GRP", "disc-2160p", true},
		// missing information picks the most conservative rung, flagged low confidence
		{"Movie.2020.1080p.x264-GRP", "hdtv-1080p", false},
		{"Movie.2020.480p.x264-GRP", "sdtv", false},
		{"Movie.2020.WEB-DL.x264-GRP", "webdl-480p", false},
		{"Movie.2020.BluRay.REMUX-GRP", "remux-1080p", false},
		{"Movie.2020.720p.REMUX-GRP", "remux-1080p", false},
		{"Movie.2020.x264-GRP", "unknown", false},
	}
	for _, c := range cases {
		q, ok := QualityOf(parse.ParseRelease(c.title))
		if q.ID != c.want || ok != c.confident {
			t.Errorf("%s: got %s/%v, want %s/%v", c.title, q.ID, ok, c.want, c.confident)
		}
	}
}

func TestLadderIsOrderedAndComplete(t *testing.T) {
	l := Ladder()
	for i, q := range l {
		if q.Rank != i {
			t.Fatalf("%s has rank %d at position %d", q.ID, q.Rank, i)
		}
		if got, ok := LookupQuality(q.ID); !ok || got != q {
			t.Fatalf("lookup %s failed", q.ID)
		}
	}
	if _, ok := LookupQuality("nope"); ok {
		t.Fatal("unknown id should not resolve")
	}
	webdl1080, _ := LookupQuality("webdl-1080p")
	bluray720, _ := LookupQuality("bluray-720p")
	if webdl1080.Rank <= bluray720.Rank {
		t.Fatal("1080p WEB-DL should outrank 720p Bluray")
	}
}

func TestDefaultDefinitions_AllFiniteWithOwnerCaps(t *testing.T) {
	d := DefaultDefinitions()
	want := map[Tier]float64{Tier2160: 250, Tier1080: 70, Tier720: 35, TierSD: 25, TierUnknown: 25}
	for _, q := range Ladder() {
		def := d.For(q.ID)
		if math.IsInf(def.Max(), 0) || def.Max() <= def.Min() || def.Max() != want[q.Tier] {
			t.Errorf("%s: min %.0f pref %.0f max %.0f", q.ID, def.Min(), def.Preferred(), def.Max())
		}
		if def.Preferred() < def.Min() || def.Preferred() > def.Max() {
			t.Errorf("%s: preferred out of range", q.ID)
		}
	}
	if (Definitions(nil)).For("webdl-1080p").Max() != 70 {
		t.Fatal("nil Definitions must fall back to defaults")
	}
}

func TestNewQualityDefinition(t *testing.T) {
	bad := [][3]float64{
		{0, 10, 0},                  // max zero
		{0, 10, math.Inf(1)},        // unlimited
		{math.NaN(), 1, 2},          // NaN
		{-1, 1, 2},                  // negative
		{10, 5, 20},                 // preferred below min
		{1, 30, 20},                 // preferred above max
		{20, 20, 20},                // max not above min
		{0, 10, MaxMBPerMinute + 1}, // above largest allowed
	}
	for _, b := range bad {
		if _, err := NewQualityDefinition(b[0], b[1], b[2]); err == nil {
			t.Errorf("%v should be refused", b)
		}
	}
	d, err := NewQualityDefinition(1, 5, 10)
	if err != nil || d.Min() != 1 || d.Preferred() != 5 || d.Max() != 10 {
		t.Fatalf("valid definition: %+v %v", d, err)
	}
}

func TestBounds(t *testing.T) {
	def := DefaultDefinitions().For("bluray-1080p")
	b := Bounds(def, domain.KindMovie, 0)
	if !b.DefaultRuntime || b.Runtime != 120 || b.Max != 8_400_000_000 || b.Ceiling {
		t.Fatalf("unknown runtime: %+v", b)
	}
	b = Bounds(def, domain.KindMovie, 100)
	if b.DefaultRuntime || b.Max != 7_000_000_000 || b.Min != 600_000_000 || b.Preferred != 3_500_000_000 {
		t.Fatalf("known runtime: %+v", b)
	}
	// a wildly wrong runtime is still capped by the 50 GB movie ceiling
	b = Bounds(DefaultDefinitions().For("bluray-2160p"), domain.KindMovie, 600)
	if b.Max != 50*gb || !b.Ceiling || b.Preferred > b.Max {
		t.Fatalf("ceiling: %+v", b)
	}
	if LimitsFor(domain.KindTV).DefaultRuntime != 45 || LimitsFor("comic").CeilingBytes != 50*gb {
		t.Fatal("kind limits")
	}
}

func TestBluray576pRungSitsAfterBluray480p(t *testing.T) {
	q480, _ := LookupQuality("bluray-480p")
	q576, ok := LookupQuality("bluray-576p")
	hdtv720, _ := LookupQuality("hdtv-720p")
	if !ok || q576.Name != "Bluray-576p" || q576.Rank != q480.Rank+1 || hdtv720.Rank != q576.Rank+1 || q576.Tier != TierSD {
		t.Fatalf("576p rung %+v", q576)
	}
	cases := map[string]QualityID{
		"Movie.2001.576p.BluRay.x264-GRP": "bluray-576p",
		"Movie.2001.480p.BluRay.x264-GRP": "bluray-480p",
		"Movie.2001.BluRay.x264-GRP":      "bluray-480p",
		"Movie.2001.576p.WEB-DL.x264-GRP": "webdl-480p",
	}
	for title, want := range cases {
		if got, _ := QualityOf(parse.ParseRelease(title)); got.ID != want {
			t.Errorf("%s → %s, want %s", title, got.ID, want)
		}
	}
	any := DefaultProfiles(domain.KindMovie)[0]
	if _, ok := any.Rank("bluray-576p"); !ok {
		t.Fatal("Any must allow bluray-576p like bluray-480p")
	}
}
