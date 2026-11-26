package tv

import (
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
	"github.com/jrand0m/corsarr/internal/plugin"
)

var day0 = time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)

func at(days int) *time.Time { t := day0.AddDate(0, 0, days); return &t }

// threeSeasons: specials, S1 (2 eps), S2 (2 eps), S3 (2 eps, last unaired).
func threeSeasons() []domain.Episode {
	var out []domain.Episode
	id := int64(1)
	add := func(s, n int, air *time.Time, file int64) {
		out = append(out, domain.Episode{ID: id, Season: s, Number: n, AirAt: air, RuntimeMin: 30, FileID: file, InMetadata: true})
		id++
	}
	add(0, 1, at(-400), 0)
	add(1, 1, at(-300), 7)
	add(1, 2, at(-293), 0)
	add(2, 1, at(-200), 0)
	add(2, 2, at(-193), 8)
	add(3, 1, at(-7), 0)
	add(3, 2, at(7), 0)
	return out
}

func monitoredCodes(eps []domain.Episode) []string {
	var out []string
	for _, e := range eps {
		if e.Monitored {
			out = append(out, e.Code())
		}
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestApplyModes(t *testing.T) {
	cases := map[domain.MonitorMode][]string{
		domain.MonitorAll:          {"S01E01", "S01E02", "S02E01", "S02E02", "S03E01", "S03E02"},
		domain.MonitorFuture:       {"S03E02"},
		domain.MonitorMissing:      {"S01E02", "S02E01", "S03E01", "S03E02"},
		domain.MonitorExisting:     {"S01E01", "S02E02"},
		domain.MonitorFirstSeason:  {"S01E01", "S01E02"},
		domain.MonitorLatestSeason: {"S03E01", "S03E02"},
		domain.MonitorPilot:        {"S01E01"},
		domain.MonitorNone:         nil,
	}
	for mode, want := range cases {
		got, seasons := ApplyMode(mode, threeSeasons(), day0)
		if !eq(monitoredCodes(got), want) {
			t.Errorf("%s: %v, want %v", mode, monitoredCodes(got), want)
		}
		if len(seasons) != 4 || seasons[0].Monitored {
			t.Errorf("%s: specials season must stay off: %+v", mode, seasons)
		}
	}
	_, seasons := ApplyMode(domain.MonitorFuture, threeSeasons(), day0)
	if !seasons[1].Monitored || !seasons[3].Monitored {
		t.Errorf("future keeps regular seasons open for new episodes: %+v", seasons)
	}
	_, seasons = ApplyMode(domain.MonitorPilot, threeSeasons(), day0)
	for _, s := range seasons {
		if s.Monitored {
			t.Errorf("pilot leaves no season open: %+v", seasons)
		}
	}
}

func TestAvailability(t *testing.T) {
	e := domain.Episode{AirAt: &day0}
	if got, _ := AvailableAt(e); !got.Equal(day0.Add(DefaultRuntime * time.Minute)) {
		t.Fatalf("default runtime: %v", got)
	}
	if IsAvailable(e, day0) || !IsAvailable(e, day0.Add(time.Hour)) {
		t.Fatal("available after air time plus runtime")
	}
	if _, ok := AvailableAt(domain.Episode{}); ok || IsAvailable(domain.Episode{}, day0) {
		t.Fatal("TBA episodes are never available")
	}
}

func TestProfilesAndSortTitle(t *testing.T) {
	p, ok := ProfileByKey(DefaultProfileKey)
	if !ok || p.Spec().Kind != domain.KindTV {
		t.Fatalf("default tv profile: %v %v", p.Spec(), ok)
	}
	if _, ok := ProfileByKey("nope"); ok {
		t.Fatal("unknown profile")
	}
	if SortTitle("The Show") != "show" || SortTitle("Anvil") != "anvil" {
		t.Fatal("sort title")
	}
}

var show = domain.Series{ID: 1, TMDBID: 77, TVDBID: 321, IMDBID: "tt0077", Title: "Example Show", Year: 2019,
	AltTitles: []string{"Beispiel"}, Monitored: true, Path: "/tv/Example Show (2019)", SeasonFolders: true}

func TestMatchesAndCovered(t *testing.T) {
	if !Matches(show, parse.ParseRelease("Example.Show.S01E02.1080p.WEB-DL-GRP")) {
		t.Fatal("plain title")
	}
	if !Matches(show, parse.ParseRelease("Beispiel.S01E02.720p.HDTV-GRP")) {
		t.Fatal("alternate title")
	}
	if Matches(show, parse.ParseRelease("Example.Show.1995.S01E02.720p.HDTV-GRP")) {
		t.Fatal("a remake with another year must not match")
	}
	if Matches(show, parse.ParseRelease("Other.Show.S01E02.720p.HDTV-GRP")) {
		t.Fatal("other show")
	}
	eps := threeSeasons()
	got := Covered(parse.ParseRelease("Example.Show.S01E01-E02.1080p.WEB-DL-GRP").Episode, eps)
	if len(got) != 2 {
		t.Fatalf("multi-episode covers 2: %v", got)
	}
	if got := Covered(parse.ParseRelease("Example.Show.S02.1080p.WEB-DL-GRP").Episode, eps); len(got) != 2 || got[0].Season != 2 {
		t.Fatalf("season pack: %v", got)
	}
	daily := parse.ParseRelease("Example.Show.2024.03.08.720p.HDTV-GRP").Episode
	if got := Covered(daily, eps); len(got) != 1 || got[0].Code() != "S03E02" {
		t.Fatalf("daily: %v", got)
	}
	if got := Covered(parse.Episode{}, eps); len(got) != 0 {
		t.Fatal("movie-shaped title covers nothing")
	}
}

func TestSearchRequests(t *testing.T) {
	e := domain.Episode{ID: 4, Season: 2, Number: 3}
	r := EpisodeRequest(show, e, domain.OriginAutomatic)
	if r.Mode != domain.SearchTV || r.Season != 2 || r.Episodes[0] != 3 || r.IDs["tvdb"] != "321" || r.IDs["imdb"] != "tt0077" ||
		r.Text != "Example Show S02E03" || r.ItemRefs[0] != e.Ref() {
		t.Fatalf("%+v", r)
	}
	s := SeasonRequest(show, 2, []domain.Episode{e}, domain.OriginAutomatic)
	if s.Season != 2 || len(s.Episodes) != 0 || s.Text != "Example Show S02" || len(s.ItemRefs) != 1 {
		t.Fatalf("%+v", s)
	}
	if bare := EpisodeRequest(domain.Series{TMDBID: 5, Title: "X"}, e, domain.OriginRSS); bare.IDs["tvdb"] != "" || bare.IDs["tmdb"] != "5" {
		t.Fatalf("ids %+v", bare.IDs)
	}
}

func rel(title string, size int64) domain.Release {
	return domain.Release{ID: title, Title: title, Protocol: domain.ProtoTorrent, Size: size, PublishedAt: day0.AddDate(0, 0, -1),
		Seeders: 20, Indexers: []domain.IndexerRef{{Key: "ix"}}}
}

func targets(eps []domain.Episode, cur map[int64]*decide.CurrentFile) []Target {
	var out []Target
	for _, e := range eps {
		out = append(out, Target{Episode: e, Current: cur[e.ID], Context: decide.Context{Now: day0}})
	}
	return out
}

func TestDecidePackAcceptedWhenOneEpisodeNeedsIt(t *testing.T) {
	profile, _ := ProfileByKey(DefaultProfileKey)
	eps, _ := ApplyMode(domain.MonitorAll, threeSeasons(), day0)
	season2 := Covered(parse.ParseRelease("Example.Show.S02.1080p.WEB-DL-GRP").Episode, eps)
	cur := map[int64]*decide.CurrentFile{5: {Quality: "webdl-1080p", Revision: 1}}
	d := Decide(show, profile, rel("Example.Show.S02.1080p.WEB-DL-GRP", 3_000_000_000), targets(season2, cur), nil)
	if !d.Accepted || len(d.ItemRefs) != 2 {
		t.Fatalf("pack: %+v", d)
	}
	// the pack is the size of two episodes: per-episode limits apply
	huge := Decide(show, profile, rel("Example.Show.S02.1080p.WEB-DL-GRP", 2*16_000_000_000), targets(season2, cur), nil)
	if huge.Accepted {
		t.Fatalf("oversized pack accepted: %+v", huge)
	}
}

func TestDecideRejectsOtherShowsAndEpisodeLessTitles(t *testing.T) {
	profile, _ := ProfileByKey(DefaultProfileKey)
	eps, _ := ApplyMode(domain.MonitorAll, threeSeasons(), day0)
	d := Decide(show, profile, rel("Other.Show.S01E02.1080p.WEB-DL-GRP", 1_000_000_000), targets(eps[2:3], nil), nil)
	if d.Accepted || d.Reasons[len(d.Reasons)-1].RuleID != "tv.mismatch" {
		t.Fatalf("other show: %+v", d)
	}
	none := Decide(show, profile, rel("Example.Show.1080p.WEB-DL-GRP", 1_000_000_000), nil, nil)
	if none.Accepted || none.Reasons[0].RuleID != "tv.no_episode" || none.Override != domain.OverrideConfirm {
		t.Fatalf("no episode: %+v", none)
	}
	unmon := Decide(show, profile, rel("Example.Show.S01E02.1080p.WEB-DL-GRP", 1_000_000_000), targets(threeSeasons()[2:3], nil), nil)
	if unmon.Accepted {
		t.Fatalf("unmonitored episode accepted: %+v", unmon)
	}
}

func TestMergeRecord(t *testing.T) {
	stored, seasons := ApplyMode(domain.MonitorAll, threeSeasons(), day0)
	for i := range seasons {
		seasons[i].SeriesID = 1
	}
	s := show
	s.MonitorNew = domain.MonitorNewNone
	seasons[2].Monitored = false // season 2 toggled off
	rec := plugin.SeriesRecord{Episodes: []plugin.EpisodeRecord{
		{Season: 1, Number: 1, Title: "Pilot (renamed)", AirAt: at(-300)},
		// S01E02 gone upstream, no file → deleted
		{Season: 2, Number: 1}, {Season: 2, Number: 3, Title: "new in an off season"},
		// S02E02 gone upstream, has a file → kept, flagged
		{Season: 3, Number: 1}, {Season: 3, Number: 2}, {Season: 3, Number: 3, Title: "new in an open season"},
		{Season: 4, Number: 1, Title: "new season"},
		// S00E01 gone, no file → deleted
	}}
	m := MergeRecord(s, seasons, stored, rec)
	if m.Added != 3 || m.Orphaned != 1 || len(m.Drop) != 2 || len(m.Seasons) != 1 || m.Seasons[0].Monitored {
		t.Fatalf("merge %+v", m)
	}
	byCode := map[string]domain.Episode{}
	for _, e := range m.Upserts {
		byCode[e.Code()] = e
	}
	if e := byCode["S01E01"]; e.Title != "Pilot (renamed)" || e.FileID != 7 || !e.Monitored || e.ID != 2 {
		t.Errorf("existing episode keeps id, file and monitoring: %+v", e)
	}
	if byCode["S02E03"].Monitored || !byCode["S03E03"].Monitored || byCode["S04E01"].Monitored {
		t.Errorf("new episodes follow season flags and monitor-new: %+v", byCode)
	}
	if e := byCode["S02E02"]; e.InMetadata || e.FileID != 8 {
		t.Errorf("orphan with file kept: %+v", e)
	}
}

func TestPaths(t *testing.T) {
	if FolderName(show) != "Example Show (2019)" {
		t.Fatal(FolderName(show))
	}
	eps := []domain.Episode{{Season: 1, Number: 1, Title: "Pilot"}}
	if got := FilePath(show, eps, "WEBDL-1080p", "mkv"); got != "/tv/Example Show (2019)/Season 01/Example Show - S01E01 - Pilot [WEBDL-1080p].mkv" {
		t.Fatal(got)
	}
	multi := []domain.Episode{{Season: 1, Number: 1}, {Season: 1, Number: 2}, {Season: 1, Number: 3}}
	if got := FilePath(show, multi, "", ".mkv"); got != "/tv/Example Show (2019)/Season 01/Example Show - S01E01-E03.mkv" {
		t.Fatal(got)
	}
	flat := show
	flat.SeasonFolders = false
	if got := FilePath(flat, []domain.Episode{{Season: 0, Number: 12}}, "", "mkv"); got != "/tv/Example Show (2019)/Example Show - S00E12.mkv" {
		t.Fatal(got)
	}
	if SeasonFolder(show, 0) != "Specials" {
		t.Fatal("specials folder")
	}
	s := domain.Series{}
	ApplyRecord(&s, plugin.SeriesRecord{IDs: map[string]string{"tmdb": "7", "tvdb": "8", "imdb": "tt9"}, Title: "The Show"})
	if s.TMDBID != 7 || s.TVDBID != 8 || s.IMDBID != "tt9" || s.SortTitle != "show" || s.Genres == nil || s.AltTitles == nil {
		t.Fatalf("%+v", s)
	}
	if eps := Episodes(plugin.SeriesRecord{Episodes: []plugin.EpisodeRecord{{Season: 1, Number: 1}}}); !eps[0].InMetadata || eps[0].Monitored {
		t.Fatalf("%+v", eps)
	}
}

func TestApplySeasonsMonitorsExactlyTheListedSeasons(t *testing.T) {
	got, seasons := ApplySeasons([]int{2, 3}, threeSeasons())
	if want := []string{"S02E01", "S02E02", "S03E01", "S03E02"}; !eq(monitoredCodes(got), want) {
		t.Fatalf("%v, want %v", monitoredCodes(got), want)
	}
	if len(seasons) != 4 || seasons[0].Monitored || seasons[1].Monitored || !seasons[2].Monitored || !seasons[3].Monitored {
		t.Fatalf("season flags %+v", seasons)
	}
}
