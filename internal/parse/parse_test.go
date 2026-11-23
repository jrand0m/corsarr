package parse

import (
	"reflect"
	"testing"
)

func TestParseRelease_EmptyAndJunkStayUnknown(t *testing.T) {
	for _, in := range []string{"", "   ", "-", "[]", "...."} {
		p := ParseRelease(in)
		if p.Resolution.Known || p.Source.Known || p.Codec.Known || p.Year.Known || p.Group.Known {
			t.Errorf("%q: nothing should be known, got %+v", in, p)
		}
		if p.Episode.Kind != EpisodeNone {
			t.Errorf("%q: unexpected episode %+v", in, p.Episode)
		}
	}
}

func TestParseRelease_KeepsRaw(t *testing.T) {
	in := "Some.Movie.2020.1080p.WEB.H264-GRP"
	if got := ParseRelease(in).Raw; got != in {
		t.Fatalf("Raw = %q", got)
	}
}

func TestRevision(t *testing.T) {
	cases := []struct {
		title string
		want  int
	}{
		{"Movie.2020.1080p.WEB.H264-GRP", 1},
		{"Movie.2020.PROPER.1080p.WEB.H264-GRP", 2},
		{"Movie.2020.REPACK.1080p.WEB.H264-GRP", 2},
		{"Show.S01E01v2.1080p.WEB.H264-GRP", 2},
		{"Show.S01E01v3.REPACK.1080p.WEB.H264-GRP", 4},
	}
	for _, c := range cases {
		if got := ParseRelease(c.title).Revision(); got != c.want {
			t.Errorf("%s: Revision = %d, want %d", c.title, got, c.want)
		}
	}
}

func TestEpisodes(t *testing.T) {
	cases := []struct {
		title string
		want  Episode
	}{
		{"Show.S01E03-E01.720p.WEB.H264-GRP", Episode{Kind: EpisodeStandard, Seasons: []int{1}, Episodes: []int{3}}},
		{"Show.S02E01E01.720p.WEB.H264-GRP", Episode{Kind: EpisodeStandard, Seasons: []int{2}, Episodes: []int{1}}},
		{"Show.1x01x02.HDTV.x264-GRP", Episode{Kind: EpisodeStandard, Seasons: []int{1}, Episodes: []int{1, 2}}},
		{"Show.S03-S01.1080p.WEB.H264-GRP", Episode{Kind: EpisodeSeason, Seasons: []int{3}}},
		{"Show.S01E011234.WEB-GRP", Episode{}},
	}
	for _, c := range cases {
		if got := ParseRelease(c.title).Episode; !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.title, got, c.want)
		}
	}
}

func TestDashSuffixThatIsAToken_IsNotAGroup(t *testing.T) {
	for _, in := range []string{"Movie.2020.1080p.WEB-DL", "Movie.2020.1080p.BluRay.DTS-HD", "Movie.2020.Part-2"} {
		if g := ParseRelease(in).Group; g.Known {
			t.Errorf("%s: group should be unknown, got %q", in, g.Value)
		}
	}
}
