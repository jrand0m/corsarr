package music

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/audiotag"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// edition is one CD whose tracks are "Song 1".."Song N" of the given lengths.
func edition(mbid string, lengths ...int) plugin.ReleaseRecord {
	m := plugin.MediumRecord{Position: 1, Format: "CD"}
	for i, l := range lengths {
		m.Tracks = append(m.Tracks, plugin.TrackRecord{Position: i + 1, Title: fmt.Sprintf("Song %d", i+1), LengthSec: l,
			RecordingMBID: fmt.Sprintf("%s-rec%d", mbid, i+1)})
	}
	return plugin.ReleaseRecord{MBID: mbid, Status: "official", Media: []plugin.MediumRecord{m}}
}

// payload is one tagged file per length, named and numbered like Song N.
func payload(lengths ...int) []AudioFile {
	out := make([]AudioFile, len(lengths))
	for i, l := range lengths {
		out[i] = AudioFile{Rel: fmt.Sprintf("%02d Song %d.flac", i+1, i+1),
			Tags: audiotag.Tags{Title: fmt.Sprintf("Song %d", i+1), Track: i + 1, Duration: sec(l)}}
	}
	return out
}

func current(al domain.Album, r plugin.ReleaseRecord) (domain.Album, []domain.Track) {
	al, ts := Edition(al, r)
	for i := range ts {
		ts[i].ID = int64(i + 1)
	}
	return al, ts
}

func TestBetterEditionSwitchesOnTrackCountAndLength(t *testing.T) {
	short, long := edition("r1", 200, 200), edition("r2", 200, 200, 200)
	al, tracks := current(domain.Album{ID: 1}, short)
	rels := []plugin.ReleaseRecord{short, long}
	if got, ok := BetterEdition(payload(200, 200, 200), al, tracks, rels); !ok || got.MBID != "r2" {
		t.Fatalf("a third file fits the longer edition: %v %v", got.MBID, ok)
	}
	if _, ok := BetterEdition(payload(200, 200), al, tracks, rels); ok {
		t.Fatal("the current edition fits")
	}
	// Same track count, different lengths: the remaster runs longer.
	original, remaster := edition("r1", 200, 200, 200), edition("r3", 290, 300, 310)
	_, three := current(domain.Album{ID: 1}, original)
	if got, ok := BetterEdition(payload(291, 299, 309), al, three, []plugin.ReleaseRecord{original, remaster}); !ok || got.MBID != "r3" {
		t.Fatalf("lengths decide: %v %v", got.MBID, ok)
	}
	// A small edge is not enough to switch.
	if _, ok := BetterEdition(payload(200, 215), al, tracks, []plugin.ReleaseRecord{short, edition("r4", 200, 225)}); ok {
		t.Fatal("switched on a small difference")
	}
}

func TestBetterEditionFollowsTaggedReleaseAndSkipsVideo(t *testing.T) {
	a, b := edition("r1", 200, 200), edition("r2", 200, 200)
	al, tracks := current(domain.Album{ID: 1}, a)
	files := payload(200, 200)
	for i := range files {
		files[i].Tags.MusicBrainz.Album = "R2"
	}
	if got, ok := BetterEdition(files, al, tracks, []plugin.ReleaseRecord{a, b}); !ok || got.MBID != "r2" {
		t.Fatalf("the tagged release wins a tie: %v %v", got.MBID, ok)
	}
	video := edition("r5", 200, 200, 200)
	video.Media[0].Format = "DVD-Video"
	if _, ok := BetterEdition(payload(200, 200, 200), al, tracks, []plugin.ReleaseRecord{a, video}); ok {
		t.Fatal("a video edition is never chosen")
	}
}

func TestFitEditionSwitchesStoresAndPublishes(t *testing.T) {
	f := newFixture(t)
	f.meta.releases["g1"] = []plugin.ReleaseRecord{edition("r1", 200, 200), edition("r2", 200, 200, 200)}
	f.add(t)
	ctx := context.Background()
	al := albumNamed(t, f, "First Album")
	if al.EditionMBID != "r1" {
		t.Fatalf("the more common edition starts: %+v", al)
	}
	fit, err := f.svc.FitEdition(ctx, al.ID, payload(200, 200, 200))
	if err != nil || fit.Album.EditionMBID != "r2" || len(fit.Tracks) != 3 || fit.Tracks[2].ID == 0 ||
		!strings.Contains(fit.Switched, "r2") {
		t.Fatalf("%+v %v", fit, err)
	}
	stored, _ := f.svc.Album(ctx, al.ID)
	if stored.EditionMBID != "r2" || stored.TrackCount != 3 {
		t.Fatalf("stored %+v", stored)
	}
	ev := f.pub.of(domain.EvItemUpdated)
	if len(ev) != 1 || ev[0].Data.(domain.ItemEventData).Changed[0] != "edition" || ev[0].Items[0].Ref != al.Ref() {
		t.Fatalf("%+v", ev)
	}
	// Fitting again keeps the edition.
	if fit, err := f.svc.FitEdition(ctx, al.ID, payload(200, 200, 200)); err != nil || fit.Switched != "" {
		t.Fatalf("%+v %v", fit, err)
	}
}

func TestFitEditionKeepsEditionsItMayNotChange(t *testing.T) {
	f := newFixture(t)
	f.meta.releases["g1"] = []plugin.ReleaseRecord{edition("r1", 200, 200), edition("r2", 200, 200, 200)}
	f.add(t)
	ctx := context.Background()
	al := albumNamed(t, f, "First Album")
	files := payload(200, 200, 200)
	// a track already has a file: its edition stays
	tracks, _ := f.svc.Tracks(ctx, al.ID)
	must(t, f.svc.SetTrackFile(ctx, tracks[0].ID, 99))
	if fit, err := f.svc.FitEdition(ctx, al.ID, files); err != nil || fit.Switched != "" || fit.Album.EditionMBID != "r1" {
		t.Fatalf("%+v %v", fit, err)
	}
	must(t, f.svc.SetTrackFile(ctx, tracks[0].ID, 0))
	// an edition chosen by hand stays
	al.EditionLocked = true
	must(t, f.st.Music().SetEdition(ctx, al, tracks))
	if fit, err := f.svc.FitEdition(ctx, al.ID, files); err != nil || fit.Switched != "" {
		t.Fatalf("%+v %v", fit, err)
	}
	al.EditionLocked = false
	must(t, f.st.Music().SetEdition(ctx, al, tracks))
	// without metadata the current edition is used
	f.meta.err = errFake
	if fit, err := f.svc.FitEdition(ctx, al.ID, files); err != nil || fit.Switched != "" || len(fit.Tracks) != 2 {
		t.Fatalf("%+v %v", fit, err)
	}
	if _, err := f.svc.FitEdition(ctx, 999, files); err == nil {
		t.Fatal("unknown album")
	}
}

var errFake = fmt.Errorf("metadata down")

func albumNamed(t *testing.T, f *fixture, title string) domain.Album {
	t.Helper()
	ars, _, err := f.svc.d.Repo.ListArtists(context.Background(), 0, 10)
	must(t, err)
	albums, err := f.svc.Albums(context.Background(), ars[0].ID)
	must(t, err)
	for _, a := range albums {
		if a.Title == title {
			return a
		}
	}
	t.Fatalf("no album %q", title)
	return domain.Album{}
}
