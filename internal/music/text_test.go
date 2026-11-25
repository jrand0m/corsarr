package music

import (
	"testing"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
)

func TestQueryTextCleansForSearchEngines(t *testing.T) {
	cases := map[string]string{
		"Example Artist":            "Example Artist",
		"Björk":                     "Bjork",
		"Sigur Rós":                 "Sigur Ros",
		"Motörhead – Ace of Spades": "Motorhead Ace of Spades",
		"Don’t Stop Me Now":         "Dont Stop Me Now",
		"Don't `Stop`":              "Dont Stop",
		"The Beatles":               "Beatles",
		"the the":                   "the",
		"The":                       "The",
		"Theatre of Tragedy":        "Theatre of Tragedy",
		"AC/DC":                     "AC DC",
		"Simon & Garfunkel":         "Simon Garfunkel",
		"Ørjan Ærø Straße Łódź":     "Orjan Aero Strasse Lodz",
		"  ...  ":                   "",
	}
	for in, want := range cases {
		if got := QueryText(in); got != want {
			t.Errorf("QueryText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVariousArtistsSearchAsVA(t *testing.T) {
	byID := domain.Artist{MBID: VariousArtistsMBID, Name: "Some Label Name"}
	byName := domain.Artist{Name: "Various Artists"}
	for _, ar := range []domain.Artist{byID, byName} {
		if !IsVarious(ar) {
			t.Fatalf("%+v is various artists", ar)
		}
		req := AlbumRequest(ar, domain.Album{Title: "The Best Hits"}, domain.OriginAutomatic)
		if req.Text != "VA Best Hits" || req.IDs["artist"] != "VA" || req.IDs["album"] != "Best Hits" {
			t.Fatalf("%+v", req)
		}
	}
	if IsVarious(domain.Artist{Name: "Various"}) || IsVarious(domain.Artist{Name: "Example Artist"}) {
		t.Fatal("ordinary artists")
	}
}

func TestAlbumRequestCleansTextAndKeepsTheMBID(t *testing.T) {
	ar := domain.Artist{Name: "Björk"}
	al := domain.Album{Title: "Homogenic: Live!", MBID: "rg"}
	req := AlbumRequest(ar, al, domain.OriginAutomatic)
	if req.Text != "Bjork Homogenic Live" || req.IDs["artist"] != "Bjork" || req.IDs["album"] != "Homogenic Live" || req.IDs["mbid"] != "rg" {
		t.Fatalf("%+v", req)
	}
	if req.Titles[0] != "Homogenic: Live!" {
		t.Fatal("the stored title stays as it is")
	}
}

func TestSameNameToleratesSmallDifferences(t *testing.T) {
	same := [][2]string{
		{"Björk", "Bjork"},
		{"The Beatles", "Beatles"},
		{"Simon & Garfunkel", "Simon and Garfunkel"},
		{"Will.I.Am", "will i am"},
		{"Sgt. Pepper's Lonely Hearts Club Band", "Sgt Peppers Lonely Heart Club Band"},
		{"Kid A", "kid a"},
	}
	for _, c := range same {
		if !SameName(c[0], c[1]) {
			t.Errorf("%q ~ %q", c[0], c[1])
		}
	}
	differ := [][2]string{
		{"Greatest Hits Vol. 1", "Greatest Hits Vol. 2"},
		{"Ten", "Tan"},
		{"First Album", "Second Album"},
		{"Example Artist", ""},
		{"", ""},
		{"Album 1999", "Album"},
	}
	for _, c := range differ {
		if SameName(c[0], c[1]) {
			t.Errorf("%q !~ %q", c[0], c[1])
		}
	}
}

func TestSimilarityIsNormalisedEditDistance(t *testing.T) {
	cases := []struct {
		a, b string
		want float64
	}{{"", "", 1}, {"abc", "abc", 1}, {"abc", "", 0}, {"kitten", "sitting", 1 - 3.0/7}, {"abcd", "abdc", 0.5}}
	for _, c := range cases {
		if got := similarity(c.a, c.b); got != c.want {
			t.Errorf("similarity(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestDecideAcceptsNearMatchesAndVariousArtists(t *testing.T) {
	p, _ := ProfileByKey("lossless")
	ar := domain.Artist{ID: 1, Name: "Mötley Crüe", Monitored: true}
	al := domain.Album{ID: 2, Title: "The Dirt Soundtrack", ReleaseDate: date(2019, 1), Monitored: true}
	d := func(ar domain.Artist, al domain.Album, title string) domain.Decision {
		tg := Target{Artist: ar, Album: al, Released: true, Context: decide.Context{Now: now}}
		return Decide(tg, p, domain.Release{ID: title, Title: title, Protocol: domain.ProtoTorrent, Size: 250e6, Seeders: 9})
	}
	if got := d(ar, al, "Motley Crue - Dirt Soundtrak (2019) [FLAC]"); !got.Accepted {
		t.Fatalf("near match: %+v", got.Reasons)
	}
	if got := d(ar, al, "Motley Crue - Dr. Feelgood (2019) [FLAC]"); got.Accepted || !hasRule(got, "music.mismatch") {
		t.Fatalf("another album: %+v", got)
	}
	va := domain.Artist{ID: 3, MBID: VariousArtistsMBID, Name: "Various Artists", Monitored: true}
	comp := domain.Album{ID: 4, Title: "Summer Hits", ReleaseDate: date(2019, 1), Monitored: true}
	for _, title := range []string{"VA - Summer Hits (2019) [FLAC]", "Various Artists - Summer Hits (2019) [FLAC]"} {
		if got := d(va, comp, title); !got.Accepted {
			t.Fatalf("%s: %+v", title, got.Reasons)
		}
	}
	if !Matches(va, comp, parse.ParseMusic("Some Label - Summer Hits")) {
		t.Fatal("the artist is not compared for various-artists albums")
	}
}
