package music

import (
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/audiotag"
	"github.com/jrand0m/corsarr/internal/domain"
)

func sec(n int) time.Duration { return time.Duration(n) * time.Second }

func timed(spec ...[3]any) []domain.Track {
	out := make([]domain.Track, len(spec))
	for i, s := range spec {
		out[i] = domain.Track{ID: int64(i + 1), Medium: 1, Number: i + 1, Title: s[0].(string), DurationSec: s[1].(int),
			RecordingID: s[2].(string)}
	}
	return out
}

func tagged(rel string, tags audiotag.Tags) AudioFile { return AudioFile{Rel: rel, Tags: tags} }

func placedOn(m FileMatch) map[string]int64 {
	out := map[string]int64{}
	for _, p := range m.Placements {
		out[p.File] = p.Track.ID
	}
	return out
}

func TestMatchFilesTrustsTagsOverFileNames(t *testing.T) {
	album := timed([3]any{"Opening", 200, ""}, [3]any{"Middle", 240, ""}, [3]any{"Closing", 300, ""})
	files := []AudioFile{
		tagged("01.flac", audiotag.Tags{Title: "Closing", Track: 3, Duration: sec(301)}),
		tagged("02.flac", audiotag.Tags{Title: "Opening", Track: 1, Duration: sec(199)}),
		tagged("03.flac", audiotag.Tags{Title: "Middle", Track: 2, Duration: sec(246)}),
	}
	m := MatchFiles(files, album)
	got := placedOn(m)
	if got["01.flac"] != 3 || got["02.flac"] != 1 || got["03.flac"] != 2 || !m.Auto() || m.Distance != 0 {
		t.Fatalf("%+v", m)
	}
}

func TestMatchFilesUsesRecordingIDs(t *testing.T) {
	album := timed([3]any{"Untitled", 0, "rec-1"}, [3]any{"Untitled", 0, "rec-2"})
	files := []AudioFile{
		tagged("a.flac", audiotag.Tags{Title: "Untitled", MusicBrainz: audiotag.MBIDs{Recording: "REC-2"}}),
		tagged("b.flac", audiotag.Tags{Title: "Untitled", MusicBrainz: audiotag.MBIDs{Recording: "rec-1"}}),
	}
	m := MatchFiles(files, album)
	if got := placedOn(m); got["a.flac"] != 2 || got["b.flac"] != 1 || m.Placements[0].By != "recording" {
		t.Fatalf("%+v", m)
	}
}

func TestMatchFilesAssignsOptimallyRatherThanInFileOrder(t *testing.T) {
	// Both files carry the title "Intro"; only their lengths tell them
	// apart. Taking files in order would give the long one to "Intro".
	album := timed([3]any{"Intro", 60, ""}, [3]any{"Intro (Reprise)", 300, ""})
	files := []AudioFile{
		tagged("a.flac", audiotag.Tags{Title: "Intro", Duration: sec(298)}),
		tagged("b.flac", audiotag.Tags{Title: "Intro", Duration: sec(61)}),
	}
	m := MatchFiles(files, album)
	if got := placedOn(m); got["a.flac"] != 2 || got["b.flac"] != 1 || !m.Auto() {
		t.Fatalf("%+v", m)
	}
}

func TestMatchFilesHoldsDoubtfulMatchesForReview(t *testing.T) {
	album := timed([3]any{"Opening", 200, ""}, [3]any{"Closing", 200, ""})
	// Numbers and lengths agree but every tagged title differs: each pair
	// is plausible on its own, the album as a whole is not certain.
	doubtful := []AudioFile{
		tagged("1.flac", audiotag.Tags{Title: "Something Else", Track: 1, Duration: sec(200)}),
		tagged("2.flac", audiotag.Tags{Title: "Another Song", Track: 2, Duration: sec(200)}),
	}
	m := MatchFiles(doubtful, album)
	if len(m.Placements) != 2 || m.Auto() || m.Distance <= AlbumFloor {
		t.Fatalf("%+v", m)
	}
	// A file that disagrees on title, number and length is never placed.
	wrong := []AudioFile{tagged("x.flac", audiotag.Tags{Title: "Other", Track: 5, Duration: sec(500)})}
	if m := MatchFiles(wrong, album); len(m.Placements) != 0 || len(m.Unmatched) != 1 || len(m.Missing) != 2 {
		t.Fatalf("%+v", m)
	}
	// A different recording outweighs an agreeing title.
	album[0].RecordingID = "rec-1"
	other := []AudioFile{tagged("y.flac", audiotag.Tags{Title: "Opening", MusicBrainz: audiotag.MBIDs{Recording: "rec-9"}})}
	if m := MatchFiles(other, album); len(m.Placements) != 0 {
		t.Fatalf("%+v", m)
	}
}

func TestDurationDistanceAllowsSlack(t *testing.T) {
	cases := []struct {
		file, track int
		want        float64
		known       bool
	}{
		{200, 210, 0, true}, {200, 225, 0.5, true}, {200, 240, 1, true}, {500, 200, 1, true}, {0, 200, 0, false}, {200, 0, 0, false},
	}
	for _, c := range cases {
		d, ok := durationDistance(sec(c.file), c.track)
		if ok != c.known || d != c.want {
			t.Errorf("%d vs %d: %v %v", c.file, c.track, d, ok)
		}
	}
}

func TestTitleDistance(t *testing.T) {
	if d := titleDistance("Song", "song"); d != 0 {
		t.Fatal(d)
	}
	if d := titleDistance("Song (Remastered 2011)", "Song"); d != titleContained {
		t.Fatal(d)
	}
	if d := titleDistance("Abc", "Xyz"); d != 1 {
		t.Fatal(d)
	}
	if d := titleDistance("A", "Ab"); d != 1 {
		t.Fatal("short titles are compared, never contained", d)
	}
	if d := titleDistance("Abcd", "Abce"); d != 0.5 {
		t.Fatal("a typo stays close", d)
	}
}
