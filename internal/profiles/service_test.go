package profiles

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

type fakeRepo struct {
	mu      sync.Mutex
	rows    map[store.QualityTable]map[string]store.QualityObject
	use     map[string]int
	listErr error
}

func newRepo() *fakeRepo {
	return &fakeRepo{rows: map[store.QualityTable]map[string]store.QualityObject{store.TableFormats: {}, store.TableProfiles: {}}, use: map[string]int{}}
}

func (r *fakeRepo) List(_ context.Context, t store.QualityTable) ([]store.QualityObject, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErr != nil {
		return nil, r.listErr
	}
	var out []store.QualityObject
	for _, o := range r.rows[t] {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (r *fakeRepo) Save(_ context.Context, t store.QualityTable, o store.QualityObject, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[t][o.Key] = o
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, t store.QualityTable, key string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.rows[t][key]
	delete(r.rows[t], key)
	return ok, nil
}

func (r *fakeRepo) ProfileUse(_ context.Context, key string) (int, error) { return r.use[key], nil }

type fakeBus struct{ events []domain.Event }

func (b *fakeBus) Publish(ev domain.Event) (domain.Event, error) {
	b.events = append(b.events, ev)
	return ev, nil
}

var ps5Format = decide.FormatDoc{Name: "Audio - PS5 Safe", Conditions: []decide.ConditionDoc{{Kind: decide.CondAudio, Values: []string{"aac", "eac3"}}}}

func ps5Profile(format string) decide.ProfileDoc {
	return decide.ProfileDoc{Name: "High - PS5", Kinds: []domain.MediaKind{domain.KindTV, domain.KindMovie},
		Groups: []decide.GroupDoc{{Qualities: []decide.QualityID{"webdl-1080p"}}}, Cutoff: "webdl-1080p", Upgrades: true,
		Formats: []decide.ScoreDoc{{Format: format, Score: 50}}}
}

func newService(t *testing.T, repo *fakeRepo, cfg *config.Config) (*Service, *fakeBus) {
	t.Helper()
	t.Cleanup(decide.WithCatalog(nil))
	bus := &fakeBus{}
	s := New(Deps{Repo: repo, Config: func() *config.Config { return cfg }, Pub: bus})
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, bus
}

func TestService_FormatAndProfileLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	s, bus := newService(t, repo, nil)
	f, err := s.CreateFormat(ctx, ps5Format)
	if err != nil || f.Key != "audiops5safe" || f.Origin != "database" {
		t.Fatalf("create format %+v %v", f, err)
	}
	if _, err := s.CreateFormat(ctx, decide.FormatDoc{Key: f.Key, Name: "Again", Conditions: ps5Format.Conditions}); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate key: %v", err)
	}
	var inv InvalidError
	if _, err := s.CreateFormat(ctx, decide.FormatDoc{Name: "Broken", Conditions: []decide.ConditionDoc{{Kind: decide.CondTitle, Pattern: "("}}}); !errors.As(err, &inv) ||
		inv.Errs[0].Path != "conditions.0.pattern" {
		t.Fatalf("invalid: %v", err)
	}
	p, err := s.CreateProfile(ctx, ps5Profile(f.Key))
	if err != nil || p.Key != "highps5" {
		t.Fatalf("create profile %+v %v", p, err)
	}
	if _, ok := decide.Lookup(domain.KindTV, "highps5"); !ok {
		t.Fatal("catalog not swapped")
	}
	if row, _ := s.Format(f.Key); len(row.UsedBy) != 1 || row.UsedBy[0] != "highps5" {
		t.Fatalf("used by %v", row.UsedBy)
	}
	var use InUseError
	if err := s.DeleteFormat(ctx, f.Key); !errors.As(err, &use) || use.Profiles[0] != "High - PS5" {
		t.Fatalf("delete used format: %v", err)
	}
	repo.use["highps5"] = 3
	if err := s.DeleteProfile(ctx, "highps5"); !errors.As(err, &use) || use.Items != 3 {
		t.Fatalf("delete used profile: %v", err)
	}
	repo.use["highps5"] = 0
	upd := ps5Profile(f.Key)
	upd.Name, upd.MinFormatScore = "High - PS5 v2", 10
	if p, err = s.UpdateProfile(ctx, "highps5", upd); err != nil || p.Name != "High - PS5 v2" || p.MinFormatScore != 10 {
		t.Fatalf("update %+v %v", p, err)
	}
	if err := s.DeleteProfile(ctx, "highps5"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFormat(ctx, f.Key); err != nil {
		t.Fatal(err)
	}
	if len(bus.events) != 5 || bus.events[0].Type != domain.EvQualityChanged {
		t.Fatalf("events %v", bus.events)
	}
	if d := bus.events[4].Data.(domain.QualityChange); d.Object != "format" || d.Action != "deleted" {
		t.Fatalf("last event %+v", d)
	}
}

func TestService_BuiltinsAreReadOnlyButDuplicate(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t, newRepo(), nil)
	if _, err := s.UpdateProfile(ctx, "hd1080p", ps5Profile("x")); !errors.Is(err, ErrBuiltin) {
		t.Fatalf("update built-in: %v", err)
	}
	if err := s.DeleteProfile(ctx, "hd1080p"); !errors.Is(err, ErrBuiltin) {
		t.Fatalf("delete built-in: %v", err)
	}
	dup, err := s.DuplicateProfile(ctx, "hd1080p", DuplicateRequest{})
	if err != nil || dup.Key != "hd1080pcopy" || dup.Name != "HD-1080p (copy)" || len(dup.Kinds) != 2 || dup.Builtin {
		t.Fatalf("duplicate %+v %v", dup, err)
	}
	music, err := s.DuplicateProfile(ctx, "any", DuplicateRequest{Kind: domain.KindMusic, Name: "My any"})
	if err != nil || music.Kinds[0] != domain.KindMusic {
		t.Fatalf("music duplicate %+v %v", music, err)
	}
	rows, _ := s.Profiles(ctx, domain.KindMusic)
	if len(rows) != 4 || !rows[1].Builtin || rows[3].Key != "myany" {
		t.Fatalf("music rows %+v", rows)
	}
}

func TestService_ConfigWinsAndIsManaged(t *testing.T) {
	repo := newRepo()
	doc := ps5Format
	doc.Key = "ps5"
	raw, _ := json.Marshal(doc)
	repo.rows[store.TableFormats]["ps5"] = store.QualityObject{Key: "ps5", Name: "DB", Doc: raw}
	fileDoc := doc
	fileDoc.Name = "From file"
	cfg := &config.Config{CustomFormats: []decide.FormatDoc{fileDoc}}
	s, _ := newService(t, repo, cfg)
	rows := s.Formats()
	if len(rows) != 1 || rows[0].Name != "From file" {
		t.Fatalf("rows %+v", rows)
	}
	if mergeDocs([]int{1, 2}, []int{2, 3}, func(i int) string { return string(rune('a' + i)) })[2] != 3 {
		t.Fatal("merge keeps database then file")
	}
}

func TestService_TestBox(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t, newRepo(), nil)
	if _, err := s.CreateFormat(ctx, ps5Format); err != nil {
		t.Fatal(err)
	}
	res, err := s.Test(TestRequest{Titles: []string{"Show.S01E01.1080p.WEB-DL.EAC3.H.264-GRP", " "}})
	if err != nil || len(res) != 1 || !res[0].Formats[0].Matched || !res[0].Formats[0].Conditions[0] {
		t.Fatalf("test %+v %v", res, err)
	}
	draft := decide.FormatDoc{Conditions: []decide.ConditionDoc{{Kind: decide.CondResolution, Values: []string{"2160"}}}}
	res, _ = s.Test(TestRequest{Titles: []string{"Show.S01E01.1080p.WEB-DL-GRP"}, Format: &draft})
	if len(res[0].Formats) != 1 || res[0].Formats[0].Matched || res[0].Formats[0].Name != "Draft" {
		t.Fatalf("draft %+v", res)
	}
}

func TestHTTP(t *testing.T) {
	s, _ := newService(t, newRepo(), nil)
	mux := http.NewServeMux()
	s.Mount(mounter{mux})
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	cases := []struct {
		method, path, body string
		status             int
		contains           string
	}{
		{"POST", "/api/v1/customformats", `{"name":"PS5","conditions":[{"kind":"audio","values":["aac"]}]}`, 201, `"key":"ps5"`},
		{"POST", "/api/v1/customformats", `{"name":"Bad","conditions":[{"kind":"title","pattern":"("}]}`, 422, `"fields":[{"field":"conditions.0.pattern"`},
		{"GET", "/api/v1/customformats", "", 200, `"usedBy":[]`},
		{"POST", "/api/v1/qualityprofiles", `{"name":"P","kinds":["movie"],"groups":[{"qualities":["webdl-1080p"]}],"cutoff":"webdl-1080p","formats":[{"format":"ps5","score":5}]}`, 201, `"key":"p"`},
		{"GET", "/api/v1/qualityprofiles?kind=movie", "", 200, `"isDefault":true`},
		{"GET", "/api/v1/qualityprofiles/p", "", 200, `"items":0`},
		{"PUT", "/api/v1/qualityprofiles/hd1080p", `{"name":"x"}`, 409, `"builtin"`},
		{"DELETE", "/api/v1/customformats/ps5", "", 409, `"in_use"`},
		{"POST", "/api/v1/qualityprofiles/ultrahd/duplicate", "", 201, `"key":"ultrahdcopy"`},
		{"POST", "/api/v1/customformats/test", `{"titles":["Movie.2020.1080p.WEB-DL.AAC-GRP"]}`, 200, `"matched":true`},
		{"GET", "/api/v1/quality/choices", "", 200, `"conditionKinds"`},
		{"DELETE", "/api/v1/qualityprofiles/p", "", 204, ""},
		{"GET", "/api/v1/customformats/nope", "", 404, `"not_found"`},
	}
	for _, c := range cases {
		w := do(c.method, c.path, c.body)
		if w.Code != c.status || !strings.Contains(w.Body.String(), c.contains) {
			t.Errorf("%s %s: %d %s", c.method, c.path, w.Code, w.Body)
		}
	}
}

type mounter struct{ m *http.ServeMux }

func (m mounter) Handle(p string, h http.HandlerFunc) { m.m.Handle(p, h) }

func TestFreeKeyAndUpdates(t *testing.T) {
	taken := map[string]bool{"ps5": true, "ps52": true}
	if k := freeKey("PS5", func(k string) bool { return taken[k] }); k != "ps53" {
		t.Fatalf("key %q", k)
	}
	if k := freeKey("HD-1080p", func(string) bool { return false }); k != "hd1080p2" {
		t.Fatalf("built-in key %q", k)
	}
	long := strings.Repeat("a", 60)
	if k := freeKey(long, func(k string) bool { return len(k) == 48 && !strings.HasSuffix(k, "2") }); len(k) != 48 || !strings.HasSuffix(k, "2") {
		t.Fatalf("long key %q", k)
	}
	ctx := context.Background()
	s, _ := newService(t, newRepo(), &config.Config{CustomFormats: []decide.FormatDoc{{Key: "cfg", Name: "Cfg", Conditions: ps5Format.Conditions}}})
	if _, err := s.UpdateFormat(ctx, "nope", ps5Format); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	f, _ := s.CreateFormat(ctx, ps5Format)
	upd := ps5Format
	upd.Name = "Renamed"
	if row, err := s.UpdateFormat(ctx, f.Key, upd); err != nil || row.Name != "Renamed" || row.Key != f.Key {
		t.Fatalf("update %+v %v", row, err)
	}
	if _, err := s.CreateProfile(ctx, decide.ProfileDoc{Key: f.Key + "p", Name: "x", Kinds: []domain.MediaKind{domain.KindTV},
		Groups: []decide.GroupDoc{{Qualities: []decide.QualityID{"webdl-720p"}}}, Cutoff: "webdl-720p"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProfile(ctx, decide.ProfileDoc{Key: f.Key + "p", Name: "y"}); !errors.Is(err, ErrExists) {
		t.Fatalf("exists: %v", err)
	}
	if _, err := s.UpdateProfile(ctx, "nope", ps5Profile(f.Key)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing profile: %v", err)
	}
	if err := s.DeleteFormat(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
	if _, err := s.Profile(ctx, "nope", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}
}

func loadedConfig(t *testing.T, yml string) *config.Config {
	t.Helper()
	cfg, err := config.LoadFrom(config.Source{Kind: config.SourceFile, Path: "c.yml"},
		config.Inputs{ReadFile: func(string) ([]byte, error) { return []byte(yml), nil }}, plugin.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

const managedProfileYAML = `version: 1
qualityProfiles:
  cfgp:
    name: From file
    kinds: [tv]
    groups:
      - qualities: [webdl-720p]
    cutoff: webdl-720p
`

func TestService_ProfileOriginsAndKeys(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t, newRepo(), loadedConfig(t, managedProfileYAML))
	if _, err := s.CreateProfile(ctx, decide.ProfileDoc{Key: "cfgp", Name: "Mine", Kinds: []domain.MediaKind{domain.KindTV},
		Groups: []decide.GroupDoc{{Qualities: []decide.QualityID{"webdl-720p"}}}, Cutoff: "webdl-720p"}); !errors.Is(err, ErrExists) {
		t.Fatalf("config key reused: %v", err)
	}
	mine := ps5Profile("")
	mine.Formats = nil
	own, err := s.CreateProfile(ctx, mine)
	if err != nil || own.Origin != "database" || own.Managed != nil {
		t.Fatalf("own profile %+v %v", own, err)
	}
	if row, err := s.Profile(ctx, "cfgp", ""); err != nil || row.Origin != "config" || row.Managed == nil {
		t.Fatalf("config profile %+v %v", row, err)
	}
	rows, _ := s.Profiles(ctx, "")
	for _, r := range rows {
		want := map[string][]domain.MediaKind{"hd1080p": {domain.KindTV, domain.KindMovie}, "standard": {domain.KindMusic}}[r.Key]
		if !slices.Equal(r.Default, want) {
			t.Errorf("%s defaults %v", r.Key, r.Default)
		}
	}
	var inv InvalidError
	bad := ps5Profile("")
	bad.Formats, bad.Cutoff = nil, "bluray-1080p"
	if _, err := s.CreateProfile(ctx, bad); !errors.As(err, &inv) || len(inv.Errs) != 1 || inv.Errs[0].Path != "cutoff" {
		t.Fatalf("one problem: %v", err)
	}
}

func TestService_InUseMessagesAndDeletes(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	s, _ := newService(t, repo, nil)
	f, _ := s.CreateFormat(ctx, ps5Format)
	if _, err := s.CreateProfile(ctx, ps5Profile(f.Key)); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFormat(ctx, f.Key); err == nil || err.Error() != "used by the profiles High - PS5" {
		t.Fatalf("format in use: %v", err)
	}
	repo.use["highps5"] = 1
	if err := s.DeleteProfile(ctx, "highps5"); err == nil || err.Error() != "used by 1 series, movies or artists; give them another profile first" {
		t.Fatalf("profile in use: %v", err)
	}
	repo.use["highps5"] = 0
	if err := s.DeleteProfile(ctx, "highps5"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFormat(ctx, f.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Format(f.Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted format still served: %v", err)
	}
	if len(repo.rows[store.TableFormats]) != 0 || len(repo.rows[store.TableProfiles]) != 0 {
		t.Fatalf("rows left %v", repo.rows)
	}
}

func TestService_LoadReadsTheRepo(t *testing.T) {
	ctx := context.Background()
	repo := newRepo()
	s, _ := newService(t, repo, nil)
	if _, err := s.CreateFormat(ctx, ps5Format); err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.rows[store.TableFormats]["audiops5safe"]; !ok {
		t.Fatal("format not saved")
	}
	again, _ := newService(t, repo, nil)
	if rows := again.Formats(); len(rows) != 1 || rows[0].Key != "audiops5safe" {
		t.Fatalf("reloaded %+v", rows)
	}
	repo.listErr = errors.New("disk gone")
	if err := New(Deps{Repo: repo}).Load(ctx); err == nil || err.Error() != "disk gone" {
		t.Fatalf("list error: %v", err)
	}
}

func TestService_LogsDocumentsLeftOut(t *testing.T) {
	t.Cleanup(decide.WithCatalog(nil))
	repo := newRepo()
	raw, _ := json.Marshal(decide.FormatDoc{Key: "broken", Name: "Broken", Conditions: []decide.ConditionDoc{{Kind: decide.CondTitle, Pattern: "("}}})
	repo.rows[store.TableFormats]["broken"] = store.QualityObject{Key: "broken", Doc: raw}
	var buf bytes.Buffer
	s := New(Deps{Repo: repo, Log: slog.New(slog.NewTextHandler(&buf, nil))})
	if err := s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "left out") || !strings.Contains(buf.String(), "key=broken") {
		t.Fatalf("log %q", buf.String())
	}
	if err := New(Deps{Repo: repo}).Load(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestService_TestBoxLimits(t *testing.T) {
	s, _ := newService(t, newRepo(), nil)
	var inv InvalidError
	draft := decide.FormatDoc{Conditions: []decide.ConditionDoc{{Kind: decide.CondResolution, Values: []string{"999"}}}}
	if _, err := s.Test(TestRequest{Titles: []string{"x"}, Format: &draft}); !errors.As(err, &inv) || len(inv.Errs) != 1 {
		t.Fatalf("draft with one problem: %v", err)
	}
	titles := make([]string, maxTestTitles+1)
	for i := range titles {
		titles[i] = "Movie.2020.1080p.WEB-DL-GRP"
	}
	if res, _ := s.Test(TestRequest{Titles: titles}); len(res) != maxTestTitles {
		t.Fatalf("results %d", len(res))
	}
}

func TestEditorChoices(t *testing.T) {
	c := EditorChoices()
	video, music := c.Qualities[domain.KindTV], c.Qualities[domain.KindMusic]
	if len(video) != len(decide.Ladder()) || video[0].ID != decide.Ladder()[0].ID || !video[0].BelowFloor {
		t.Fatalf("video %+v", video[:2])
	}
	if len(music) != len(decide.AudioLadder()) || music[0].ID != decide.AudioLadder()[0].ID {
		t.Fatalf("music %+v", music[:2])
	}
}

func TestHTTP_DefaultsDuplicateAndUpdate(t *testing.T) {
	s, _ := newService(t, newRepo(), nil)
	mux := http.NewServeMux()
	s.Mount(mounter{mux})
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	if w := do("GET", "/api/v1/qualityprofiles", ""); w.Code != 200 || strings.Contains(w.Body.String(), `"isDefault":true`) {
		t.Fatalf("defaults without a kind: %d %s", w.Code, w.Body)
	}
	if w := do("GET", "/api/v1/qualityprofiles?kind=movie", ""); w.Code != 200 || strings.Count(w.Body.String(), `"isDefault":true`) != 1 {
		t.Fatalf("one default for movies: %d %s", w.Code, w.Body)
	}
	if w := do("POST", "/api/v1/qualityprofiles/hd1080p/duplicate", `{"name":"Mine"}`); w.Code != 201 || !strings.Contains(w.Body.String(), `"key":"mine"`) {
		t.Fatalf("named duplicate: %d %s", w.Code, w.Body)
	}
	if w := do("POST", "/api/v1/qualityprofiles/hd1080p/duplicate", `{"nope":1}`); w.Code != 400 {
		t.Fatalf("bad body: %d %s", w.Code, w.Body)
	}
	do("POST", "/api/v1/customformats", `{"name":"PS5","conditions":[{"kind":"audio","values":["aac"]}]}`)
	if w := do("PUT", "/api/v1/customformats/ps5", `{"name":"PS5 v2","conditions":[{"kind":"audio","values":["eac3"]}]}`); w.Code != 200 ||
		!strings.Contains(w.Body.String(), `"name":"PS5 v2"`) {
		t.Fatalf("update: %d %s", w.Code, w.Body)
	}
	if w := do("PUT", "/api/v1/customformats/ps5", `{`); w.Code != 400 {
		t.Fatalf("bad update body: %d %s", w.Code, w.Body)
	}
}
