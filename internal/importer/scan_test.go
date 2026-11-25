package importer

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/jrand0m/corsarr/internal/decide"
)

func relsOf(files []File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = filepath.ToSlash(f.Rel)
	}
	slices.Sort(out)
	return out
}

func TestScanSkipsSystemFilesAndPartials(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Album")
	keep := []string{"01 - Track.flac", "cover.jpg", "CD2/02 - Track.flac", "Movie.mkv", "Subs/en.srt", "Movie.sample.mkv"}
	skip := []string{"._01 - Track.flac", ".DS_Store", "Thumbs.db", "desktop.ini", "DESKTOP.INI", "03 - Track.flac.part",
		"04 - Track.flac.!qB", "05.flac.partial~", ".hidden/x.flac", "@eaDir/01 - Track.flac@SynoEAStream",
		".@__thumb/cover.jpg", "CD2/._02 - Track.flac", "Sample/Movie.sample.mkv", "samples/clip.mkv", ".nfs000123"}
	for _, p := range append(slices.Clone(keep), skip...) {
		write(t, filepath.Join(root, p), "x")
	}
	files, err := Scan(root, decide.PayloadPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	want := make([]string, len(keep))
	for i, k := range keep {
		want[i] = "Album/" + k
	}
	slices.Sort(want)
	if got := relsOf(files); !slices.Equal(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestScanOfASingleFileKeepsIt(t *testing.T) {
	p := write(t, filepath.Join(t.TempDir(), "Movie.mkv"), "x")
	files, err := Scan(p, decide.PayloadPolicy{})
	if err != nil || len(files) != 1 || files[0].Rel != "Movie.mkv" {
		t.Fatal(files, err)
	}
}

func TestScanListReadsOnlyTheListedFiles(t *testing.T) {
	base := t.TempDir()
	write(t, filepath.Join(base, "Mine.S01E01.mkv"), "aa")
	write(t, filepath.Join(base, "Mine.S01E02.mkv"), "bb")
	write(t, filepath.Join(base, "Someone.Elses.Movie.mkv"), "cccccc")
	write(t, filepath.Join(base, "._Mine.S01E01.mkv"), "x")
	files, err := ScanList(base, []string{"Mine.S01E01.mkv", "Mine.S01E02.mkv", "._Mine.S01E01.mkv"}, decide.PayloadPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if got := relsOf(files); !slices.Equal(got, []string{"Mine.S01E01.mkv", "Mine.S01E02.mkv"}) {
		t.Fatalf("%v", got)
	}
	if files[0].Class != ClassVideo || files[0].Size != 2 || files[0].Path != filepath.Join(base, files[0].Rel) {
		t.Fatalf("%+v", files[0])
	}
	if _, err := ScanList(base, []string{"missing.mkv"}, decide.PayloadPolicy{}); err == nil {
		t.Fatal("a listed file that is missing is an error")
	}
	if _, err := ScanList(base, []string{"../escape.mkv"}, decide.PayloadPolicy{}); err == nil {
		t.Fatal("a listed path outside the save folder is an error")
	}
}

func TestJunk(t *testing.T) {
	for name, want := range map[string]bool{
		"._x": true, ".DS_Store": true, "Thumbs.db": true, "thumbs.db": true, "x.part": true, "x.!qB": true, "x.partial~": true,
		"@eaDir": true, "#recycle": true, "x.mkv": false, "part.mkv": false, "Sample.Show.mkv": false,
	} {
		if junk(name) != want {
			t.Errorf("junk(%q) = %v", name, !want)
		}
	}
	for name, want := range map[string]bool{"Sample": true, "samples": true, "Sample.Show.S01": false} {
		if junkDir(name) != want {
			t.Errorf("junkDir(%q) = %v", name, !want)
		}
	}
}
