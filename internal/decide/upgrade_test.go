package decide

import (
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestIsUpgrade(t *testing.T) {
	base := ProfileSpec{Name: "p", Kind: domain.KindMovie, Upgrades: true,
		Groups: single("webdl-720p", "webdl-1080p", "bluray-1080p"), Cutoff: "webdl-1080p", CutoffFormatScore: 100}
	on, err := NewQualityProfile(base)
	if err != nil {
		t.Fatal(err)
	}
	base.Upgrades = false
	off, _ := NewQualityProfile(base)

	cases := []struct {
		name string
		p    QualityProfile
		cur  CurrentFile
		cand Candidate
		want bool
	}{
		// the upgrade ladder: 720p/0 → 1080p/0 → 1080p/100 → nothing more
		{"higher quality", on, CurrentFile{Quality: "webdl-720p"}, Candidate{Quality: "webdl-1080p"}, true},
		{"same quality, higher score", on, CurrentFile{Quality: "webdl-1080p"}, Candidate{Quality: "webdl-1080p", FormatScore: 100}, true},
		{"both cutoffs met", on, CurrentFile{Quality: "webdl-1080p", FormatScore: 100}, Candidate{Quality: "bluray-1080p", FormatScore: 200}, false},
		{"same quality, same score", on, CurrentFile{Quality: "webdl-720p"}, Candidate{Quality: "webdl-720p"}, false},
		{"lower quality", on, CurrentFile{Quality: "webdl-1080p"}, Candidate{Quality: "webdl-720p", FormatScore: 50}, false},
		{"upgrades off", off, CurrentFile{Quality: "webdl-720p"}, Candidate{Quality: "webdl-1080p"}, false},
		{"candidate not allowed", on, CurrentFile{Quality: "webdl-720p"}, Candidate{Quality: "remux-1080p"}, false},
		{"current outside profile", on, CurrentFile{Quality: "sdtv"}, Candidate{Quality: "webdl-720p"}, true},
		// propers and repacks upgrade even past the cutoffs
		{"proper any group", on, CurrentFile{Quality: "webdl-1080p", FormatScore: 100, Group: "A", Revision: 1},
			Candidate{Quality: "webdl-1080p", FormatScore: 100, Group: "B", Revision: 2, Proper: true}, true},
		{"repack same group", on, CurrentFile{Quality: "webdl-1080p", FormatScore: 100, Group: "A"},
			Candidate{Quality: "webdl-1080p", FormatScore: 100, Group: "a", Revision: 2}, true},
		{"repack other group", on, CurrentFile{Quality: "webdl-1080p", FormatScore: 100, Group: "A"},
			Candidate{Quality: "webdl-1080p", FormatScore: 100, Group: "B", Revision: 2}, false},
		{"older revision", on, CurrentFile{Quality: "webdl-1080p", FormatScore: 100, Group: "A", Revision: 3},
			Candidate{Quality: "webdl-1080p", FormatScore: 100, Group: "A", Revision: 2, Proper: true}, false},
	}
	for _, c := range cases {
		got, why := IsUpgrade(c.p, c.cur, c.cand)
		if got != c.want {
			t.Errorf("%s: got %v (%s)", c.name, got, why)
		}
		if !got && why == "" {
			t.Errorf("%s: a refusal must say why", c.name)
		}
	}
}
