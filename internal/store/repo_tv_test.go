package store

import (
	"context"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func tvFixture() (domain.Series, []domain.Season, []domain.Episode) {
	air := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	s := domain.Series{TMDBID: 77, Title: "Show", Year: 2024, Genres: []string{"Drama"}, Tags: []string{}, RootFolder: "/tv",
		Path: "/tv/Show (2024)", ProfileKey: "hd1080p", Type: domain.SeriesStandard, SeasonFolders: true,
		MonitorNew: domain.MonitorNewAll, Monitored: true, AddedAt: air}
	seasons := []domain.Season{{Number: 0}, {Number: 1, Monitored: true}}
	eps := []domain.Episode{
		{Season: 0, Number: 1, Title: "Special", InMetadata: true},
		{Season: 1, Number: 1, Title: "Pilot", AirAt: &air, Monitored: true, InMetadata: true},
		{Season: 1, Number: 2, Title: "Two", AirAt: ptrTime(air.Add(7 * 24 * time.Hour)), Monitored: true, InMetadata: true},
	}
	return s, seasons, eps
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestTVAddIsIdempotentAndStoresChildren(t *testing.T) {
	st := open(t, t.TempDir())
	ctx := context.Background()
	repo := st.TV()
	s, seasons, eps := tvFixture()
	got, created, err := repo.AddSeries(ctx, s, seasons, eps)
	if err != nil || !created || got.ID == 0 || got.Genres[0] != "Drama" || got.Type != domain.SeriesStandard {
		t.Fatalf("%+v %v %v", got, created, err)
	}
	if again, created, err := repo.AddSeries(ctx, s, seasons, eps); err != nil || created || again.ID != got.ID {
		t.Fatalf("second add: %+v %v %v", again, created, err)
	}
	list, err := repo.Episodes(ctx, got.ID)
	if err != nil || len(list) != 3 || list[0].AirAt != nil || !list[1].AirAt.Equal(*eps[1].AirAt) {
		t.Fatalf("episodes %+v %v", list, err)
	}
	if ss, _ := repo.Seasons(ctx, got.ID); len(ss) != 2 || ss[0].Monitored || !ss[1].Monitored {
		t.Fatalf("seasons %+v", ss)
	}
	if bt, ok, _ := repo.SeriesByTMDB(ctx, 77); !ok || bt.ID != got.ID {
		t.Fatal("by tmdb")
	}
	page, next, _ := repo.ListSeries(ctx, 0, 1)
	if len(page) != 1 || next != got.ID {
		t.Fatalf("page %v next %d", page, next)
	}
}

func TestTVSyncKeepsFileLinksAndNeverDropsEpisodesWithFiles(t *testing.T) {
	st := open(t, t.TempDir())
	ctx := context.Background()
	repo := st.TV()
	s, seasons, eps := tvFixture()
	s, _, _ = repo.AddSeries(ctx, s, seasons, eps)
	list, _ := repo.Episodes(ctx, s.ID)
	if err := repo.SetFile(ctx, []int64{list[1].ID}, 42); err != nil {
		t.Fatal(err)
	}
	list[1].Title, list[1].FileID = "Renamed", 0 // the sync must not clear the link
	s.Title = "Show!"
	if err := repo.Sync(ctx, s, nil, []domain.Episode{list[1]}, []int64{list[1].ID, list[2].ID}); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.Episodes(ctx, s.ID)
	if len(after) != 2 || after[1].Title != "Renamed" || after[1].FileID != 42 {
		t.Fatalf("after sync %+v", after)
	}
	if got, _, _ := repo.Series(ctx, s.ID); got.Title != "Show!" {
		t.Fatalf("series title %q", got.Title)
	}
	if n, err := repo.SetMonitored(ctx, []int64{after[0].ID, after[1].ID}, true); err != nil || n != 1 {
		t.Fatalf("set monitored %d %v", n, err)
	}
	if err := repo.MarkRefreshed(ctx, s.ID, time.UnixMilli(5000)); err != nil {
		t.Fatal(err)
	}
	if at, _ := repo.RefreshedAt(ctx, s.ID); at.UnixMilli() != 5000 {
		t.Fatalf("refreshed at %v", at)
	}
}

func TestTVAvailabilityAndAiring(t *testing.T) {
	st := open(t, t.TempDir())
	ctx := context.Background()
	repo := st.TV()
	s, seasons, eps := tvFixture()
	s, _, _ = repo.AddSeries(ctx, s, seasons, eps)
	var pending []domain.Episode
	_ = repo.PendingAvailability(ctx, func(e domain.Episode) error { pending = append(pending, e); return nil })
	if len(pending) != 2 {
		t.Fatalf("pending %+v", pending)
	}
	_ = repo.MarkAvailable(ctx, []int64{pending[0].ID})
	pending = nil
	_ = repo.PendingAvailability(ctx, func(e domain.Episode) error { pending = append(pending, e); return nil })
	if len(pending) != 1 {
		t.Fatalf("pending after publish %+v", pending)
	}
	var airing []domain.Episode
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = repo.AiringBetween(ctx, from, from.Add(3*24*time.Hour), func(e domain.Episode) error { airing = append(airing, e); return nil })
	if len(airing) != 1 || airing[0].Title != "Pilot" {
		t.Fatalf("airing %+v", airing)
	}
	n := 0
	_ = repo.EachEpisode(ctx, func(domain.Episode) error { n++; return nil })
	_ = repo.EachSeries(ctx, func(domain.Series) error { n++; return nil })
	if n != 4 {
		t.Fatalf("each %d", n)
	}
	if e, ok, _ := repo.Episode(ctx, airing[0].ID); !ok || e.Title != "Pilot" {
		t.Fatal("episode by id")
	}
	if err := repo.DeleteSeries(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	if left, _ := repo.Episodes(ctx, s.ID); len(left) != 0 {
		t.Fatalf("episodes outlived their series: %v", left)
	}
}
