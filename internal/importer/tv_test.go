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
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/tv"
)

type showMeta struct{}

func (showMeta) Test(context.Context) error { return nil }
func (showMeta) SearchSeries(context.Context, string) ([]plugin.SeriesRecord, error) {
	return nil, nil
}
func (showMeta) Series(context.Context, map[string]string) (plugin.SeriesRecord, error) {
	air := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	r := plugin.SeriesRecord{IDs: map[string]string{"tmdb": "77"}, Title: "Example Show", Year: 2022, RuntimeMin: 30}
	for n := 1; n <= 3; n++ {
		r.Episodes = append(r.Episodes, plugin.EpisodeRecord{Season: 1, Number: n, Title: "Ep", AirAt: &air})
	}
	return r, nil
}

// tvEnv adds a three-episode series and links the download to all three.
func tvEnv(t *testing.T, title string) (*env, *tv.Service, []domain.Episode) {
	t.Helper()
	e := newEnv(t)
	root := filepath.Join(filepath.Dir(e.lib), "tv")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := tv.New(tv.Deps{Repo: e.st.TV(), Files: e.st.Library(), Pub: e.rec,
		Metadata:    func() (plugin.SeriesMetadata, bool) { return showMeta{}, true },
		RootFolders: func() []string { return []string{root} }, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	s, _, err := svc.Add(ctx, tv.AddRequest{TMDBID: 77, ProfileKey: "any"})
	if err != nil {
		t.Fatal(err)
	}
	eps, _ := svc.Episodes(ctx, s.ID)
	e.svc.d.TV = svc
	e.q.title = title
	e.q.d.Kind, e.q.d.Title, e.q.d.Links = domain.KindTV, title, nil
	for _, ep := range eps {
		e.q.d.Links = append(e.q.d.Links, domain.DownloadLink{Item: ep.Ref(), Status: domain.LinkPending})
	}
	return e, svc, eps
}

func TestTVSeasonPackImportsEachEpisodeAndSkipsBetterOnes(t *testing.T) {
	title := "Example.Show.S01.720p.HDTV.x264-GRP"
	e, svc, eps := tvEnv(t, title)
	// E03 already has a better file
	better := write(t, filepath.Join(e.lib, "old", "e3.mkv"), "old")
	f, _ := e.st.Library().SaveFile(ctx, domain.LibraryFile{Item: eps[2].Ref(), Path: better, Quality: "bluray-1080p", Revision: 1})
	_ = svc.SetEpisodeFile(ctx, []int64{eps[2].ID}, f.ID)
	e.payload(t, map[string]string{
		"Example.Show.S01E01.720p.HDTV.x264-GRP.mkv": strings.Repeat("a", 900),
		"Example.Show.S01E02.720p.HDTV.x264-GRP.mkv": strings.Repeat("b", 900),
		"Example.Show.S01E03.720p.HDTV.x264-GRP.mkv": strings.Repeat("c", 900),
		"Example.Show.S01E02.720p.HDTV.x264-GRP.srt": "sub",
	})
	e.completed(t, 7)
	o := e.q.outcomes[0]
	if o.Status != domain.ImportPartial || len(o.Items) != 3 || o.Items[2].Outcome != "skipped" || o.Items[2].Reason == "" {
		t.Fatalf("%+v", o)
	}
	want := filepath.Join(filepath.Dir(e.lib), "tv", "Example Show (2022)", "Season 01", "Example Show - S01E01 - Ep [HDTV-720p].mkv")
	if o.Items[0].NewPaths[0] != want {
		t.Fatalf("path %q", o.Items[0].NewPaths[0])
	}
	got, _ := svc.Episodes(ctx, eps[0].SeriesID)
	if got[0].FileID == 0 || got[1].FileID == 0 || got[2].FileID != f.ID {
		t.Fatalf("episode links %+v", got)
	}
	ev := e.rec.Of(domain.EvImportCompleted)
	if len(ev) != 1 || ev[0].Kind != domain.KindTV || ev[0].Data.(domain.ImportEventData).Folders[0] != filepath.Dir(want) ||
		domain.ItemsTitle(ev[0].Items) != "Example Show · S01 · 3 episodes" {
		t.Fatalf("one import.completed for the pack: %+v", ev)
	}
	// re-running ends in the same state
	e.q.d.State = domain.StateImporting
	if err := e.svc.Import(ctx, 7); err != nil || e.q.outcomes[1].Status != domain.ImportPartial {
		t.Fatalf("idempotent re-run: %v %+v", err, e.q.outcomes)
	}
}

func TestTVMultiEpisodeFileAndUpgrade(t *testing.T) {
	e, svc, eps := tvEnv(t, "Example.Show.S01E01-E02.1080p.WEB-DL.x264-GRP")
	old := write(t, filepath.Join(e.lib, "old", "e1.mkv"), "old")
	f, _ := e.st.Library().SaveFile(ctx, domain.LibraryFile{Item: eps[0].Ref(), Path: old, Quality: "hdtv-720p", Revision: 1})
	_ = svc.SetEpisodeFile(ctx, []int64{eps[0].ID}, f.ID)
	e.q.d.Links = e.q.d.Links[:2]
	e.payload(t, map[string]string{"Example.Show.S01E01-E02.1080p.WEB-DL.x264-GRP.mkv": strings.Repeat("a", 900)})
	e.completed(t, 8)
	o := e.q.outcomes[0]
	if o.Status != domain.ImportImported || o.Items[0].Outcome != "upgraded" || o.Items[0].ReplacedPaths[0] != old ||
		!strings.HasSuffix(o.Items[1].NewPaths[0], "Example Show - S01E01-E02 [WEBDL-1080p].mkv") {
		t.Fatalf("%+v", o)
	}
	got, _ := svc.Episodes(ctx, eps[0].SeriesID)
	linked, ok, _ := e.st.Library().File(ctx, got[0].FileID)
	if got[0].FileID != got[1].FileID || !ok || linked.Path != o.Items[0].NewPaths[0] {
		t.Fatalf("both episodes link the new file: %+v %+v", got, linked)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("the replaced file was recycled")
	}
}

func TestTVLoneFileWithoutMarkerTakesTheGrabsEpisode(t *testing.T) {
	e, _, _ := tvEnv(t, "Example.Show.S01E03.1080p.WEB-DL.x264-GRP")
	e.q.d.Links = e.q.d.Links[2:]
	e.payload(t, map[string]string{"video.mkv": strings.Repeat("a", 900)})
	e.completed(t, 9)
	if o := e.q.outcomes[0]; o.Status != domain.ImportImported || !strings.Contains(o.Items[0].NewPaths[0], "S01E03") {
		t.Fatalf("%+v", o)
	}
}

func TestTVFailures(t *testing.T) {
	e, _, eps := tvEnv(t, "Example.Show.S01E01.1080p.WEB-DL.x264-GRP")
	e.payload(t, map[string]string{"Example.Show.S01E01.1080p.WEB-DL.x264-GRP.mkv": "a", "setup.exe": "x"})
	e.completed(t, 10)
	if o := e.q.outcomes[0]; o.Status != domain.ImportFailed || o.ReasonCode != "payload-unsafe" {
		t.Fatalf("unsafe %+v", o)
	}
	if len(e.rec.Of(domain.EvPayloadRejected)) != 1 {
		t.Fatal("payload.rejected")
	}
	e.q.d.ContentPath = filepath.Join(e.dl, "missing")
	e.completed(t, 11)
	if o := e.q.outcomes[1]; o.ReasonCode != "path-not-found" {
		t.Fatalf("missing payload %+v", o)
	}
	e.payload(t, map[string]string{"Other.Show.S02E09.1080p.WEB-DL.x264-GRP.mkv": "a", "Other.Show.S02E10.1080p.WEB-DL.x264-GRP.mkv": "b"})
	e.q.d.ContentPath = filepath.Join(e.dl, e.q.d.Title)
	_ = os.Remove(filepath.Join(e.q.d.ContentPath, "setup.exe"))
	_ = os.Remove(filepath.Join(e.q.d.ContentPath, "Example.Show.S01E01.1080p.WEB-DL.x264-GRP.mkv"))
	e.completed(t, 12)
	if o := e.q.outcomes[2]; o.Status != domain.ImportBlocked || o.ReasonCode != "no-match" || o.Items[0].Outcome != "skipped" {
		t.Fatalf("no file for the linked episodes %+v", o)
	}
	e.q.d.Links = []domain.DownloadLink{{Item: domain.ItemRef{Kind: domain.KindTV, ID: 999}}}
	e.completed(t, 13)
	if o := e.q.outcomes[3]; o.Status != domain.ImportBlocked || o.ReasonCode != "no-match" {
		t.Fatalf("no linked episode %+v", o)
	}
	e.svc.d.TV = nil
	e.q.d.Links = []domain.DownloadLink{{Item: eps[0].Ref()}}
	e.completed(t, 14)
	if o := e.q.outcomes[4]; o.Status != domain.ImportBlocked {
		t.Fatalf("unwired %+v", o)
	}
}
