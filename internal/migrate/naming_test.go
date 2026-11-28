package migrate

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/naming"
	"github.com/jrand0m/corsarr/internal/plugin"
)

func TestConvertFormatMapsTokenByToken(t *testing.T) {
	cases := []struct{ in, want string }{
		{"{Series Title} - S{season:00}E{episode:00}{ - Episode Title}{ [Quality Title]}", "{Series Title} - S{Season}E{Episode}{ - Episode Title}{ [Quality Full]}"},
		{"{Movie TitleYear} {imdb-{ImdbId}}", "{Movie Title}{ (Release Year)} {imdb-IMDb Id}"},
		{"{Movie.Title}.{Release.Year}.{MediaInfo VideoCodec}", "{Movie Title}.{Release Year}."},
		{"Season {season}", "Season {Season}"},
		{"{track:00} - {Track Title}{ (Medium Format)}", "{Track} - {Track Title}{ (Medium Format)}"},
		{"{{ImdbId}}", "{IMDb Id}"},
		{"{Episode Title}{00}", "{Episode Title}"},
	}
	for _, c := range cases {
		if got, _ := convertFormat(Sonarr, "f", c.in); got != c.want {
			t.Errorf("%q → %q, want %q", c.in, got, c.want)
		}
		if _, err := naming.ParseTemplate(c.want); err != nil {
			t.Errorf("%q does not parse: %v", c.want, err)
		}
	}
	_, tokens := convertFormat(Radarr, "standardMovieFormat", "{Movie CleanTitle} {season} {Custom Formats} {Release Group}")
	want := []struct{ status, target string }{{TokenApproximate, "Movie Title"}, {TokenApproximate, "Season"}, {TokenNone, ""}, {TokenExact, "Release Group"}}
	for i, w := range want {
		if tokens[i].Status != w.status || tokens[i].Target != w.target || tokens[i].Source != Radarr || tokens[i].Field != "standardMovieFormat" {
			t.Errorf("token %d: %+v", i, tokens[i])
		}
	}
	if tokens[2].Token != "{Custom Formats}" || tokens[1].Note == "" {
		t.Errorf("%+v", tokens)
	}
}

func TestMapNamingPerApp(t *testing.T) {
	off := false
	tv := mapNaming(Sonarr, SrcNaming{Rename: &off, Colon: "spaceDash", Formats: map[string]string{
		"seriesFolderFormat": "{Series Title}", "seasonFolderFormat": "S{season:00}", "standardEpisodeFormat": "{Series Title} {season:00}x{episode:00}",
		"animeEpisodeFormat": "{Series Title} {absolute:000}", "specialsFolderFormat": "Extras"}})
	if tv.Scheme.Rename || tv.Scheme.Colon != naming.ColonSpaceDash || tv.Scheme.Folder != "{Series Title}" || tv.Scheme.Sub != "S{Season}" ||
		tv.Scheme.File != "{Series Title} {Season}x{Episode}" {
		t.Fatalf("%+v", tv.Scheme)
	}
	if notes := strings.Join(tv.Notes, "|"); !strings.Contains(notes, "animeEpisodeFormat") || !strings.Contains(notes, "Extras") || !strings.Contains(notes, "space on both") {
		t.Fatal(notes)
	}
	music := mapNaming(Lidarr, SrcNaming{Formats: map[string]string{"standardTrackFormat": "{Artist Name}/{Album Title}/{track:00} {Track Title}"}})
	if !music.Scheme.Rename || music.Scheme.Sub != "{Artist Name}/{Album Title}" || music.Scheme.File != "{Track} {Track Title}" ||
		music.Scheme.Folder != naming.DefaultArtistFolder {
		t.Fatalf("%+v", music.Scheme)
	}
	flat := mapNaming(Lidarr, SrcNaming{Formats: map[string]string{"standardTrackFormat": "{track:00} {Track Title}"}})
	if flat.Scheme.Sub != naming.DefaultAlbumFolder || len(flat.Notes) != 1 {
		t.Fatalf("%+v %v", flat.Scheme, flat.Notes)
	}
	empty := mapNaming(Radarr, SrcNaming{Formats: map[string]string{"standardMovieFormat": "{MediaInfo Full}"}})
	if empty.Scheme.File != naming.DefaultMovieFile || !strings.Contains(strings.Join(empty.Notes, ""), "built-in template is kept") {
		t.Fatalf("%+v %v", empty.Scheme, empty.Notes)
	}
}

func TestMapNamingNamesTheSourceSettingOfEachToken(t *testing.T) {
	fields := func(m NamingMapping) string {
		var out []string
		for _, tok := range m.Tokens {
			out = append(out, tok.Field)
		}
		return strings.Join(out, ",")
	}
	tv := mapNaming(Sonarr, SrcNaming{Formats: map[string]string{
		"seriesFolderFormat": "{Series Title}", "seasonFolderFormat": "Season {season}", "standardEpisodeFormat": "{Episode Title}"}})
	if got := fields(tv); got != "seriesFolderFormat,seasonFolderFormat,standardEpisodeFormat" {
		t.Errorf("tv: %s", got)
	}
	music := mapNaming(Lidarr, SrcNaming{Formats: map[string]string{
		"artistFolderFormat": "{Artist Name}", "standardTrackFormat": "{Album Title}/{Track Title}"}})
	if got := fields(music); got != "artistFolderFormat,standardTrackFormat,standardTrackFormat" {
		t.Errorf("music: %s", got)
	}
}

func TestATrackFormatStartingWithASlashHasNoAlbumFolder(t *testing.T) {
	m := mapNaming(Lidarr, SrcNaming{Formats: map[string]string{"standardTrackFormat": "/{Track Title}"}})
	if m.Scheme.File != "{Track Title}" || m.Scheme.Sub != naming.DefaultAlbumFolder || len(m.Notes) != 0 {
		t.Fatalf("%+v %v", m.Scheme, m.Notes)
	}
}

func TestNamingSummaryListsOnlyWhatIsThere(t *testing.T) {
	m := mapNaming(Radarr, SrcNaming{Formats: map[string]string{"movieFolderFormat": "{Movie Title}", "standardMovieFormat": "{Movie Title}"}})
	want := "add naming.movie to your config file (see the snippet); renaming on; tokens: 2 exact, 0 approximate, 0 without an equivalent"
	if got := namingSummary(m); got != want {
		t.Fatalf("%q", got)
	}
	m = mapNaming(Radarr, SrcNaming{Colon: "custom", Formats: map[string]string{"standardMovieFormat": "{Movie Title} {Custom Formats}"}})
	want = "add naming.movie to your config file (see the snippet); renaming on; tokens: 1 exact, 0 approximate, 1 without an equivalent" +
		" ({Custom Formats}); a custom colon replacement became smart"
	if got := namingSummary(m); got != want {
		t.Fatalf("%q", got)
	}
}

func TestColonNameReadsNumbersAndNames(t *testing.T) {
	for raw, want := range map[string]string{`0`: "delete", `4`: "smart", `5`: "custom", `6`: "", `-1`: "", `"dash"`: "dash", `9`: "", `null`: "", `true`: ""} {
		if got := colonName(json.RawMessage(raw)); got != want {
			t.Errorf("%s → %q", raw, got)
		}
	}
}

func TestNamingSnippetLoadsAndReportsTokens(t *testing.T) {
	r := newRig(t, "aaaa")
	rep := r.plan()
	in := config.Inputs{ReadFile: func(string) ([]byte, error) { return []byte(rep.Config), nil },
		Resolver: config.Resolver{Getenv: func(string) (string, bool) { return "x", true }}}
	cfg, err := config.LoadFrom(config.Source{Kind: config.SourceFile, Path: "migrated.yml"}, in, plugin.Default)
	if err != nil {
		t.Fatalf("%v\n%s", err, rep.Config)
	}
	tv := cfg.NamingFor(domain.KindTV)
	if tv.Rename || tv.Folder != "{Series Title}{ (Release Year)}" || tv.Sub != "Season {Season}" ||
		tv.File != "{Series Title} - S{Season}E{Episode} - {Episode Title} {Quality Full} " {
		t.Fatalf("%+v\n%s", tv, rep.Config)
	}
	e := entry(t, rep, "sonarr:naming:tv")
	if !strings.Contains(e.Reason, "{MediaInfo VideoCodec}") || !strings.Contains(e.Reason, "dailyEpisodeFormat") || !strings.Contains(e.Reason, "renaming off") {
		t.Errorf("reason %q", e.Reason)
	}
	var none int
	for _, tm := range rep.Naming {
		if tm.Status == TokenNone {
			none++
		}
	}
	if none != 1 || len(rep.Naming) < 7 {
		t.Fatalf("token report %+v", rep.Naming)
	}
}

func TestNamingTheConfigSetsIsNotRepeated(t *testing.T) {
	r := newRig(t)
	snaps := r.svc.read(context.Background(), r.req, func(Progress) {})
	st, err := r.svc.state(context.Background(), snaps, r.req.Options)
	if err != nil {
		t.Fatal(err)
	}
	st.Config.Naming = map[string]bool{"tv": true}
	snaps = append(snaps, Snapshot{Source: SourceInfo{App: Radarr, Instance: "second"}, Naming: &SrcNaming{}})
	rep := BuildPlan(snaps, st, r.req.Options)
	if e := entry(t, rep, "sonarr:naming:tv"); e.Outcome != OutManaged || e.Reason != "already in your config file" {
		t.Errorf("%+v", e)
	}
	if strings.Contains(rep.Config, "  tv:\n        rename") || !strings.Contains(rep.Config, "movieFile") {
		t.Errorf("snippet:\n%s", rep.Config)
	}
	skipped := 0
	for _, e := range rep.Entries {
		if e.ID == "radarr:naming:movie" && e.Outcome == OutSkip {
			skipped++
		}
	}
	if skipped != 1 {
		t.Fatal("second radarr naming must be skipped")
	}
}

func TestConfigViewSeesNamingTheConfigSets(t *testing.T) {
	yml := "naming:\n  movie:\n    rename: false\n"
	in := config.Inputs{ReadFile: func(string) ([]byte, error) { return []byte(yml), nil }}
	cfg, err := config.LoadFrom(config.Source{Kind: config.SourceFile, Path: "c.yml"}, in, plugin.Default)
	if err != nil {
		t.Fatal(err)
	}
	if v := configView(cfg); !v.Naming["movie"] || v.Naming["tv"] {
		t.Fatalf("%v", v.Naming)
	}
}

func TestATemplateThatDoesNotParseKeepsTheBuiltin(t *testing.T) {
	m := mapNaming(Radarr, SrcNaming{Formats: map[string]string{"standardMovieFormat": "{/Movie Title}"}})
	if m.Scheme.File != naming.DefaultMovieFile || !strings.Contains(strings.Join(m.Notes, "|"), "standardMovieFormat could not be converted (") {
		t.Fatalf("%+v %v", m.Scheme, m.Notes)
	}
}
