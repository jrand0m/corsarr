package importer

import (
	"context"
	"encoding/binary"
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
	"github.com/jrand0m/corsarr/internal/rootfolders"
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

// twoEditionsMeta adds a one-disc promotional edition with a bonus track
// next to the official two-disc one.
type twoEditionsMeta struct{ albumMeta }

func (twoEditionsMeta) Releases(ctx context.Context, mbid string) ([]plugin.ReleaseRecord, error) {
	rels, _ := albumMeta{}.Releases(ctx, mbid)
	return append(rels, plugin.ReleaseRecord{MBID: "r2", Status: "promotion", Media: []plugin.MediumRecord{
		{Position: 1, Format: "CD", Tracks: []plugin.TrackRecord{{Position: 1, Title: "Opening"}, {Position: 2, Title: "Middle"},
			{Position: 3, Title: "Closing"}, {Position: 4, Title: "Bonus"}}}}}), nil
}

func musicEnv(t *testing.T, title string) (*env, *music.Service, domain.Album) {
	t.Helper()
	return musicEnvWith(t, title, albumMeta{})
}

func musicEnvWith(t *testing.T, title string, md plugin.MusicMetadata) (*env, *music.Service, domain.Album) {
	t.Helper()
	e := newEnv(t)
	root := filepath.Join(filepath.Dir(e.lib), "music")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := music.New(music.Deps{Repo: e.st.Music(), Files: e.st.Library(), Pub: e.rec,
		Metadata:    func() (plugin.MusicMetadata, bool) { return md, true },
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
	if o.Status != domain.ImportImported || len(o.Items) != 1 || o.Items[0].Item != al.Ref() || len(o.Items[0].NewPaths) != 3 ||
		o.Items[0].Outcome != "imported" {
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
	if f.Quality != "flac" || f.Item.Kind != domain.KindMusic || f.Item.ID != tracks[0].ID || f.Revision != 1 {
		t.Fatalf("%+v", f)
	}
	ev := e.rec.Of(domain.EvImportCompleted)
	if len(ev) != 1 || ev[0].Kind != domain.KindMusic || ev[0].Data.(domain.ImportEventData).Folders[0] != albumDir ||
		ev[0].Items[0].Title != "Example Artist - First Album" {
		t.Fatalf("%+v", ev)
	}
	// re-running the same download ends in the same state
	e.q.d.State = domain.StateImporting
	if err := e.svc.Import(ctx, 7); err != nil || e.q.outcomes[1].Status != domain.ImportImported || len(e.q.outcomes[1].Items[0].NewPaths) != 3 {
		t.Fatalf("idempotent re-run: %v %+v", err, e.q.outcomes[1])
	}
	e.q.outcomes = e.q.outcomes[:1]
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

// taggedFLAC is a FLAC stream header followed by a Vorbis comment block
// holding the given KEY=value entries.
func taggedFLAC(entries ...string) string {
	head := flacHeader(16)
	head[4] = 0 // STREAMINFO is no longer the last block
	body := binary.LittleEndian.AppendUint32(nil, 0)
	body = binary.LittleEndian.AppendUint32(body, uint32(len(entries)))
	for _, e := range entries {
		body = binary.LittleEndian.AppendUint32(body, uint32(len(e)))
		body = append(body, e...)
	}
	block := append([]byte{0x80 | 4, byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body))}, body...)
	return string(append(head, block...))
}

func TestMusicImportReadsTrackTagsWhenNamesSayNothing(t *testing.T) {
	e, svc, al := musicEnv(t, "Example Artist - First Album (2020) [FLAC]")
	e.payload(t, map[string]string{
		"a.flac": taggedFLAC("TITLE=Closing", "TRACKNUMBER=1", "DISCNUMBER=2"),
		"b.flac": taggedFLAC("TITLE=Opening", "TRACKNUMBER=1", "DISCNUMBER=1"),
		"c.flac": taggedFLAC("TITLE=Middle", "TRACKNUMBER=2", "DISCNUMBER=1"),
	})
	e.completed(t, 7)
	if o := e.q.outcomes[0]; o.Status != domain.ImportImported || len(o.Items[0].NewPaths) != 3 {
		t.Fatalf("%+v", o)
	}
	tracks, _ := svc.Tracks(ctx, al.ID)
	for _, tr := range tracks {
		f, _, _ := e.st.Library().File(ctx, tr.FileID)
		if !strings.HasSuffix(f.Path, tr.Title+".flac") {
			t.Fatalf("track %q got %s", tr.Title, f.Path)
		}
	}
	src, _ := os.Stat(filepath.Join(e.dl, e.q.d.Title, "a.flac"))
	dst, err := os.Stat(filepath.Join(filepath.Dir(e.lib), "music", "Example Artist", "First Album (2020)", "CD 02",
		"Example Artist - First Album - 01 - Closing.flac"))
	if err != nil || !os.SameFile(src, dst) {
		t.Fatalf("tagged disc and track decide the place: %v", err)
	}
}

func TestMusicImportHoldsLooseMatchesForReview(t *testing.T) {
	e, _, _ := musicEnv(t, "Example Artist - First Album (2020) [FLAC]")
	// Every file finds a track, but the titles are misspelt and the
	// numbers lack their discs: too uncertain to guess.
	e.payload(t, map[string]string{
		"x.flac": taggedFLAC("TITLE=Opining", "TRACKNUMBER=1"),
		"y.flac": taggedFLAC("TITLE=Midday", "TRACKNUMBER=2"),
		"z.flac": taggedFLAC("TITLE=Closinq", "TRACKNUMBER=1"),
	})
	e.completed(t, 7)
	if o := e.q.outcomes[0]; o.Status != domain.ImportBlocked || o.ReasonCode != "no-match" ||
		!strings.Contains(o.Items[0].Reason, "too loosely") {
		t.Fatalf("%+v", o)
	}
}

func TestMusicImportSwitchesToTheEditionThePayloadFits(t *testing.T) {
	e, svc, al := musicEnvWith(t, "Example Artist - First Album (2020) [FLAC]", twoEditionsMeta{})
	e.payload(t, map[string]string{"01 - Opening.flac": "a", "02 - Middle.flac": "b", "03 - Closing.flac": "c", "04 - Bonus.flac": "d"})
	e.completed(t, 7)
	o := e.q.outcomes[0]
	if o.Status != domain.ImportImported || len(o.Items[0].NewPaths) != 4 || !strings.Contains(o.Items[0].Reason, "edition changed") {
		t.Fatalf("%+v", o)
	}
	if got, _ := svc.Album(ctx, al.ID); got.EditionMBID != "r2" || got.TrackCount != 4 {
		t.Fatalf("%+v", got)
	}
	if ev := e.rec.Of(domain.EvItemUpdated); len(ev) != 1 || ev[0].Data.(domain.ItemEventData).Changed[0] != "edition" {
		t.Fatalf("%+v", ev)
	}
	want := filepath.Join(filepath.Dir(e.lib), "music", "Example Artist", "First Album (2020)", "Example Artist - First Album - 04 - Bonus.flac")
	if _, err := os.Stat(want); err != nil {
		t.Fatal(err)
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

// singleMeta serves the album as one edition of a single track.
type singleMeta struct{ albumMeta }

func (singleMeta) Releases(context.Context, string) ([]plugin.ReleaseRecord, error) {
	return []plugin.ReleaseRecord{{MBID: "r1", Status: "official", Media: []plugin.MediumRecord{
		{Position: 1, Format: "CD", Tracks: []plugin.TrackRecord{{Position: 1, Title: "Opening"}}}}}}, nil
}

// noEditionMeta knows the album but none of its editions.
type noEditionMeta struct{ albumMeta }

func (noEditionMeta) Releases(context.Context, string) ([]plugin.ReleaseRecord, error) {
	return nil, nil
}

func TestMusicImportOfAOneTrackAlbum(t *testing.T) {
	e, _, _ := musicEnvWith(t, "Example Artist - First Album (2020) [FLAC]", singleMeta{})
	e.payload(t, map[string]string{"01 Opening.flac": "a"})
	e.completed(t, 7)
	if o := e.q.outcomes[0]; o.Status != domain.ImportImported || len(o.Items[0].NewPaths) != 1 {
		t.Fatalf("%+v", o)
	}
}

func TestMusicImportNeedsTheTrackList(t *testing.T) {
	e, _, _ := musicEnvWith(t, "Example Artist - First Album (2020) [FLAC]", noEditionMeta{})
	e.payload(t, map[string]string{"01 Opening.flac": "a"})
	e.completed(t, 7)
	if o := e.q.outcomes[0]; o.Status != domain.ImportBlocked || !strings.Contains(o.Items[0].Reason, "the track list of First Album is not known yet") {
		t.Fatalf("%+v", o)
	}
	e.q.d.Links = []domain.DownloadLink{{Item: domain.ItemRef{Kind: domain.KindMusic, ID: 999}, Status: domain.LinkPending}}
	e.completed(t, 8)
	if o := e.q.outcomes[1]; o.Status != domain.ImportBlocked {
		t.Fatalf("unknown album: %+v", o)
	}
}

func TestMusicQualityComesFromTheGrabbedTitle(t *testing.T) {
	e, _, _ := musicEnv(t, "Example Artist - First Album (2020) [MP3 320kbps]")
	e.q.d.Title = "plain rip"
	e.payload(t, map[string]string{"01 Opening.flac": "a"})
	e.completed(t, 7)
	if o := e.q.outcomes[0]; o.Status != domain.ImportImported || o.Items[0].Quality != "MP3-320" {
		t.Fatalf("grabbed title: %+v", o)
	}
	e2, _, _ := musicEnv(t, "Example Artist - First Album (2020) [MP3 320kbps]")
	e2.q.title = ""
	e2.payload(t, map[string]string{"01 Opening.flac": "a"})
	e2.completed(t, 7)
	if o := e2.q.outcomes[0]; o.Status != domain.ImportImported || o.Items[0].Quality != "MP3-320" {
		t.Fatalf("no grab: the download's title: %+v", o)
	}
}

func TestMusicImportStopsWithoutRoomOrOnAConflict(t *testing.T) {
	e, _, _ := musicEnv(t, "Example Artist - First Album (2020) [FLAC]")
	var short []string
	e.svc.d.NoSpace = func(root string) { short = append(short, root) }
	free := e.svc.d.FreeSpace
	e.svc.d.FreeSpace = func(string) int64 { return rootfolders.Reserve - 1 }
	e.payload(t, map[string]string{"01 Opening.flac": "a"})
	e.completed(t, 7)
	root := filepath.Join(filepath.Dir(e.lib), "music")
	if o := e.q.outcomes[0]; o.Status != domain.ImportBlocked || o.ReasonCode != "no-space" || len(short) != 1 || short[0] != root {
		t.Fatalf("%+v %v", o, short)
	}
	e.svc.d.FreeSpace = free
	write(t, filepath.Join(root, "Example Artist", "First Album (2020)", "CD 01", "Example Artist - First Album - 01 - Opening.flac"), "foreign")
	e.completed(t, 8)
	if o := e.q.outcomes[1]; o.Status != domain.ImportBlocked || o.ReasonCode != "conflict" {
		t.Fatalf("%+v", o)
	}
}
