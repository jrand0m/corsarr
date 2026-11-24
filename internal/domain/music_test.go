package domain

import (
	"testing"
	"time"
)

func TestParseMusicInputs(t *testing.T) {
	for in, want := range map[string]MonitorNewItems{"": MonitorNewItemsAll, "new": MonitorNewItemsNew, "none": MonitorNewItemsNone} {
		if got, err := ParseMonitorNewItems(in); err != nil || got != want {
			t.Errorf("%q: %v %v", in, got, err)
		}
	}
	if _, err := ParseMonitorNewItems("x"); err == nil {
		t.Error("bad monitor new items")
	}
	for _, in := range []string{"", "all", "missing", "existing", "future", "first", "latest", "none"} {
		if _, err := ParseAlbumMonitor(in); err != nil {
			t.Errorf("%q: %v", in, err)
		}
	}
	if m, _ := ParseAlbumMonitor(""); m != AlbumMonitorAll {
		t.Error("default monitoring")
	}
	if _, err := ParseAlbumMonitor("x"); err == nil {
		t.Error("bad monitoring")
	}
	if ParseAlbumType("EP") != AlbumTypeEP || ParseAlbumType("Album") != AlbumTypeAlbum || ParseAlbumType("Mixtape") != AlbumTypeOther {
		t.Error("album types")
	}
}

func TestMetadataProfilesGovernTypes(t *testing.T) {
	std, ok := MetadataProfileByKey("standard")
	if !ok {
		t.Fatal("standard")
	}
	if _, ok := MetadataProfileByKey("x"); ok {
		t.Fatal("unknown profile")
	}
	if !std.Allows(AlbumTypeAlbum, nil) || std.Allows(AlbumTypeAlbum, []string{"Live"}) || std.Allows(AlbumTypeSingle, nil) {
		t.Fatal("standard covers studio albums only")
	}
	singles, _ := MetadataProfileByKey("standardsingles")
	if !singles.Allows(AlbumTypeSingle, nil) || !singles.Allows(AlbumTypeEP, []string{}) {
		t.Fatal("singles profile")
	}
}

func TestMusicShapes(t *testing.T) {
	d := time.Date(2020, 5, 1, 0, 0, 0, 0, time.UTC)
	al := Album{ID: 3, Title: "First", ReleaseDate: &d, PinReason: PinImport}
	if al.Year() != 2020 || (Album{}).Year() != 0 || !al.Pinned() || (Album{}).Pinned() {
		t.Fatal("album")
	}
	if it := al.EventItem("Artist"); it.Title != "Artist - First" || it.Ref != (ItemRef{Kind: KindMusic, ID: 3}) {
		t.Fatalf("%+v", it)
	}
	if al.EventItem("").Title != "First" {
		t.Fatal("no artist")
	}
	ar := Artist{ID: 2, Name: "Artist", Tags: []string{"x"}}
	if it := ar.EventItem(); it.Ref.Variant != VariantArtist || it.Tags[0] != "x" {
		t.Fatalf("%+v", it)
	}
	if (Track{ID: 4}).Ref() != (ItemRef{Kind: KindMusic, ID: 4, Variant: VariantTrack}) {
		t.Fatal("track")
	}
}
