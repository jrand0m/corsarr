package store

import (
	"context"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func musicFixture() (domain.Artist, []AdoptAlbum) {
	a := domain.Artist{MBID: "mb-a", Name: "The Band", SortName: "band, the", RootFolder: "/music", Path: "/music/The Band",
		ProfileKey: "lossless", MetadataProfile: "standard", MonitorNew: domain.MonitorNewItemsNew, Monitored: true,
		Tags: []string{"vinyl"}, AddedAt: time.Now()}
	first := AdoptAlbum{
		Album: domain.Album{MBID: "mb-1", Title: "First", PrimaryType: domain.AlbumTypeAlbum, Monitored: true, EditionMBID: "rel-1",
			MediumCount: 1, TrackCount: 2, DurationSec: 380},
		Tracks: []domain.Track{{RecordingID: "rec-1", Medium: 1, Number: 1, Title: "One", DurationSec: 200},
			{Medium: 1, Number: 2, Title: "Two", DurationSec: 180}},
		Files: []TrackFile{
			{File: domain.LibraryFile{Path: "/music/The Band/First/01.flac", Size: 3, Quality: "flac", Revision: 1},
				Tracks: []TrackKey{{1, 1}}},
			{File: domain.LibraryFile{Path: "/music/The Band/First/09.flac", Size: 3, Quality: "flac", Revision: 1},
				Tracks: []TrackKey{{1, 9}}},
		},
	}
	bare := AdoptAlbum{Album: domain.Album{MBID: "mb-2", Title: "Second", PrimaryType: domain.AlbumTypeSingle}}
	return a, []AdoptAlbum{first, bare}
}

func TestAdoptArtistSeedsEditionsLinksFilesAndConverges(t *testing.T) {
	st := open(t, t.TempDir())
	ctx := context.Background()
	repo := st.Migration()
	a, albums := musicFixture()
	prov := Provenance{Key: ProvenanceKey{"lidarr", "i1", "artist", "3"}, SourceHash: "a", TargetHash: "b", ImportedAt: time.Now()}
	id, err := repo.AdoptArtist(ctx, a, albums, prov)
	if err != nil || id == 0 {
		t.Fatal(id, err)
	}
	got, _, _ := st.Music().Artist(ctx, id)
	if got.ProfileKey != "lossless" || got.MonitorNew != domain.MonitorNewItemsNew || len(got.Tags) != 1 {
		t.Fatalf("%+v", got)
	}
	if at, _ := st.Music().RefreshedAt(ctx, id); at.Unix() != 0 {
		t.Fatalf("artist not due for a refresh: %s", at)
	}
	ids, err := repo.AlbumIDs(ctx, "mb-a")
	if err != nil || len(ids) != 2 {
		t.Fatal(ids, err)
	}
	al, _, _ := st.Music().Album(ctx, ids["mb-1"])
	tracks, _ := st.Music().Tracks(ctx, al.ID)
	if al.EditionMBID != "rel-1" || al.TrackCount != 2 || len(tracks) != 2 || tracks[0].FileID == 0 || tracks[1].FileID != 0 {
		t.Fatalf("album %+v tracks %+v", al, tracks)
	}
	f, _, _ := st.Library().File(ctx, tracks[0].FileID)
	if f.Item.Kind != domain.KindMusic || f.Item.ID != tracks[0].ID {
		t.Fatalf("file %+v", f)
	}

	// A second adopt updates settings, keeps the edition and the links, and
	// leaves a file whose track the edition lacks unlinked.
	prov.TargetID = id
	a.Monitored, a.ProfileKey = false, "standard"
	albums[0].Album.Monitored = false
	albums[0].Tracks = []domain.Track{{Medium: 1, Number: 1, Title: "Other"}}
	if again, err := repo.AdoptArtist(ctx, a, albums, prov); err != nil || again != id {
		t.Fatal(again, err)
	}
	got, _, _ = st.Music().Artist(ctx, id)
	al, _, _ = st.Music().Album(ctx, ids["mb-1"])
	tracks, _ = st.Music().Tracks(ctx, al.ID)
	if got.Monitored || got.ProfileKey != "standard" || al.Monitored || len(tracks) != 2 || tracks[0].Title != "One" {
		t.Fatalf("artist %+v album %+v tracks %+v", got, al, tracks)
	}
	ix, err := repo.Index(ctx)
	if err != nil || ix.ArtistByMBID["mb-a"] != id || len(ix.FilePaths) != 1 {
		t.Fatalf("%+v %v", ix, err)
	}
	p, err := repo.Provenance(ctx, "lidarr", "i1")
	if err != nil || p[prov.Key].TargetKind != "music" || p[prov.Key].TargetID != id {
		t.Fatal(p, err)
	}
}

func TestAdoptArtistFailsWholeOnAnUnknownArtist(t *testing.T) {
	st := open(t, t.TempDir())
	ctx := context.Background()
	a, albums := musicFixture()
	if _, err := st.Migration().AdoptArtist(ctx, a, albums, Provenance{Key: ProvenanceKey{"lidarr", "i", "artist", "1"}, TargetID: 99}); err == nil {
		t.Fatal("adopting onto a missing artist succeeded")
	}
	if ids, _ := st.Migration().AlbumIDs(ctx, "mb-a"); len(ids) != 0 {
		t.Fatal("partial write", ids)
	}
}
