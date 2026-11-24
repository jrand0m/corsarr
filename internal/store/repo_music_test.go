package store

import (
	"context"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func TestMusicArtistsAlbumsAndTracks(t *testing.T) {
	ctx := context.Background()
	r := open(t, t.TempDir()).Music()
	day := time.Date(2020, 3, 1, 0, 0, 0, 0, time.UTC)
	art := domain.Artist{MBID: "a1", Name: "Example Artist", SortName: "Artist, Example", RootFolder: "/m", Path: "/m/Example Artist",
		ProfileKey: "lossless", MetadataProfile: "standard", MonitorNew: domain.MonitorNewItemsNew, Monitored: true,
		Tags: []string{"x"}, AddedAt: day}
	albums := []domain.Album{{MBID: "g1", Title: "First", PrimaryType: domain.AlbumTypeAlbum, ReleaseDate: &day, Monitored: true},
		{MBID: "g2", Title: "Live", PrimaryType: domain.AlbumTypeAlbum, SecondaryTypes: []string{"live"}}}
	got, created, err := r.AddArtist(ctx, art, albums)
	must(t, err)
	if !created || got.ID == 0 || got.MonitorNew != domain.MonitorNewItemsNew || got.Tags[0] != "x" {
		t.Fatalf("%+v %v", got, created)
	}
	if _, again, err := r.AddArtist(ctx, art, albums); err != nil || again {
		t.Fatal("adding twice must not create")
	}
	list, err := r.Albums(ctx, got.ID)
	must(t, err)
	if len(list) != 2 || list[0].Title != "First" || list[1].ReleaseDate != nil || list[1].SecondaryTypes[0] != "live" {
		t.Fatalf("%+v", list)
	}
	first := list[0]
	first.EditionMBID, first.MediumCount, first.TrackCount, first.DurationSec = "r1", 1, 2, 400
	must(t, r.SetEdition(ctx, first, []domain.Track{{Medium: 1, Number: 1, Title: "One", DurationSec: 200},
		{Medium: 1, Number: 2, Title: "Two", DurationSec: 200}}))
	tracks, err := r.Tracks(ctx, first.ID)
	must(t, err)
	must(t, r.SetTrackFile(ctx, tracks[0].ID, 7))
	// a new edition keeps the track with a file and drops the other
	must(t, r.SetEdition(ctx, first, []domain.Track{{Medium: 1, Number: 1, Title: "One (remaster)"}, {Medium: 2, Number: 1, Title: "Bonus"}}))
	tracks, err = r.Tracks(ctx, first.ID)
	must(t, err)
	if len(tracks) != 2 || tracks[0].FileID != 7 || tracks[0].Title != "One (remaster)" || tracks[1].Medium != 2 {
		t.Fatalf("%+v", tracks)
	}
	a, ok, err := r.Album(ctx, first.ID)
	if err != nil || !ok || a.EditionMBID != "r1" || a.TrackCount != 2 {
		t.Fatalf("%+v %v %v", a, ok, err)
	}
	n, err := r.SetAlbumsMonitored(ctx, []int64{list[1].ID, list[1].ID}, true, domain.PinOwner)
	if err != nil || n != 1 {
		t.Fatalf("monitor %d %v", n, err)
	}
	var pending []int64
	must(t, r.PendingAvailability(ctx, func(a domain.Album) error { pending = append(pending, a.ID); return nil }))
	if len(pending) != 2 {
		t.Fatalf("pending %v", pending)
	}
	must(t, r.MarkAvailable(ctx, pending))
	pending = nil
	must(t, r.PendingAvailability(ctx, func(a domain.Album) error { pending = append(pending, a.ID); return nil }))
	if len(pending) != 0 {
		t.Fatal("published albums stay published")
	}
	got.Name = "Renamed"
	must(t, r.SyncArtist(ctx, got, []domain.Album{{MBID: "g3", Title: "New"}}))
	must(t, r.MarkRefreshed(ctx, got.ID, day))
	if at, err := r.RefreshedAt(ctx, got.ID); err != nil || !at.Equal(day) {
		t.Fatal("refreshed at")
	}
	byMB, ok, _ := r.ArtistByMBID(ctx, "a1")
	page, next, _ := r.ListArtists(ctx, 0, 1)
	if !ok || byMB.Name != "Renamed" || len(page) != 1 || next != got.ID {
		t.Fatalf("%+v %v %d", byMB, page, next)
	}
	count := 0
	must(t, r.EachArtist(ctx, func(domain.Artist) error { count++; return nil }))
	must(t, r.DeleteArtist(ctx, got.ID))
	if _, ok, _ := r.Artist(ctx, got.ID); ok || count != 1 {
		t.Fatal("delete")
	}
	if left, _ := r.Tracks(ctx, first.ID); len(left) != 0 {
		t.Fatal("tracks cascade")
	}
}
