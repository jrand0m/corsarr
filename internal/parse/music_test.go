package parse

import "testing"

func TestParseMusicReadsArtistAlbumYearAndAudio(t *testing.T) {
	cases := []struct {
		title, artist, album string
		year                 int
		format               AudioFormat
		bitrate, depth       int
		disco                bool
	}{
		{"Example Artist - First Album (2020) [FLAC]", "Example Artist", "First Album", 2020, FormatFLAC, 0, 0, false},
		{"Example Artist - First Album (2020) [FLAC 24bit]", "Example Artist", "First Album", 2020, FormatFLAC, 0, 24, false},
		{"Example Artist - First Album 2019 MP3 320kbps", "Example Artist", "First Album", 2019, FormatMP3, 320, 0, false},
		{"Example_Artist-First_Album-WEB-2018-GRP", "Example Artist", "First Album", 2018, "", 0, 0, false},
		{"Example Artist - Second Album (2021) [MP3 V0]", "Example Artist", "Second Album", 2021, FormatMP3, 245, 0, false},
		{"Example Artist - Discography 2001-2020 [MP3 320]", "Example Artist", "Discography", 2001, FormatMP3, 320, 0, true},
		{"Example Artist - Live Set (2022) [24-96 FLAC]", "Example Artist", "Live Set", 2022, FormatFLAC, 0, 24, false},
		{"Example Artist - Third (2017) [AAC 256]", "Example Artist", "Third", 2017, FormatAAC, 256, 0, false},
	}
	for _, c := range cases {
		m := ParseMusic(c.title)
		if m.Artist != c.artist || m.Album != c.album {
			t.Errorf("%q: artist %q album %q", c.title, m.Artist, m.Album)
		}
		if m.Year.Value != c.year || m.Format.Value != c.format || m.Bitrate.Value != c.bitrate || m.BitDepth.Value != c.depth {
			t.Errorf("%q: year %v format %v bitrate %v depth %v", c.title, m.Year, m.Format, m.Bitrate, m.BitDepth)
		}
		if m.Discography != c.disco {
			t.Errorf("%q: discography %v", c.title, m.Discography)
		}
	}
}

func TestParseMusicKeepsUnknownsUnknown(t *testing.T) {
	m := ParseMusic("Just A Title")
	if m.Format.Known || m.Bitrate.Known || m.BitDepth.Known || m.Year.Known || m.Artist != "" || m.Album != "Just A Title" {
		t.Fatalf("%+v", m)
	}
	if g := ParseMusic("Example_Artist-First_Album-WEB-2018-GRP").Group; g.Value != "GRP" {
		t.Fatalf("group %v", g)
	}
	if !FormatFLAC.Lossless() || FormatMP3.Lossless() {
		t.Fatal("lossless")
	}
}
