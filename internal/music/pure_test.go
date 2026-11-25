package music

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
	"github.com/jrand0m/corsarr/internal/plugin"
)

var now = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func date(y, m int) *time.Time { t := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC); return &t }

func album(mbid string, typ domain.AlbumType, d *time.Time, sec ...string) domain.Album {
	return domain.Album{MBID: mbid, Title: mbid, PrimaryType: typ, ReleaseDate: d, SecondaryTypes: sec}
}

func monitored(albums []domain.Album) []string {
	var out []string
	for _, a := range albums {
		if a.Monitored {
			out = append(out, a.MBID)
		}
	}
	return out
}

func TestApplyMonitorFollowsTheMetadataProfile(t *testing.T) {
	std, _ := domain.MetadataProfileByKey("standard")
	albums := []domain.Album{album("first", domain.AlbumTypeAlbum, date(2001, 1)), album("live", domain.AlbumTypeAlbum, date(2005, 1), "Live"),
		album("single", domain.AlbumTypeSingle, date(2010, 1)), album("latest", domain.AlbumTypeAlbum, date(2020, 1)),
		album("soon", domain.AlbumTypeAlbum, date(2027, 1)), album("undated", domain.AlbumTypeAlbum, nil)}
	cases := map[domain.AlbumMonitor][]string{
		domain.AlbumMonitorAll:      {"first", "latest", "soon", "undated"},
		domain.AlbumMonitorMissing:  {"first", "latest", "soon", "undated"},
		domain.AlbumMonitorFuture:   {"soon"},
		domain.AlbumMonitorFirst:    {"first"},
		domain.AlbumMonitorLatest:   {"latest"},
		domain.AlbumMonitorExisting: nil,
		domain.AlbumMonitorNone:     nil,
	}
	for mode, want := range cases {
		if got := monitored(ApplyMonitor(mode, std, albums, now)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v want %v", mode, got, want)
		}
	}
	none, _ := domain.MetadataProfileByKey("none")
	if got := monitored(ApplyMonitor(domain.AlbumMonitorAll, none, albums, now)); got != nil {
		t.Errorf("profile none monitors nothing: %v", got)
	}
	if got := monitored(ApplyMonitor(domain.AlbumMonitorLatest, std, albums[1:3], now)); got != nil {
		t.Errorf("no released album in profile: %v", got)
	}
}

func TestMergeAlbumsKeepsMonitoringAndAppliesMonitorNew(t *testing.T) {
	std, _ := domain.MetadataProfileByKey("standard")
	ar := domain.Artist{ID: 1, AddedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), MonitorNew: domain.MonitorNewItemsNew}
	stored := []domain.Album{{ID: 5, ArtistID: 1, MBID: "kept", Title: "Old", Monitored: true, PinReason: domain.PinOwner}}
	recs := []plugin.AlbumRecord{{MBID: "kept", Title: "Renamed", PrimaryType: "Album"},
		{MBID: "newer", Title: "New", PrimaryType: "Album", FirstRelease: date(2025, 1)},
		{MBID: "older", Title: "Back catalogue", PrimaryType: "Album", FirstRelease: date(2019, 1)},
		{MBID: "newsingle", Title: "Single", PrimaryType: "Single", FirstRelease: date(2025, 2)},
		{MBID: "newlive", Title: "Live", PrimaryType: "Album", SecondaryTypes: []string{"Live"}, FirstRelease: date(2025, 3)}}
	m := MergeAlbums(ar, std, stored, recs)
	if m.Added != 4 || m.Albums[0].ID != 5 || m.Albums[0].Title != "Renamed" || !m.Albums[0].Pinned() {
		t.Fatalf("%+v", m)
	}
	if got := monitored(m.Albums); !reflect.DeepEqual(got, []string{"kept", "newer"}) {
		t.Fatalf("monitored %v", got)
	}
	if m.Albums[4].SecondaryTypes[0] != "live" {
		t.Fatal("secondary types are lower case")
	}
	ar.MonitorNew = domain.MonitorNewItemsAll
	if got := monitored(MergeAlbums(ar, std, nil, recs).Albums); len(got) != 3 {
		t.Fatalf("all: %v", got)
	}
	ar.MonitorNew = domain.MonitorNewItemsNone
	if got := monitored(MergeAlbums(ar, std, nil, recs).Albums); got != nil {
		t.Fatalf("none: %v", got)
	}
}

func rel(mbid, status, format string, d *time.Time, tracks ...int) plugin.ReleaseRecord {
	r := plugin.ReleaseRecord{MBID: mbid, Status: status, Date: d}
	for i, n := range tracks {
		m := plugin.MediumRecord{Position: i + 1, Format: format}
		for j := range n {
			m.Tracks = append(m.Tracks, plugin.TrackRecord{Position: j + 1, Title: "T", LengthSec: 60})
		}
		r.Media = append(r.Media, m)
	}
	return r
}

func TestChooseEditionPrefersOfficialDigitalCommonEarliest(t *testing.T) {
	if _, ok := ChooseEdition(nil); ok {
		t.Fatal("no edition")
	}
	rels := []plugin.ReleaseRecord{
		rel("bootleg", "bootleg", "Digital Media", date(2000, 1), 10),
		rel("vinyl", "official", "12\" Vinyl", date(2000, 1), 10),
		rel("deluxe", "official", "CD", date(2001, 1), 10, 8),
		rel("cd-late", "official", "CD", date(2003, 1), 10),
		rel("cd", "official", "CD", date(2002, 1), 10),
		rel("cd-undated", "official", "CD", nil, 10),
	}
	got, _ := ChooseEdition(rels)
	if got.MBID != "cd" {
		t.Fatalf("picked %s", got.MBID)
	}
	al, tracks := Edition(domain.Album{ID: 3}, rels[2])
	if al.EditionMBID != "deluxe" || al.MediumCount != 2 || al.TrackCount != 18 || al.DurationSec != 18*60 || tracks[10].Medium != 2 {
		t.Fatalf("%+v", al)
	}
	if Minutes(al) != 18 || Minutes(domain.Album{}) != DefaultAlbumMinutes {
		t.Fatal("minutes")
	}
}

func TestPathsUseArtistAlbumYearAndDiscFolders(t *testing.T) {
	ar := domain.Artist{Name: "Example Artist", Path: "/music/Example Artist"}
	if ArtistFolder(ar) != "Example Artist" {
		t.Fatal(ArtistFolder(ar))
	}
	al := domain.Album{Title: "First: Album", ReleaseDate: date(2020, 3), MediumCount: 1}
	tr := domain.Track{Medium: 1, Number: 3, Title: "Third Song"}
	want := filepath.Join("/music/Example Artist", "First - Album (2020)", "Example Artist - First - Album - 03 - Third Song.flac")
	if got := TrackPath(ar, al, tr, ".flac"); got != want {
		t.Fatalf("%s want %s", got, want)
	}
	al.MediumCount = 2
	tr.Medium, tr.MediumFormat = 2, "CD"
	if got := filepath.Base(filepath.Dir(TrackPath(ar, al, tr, ".flac"))); got != "CD 02" {
		t.Fatalf("medium folder %q", got)
	}
	tr.MediumFormat = "Digital Media"
	if got := MediumFolder(al, tr); got != "Disc 02" {
		t.Fatalf("digital medium folder %q", got)
	}
	tr.Medium = 12
	if got := MediumFolder(al, tr); got != "Disc 12" {
		t.Fatal(got)
	}
	if AlbumPath(ar, al) != filepath.Join(ar.Path, "First - Album (2020)") {
		t.Fatal("album path")
	}
}

func tracks(spec ...[3]any) []domain.Track {
	out := make([]domain.Track, len(spec))
	for i, s := range spec {
		out[i] = domain.Track{ID: int64(i + 1), Medium: s[0].(int), Number: s[1].(int), Title: s[2].(string)}
	}
	return out
}

func TestMatchFilesByNumberDiscAndTitle(t *testing.T) {
	album := tracks([3]any{1, 1, "Opening"}, [3]any{1, 2, "Middle"}, [3]any{2, 1, "Return"}, [3]any{2, 2, "Closing Time"})
	files := []string{"Album/CD1/01 - Opening.flac", "Album/CD1/02 - Middle.flac", "Album/CD2/01 - Return.flac",
		"Album/CD2/Closing Time.flac", "Album/bonus - remix.flac"}
	m := MatchFiles(Named(files...), album)
	if len(m.Placements) != 4 || m.Unmatched[0] != "Album/bonus - remix.flac" || len(m.Missing) != 0 {
		t.Fatalf("%+v", m)
	}
	if m.Placements[2].Track.Medium != 2 || m.Placements[3].By != "title" || m.Auto() {
		t.Fatalf("%+v auto=%v", m.Placements, m.Auto())
	}
	// disc-track prefixes, and "Artist - Album - 03 - Title" names
	m = MatchFiles(Named("x/1-01 a.flac", "x/2-02 b.flac", "x/Artist - Album - 02 - Middle.mp3"), album)
	if len(m.Placements) != 3 || m.Placements[1].Track.ID != 4 || m.Placements[2].Track.ID != 2 || !m.Auto() {
		t.Fatalf("%+v", m.Placements)
	}
	// the same number on two discs without a disc marker falls to the title
	m = MatchFiles(Named("01 Return.flac"), album)
	if len(m.Placements) != 1 || m.Placements[0].Track.ID != 3 || len(m.Missing) != 3 {
		t.Fatalf("%+v", m)
	}
	if m := MatchFiles(nil, album); m.Score != 0 || m.Auto() {
		t.Fatal("empty payload")
	}
	if keyOf("x/101 Opening.flac").number != 1 {
		t.Fatal("three-digit numbers drop the disc digit")
	}
}

func TestDecideAddsMusicRules(t *testing.T) {
	p, _ := ProfileByKey("lossless")
	ar := domain.Artist{ID: 1, Name: "Example Artist", Monitored: true}
	al := domain.Album{ID: 2, Title: "First Album", ReleaseDate: date(2020, 1), Monitored: true, TrackCount: 10, DurationSec: 2400}
	tg := Target{Artist: ar, Album: al, Released: true, Context: decide.Context{Now: now}}
	d := func(title string, files ...string) domain.Decision {
		return Decide(tg, p, domain.Release{ID: title, Title: title, Protocol: domain.ProtoTorrent, Size: 250e6, Seeders: 9, FileList: files})
	}
	if got := d("Example Artist - First Album (2020) [FLAC]"); !got.Accepted || got.ItemRefs[0] != al.Ref() {
		t.Fatalf("%+v", got)
	}
	cases := map[string]domain.Decision{
		"music.mismatch":    d("Other Artist - First Album (2020) [FLAC]"),
		"music.discography": d("Example Artist - First Album Discography 2001-2020 [FLAC]"),
		"payload.no_audio":  d("Example Artist - First Album (2020) [FLAC]", "cover.jpg"),
		"music.track_count": d("Example Artist - First Album (2020) [FLAC]", "01.flac", "02.flac"),
	}
	for rule, got := range cases {
		if got.Accepted || !hasRule(got, rule) {
			t.Errorf("%s: %+v", rule, got)
		}
	}
	if got := cases["payload.no_audio"]; got.Override != domain.OverrideNever {
		t.Fatal("a payload without audio can never be grabbed")
	}
	if !Matches(ar, al, parse.ParseMusic("Example Artist - First Album")) || Matches(ar, al, parse.ParseMusic("Example Artist - First Album (2015)")) {
		t.Fatal("year tolerance")
	}
	req := AlbumRequest(ar, al, domain.OriginAutomatic)
	if req.Mode != domain.SearchMusic || req.IDs["artist"] != "Example Artist" || req.Year != 2020 || req.Text != "Example Artist First Album" {
		t.Fatalf("%+v", req)
	}
	if IndexKey("The Band!", "Album") != "theband|album" || !IsAudio("x.FLAC") || IsAudio("x.jpg") {
		t.Fatal("helpers")
	}
}

func hasRule(d domain.Decision, id string) bool {
	for _, r := range d.Reasons {
		if r.RuleID == id {
			return true
		}
	}
	return false
}

func TestStatusAndCurrentQuality(t *testing.T) {
	al := domain.Album{TrackCount: 2, Monitored: true, ReleaseDate: date(2020, 1)}
	if Status(al, 2, now) != StatusDownloaded || Status(al, 1, now) != StatusPartial || Status(al, 0, now) != StatusMissing {
		t.Fatal("status")
	}
	if Status(domain.Album{ReleaseDate: date(2030, 1), Monitored: true}, 0, now) != StatusNotReleased ||
		Status(domain.Album{}, 0, now) != StatusUnmonitored {
		t.Fatal("status 2")
	}
	p, _ := ProfileByKey("any")
	cur := Current(p, []domain.LibraryFile{{Quality: "flac"}, {Quality: "mp3-320"}, {Quality: "flac-24"}})
	if cur == nil || cur.Quality != "mp3-320" || Current(p, nil) != nil {
		t.Fatalf("%+v", cur)
	}
	if !belowCutoff(p, *cur) || belowCutoff(p, decide.CurrentFile{Quality: "flac"}) {
		t.Fatal("cutoff")
	}
	if qualityName("flac-24") != "FLAC 24bit" || qualityName("nope") != "nope" {
		t.Fatal("names")
	}
}

func TestApplyPatchValidates(t *testing.T) {
	ar := domain.Artist{ProfileKey: "standard", MetadataProfile: "standard", MonitorNew: domain.MonitorNewItemsAll, Tags: []string{}}
	on, any, single, newOnly, tags := false, "any", "standardsingles", "new", []string{"a"}
	changed, err := ApplyPatch(&ar, Patch{Monitored: &on, ProfileKey: &any, MetadataProfile: &single, MonitorNew: &newOnly, Tags: &tags})
	if err != nil || len(changed) != 4 || ar.ProfileKey != "any" || ar.MonitorNew != domain.MonitorNewItemsNew {
		t.Fatalf("%v %v %+v", changed, err, ar)
	}
	bad := "x"
	for _, p := range []Patch{{ProfileKey: &bad}, {MetadataProfile: &bad}, {MonitorNew: &bad}} {
		if _, err := ApplyPatch(&ar, p); err == nil {
			t.Errorf("accepted %+v", p)
		}
	}
}
