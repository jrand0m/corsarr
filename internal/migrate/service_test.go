package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

var day = time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)

type rig struct {
	t        *testing.T
	st       *store.Store
	svc      *Service
	lib      library
	sonarr   *fakeArr
	radarr   *fakeArr
	prowlarr *fakeArr
	events   []domain.Event
	req      Request
}

func newRig(t *testing.T, hashes ...string) *rig {
	r := &rig{t: t, st: openStore(t), lib: newLibrary(t)}
	r.sonarr = newFake(t, Sonarr, sonarrRoutes(day))
	r.radarr = newFake(t, Radarr, radarrRoutes(day))
	r.prowlarr = newFake(t, Prowlarr, prowlarrRoutes())
	r.svc = New(Deps{Store: r.st, HTTP: http.DefaultClient, Now: func() time.Time { return day.Add(24 * time.Hour) },
		Downloaders: func() []Downloader { return []Downloader{{Key: "qbittorrent", List: clientFake{hashes}}} },
		Publish:     func(ev domain.Event) { r.events = append(r.events, ev) }})
	scope := DefaultScope()
	scope[CatHistory] = true
	r.req = Request{Sources: []Endpoint{r.sonarr.endpoint(t, Sonarr), r.radarr.endpoint(t, Radarr), r.prowlarr.endpoint(t, Prowlarr)},
		Options: Options{Scope: scope, Rewrite: Rewrite{From: "/data/media", To: r.lib.root}}}
	return r
}

func (r *rig) plan() Report {
	rep, err := r.svc.Plan(context.Background(), r.req, nil)
	if err != nil {
		r.t.Fatal(err)
	}
	return rep
}

func (r *rig) apply() Report {
	rep, err := r.svc.Apply(context.Background(), r.req, "", nil)
	if err != nil {
		r.t.Fatal(err)
	}
	return rep
}

func entry(t *testing.T, rep Report, id string) Entry {
	t.Helper()
	for _, e := range rep.Entries {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("no entry %s", id)
	return Entry{}
}

func TestPlanReadsOnlyAndWritesNoLibraryState(t *testing.T) {
	r := newRig(t, "aaaa")
	rep := r.plan()
	for _, f := range []*fakeArr{r.sonarr, r.radarr, r.prowlarr} {
		f.onlyGets(t)
	}
	ix, _ := r.st.Migration().Index(context.Background())
	ov, _ := r.st.ModeOverrides().List(context.Background())
	if len(ix.SeriesByTMDB)+len(ix.MovieByTMDB)+len(ix.FilePaths)+len(ov) != 0 {
		t.Fatalf("plan wrote: %+v %+v", ix, ov)
	}
	if rep.Counts[OutCreate] == 0 || rep.PlanHash == "" || len(rep.Sources) != 3 {
		t.Fatalf("%+v", rep.Counts)
	}
	seen := map[string]bool{}
	for _, e := range rep.Entries {
		if seen[e.ID] || e.Outcome == "" || e.Reason == "" {
			t.Fatalf("entry %+v duplicated or without outcome/reason", e)
		}
		seen[e.ID] = true
	}
}

func TestPlanOutcomesPerObject(t *testing.T) {
	r := newRig(t, "aaaa")
	rep := r.plan()
	want := map[string]Outcome{
		"sonarr:series:1": OutCreate, "sonarr:series:2": OutUnsupported,
		"sonarr:episodefile:501": OutCreate, "sonarr:episodefile:502": OutFileMissing,
		"sonarr:profile:1": OutUnchanged, "sonarr:profile:2": OutApproximate, "sonarr:customformat:0": OutUnsupported,
		"sonarr:naming:rename": OutUnsupported, "sonarr:downloadclient:1": OutSecretNeeded, "sonarr:downloadclient:2": OutUnsupported,
		"radarr:downloadclient:1": OutUnchanged, "sonarr:indexer:3": OutSkip,
		"prowlarr:indexer:4": OutApproximate, "prowlarr:indexer:5": OutSecretNeeded, "prowlarr:indexer:6": OutUnsupported,
		"sonarr:download:aaaa": OutCreate, "sonarr:download:bbbb": OutGone,
		"sonarr:blocklist:9": OutCreate, "sonarr:history:70": OutCreate, "sonarr:history:71": OutSkip,
		"radarr:movie:1": OutCreate, "radarr:moviefile:31": OutCreate, "radarr:movie:2": OutCreate,
		"corsarr:mode:tv": OutCreate, "corsarr:mode:movie": OutCreate, "corsarr:rules:all": OutCreate,
	}
	for id, out := range want {
		if e := entry(t, rep, id); e.Outcome != out {
			t.Errorf("%s: %s (%s), want %s", id, e.Outcome, e.Reason, out)
		}
	}
	if e := entry(t, rep, "radarr:movie:2"); !strings.Contains(e.Reason, "minimum availability") {
		t.Errorf("folded availability not reported: %s", e.Reason)
	}
	if e := entry(t, rep, "sonarr:profile:2"); !strings.Contains(e.Reason, "ultrahd") || !strings.Contains(e.Reason, "1 custom format") {
		t.Errorf("profile reason: %s", e.Reason)
	}
}

func TestApplyAdoptsLibraryDownloadsAndSetsShadow(t *testing.T) {
	r := newRig(t, "aaaa", "cccc")
	rep := r.apply()
	ctx := context.Background()
	for _, f := range []*fakeArr{r.sonarr, r.radarr, r.prowlarr} {
		f.onlyGets(t)
	}
	if rep.Counts[OutFailed] != 0 {
		t.Fatalf("failures: %+v", rep.Entries)
	}
	ix, _ := r.st.Migration().Index(ctx)
	ser, _, _ := r.st.TV().Series(ctx, ix.SeriesByTMDB[100])
	eps, _ := r.st.TV().Episodes(ctx, ser.ID)
	if ser.Path != r.lib.root+"/tv/Show" || ser.ProfileKey != "ultrahd" || len(ser.Tags) != 1 || len(eps) != 3 {
		t.Fatalf("series %+v eps %d", ser, len(eps))
	}
	if eps[0].FileID == 0 || eps[1].FileID != 0 || !eps[0].Monitored || eps[2].Monitored {
		t.Fatalf("episodes %+v", eps)
	}
	f, _, _ := r.st.Library().File(ctx, eps[0].FileID)
	if f.Quality != "webdl-1080p" || f.Group != "GRP" {
		t.Fatalf("file %+v", f)
	}
	m, _, _ := r.st.Movies().Get(ctx, ix.MovieByTMDB[200])
	if m.FileID == 0 || !m.Monitored {
		t.Fatalf("movie %+v", m)
	}
	dls, _ := r.st.Queue().Downloads(ctx, domain.StateQueued)
	byID := map[string]domain.Download{}
	for _, d := range dls {
		byID[d.DownloadID] = d.Download
	}
	if tvd := byID["aaaa"]; len(dls) != 2 || len(tvd.Links) != 2 || tvd.Downloader != "qbittorrent" || tvd.Kind != domain.KindTV {
		t.Fatalf("downloads %+v", dls)
	}
	if mvd := byID["cccc"]; mvd.Kind != domain.KindMovie || mvd.Links[0].Item.ID != m.ID {
		t.Fatalf("movie download %+v", mvd)
	}
	if ok, _ := r.st.Blocklist().Has(ctx, domain.ItemRef{Kind: domain.KindMovie, ID: ix.MovieByTMDB[201]},
		domain.ReleaseIdentity{Title: "Later.CAM"}); !ok {
		t.Fatal("movie blocklist entry missing")
	}
	ov, _ := r.st.ModeOverrides().List(ctx)
	if len(ov) != 3 || ov[0].Name != "movie" || ov[0].Mode != "shadow" || ov[2].Scope != "rule" || ov[2].Mode != "dry-run" {
		t.Fatalf("overrides %+v", ov)
	}
	hist, _, _ := r.st.History().List(ctx, 0, 10)
	if len(hist) != 1 {
		t.Fatalf("history %+v", hist)
	}
	if r.events[len(r.events)-1].Type != domain.EvMigrationComplete {
		t.Fatalf("last event %s", r.events[len(r.events)-1].Type)
	}
}

func TestReapplyConvergesWithoutWrites(t *testing.T) {
	r := newRig(t, "aaaa")
	r.apply()
	rep := r.apply()
	for _, e := range rep.Entries {
		if e.Outcome.writes() && e.op != nil {
			t.Errorf("second run still writes %s: %s (%s)", e.ID, e.Outcome, e.Reason)
		}
	}
	if rep.Counts[OutUnchanged] == 0 {
		t.Fatalf("%+v", rep.Counts)
	}
}

func TestSourceChangeUpdatesAndCorsarrEditConflicts(t *testing.T) {
	r := newRig(t)
	r.apply()
	ctx := context.Background()
	movies := r.radarr.routes["/movie"].([]any)
	movies[0].(map[string]any)["monitored"] = false
	if e := entry(t, r.plan(), "radarr:movie:1"); e.Outcome != OutUpdate {
		t.Fatalf("%+v", e)
	}
	ix, _ := r.st.Migration().Index(ctx)
	m, _, _ := r.st.Movies().Get(ctx, ix.MovieByTMDB[200])
	m.ProfileKey = "any"
	if err := r.st.Movies().Update(ctx, m); err != nil {
		t.Fatal(err)
	}
	if e := entry(t, r.plan(), "radarr:movie:1"); e.Outcome != OutConflict {
		t.Fatalf("%+v", e)
	}
	r.radarr.routes["/movie"] = movies[1:]
	if e := entry(t, r.plan(), "radarr:movie:1"); e.Outcome != OutSkip || !strings.Contains(e.Reason, "removed at source") {
		t.Fatalf("%+v", e)
	}
}

func TestExistingItemAndOwnerChoicesAreKept(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_, _, _ = r.st.Movies().Add(ctx, domain.Movie{TMDBID: 200, Title: "Film", ProfileKey: "any", RootFolder: "/m", Path: "/m/f",
		MinAvailability: domain.AvailReleased, AddedAt: day})
	_ = r.st.ModeOverrides().Set(ctx, store.ModeOverride{Scope: store.ScopeKind, Name: "tv", Mode: "active"})
	rep := r.plan()
	if e := entry(t, rep, "radarr:movie:1"); e.Outcome != OutConflict {
		t.Fatalf("%+v", e)
	}
	if e := entry(t, rep, "corsarr:mode:tv"); e.Outcome != OutUnchanged {
		t.Fatalf("%+v", e)
	}
}

func TestExcludedEntriesAreSkipped(t *testing.T) {
	r := newRig(t)
	reviewed := r.plan().PlanHash
	r.req.Options.Exclude = map[string]bool{"radarr:movie:2": true}
	rep, err := r.svc.Apply(context.Background(), r.req, reviewed, nil)
	if err != nil {
		t.Fatal("leaving an object out changed the reviewed plan:", err)
	}
	if e := entry(t, rep, "radarr:movie:2"); e.Outcome != OutSkip {
		t.Fatalf("%+v", e)
	}
	ix, _ := r.st.Migration().Index(context.Background())
	if _, ok := ix.MovieByTMDB[201]; ok {
		t.Fatal("excluded movie was created")
	}
}

func TestApplyRefusesAChangedPlan(t *testing.T) {
	r := newRig(t)
	if _, err := r.svc.Apply(context.Background(), r.req, "stale", nil); !errors.Is(err, ErrPlanChanged) {
		t.Fatal(err)
	}
	ix, _ := r.st.Migration().Index(context.Background())
	if len(ix.MovieByTMDB) != 0 {
		t.Fatal("changed plan was applied")
	}
	rep := r.plan()
	if _, err := r.svc.Apply(context.Background(), r.req, rep.PlanHash, nil); err != nil {
		t.Fatal(err)
	}
}

func TestWrongKeyStopsThatSourceOnly(t *testing.T) {
	r := newRig(t)
	bad, _ := ParseEndpoint("sonarr", r.sonarr.srv.URL, NewSecret("wrong"))
	r.req.Sources = []Endpoint{bad, r.radarr.endpoint(t, Radarr)}
	rep := r.plan()
	if !strings.Contains(rep.Sources[0].Error, "unauthorised") || rep.Sources[1].Error != "" {
		t.Fatalf("%+v", rep.Sources)
	}
	if len(r.sonarr.paths) != 1 {
		t.Fatalf("more requests after a refused key: %v", r.sonarr.paths)
	}
	if entry(t, rep, "radarr:movie:1").Outcome != OutCreate {
		t.Fatal("the other source did not proceed")
	}
}

func TestUnsupportedVersionIsRefused(t *testing.T) {
	r := newRig(t)
	r.sonarr.version = "2.0.0"
	r.req.Sources = r.req.Sources[:1]
	rep, err := r.svc.Plan(context.Background(), r.req, nil)
	if !errors.Is(err, ErrUnsupportedVersion) && (err == nil || !strings.Contains(err.Error(), "unsupported version")) {
		t.Fatal(err)
	}
	if !strings.Contains(rep.Sources[0].Error, "supported 3.x to 4.x") || len(rep.Entries) != 0 {
		t.Fatalf("%+v %d", rep.Sources, len(rep.Entries))
	}
}

func TestReportNeverCarriesSecrets(t *testing.T) {
	r := newRig(t, "aaaa")
	rep := r.apply()
	body, _ := json.Marshal(rep)
	stored, _, _ := r.svc.RunReport(context.Background(), rep.RunID)
	for _, b := range [][]byte{body, stored} {
		if strings.Contains(string(b), testKey) {
			t.Fatal("API key in a report")
		}
	}
	if !strings.Contains(rep.Config, "${env:CORSARR_SECRET_DOWNLOADCLIENTS_QBITTORRENT_PASSWORD}") || strings.Contains(rep.Config, "****") {
		t.Fatalf("config snippet:\n%s", rep.Config)
	}
}

func TestNoDownloadClientSkipsAdoption(t *testing.T) {
	r := newRig(t)
	r.svc.d.Downloaders = nil
	if e := entry(t, r.plan(), "sonarr:download:aaaa"); e.Outcome != OutSkip {
		t.Fatalf("%+v", e)
	}
}

func TestOneRunAtATime(t *testing.T) {
	r := newRig(t)
	r.svc.mu.Lock()
	defer r.svc.mu.Unlock()
	if _, err := r.svc.Plan(context.Background(), r.req, nil); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	if _, err := r.svc.Plan(context.Background(), Request{}, nil); !errors.Is(err, ErrNoSources) {
		t.Fatal(err)
	}
}

func TestRunsAreListed(t *testing.T) {
	r := newRig(t)
	r.plan()
	runs, err := r.svc.Runs(context.Background())
	if err != nil || len(runs) != 1 || runs[0].Mode != "plan" || runs[0].Status != "done" {
		t.Fatalf("%+v %v", runs, err)
	}
}
