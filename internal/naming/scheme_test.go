package naming

import "testing"

func TestDefaultSchemesAreTheBuiltInTemplates(t *testing.T) {
	v := Values{MovieTitle: "Heat", ReleaseYear: 1995, QualityFull: "Bluray-1080p"}
	m := DefaultMovie()
	if got := m.FolderName(v, DefaultMovieFolder); got != "Heat (1995)" {
		t.Fatal(got)
	}
	if got := m.FileName(v, DefaultMovieFile, ".mkv", "/dl/heat.mkv"); got != "Heat (1995) [Bluray-1080p].mkv" {
		t.Fatal(got)
	}
	for _, d := range []Scheme{DefaultTV(), DefaultMovie(), DefaultMusic()} {
		if !d.Rename || d.Colon != ColonSmart {
			t.Errorf("built-in naming renames with smart colons: %+v", d)
		}
	}
	if DefaultTV().Sub != DefaultSeasonFolder || DefaultMusic().Sub != DefaultAlbumFolder || DefaultMovie().Sub != "" {
		t.Fatal("defaults")
	}
}

func TestSchemeRenderingFollowsItsSettings(t *testing.T) {
	s := Scheme{Rename: true, Colon: ColonDelete, Folder: "{Movie Title}", File: "{Movie Title} - {Quality Full}"}
	v := Values{MovieTitle: "Alien: Romulus", QualityFull: "WEBDL-1080p"}
	if got := s.FolderName(v, DefaultMovieFolder); got != "Alien Romulus" {
		t.Fatal(got)
	}
	if got := s.FileName(v, DefaultMovieFile, "mkv", "x.mkv"); got != "Alien Romulus - WEBDL-1080p.mkv" {
		t.Fatal(got)
	}
	s.Rename = false
	if got := s.FileName(v, DefaultMovieFile, ".mkv", "/downloads/Alien.Romulus.2024.mkv"); got != "Alien.Romulus.2024.mkv" {
		t.Fatal(got)
	}
	bad := Scheme{Rename: true, Folder: "{Nope}"}
	if got := bad.FolderName(v, DefaultMovieFolder); got != "Alien - Romulus" {
		t.Fatal(got)
	}
	if got := (Scheme{}).Or(DefaultMusic()); got != (Scheme{Colon: ColonSmart, Folder: DefaultArtistFolder, Sub: DefaultAlbumFolder, File: DefaultTrackFile}) {
		t.Fatalf("%+v", got)
	}
	own := Scheme{Colon: ColonDash, Folder: "{Artist Name}", Sub: "{Album Title}", File: "{Track Title}"}
	if got := own.Or(DefaultMusic()); got != own {
		t.Fatalf("%+v", got)
	}
}
