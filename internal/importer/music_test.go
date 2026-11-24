package importer

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/music"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// albumMeta serves one artist with one two-disc album of 2 + 1 tracks.
type albumMeta struct{}

func (albumMeta) Test(context.Context) error { return nil }
func (albumMeta) SearchArtists(context.Context, string) ([]plugin.ArtistRecord, error) {
	return nil, nil
}
func (albumMeta) Artist(context.Context, string) (plugin.ArtistRecord, error) {
	d := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	return plugin.ArtistRecord{MBID: "a1", Name: "Example Artist", Albums: []plugin.AlbumRecord{
		{MBID: "g1", Title: "First Album", PrimaryType: "Album", FirstRelease: &d}}}, nil
}
func (albumMeta) Releases(context.Context, string) ([]plugin.ReleaseRecord, error) {
	return []plugin.ReleaseRecord{{MBID: "r1", Status: "official", Media: []plugin.MediumRecord{
		{Position: 1, Format: "CD", Tracks: []plugin.TrackRecord{{Position: 1, Title: "Opening"}, {Position: 2, Title: "Middle"}}},
		{Position: 2, Format: "CD", Tracks: []plugin.TrackRecord{{Position: 1, Title: "Closing"}}}}}}, nil
}

func musicEnv(t *testing.T, title string) (*env, *music.Service, domain.Album) {
	t.Helper()
	e := newEnv(t)
	root := filepath.Join(filepath.Dir(e.lib), "music")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := music.New(music.Deps{Repo: e.st.Music(), Files: e.st.Library(), Pub: e.rec,
		Metadata:    func() (plugin.MusicMetadata, bool) { return albumMeta{}, true },
		RootFolders: func() []string { return []string{root} }, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	ar, _, err := svc.Add(ctx, music.AddRequest{MBID: "a1", ProfileKey: "any"})
	if err != nil {
		t.Fatal(err)
	}
	svc.Stop()
	albums, _ := svc.Albums(ctx, ar.ID)
	e.svc.d.Music = svc
	e.q.title = title
	e.q.d.Kind, e.q.d.Title = domain.KindMusic, title
	e.q.d.Links = []domain.DownloadLink{{Item: albums[0].Ref(), Status: domain.LinkPending}}
	return e, svc, albums[0]
}

func TestMusicAlbumImportsTracksIntoDiscFolders(t *testing.T) {
	title := "Example Artist - First Album (2020) [FLAC]"
	e, svc, al := musicEnv(t, title)
	e.payload(t, map[string]string{
		"CD1/01 - Opening.flac": strings.Repeat("a", 900), "CD1/02 - Middle.flac": strings.Repeat("b", 900),
		"CD2/01 - Closing.flac": strings.Repeat("c", 900), "cover.jpg": "img",
	})
	e.completed(t, 7)
	o := e.q.outcomes[0]
	if o.Status != domain.ImportImported || len(o.Items) != 1 || o.Items[0].Item != al.Ref() || len(o.Items[0].NewPaths) != 3 {
		t.Fatalf("%+v", o)
	}
	albumDir := filepath.Join(filepath.Dir(e.lib), "music", "Example Artist", "First Album (2020)")
	want := filepath.Join(albumDir, "CD 02", "Example Artist - First Album - 01 - Closing.flac")
	src, _ := os.Stat(filepath.Join(e.dl, title, "CD2", "01 - Closing.flac"))
	dst, err := os.Stat(want)
	if err != nil || !os.SameFile(src, dst) {
		t.Fatalf("hardlink %s: %v", want, err)
	}
	tracks, _ := svc.Tracks(ctx, al.ID)
	for _, tr := range tracks {
		if tr.FileID == 0 {
			t.Fatalf("track %d has no file", tr.Number)
		}
	}
	f, _, _ := e.st.Library().File(ctx, tracks[0].FileID)
	if f.Quality != "flac" || f.Item.Kind != domain.KindMusic || f.Item.ID != tracks[0].ID {
		t.Fatalf("%+v", f)
	}
	ev := e.rec.Of(domain.EvImportCompleted)
	if len(ev) != 1 || ev[0].Kind != domain.KindMusic || ev[0].Data.(domain.ImportEventData).Folders[0] != albumDir ||
		ev[0].Items[0].Title != "Example Artist - First Album" {
		t.Fatalf("%+v", ev)
	}
	// the same quality again is no upgrade
	e.q.d.ID, e.q.d.State = 8, domain.StateCompleted
	e.svc.OnDownloadCompleted(ctx, domain.Event{Type: domain.EvDownloadCompleted, Data: domain.DownloadEventData{DownloadID: 8}})
	e.svc.Wait()
	if o := e.q.outcomes[1]; o.Status != domain.ImportBlocked || o.ReasonCode != "not-upgrade" {
		t.Fatalf("%+v", o)
	}
}

func TestMusicImportUpgradesAndReportsMissingTracks(t *testing.T) {
	e, svc, al := musicEnv(t, "Example Artist - First Album (2020) [MP3 320kbps]")
	e.payload(t, map[string]string{"1-01 Opening.mp3": "a", "1-02 Middle.mp3": "b"})
	e.completed(t, 7)
	if o := e.q.outcomes[0]; o.Status != domain.ImportImported || o.Items[0].Reason != "1 tracks missing from the payload" ||
		o.Items[0].Quality != "MP3-320" {
		t.Fatalf("%+v", o)
	}
	e.q.title, e.q.d.Title = "Example Artist - First Album (2020)", "flac rip"
	e.payload(t, map[string]string{"1-01 Opening.flac": "aa"})
	e.completed(t, 9)
	if o := e.q.outcomes[1]; o.Status != domain.ImportImported || o.Items[0].Outcome != "upgraded" || len(o.Items[0].ReplacedPaths) != 1 {
		t.Fatalf("flac from the file extension upgrades mp3: %+v", o)
	}
	tracks, _ := svc.Tracks(ctx, al.ID)
	f, _, _ := e.st.Library().File(ctx, tracks[0].FileID)
	if f.Quality != "flac" || filepath.Ext(f.Path) != ".flac" {
		t.Fatalf("%+v", f)
	}
}

func TestMusicImportBlocksPoorMatchesAndUnsafePayloads(t *testing.T) {
	e, _, _ := musicEnv(t, "Example Artist - First Album (2020) [FLAC]")
	e.payload(t, map[string]string{"random.flac": "a", "other.flac": "b", "01 Opening.flac": "c"})
	e.completed(t, 7)
	if o := e.q.outcomes[0]; o.Status != domain.ImportBlocked || o.ReasonCode != "no-match" ||
		!strings.Contains(o.Items[0].Reason, "1 of 3 audio files") {
		t.Fatalf("%+v", o)
	}
	e.q.d.Title = "with exe"
	e.payload(t, map[string]string{"01 Opening.flac": "c", "setup.exe": "x"})
	e.completed(t, 8)
	if o := e.q.outcomes[1]; o.Status != domain.ImportFailed || o.ReasonCode != "payload-unsafe" {
		t.Fatalf("%+v", o)
	}
	e.q.d.Links = nil
	e.completed(t, 9)
	if o := e.q.outcomes[2]; o.Status != domain.ImportBlocked {
		t.Fatalf("no linked album: %+v", o)
	}
	e.svc.d.Music = nil
	e.completed(t, 10)
	if o := e.q.outcomes[3]; o.Status != domain.ImportBlocked {
		t.Fatalf("music not wired: %+v", o)
	}
}

func TestDeletingAnArtistOrSeriesRefRecyclesNothingOfItsOwn(t *testing.T) {
	if ownsFiles(domain.ItemRef{Kind: domain.KindMusic, ID: 1, Variant: domain.VariantArtist}) ||
		ownsFiles(domain.ItemRef{Kind: domain.KindTV, ID: 1, Variant: domain.VariantSeries}) ||
		!ownsFiles(domain.ItemRef{Kind: domain.KindMusic, ID: 1, Variant: domain.VariantTrack}) {
		t.Fatal("only items that hold files own them")
	}
	if formatOfExt("x.FLAC") != "flac" || formatOfExt("x.txt") != "" {
		t.Fatal("formats")
	}
}
