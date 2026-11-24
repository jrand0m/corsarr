package store

import (
	"context"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestAdoptSeriesLinksFilesAndConverges(t *testing.T) {
	st := open(t, t.TempDir())
	ctx := context.Background()
	repo := st.Migration()
	s, seasons, eps := tvFixture()
	file := EpisodeFile{File: domain.LibraryFile{Path: "/tv/Show/S01E01.mkv", Size: 10, Quality: "webdl-1080p", Group: "GRP", Revision: 1},
		Episodes: []EpisodeKey{{1, 1}}}
	prov := Provenance{Key: ProvenanceKey{"sonarr", "i1", "series", "7"}, SourceHash: "a", TargetHash: "b", ImportedAt: time.Now()}
	id, err := repo.AdoptSeries(ctx, s, seasons, eps, []EpisodeFile{file}, prov)
	if err != nil || id == 0 {
		t.Fatal(id, err)
	}
	prov.TargetID = id
	s.Monitored = false
	if again, err := repo.AdoptSeries(ctx, s, seasons, eps, []EpisodeFile{file}, prov); err != nil || again != id {
		t.Fatal(again, err)
	}
	got, _, _ := st.TV().Series(ctx, id)
	list, _ := st.TV().Episodes(ctx, id)
	if got.Monitored || list[1].FileID == 0 || list[2].FileID != 0 {
		t.Fatalf("series %+v episodes %+v", got, list)
	}
	ix, err := repo.Index(ctx)
	if err != nil || ix.SeriesByTMDB[s.TMDBID] != id || ix.FilePaths[file.File.Path] != list[1].FileID {
		t.Fatalf("%+v %v", ix, err)
	}
	ids, err := repo.EpisodeIDs(ctx, s.TMDBID)
	if err != nil || ids[EpisodeKey{1, 2}] != list[2].ID {
		t.Fatal(ids, err)
	}
	p, err := repo.Provenance(ctx, "sonarr", "i1")
	if err != nil || len(p) != 1 || p[prov.Key].TargetID != id || p[prov.Key].TargetKind != "tv" {
		t.Fatal(p, err)
	}
}

func TestAdoptMovieWithFile(t *testing.T) {
	st := open(t, t.TempDir())
	ctx := context.Background()
	m := domain.Movie{TMDBID: 5, Title: "Film", Year: 2020, Monitored: true, MinAvailability: domain.AvailReleased,
		ProfileKey: "hd1080p", RootFolder: "/movies", Path: "/movies/Film (2020)", AddedAt: time.Now()}
	f := &domain.LibraryFile{Path: "/movies/Film (2020)/film.mkv", Size: 3, Quality: "bluray-1080p", Revision: 1}
	prov := Provenance{Key: ProvenanceKey{"radarr", "i", "movie", "1"}}
	id, err := st.Migration().AdoptMovie(ctx, m, f, prov)
	if err != nil {
		t.Fatal(err)
	}
	got, _, _ := st.Movies().Get(ctx, id)
	if got.FileID == 0 || got.Title != "Film" {
		t.Fatalf("%+v", got)
	}
	prov.TargetID = id
	m.Monitored = false
	if _, err := st.Migration().AdoptMovie(ctx, m, nil, prov); err != nil {
		t.Fatal(err)
	}
	if got, _, _ = st.Movies().Get(ctx, id); got.Monitored || got.FileID == 0 {
		t.Fatalf("%+v", got)
	}
}

func TestMigrationRunsAndOverrides(t *testing.T) {
	st := open(t, t.TempDir())
	ctx := context.Background()
	repo := st.Migration()
	id, err := repo.StartRun(ctx, "plan", []string{"sonarr"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.FinishRun(ctx, id, map[string]int{"create": 2}, []byte(`{"x":1}`), "", time.Now()); err != nil {
		t.Fatal(err)
	}
	runs, err := repo.Runs(ctx, 5)
	if err != nil || len(runs) != 1 || runs[0].Status != "done" || runs[0].Counts["create"] != 2 || runs[0].Sources[0] != "sonarr" {
		t.Fatalf("%+v %v", runs, err)
	}
	if rep, ok, err := repo.Report(ctx, id); !ok || err != nil || string(rep) != `{"x":1}` {
		t.Fatal(string(rep), ok, err)
	}
	if _, ok, _ := repo.Report(ctx, 99); ok {
		t.Fatal("missing run found")
	}
	ov := st.ModeOverrides()
	_ = ov.Set(ctx, ModeOverride{Scope: ScopeKind, Name: "tv", Mode: "shadow", SetAt: time.Now()})
	_ = ov.Set(ctx, ModeOverride{Scope: ScopeKind, Name: "tv", Mode: "active", SetAt: time.Now()})
	list, err := ov.List(ctx)
	if err != nil || len(list) != 1 || list[0].Mode != "active" {
		t.Fatal(list, err)
	}
}
