package music

import (
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

func medium(pos int, format string, titles ...string) plugin.MediumRecord {
	m := plugin.MediumRecord{Position: pos, Format: format}
	for i, t := range titles {
		m.Tracks = append(m.Tracks, plugin.TrackRecord{Position: i + 1, Title: t, LengthSec: 60})
	}
	return m
}

func TestEditionDropsVideoMediaAndDataTracks(t *testing.T) {
	r := plugin.ReleaseRecord{MBID: "cd+dvd", Media: []plugin.MediumRecord{
		medium(1, "CD", "One", "Two", "[Data Track]"),
		medium(2, "DVD-Video", "Clip One", "Clip Two"),
		medium(3, "Blu-ray", "Concert"),
		medium(4, "DVD-Audio", "Surround One"),
		medium(5, "CD", " [data track] "),
	}}
	al, tracks := Edition(domain.Album{ID: 1}, r)
	if al.TrackCount != 3 || al.MediumCount != 2 || al.DurationSec != 180 || len(tracks) != 3 {
		t.Fatalf("%+v %+v", al, tracks)
	}
	if tracks[2].Medium != 4 || tracks[2].Title != "Surround One" {
		t.Fatalf("media keep their positions: %+v", tracks[2])
	}
	for _, f := range []string{"VHS", "vcd", "SVCD", "LaserDisc", "HD-DVD", "DVD", "Betamax", "UMD"} {
		if !videoMedium(f) {
			t.Errorf("%s is video", f)
		}
	}
	if videoMedium("Enhanced CD") || videoMedium("DualDisc") || videoMedium("Digital Media") {
		t.Fatal("audio formats are kept")
	}
}

func TestChooseEditionCountsOnlyAudio(t *testing.T) {
	cdDVD := plugin.ReleaseRecord{MBID: "cd+dvd", Status: "official", Date: date(2001, 1),
		Media: []plugin.MediumRecord{medium(1, "CD", "A", "B"), medium(2, "DVD-Video", "V1", "V2", "V3")}}
	cd := plugin.ReleaseRecord{MBID: "cd", Status: "official", Date: date(2002, 1), Media: []plugin.MediumRecord{medium(1, "CD", "A", "B")}}
	dvd := plugin.ReleaseRecord{MBID: "dvd", Status: "official", Date: date(2000, 1), Media: []plugin.MediumRecord{medium(1, "DVD", "V1")}}
	got, ok := ChooseEdition([]plugin.ReleaseRecord{dvd, cdDVD, cd})
	if !ok || got.MBID != "cd+dvd" || got.TrackCount() != 2 {
		t.Fatalf("the earliest edition with audio wins with its audio tracks only: %+v", got)
	}
	if got, ok := ChooseEdition([]plugin.ReleaseRecord{dvd}); !ok || got.MBID != "dvd" || got.TrackCount() != 0 {
		t.Fatalf("a video-only group still yields its edition: %+v", got)
	}
}
