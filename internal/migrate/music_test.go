package migrate

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
)

func allowedType(name string, allowed bool) map[string]any {
	return map[string]any{"albumType": map[string]any{"id": 0, "name": name}, "allowed": allowed}
}

func allowedStatus(name string, allowed bool) map[string]any {
	return map[string]any{"releaseStatus": map[string]any{"id": 0, "name": name}, "allowed": allowed}
}

func quality(id int, name string) map[string]any {
	return map[string]any{"quality": map[string]any{"id": id, "name": name}}
}

func lidarrRoutes(day time.Time) map[string]any {
	band := src("music/The Band")
	return map[string]any{
		"/tag": []any{map[string]any{"id": 1, "label": "vinyl"}},
		"/qualityprofile": []any{
			map[string]any{"id": 1, "name": "Lossless", "cutoff": 6, "items": []any{
				map[string]any{"quality": map[string]any{"id": 6, "name": "FLAC"}, "allowed": true},
				map[string]any{"quality": map[string]any{"id": 1, "name": "MP3-96"}, "allowed": false}}},
			map[string]any{"id": 2, "name": "Everything", "cutoff": 1001, "items": []any{
				map[string]any{"id": 1001, "name": "Lossy", "allowed": true, "items": []any{quality(2, "MP3-96"), quality(3, "MP3-320")}}}},
		},
		"/metadataprofile": []any{
			map[string]any{"id": 1, "name": "Standard",
				"primaryAlbumTypes":   []any{allowedType("Album", true), allowedType("EP", false)},
				"secondaryAlbumTypes": []any{allowedType("Studio", true), allowedType("Live", false)},
				"releaseStatuses":     []any{allowedStatus("Official", true), allowedStatus("Bootleg", false)}},
			map[string]any{"id": 2, "name": "Everything",
				"primaryAlbumTypes":   []any{allowedType("Album", true), allowedType("EP", true), allowedType("Single", true)},
				"secondaryAlbumTypes": []any{allowedType("Studio", true), allowedType("Live", true)},
				"releaseStatuses":     []any{allowedStatus("Official", true), allowedStatus("Bootleg", true)}},
		},
		"/rootfolder":    []any{map[string]any{"id": 1, "path": src("music/")}},
		"/config/naming": map[string]any{"renameTracks": true},
		"/downloadclient": []any{map[string]any{"id": 1, "name": "qbit", "implementation": "QBittorrent", "protocol": "torrent",
			"enable": true, "fields": []any{field("host", "qbit"), field("port", 8080), field("musicCategory", "music")}}},
		"/artist": []any{
			map[string]any{"id": 1, "artistName": "The Band", "foreignArtistId": "mb-artist-1", "sortName": "band, the", "path": band,
				"rootFolderPath": src("music/"), "qualityProfileId": 1, "metadataProfileId": 1, "monitored": true,
				"monitorNewItems": "new", "tags": []int{1}},
			map[string]any{"id": 2, "artistName": "Nobody", "path": src("music/Nobody"), "rootFolderPath": src("music/")},
		},
		"/album?1": []any{
			map[string]any{"id": 10, "title": "First", "foreignAlbumId": "mb-album-10", "albumType": "Album",
				"secondaryTypes": []any{"Studio"}, "releaseDate": day, "monitored": true, "releases": []any{
					map[string]any{"foreignReleaseId": "mb-rel-a", "trackCount": 2, "mediumCount": 1, "monitored": false},
					map[string]any{"foreignReleaseId": "mb-rel-b", "trackCount": 2, "mediumCount": 1, "monitored": true}}},
			map[string]any{"id": 11, "title": "Live Thing", "foreignAlbumId": "mb-album-11", "albumType": "Album",
				"secondaryTypes": []any{map[string]any{"id": 6, "name": "Live"}}, "monitored": true},
			map[string]any{"id": 12, "title": "B-Side", "foreignAlbumId": "mb-album-12", "albumType": "Single", "monitored": false},
		},
		"/track?1": []any{
			map[string]any{"id": 100, "albumId": 10, "foreignRecordingId": "rec-1", "trackFileId": 900, "trackNumber": "1",
				"absoluteTrackNumber": 1, "mediumNumber": 1, "title": "Song One", "duration": 200000},
			map[string]any{"id": 101, "albumId": 10, "trackFileId": 901, "trackNumber": "A2", "mediumNumber": 1, "title": "Song Two",
				"duration": 180400},
		},
		"/trackfile?1": []any{
			map[string]any{"id": 900, "albumId": 10, "path": band + "/First/01 Song One.flac", "size": 33,
				"quality": map[string]any{"quality": map[string]any{"name": "FLAC"}}, "releaseGroup": "GRP", "dateAdded": day},
			map[string]any{"id": 901, "albumId": 10, "path": band + "/First/02 Song Two.m4a", "size": 44,
				"quality": map[string]any{"quality": map[string]any{"name": "AAC-VBR"}}, "dateAdded": day},
			map[string]any{"id": 902, "albumId": 11, "path": band + "/Live Thing/01 Intro.flac", "size": 5,
				"quality": map[string]any{"quality": map[string]any{"name": "FLAC"}}, "dateAdded": day},
		},
		"/queue": page(map[string]any{"artistId": 1, "albumId": 10, "title": "The.Band-First-FLAC", "downloadId": "DDDD",
			"protocol": "torrent", "size": 300}),
		"/blocklist": page(map[string]any{"id": 5, "artistId": 1, "albumIds": []int{11}, "sourceTitle": "The.Band-Live-MP3",
			"date": day, "indexer": "Tracker", "message": "bad rip"}),
		"/history": page(
			map[string]any{"id": 80, "eventType": "grabbed", "date": day, "sourceTitle": "The.Band-First-FLAC", "artistId": 1,
				"albumId": 10, "downloadId": "DDDD"},
			map[string]any{"id": 81, "eventType": "trackFileImported", "date": day, "sourceTitle": "01 Song One", "albumId": 10},
			map[string]any{"id": 82, "eventType": "albumImportIncomplete", "date": day, "sourceTitle": "x", "albumId": 10},
		),
	}
}

type musicRig struct {
	*rig
	lidarr *fakeArr
}

func newMusicRig(t *testing.T) *musicRig {
	r := &musicRig{rig: newRig(t, "dddd")}
	r.lidarr = newFake(t, Lidarr, lidarrRoutes(day))
	r.lib.write(t, "music/The Band/First/01 Song One.flac", 33)
	r.lib.write(t, "music/The Band/First/02 Song Two.m4a", 44)
	r.req.Sources = []Endpoint{r.lidarr.endpoint(t, Lidarr)}
	return r
}

func TestLidarrPlanOutcomesPerObject(t *testing.T) {
	r := newMusicRig(t)
	rep := r.plan()
	r.lidarr.onlyGets(t)
	want := map[string]Outcome{
		"lidarr:artist:1": OutCreate, "lidarr:artist:2": OutUnsupported,
		"lidarr:trackfile:900": OutCreate, "lidarr:trackfile:901": OutApproximate, "lidarr:trackfile:902": OutSkip,
		"lidarr:profile:1": OutUnchanged, "lidarr:profile:2": OutApproximate,
		"lidarr:metadataprofile:1": OutUnchanged, "lidarr:metadataprofile:2": OutApproximate,
		"lidarr:naming:rename": OutUnsupported, "lidarr:downloadclient:1": OutManaged,
		"lidarr:download:dddd": OutCreate, "lidarr:blocklist:5": OutCreate,
		"lidarr:history:80": OutCreate, "lidarr:history:81": OutCreate, "lidarr:history:82": OutSkip,
		"corsarr:mode:music": OutCreate, "corsarr:rules:all": OutCreate,
	}
	for id, out := range want {
		if e := entry(t, rep, id); e.Outcome != out {
			t.Errorf("%s: %s (%s), want %s", id, e.Outcome, e.Reason, out)
		}
	}
	reasons := map[string]string{
		"lidarr:profile:2":         "MP3-96",
		"lidarr:metadataprofile:2": "release status",
		"lidarr:artist:1":          "1 monitored albums outside the metadata profile",
		"lidarr:trackfile:901":     `"AAC-VBR"`,
	}
	for id, part := range reasons {
		if e := entry(t, rep, id); !strings.Contains(e.Reason, part) {
			t.Errorf("%s reason %q lacks %q", id, e.Reason, part)
		}
	}
	if e := entry(t, rep, "lidarr:metadataprofile:2"); e.Target != "metadataprofile:standardsingles" {
		t.Errorf("%+v", e)
	}
	if !strings.Contains(rep.Config, "music: music") || !strings.Contains(rep.Config, "kind: music") {
		t.Errorf("config snippet:\n%s", rep.Config)
	}
}

func TestLidarrApplyAdoptsArtistsAlbumsAndFiles(t *testing.T) {
	r := newMusicRig(t)
	rep := r.apply()
	ctx := context.Background()
	r.lidarr.onlyGets(t)
	if rep.Counts[OutFailed] != 0 {
		t.Fatalf("failures: %+v", rep.Entries)
	}
	ar, ok, _ := r.st.Music().ArtistByMBID(ctx, "mb-artist-1")
	if !ok || ar.Path != r.lib.root+"/music/The Band" || ar.ProfileKey != "lossless" || ar.MetadataProfile != "standard" ||
		ar.MonitorNew != domain.MonitorNewItemsNew || !ar.Monitored || len(ar.Tags) != 1 || ar.SortName != "band, the" {
		t.Fatalf("artist %+v", ar)
	}
	if at, _ := r.st.Music().RefreshedAt(ctx, ar.ID); !at.IsZero() && at.Unix() != 0 {
		t.Fatalf("artist not due for a metadata refresh: %s", at)
	}
	albums, _ := r.st.Music().Albums(ctx, ar.ID)
	byMBID := map[string]domain.Album{}
	for _, al := range albums {
		byMBID[al.MBID] = al
	}
	first, live, single := byMBID["mb-album-10"], byMBID["mb-album-11"], byMBID["mb-album-12"]
	if len(albums) != 3 || !first.Monitored || first.EditionMBID != "mb-rel-b" || first.TrackCount != 2 ||
		!live.Monitored || live.PinReason != domain.PinImport || single.Monitored {
		t.Fatalf("albums %+v", albums)
	}
	tracks, _ := r.st.Music().Tracks(ctx, first.ID)
	if len(tracks) != 2 || tracks[0].FileID == 0 || tracks[1].FileID == 0 || tracks[1].Number != 2 || tracks[0].DurationSec != 200 {
		t.Fatalf("tracks %+v", tracks)
	}
	f, _, _ := r.st.Library().File(ctx, tracks[0].FileID)
	if f.Quality != "flac" || f.Group != "GRP" || f.Item.Kind != domain.KindMusic || f.Item.ID != tracks[0].ID {
		t.Fatalf("file %+v", f)
	}
	if f2, _, _ := r.st.Library().File(ctx, tracks[1].FileID); f2.Quality != string(decide.AudioUnknown) {
		t.Fatalf("file %+v", f2)
	}
	dls, _ := r.st.Queue().Downloads(ctx, domain.StateQueued)
	if len(dls) != 1 || dls[0].Kind != domain.KindMusic || len(dls[0].Links) != 1 || dls[0].Links[0].Item.ID != first.ID {
		t.Fatalf("downloads %+v", dls)
	}
	if ok, _ := r.st.Blocklist().Has(ctx, live.Ref(), domain.ReleaseIdentity{Title: "The.Band-Live-MP3"}); !ok {
		t.Fatal("album blocklist entry missing")
	}
	ov, _ := r.st.ModeOverrides().List(ctx)
	if len(ov) != 2 || ov[0].Name != "music" || ov[0].Mode != "shadow" {
		t.Fatalf("overrides %+v", ov)
	}
	hist, _, _ := r.st.History().List(ctx, 0, 10)
	if len(hist) != 2 {
		t.Fatalf("history %+v", hist)
	}
}

func TestLidarrReapplyConvergesAndOwnerEditsConflict(t *testing.T) {
	r := newMusicRig(t)
	r.apply()
	for _, e := range r.apply().Entries {
		if e.Outcome.writes() && e.op != nil {
			t.Errorf("second run still writes %s: %s (%s)", e.ID, e.Outcome, e.Reason)
		}
	}
	ctx := context.Background()
	ar, _, _ := r.st.Music().ArtistByMBID(ctx, "mb-artist-1")
	albums, _ := r.st.Music().Albums(ctx, ar.ID)
	if _, err := r.st.Music().SetAlbumsMonitored(ctx, []int64{albums[0].ID}, false, domain.PinOwner); err != nil {
		t.Fatal(err)
	}
	r.lidarr.routes["/artist"].([]any)[0].(map[string]any)["monitored"] = false
	if e := entry(t, r.plan(), "lidarr:artist:1"); e.Outcome != OutConflict {
		t.Fatalf("%+v", e)
	}
	r.lidarr.routes["/artist"] = r.lidarr.routes["/artist"].([]any)[1:]
	if e := entry(t, r.plan(), "lidarr:artist:1"); e.Outcome != OutSkip || !strings.Contains(e.Reason, "removed at source") {
		t.Fatalf("%+v", e)
	}
}

func TestLidarrSourceChangeUpdatesTheArtist(t *testing.T) {
	r := newMusicRig(t)
	r.apply()
	r.lidarr.routes["/artist"].([]any)[0].(map[string]any)["qualityProfileId"] = 2
	rep := r.apply()
	if e := entry(t, rep, "lidarr:artist:1"); e.Outcome != OutUpdate || rep.Counts[OutFailed] != 0 {
		t.Fatalf("%+v", e)
	}
	ar, _, _ := r.st.Music().ArtistByMBID(context.Background(), "mb-artist-1")
	if ar.ProfileKey != "standard" {
		t.Fatalf("%+v", ar)
	}
}

func TestLidarrArtistAlreadyInCorsarrIsKept(t *testing.T) {
	r := newMusicRig(t)
	if _, _, err := r.st.Music().AddArtist(context.Background(), domain.Artist{MBID: "mb-artist-1", Name: "The Band",
		RootFolder: "/m", Path: "/m/b", ProfileKey: "any", MetadataProfile: "standard", MonitorNew: domain.MonitorNewItemsAll,
		AddedAt: day}, nil); err != nil {
		t.Fatal(err)
	}
	if e := entry(t, r.plan(), "lidarr:artist:1"); e.Outcome != OutConflict {
		t.Fatalf("%+v", e)
	}
}

func TestLidarrVersionRange(t *testing.T) {
	r := newMusicRig(t)
	r.lidarr.version = "0.8.1.2135"
	_, err := r.svc.Plan(context.Background(), r.req, nil)
	if err == nil || !strings.Contains(err.Error(), "supported 1.x to 3.x") {
		t.Fatal(err)
	}
	for _, v := range []string{"1.0.0", "2.13.3.4711", "3.1.0.4875"} {
		if err := CheckVersion(Lidarr, v); err != nil {
			t.Error(v, err)
		}
	}
	if err := CheckVersion(Lidarr, "4.0.0"); !errors.Is(err, ErrUnsupportedVersion) {
		t.Error(err)
	}
}

func TestLidarrReadsUseTheV1API(t *testing.T) {
	r := newMusicRig(t)
	r.plan()
	for _, p := range r.lidarr.paths {
		if !strings.HasPrefix(p, "/api/v1/") {
			t.Fatalf("request outside /api/v1: %s", p)
		}
	}
	if _, err := NewClient(r.lidarr.endpoint(t, Lidarr), http.DefaultClient).Status(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAudioQualityMapping(t *testing.T) {
	cases := map[string]string{"FLAC": "flac", "FLAC 24bit": "flac-24", "MP3-320": "mp3-320", "mp3-vbr-v0": "mp3-vbr-v0",
		"OGG Vorbis Q8": "vorbis", "MP3-8": "mp3-low", "Unknown": "audio-unknown", "AAC-VBR": "audio-unknown"}
	for in, want := range cases {
		if got, _ := audioQualityID(in); string(got) != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
	if _, exact := audioQualityID("AAC-VBR"); exact {
		t.Error("unknown quality reported exact")
	}
	if got := belowMusicFloor([]string{"MP3-96", "MP3-128", "FLAC", "Unknown"}); strings.Join(got, ",") != "MP3-96,Unknown" {
		t.Error(got)
	}
}

func TestMusicProfileMapping(t *testing.T) {
	if m := mapProfile(SrcProfile{Name: "Lossless", Allowed: []string{"FLAC"}}, domain.KindMusic); !m.Exact || m.Key != "lossless" {
		t.Fatal(m)
	}
	if m := mapProfile(SrcProfile{Name: "Mine", Cutoff: "FLAC 24bit"}, domain.KindMusic); m.Exact || m.Key != "lossless" {
		t.Fatal(m)
	}
	if m := mapProfile(SrcProfile{Name: "Mine", Cutoff: "MP3-320"}, domain.KindMusic); m.Key != "standard" {
		t.Fatal(m)
	}
	if k := profileKey(Snapshot{}, 9, domain.KindMusic); k != "standard" {
		t.Fatal(k)
	}
}

func TestMetadataProfileMapping(t *testing.T) {
	cases := []struct {
		p     SrcMetaProfile
		key   string
		exact bool
	}{
		{SrcMetaProfile{Primary: []string{"album"}, Secondary: []string{"studio"}, Statuses: []string{"official"}}, "standard", true},
		{SrcMetaProfile{Primary: []string{"single", "ep", "album"}, Secondary: []string{"studio"}}, "standardsingles", true},
		{SrcMetaProfile{Primary: []string{}, Secondary: []string{}}, "none", true},
		{SrcMetaProfile{Primary: []string{"album"}, Secondary: []string{"studio", "live"}}, "standard", false},
		{SrcMetaProfile{Primary: []string{"album", "broadcast", "ep"}}, "standardsingles", false},
		{SrcMetaProfile{Primary: []string{"album"}, Statuses: []string{"official", "promotion"}}, "standard", false},
	}
	for _, c := range cases {
		if m := mapMetaProfile(c.p); m.Key != c.key || m.Exact != c.exact || m.Reason == "" {
			t.Errorf("%+v → %+v", c.p, m)
		}
	}
	if k := metaProfileKey(nil, 3); k != domain.DefaultMetadataProfile {
		t.Error(k)
	}
}

func TestTrackPlaceFallsBackToThePrintedNumber(t *testing.T) {
	cases := []struct {
		w    wTrack
		want int
	}{
		{wTrack{AbsoluteTrackNumber: 4, TrackNumber: "B1"}, 4},
		{wTrack{TrackNumber: "B3"}, 3},
		{wTrack{TrackNumber: "7"}, 7},
	}
	for _, c := range cases {
		if got := trackOf(c.w); got.Number != c.want || got.Medium != 1 {
			t.Errorf("%+v → %+v", c.w, got)
		}
	}
}
