package naming

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

var movie = Values{MovieTitle: "The Quiet Valley", ReleaseYear: 2019, QualityFull: "Bluray-1080p",
	ReleaseGroup: "SPARKS", IMDbID: "tt0000001"}

func codes(ws []Warning) []string {
	out := []string{}
	for _, w := range ws {
		out = append(out, w.Code)
	}
	return out
}

func TestPreview_DefaultMovieTemplates(t *testing.T) {
	got, warns := Options{Extension: "mkv"}.Preview(DefaultMovieFolder+"/"+DefaultMovieFile, movie)
	want := "The Quiet Valley (2019)/The Quiet Valley (2019) [Bluray-1080p] - SPARKS.mkv"
	if got != want || len(warns) != 0 {
		t.Fatalf("got %q %v", got, warns)
	}
}

func TestPreview_OptionalTextDropsWhenEmpty(t *testing.T) {
	cases := []struct {
		tmpl string
		v    Values
		want string
	}{
		{"{Movie Title}{ (Release Year)}", Values{MovieTitle: "Untitled"}, "Untitled"},
		{"{Movie Title}{ - Edition}", Values{MovieTitle: "Iron Harbor", Edition: "Director's Cut"}, "Iron Harbor - Director's Cut"},
		{"{Movie Title}{ - Edition}{ {IMDb Id}}", Values{MovieTitle: "X"}, ""}, // nested braces are refused
		{"{Movie Title} {imdb-{IMDb Id}}", Values{MovieTitle: "X", IMDbID: "tt1"}, ""},
		{"{Movie Title}{ [imdbid-IMDb Id]}", Values{MovieTitle: "X", IMDbID: "tt1"}, "X [imdbid-tt1]"},
		{"{movie title} ({release year})", Values{MovieTitle: "Case", ReleaseYear: 2000}, "Case (2000)"},
	}
	for _, c := range cases {
		got, _ := Preview(c.tmpl, c.v)
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.tmpl, got, c.want)
		}
	}
}

func TestPreview_TemplateErrorsAreWarnings(t *testing.T) {
	for _, tmpl := range []string{"", "  ", "{Movie Title", "Movie Title}", "{Director}"} {
		got, warns := Preview(tmpl, movie)
		if got != "" || !reflect.DeepEqual(codes(warns), []string{"template"}) {
			t.Errorf("%q: got %q %v", tmpl, got, warns)
		}
	}
}

func TestColonModes(t *testing.T) {
	v := Values{MovieTitle: "Mission: Part Two at 12:30"}
	cases := map[ColonMode]string{
		ColonDelete:    "Mission Part Two at 1230",
		ColonDash:      "Mission- Part Two at 12-30",
		ColonSpaceDash: "Mission - Part Two at 12 - 30",
		ColonSmart:     "Mission - Part Two at 1230",
		"":             "Mission - Part Two at 1230", // zero value is smart
	}
	for mode, want := range cases {
		if got, _ := (Options{Colon: mode}).Preview("{Movie Title}", v); got != want {
			t.Errorf("%q: got %q, want %q", mode, got, want)
		}
	}
}

func TestIllegalCharacters(t *testing.T) {
	v := Values{MovieTitle: `AC/DC: "Live" <at> Wembley? *1|2* \x` + "\x01\x7f"}
	got, _ := Preview("{Movie Title}", v)
	if got != `AC-DC - 'Live' at Wembley -1-2- -x` {
		t.Errorf("replace: %q", got)
	}
	got, _ = Options{Illegal: IllegalRemove}.Preview("{Movie Title}", v)
	if got != `ACDC - Live at Wembley 12 x` {
		t.Errorf("remove: %q", got)
	}
}

func TestTrimsDotsAndSpaces(t *testing.T) {
	got, _ := Preview("{Movie Title}", Values{MovieTitle: " ...And Justice   for All... "})
	if got != "And Justice for All" {
		t.Fatalf("got %q", got)
	}
}

func TestEmptyAndReservedSegments(t *testing.T) {
	got, warns := Preview("{Movie Title}/{Release Group}", Values{MovieTitle: "Con"})
	if got != "Con_/_" || !reflect.DeepEqual(codes(warns), []string{"reserved_name", "empty_segment"}) {
		t.Fatalf("got %q %v", got, codes(warns))
	}
	for _, name := range []string{"nul", "COM1", "lpt9.x", "aux"} {
		if !isReserved(name) {
			t.Errorf("%s should be reserved", name)
		}
	}
	for _, name := range []string{"com", "com0", "console", "lptx"} {
		if isReserved(name) {
			t.Errorf("%s should not be reserved", name)
		}
	}
}

func TestSegmentCap_TruncatesLongestTokenAndKeepsExtension(t *testing.T) {
	v := movie
	v.MovieTitle = strings.Repeat("Ä", 200) // 400 bytes
	got, warns := Options{Extension: ".mkv"}.Preview("{Movie Title}{ (Release Year)}{ - Release Group}", v)
	if len(got) > MaxSegmentBytes || !strings.HasSuffix(got, " (2019) - SPARKS.mkv") || !utf8.ValidString(got) {
		t.Fatalf("got %d bytes %q", len(got), got)
	}
	if !reflect.DeepEqual(codes(warns), []string{"truncated"}) {
		t.Fatalf("warnings %v", warns)
	}
}

func TestSegmentCap_LiteralOnlyFallsBackToHardCut(t *testing.T) {
	long := strings.Repeat("x", 300)
	got, warns := Options{Extension: "srt"}.Preview(long, Values{})
	if len(got) != MaxSegmentBytes || !strings.HasSuffix(got, "x.srt") || len(warns) != 1 {
		t.Fatalf("got %d bytes, %v", len(got), warns)
	}
}

func TestTruncBytes(t *testing.T) {
	if truncBytes("héllo", 2) != "h" || truncBytes("abc", 0) != "" || truncBytes("abc", 5) != "abc" {
		t.Fatal("truncBytes")
	}
}
