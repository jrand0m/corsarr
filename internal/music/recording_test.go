package music

import (
	"context"
	"errors"
	"testing"

	"github.com/jrand0m/corsarr/internal/plugin"
)

func titled(r plugin.ReleaseRecord, titles ...string) plugin.ReleaseRecord {
	for i, t := range titles {
		r.Media[0].Tracks[i].Title = t
	}
	return r
}

func TestFindRecordingProbesAlbumsFirstAndMatchesLoosely(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.meta.rec.Albums = append(f.meta.rec.Albums, plugin.AlbumRecord{MBID: "s1", Title: "Teardrop", PrimaryType: "Single"},
		plugin.AlbumRecord{MBID: "o1", Title: "Odd", PrimaryType: "Broadcast"})
	f.meta.releases["g1"] = []plugin.ReleaseRecord{titled(rel("r1", "official", "CD", date(2020, 1), 2), "Angel", "Tear-drop!")}
	f.meta.releases["s1"] = []plugin.ReleaseRecord{titled(rel("r2", "official", "CD", nil, 1), "Teardrop")}
	m, err := f.svc.FindRecording(ctx, "a1", "teardrop")
	if err != nil || m.Album.MBID != "g1" || m.Track.Position != 2 {
		t.Fatalf("the studio album wins: %+v %v", m, err)
	}
	if _, err := f.svc.FindRecording(ctx, "a1", "Missing Song"); !errors.Is(err, ErrNoRecording) {
		t.Fatal(err)
	}
	if _, err := f.svc.FindRecording(ctx, "zz", "x"); !errors.Is(err, plugin.ErrNotFound) {
		t.Fatal(err)
	}
	f.meta.err = errors.New("down")
	if _, err := f.svc.Discography(ctx, "a1"); err == nil {
		t.Fatal("provider errors surface")
	}
}

func TestFindRecordingStopsOnReleaseErrors(t *testing.T) {
	f := newFixture(t)
	f.meta.releases = map[string][]plugin.ReleaseRecord{}
	f.meta.rec.Albums = f.meta.rec.Albums[:1]
	flaky := &flakyMeta{fakeMeta: f.meta}
	f.svc.d.Metadata = func() (plugin.MusicMetadata, bool) { return flaky, true }
	if _, err := f.svc.FindRecording(context.Background(), "a1", "x"); err == nil || err.Error() != "releases down" {
		t.Fatal(err)
	}
	f.svc.d.Metadata = func() (plugin.MusicMetadata, bool) { return nil, false }
	if _, err := f.svc.FindRecording(context.Background(), "a1", "x"); !errors.Is(err, ErrNoMetadata) {
		t.Fatal(err)
	}
}

type flakyMeta struct{ *fakeMeta }

func (f *flakyMeta) Releases(context.Context, string) ([]plugin.ReleaseRecord, error) {
	return nil, errors.New("releases down")
}

func TestProbeOrder(t *testing.T) {
	albums := []plugin.AlbumRecord{
		{MBID: "live", PrimaryType: "Album", SecondaryTypes: []string{"Live"}},
		{MBID: "single", PrimaryType: "Single"},
		{MBID: "late", PrimaryType: "Album", FirstRelease: date(2022, 1)},
		{MBID: "early", PrimaryType: "Album", FirstRelease: date(2001, 1)},
		{MBID: "other", PrimaryType: "Broadcast"},
		{MBID: "undated", PrimaryType: "Album"},
	}
	got := ""
	for _, a := range probeOrder(albums, 5) {
		got += a.MBID + " "
	}
	if got != "early late undated single other " {
		t.Fatal(got)
	}
}
