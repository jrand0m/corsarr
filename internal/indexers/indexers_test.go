package indexers

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/instances"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugins/declarative"
	"github.com/jrand0m/corsarr/internal/store"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type repo struct {
	mu sync.Mutex
	m  map[domain.ObjectKey]store.ManagedIndexer
}

func (r *repo) List(context.Context) ([]store.ManagedIndexer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []store.ManagedIndexer
	for _, v := range r.m {
		out = append(out, v)
	}
	return out, nil
}
func (r *repo) Get(_ context.Context, k domain.ObjectKey) (store.ManagedIndexer, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.m[k]
	return v, ok, nil
}
func (r *repo) Save(_ context.Context, m store.ManagedIndexer, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[m.Key] = m
	return nil
}
func (r *repo) Delete(_ context.Context, k domain.ObjectKey) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.m[k]
	delete(r.m, k)
	return ok, nil
}

type catalog struct{ rev int }

func (c *catalog) Entries() []declarative.Entry {
	return []declarative.Entry{{ID: "fake", Valid: true}}
}
func (c *catalog) Load(string) []declarative.Entry { c.rev++; return c.Entries() }
func (c *catalog) Revision() int                   { return c.rev }
func (c *catalog) Declarative(id string) bool      { return id == "fake" }

type healthFake struct {
	mu      sync.Mutex
	m       map[domain.IntegrationRef]domain.IntegrationHealth
	blocked map[domain.ObjectKey]bool
}

func (h *healthFake) Report(ref domain.IntegrationRef, o health.Observation) domain.IntegrationHealth {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := health.Next(h.m[ref], o, time.Now())
	n.Ref = ref
	h.m[ref] = n
	return n
}
func (h *healthFake) Get(ref domain.IntegrationRef) (domain.IntegrationHealth, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	v, ok := h.m[ref]
	return v, ok
}
func (h *healthFake) Blocked(ref domain.IntegrationRef) bool { return h.blocked[ref.Key] }

type queries struct {
	mu sync.Mutex
	n  map[domain.ObjectKey]int
}

func (q *queries) Queries(_ context.Context, k domain.ObjectKey, _ time.Time) (int, time.Time, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.n[k], time.Time{}, nil
}
func (q *queries) RecordQuery(_ context.Context, k domain.ObjectKey, _ time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.n[k]++
	return nil
}

type kv map[string]string

func (k kv) Get(_ context.Context, key string) (string, bool, error) {
	v, ok := k[key]
	return v, ok, nil
}
func (k kv) Set(_ context.Context, key, v string) error { k[key] = v; return nil }

type httpFor struct{}

func (httpFor) For(domain.IntegrationRef) plugin.HTTPDoer { return http.DefaultClient }

// fakeIndexer answers from memory; its settings steer failures.
type fakeIndexer struct {
	key  domain.ObjectKey
	fail string
	rels []domain.Release
}

func (f fakeIndexer) Test(context.Context) error {
	if f.fail != "" {
		return plugin.Errorf(plugin.FailureClass(f.fail), "fake failure")
	}
	return nil
}
func (f fakeIndexer) Caps(context.Context) (plugin.Caps, error) {
	return plugin.Caps{Modes: []domain.SearchMode{domain.SearchText, domain.SearchMovie}, IDParams: []string{"imdbid"},
		Categories: map[int]string{2000: "Movies", 2040: "Movies/HD"}}, nil
}
func (f fakeIndexer) Search(context.Context, domain.SearchRequest) ([]domain.Release, error) {
	if f.fail != "" {
		return nil, plugin.Errorf(plugin.FailureClass(f.fail), "fake search failure")
	}
	return f.rels, nil
}
func (f fakeIndexer) Feed(context.Context, domain.MediaKind, string) ([]domain.Release, string, error) {
	return nil, "", nil
}
func (f fakeIndexer) Resolve(_ context.Context, r domain.Release) (plugin.Payload, error) {
	if strings.HasPrefix(r.DownloadURL, "magnet:") {
		return plugin.Payload{Kind: "magnet", URL: r.DownloadURL}, nil
	}
	if r.DownloadURL == "" {
		return plugin.Payload{}, plugin.Errorf(plugin.ClassParse, "no link")
	}
	return plugin.Payload{Kind: "torrent", Content: []byte("d4:infoe"), Name: r.Title}, nil
}

func rel(key domain.ObjectKey, id, hash string, at time.Time, link string) domain.Release {
	return domain.Release{ID: id, Title: "Release " + id, InfoHash: hash, PublishedAt: at, Size: 1 << 20, Seeders: 3,
		Categories: []int{2040}, DownloadURL: link, Flags: []string{"freeleech"}, ExternalIDs: map[string]string{"imdb": "tt01"},
		Indexers: []domain.IndexerRef{{Key: key, Name: string(key)}}}
}

var t0 = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

func registry() *plugin.Registry {
	reg := plugin.NewRegistry()
	reg.MustRegister(plugin.Registration{
		Manifest: plugin.Manifest{ID: "fake", Kind: plugin.KindIndexer, Version: "1", APIVersion: plugin.APIVersion, Name: "Fake", Delivery: "declarative",
			Settings: []plugin.Field{
				{Key: "baseUrl", Type: plugin.FieldURL, Label: "Site", Required: true},
				{Key: "cookie", Type: plugin.FieldSecret, Label: "Cookie"},
				{Key: "fail", Type: plugin.FieldText, Label: "Fail"},
				{Key: "priority", Type: plugin.FieldNumber, Label: "Priority", Default: 25, Min: ptr(1.0), Max: ptr(50.0)},
				{Key: "free", Type: plugin.FieldBool, Label: "Free"},
				{Key: "mode", Type: plugin.FieldEnum, Label: "Mode", Options: []string{"a", "b"}},
				{Key: "tags", Type: plugin.FieldList, Label: "Tags"},
			}},
		New: func(inst plugin.Instance) (any, error) {
			if inst.Settings.String("fail") == "build" {
				return nil, plugin.Errorf(plugin.ClassConfig, "cannot build")
			}
			if inst.Settings.String("fail") == "panic" {
				return panicky{}, nil
			}
			return fakeIndexer{key: inst.Key, fail: inst.Settings.String("fail"),
				rels: []domain.Release{rel(inst.Key, "r1-"+string(inst.Key), "aaaa", t0, "magnet:?xt=urn:btih:aaaa"), rel(inst.Key, "r2", "", t0.Add(time.Hour), "https://x/r2.torrent")}}, nil
		},
	})
	reg.MustRegister(plugin.Registration{Manifest: plugin.Manifest{ID: "notifier", Kind: plugin.KindNotifier, APIVersion: plugin.APIVersion},
		New: func(plugin.Instance) (any, error) { return struct{}{}, nil }})
	reg.MustRegister(plugin.Registration{Manifest: plugin.Manifest{ID: "odd", Kind: plugin.KindIndexer, APIVersion: plugin.APIVersion},
		New: func(plugin.Instance) (any, error) { return struct{}{}, nil }})
	return reg
}

type panicky struct{ fakeIndexer }

func (panicky) Test(context.Context) error { panic("boom") }

func ptr(f float64) *float64 { return &f }

type rig struct {
	s       *Service
	repo    *repo
	health  *healthFake
	queries *queries
	kv      kv
	cfg     *config.Config
	running []instances.Running
	applied int
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{repo: &repo{m: map[domain.ObjectKey]store.ManagedIndexer{}}, health: &healthFake{m: map[domain.IntegrationRef]domain.IntegrationHealth{}, blocked: map[domain.ObjectKey]bool{}},
		queries: &queries{n: map[domain.ObjectKey]int{}}, kv: kv{}}
	r.cfg = &config.Config{Instances: map[string]map[domain.ObjectKey]config.Instance{Section: {
		"fromcfg": {Section: Section, Key: "fromcfg", Type: "fake", Name: "From config", Enabled: true,
			Settings: plugin.Settings{"baseUrl": "https://cfg.test/", "cookie": "secret!", "priority": 5}},
	}}}
	reg := registry()
	r.s = New(Deps{Repo: r.repo, Catalog: &catalog{}, Plugins: reg, UserDir: "/data/plugins/definitions", Config: func() *config.Config { return r.cfg },
		Running: func() []instances.Running { return r.running }, HTTP: httpFor{}, Plain: http.DefaultClient, Health: r.health, Queries: r.queries, KV: r.kv,
		Apply: func() {
			r.applied++
			r.running = nil
			for key, inst := range r.s.Merged(r.cfg).Instances[Section] {
				if !inst.Enabled {
					continue
				}
				impl, err := reg.Get(inst.Type)
				if !err {
					continue
				}
				v, _ := impl.New(plugin.Instance{Key: key, Settings: inst.Settings})
				r.running = append(r.running, instances.Running{Key: key, Name: inst.Name, PluginID: inst.Type, Kind: plugin.KindIndexer, Settings: inst.Settings, Impl: v})
			}
		},
		Now: func() time.Time { return t0 }, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), BasePath: func() string { return "/base" }})
	if err := r.s.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.s.d.Apply()
	return r
}

func strp(s string) *string { return &s }
func boolp(b bool) *bool    { return &b }

// ── management ───────────────────────────────────────────────────────────────

func TestCreateTestsBeforeEnablingAndStoresSecretsApart(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_, err := r.s.Create(ctx, Input{Key: "bad", Plugin: "fake", Settings: map[string]any{"baseUrl": "https://x.test/", "fail": "auth"}})
	var inv *InvalidError
	if !errors.As(err, &inv) || inv.Test == nil || inv.Test.Class != "auth" {
		t.Fatalf("failing indexer enabled: %v", err)
	}
	if _, ok, _ := r.repo.Get(ctx, "bad"); ok {
		t.Fatal("failing indexer saved")
	}
	row, err := r.s.Create(ctx, Input{Key: "site", Plugin: "fake", Name: strp("Site"),
		Settings: map[string]any{"baseUrl": "https://x.test/", "cookie": "sid=1", "priority": "10", "free": "true", "mode": "a", "tags": []any{"x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if row.Source != "ui" || !row.Running || row.Priority != 10 || row.SecretsSet[0] != "cookie" || row.Settings["cookie"] != nil ||
		row.Health == nil || row.Health.Status != domain.HealthOK || row.Torznab != "/base/torznab/site/api" || row.PluginName != "Fake" {
		t.Errorf("row %+v", row)
	}
	m, _, _ := r.repo.Get(ctx, "site")
	if m.Secrets["cookie"] != "sid=1" || m.Settings["cookie"] != nil {
		t.Errorf("stored %+v", m)
	}
	if _, err := r.s.Create(ctx, Input{Key: "site", Plugin: "fake"}); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate: %v", err)
	}
	if _, err := r.s.Create(ctx, Input{Key: "fromcfg", Plugin: "fake"}); !errors.Is(err, ErrExists) {
		t.Errorf("config key: %v", err)
	}
	for _, key := range []domain.ObjectKey{"Bad Key", "all"} {
		if _, err := r.s.Create(ctx, Input{Key: key, Plugin: "fake"}); !errors.As(err, &inv) {
			t.Errorf("key %q: %v", key, err)
		}
	}
}

func TestUpdateKeepsSecretsAndRefusesConfigInstances(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if _, err := r.s.Create(ctx, Input{Key: "site", Plugin: "fake", Settings: map[string]any{"baseUrl": "https://x.test/", "cookie": "sid=1"}}); err != nil {
		t.Fatal(err)
	}
	row, err := r.s.Update(ctx, "site", Input{Enabled: boolp(false), Settings: map[string]any{"baseUrl": "https://y.test/", "cookie": ""}})
	if err != nil || row.Enabled || row.Running || row.Health.Status != domain.HealthDisabled {
		t.Fatalf("%+v %v", row, err)
	}
	if m, _, _ := r.repo.Get(ctx, "site"); m.Secrets["cookie"] != "sid=1" || m.Settings["baseUrl"] != "https://y.test/" {
		t.Errorf("secret lost or setting not changed: %+v", m)
	}
	if _, err := r.s.Update(ctx, "site", Input{Name: strp("Renamed")}); err != nil {
		t.Errorf("settings-less update: %v", err)
	}
	if _, err := r.s.Update(ctx, "fromcfg", Input{}); !errors.Is(err, ErrManaged) {
		t.Errorf("config instance edited: %v", err)
	}
	if _, err := r.s.Update(ctx, "nope", Input{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
	if err := r.s.Delete(ctx, "fromcfg"); !errors.Is(err, ErrManaged) {
		t.Error(err)
	}
	if err := r.s.Delete(ctx, "site"); err != nil {
		t.Fatal(err)
	}
	if err := r.s.Delete(ctx, "site"); !errors.Is(err, ErrNotFound) {
		t.Error(err)
	}
}

func TestListMergesConfigAndUIWithoutSecrets(t *testing.T) {
	r := newRig(t)
	_ = r.repo.Save(context.Background(), store.ManagedIndexer{Key: "fromcfg", PluginID: "fake", Name: "Shadowed"}, t0)
	_ = r.repo.Save(context.Background(), store.ManagedIndexer{Key: "zz", PluginID: "gone", Name: "Missing plugin"}, t0)
	_ = r.s.refresh(context.Background())
	rows, _ := r.s.List(context.Background())
	if len(rows) != 2 || rows[0].Key != "fromcfg" || rows[0].Source != "config" || rows[0].Name != "From config" ||
		rows[0].Settings["cookie"] != nil || rows[0].Settings[RevisionKey] != nil || rows[1].PluginName != "gone" {
		t.Fatalf("%+v", rows)
	}
	if _, err := r.s.Get(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Error(err)
	}
}

func TestMergeIsPureAndConfigWins(t *testing.T) {
	cfg := &config.Config{}
	out := Merge(cfg, []store.ManagedIndexer{{Key: "a", PluginID: "fake", Settings: map[string]any{"x": 1}, Secrets: map[string]string{"cookie": "c"}}},
		func(id string) bool { return id == "fake" }, 7)
	inst := out.Instances[Section]["a"]
	if inst.Settings["cookie"] != "c" || inst.Settings[RevisionKey] != "7" || cfg.Instances != nil {
		t.Errorf("%+v", inst)
	}
}

func TestValidate(t *testing.T) {
	m := registry()
	reg, _ := m.Get("fake")
	_, errs := Validate(reg.Manifest, map[string]any{"priority": "99", "free": "maybe", "mode": "c", "baseUrl": "ftp://x", "bogus": 1}, nil)
	if len(errs) != 5 {
		t.Errorf("%v", errs)
	}
	_, errs = Validate(reg.Manifest, map[string]any{"priority": "x"}, nil)
	if len(errs) != 2 || errs[0].Error() == "" {
		t.Errorf("%v", errs)
	}
	v, errs := Validate(reg.Manifest, map[string]any{"baseUrl": "https://ok.test", "priority": 0.5, "free": true, "queryLimit": 10, "queryLimitPer": "hour"}, map[string]string{"cookie": "old"})
	if len(errs) != 1 || v.Secrets["cookie"] != "old" || v.Settings["queryLimit"] != 10.0 {
		t.Errorf("%v %+v", errs, v)
	}
	req := plugin.Manifest{Settings: []plugin.Field{{Key: "token", Type: plugin.FieldSecret, Required: true}}}
	if _, errs := Validate(req, nil, nil); len(errs) != 1 {
		t.Error(errs)
	}
}

func TestTests(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, err := r.s.Test(ctx, "fromcfg")
	if err != nil || !res.OK || res.Caps == nil {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := r.s.Test(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Error(err)
	}
	for fail, class := range map[string]string{"build": "config", "panic": "server_error", "timeout": "timeout"} {
		res, err := r.s.TestDraft(ctx, Input{Plugin: "fake", Settings: map[string]any{"baseUrl": "https://x.test/", "fail": fail}})
		if err != nil || res.OK || res.Class != class {
			t.Errorf("%s: %+v %v", fail, res, err)
		}
	}
	if _, ok := r.health.Get(ref("draft")); ok {
		t.Error("draft test recorded health")
	}
	var inv *InvalidError
	if _, err := r.s.TestDraft(ctx, Input{Plugin: "notifier"}); !errors.As(err, &inv) {
		t.Error(err)
	}
	if _, err := r.s.TestDraft(ctx, Input{Plugin: "fake", Settings: map[string]any{"bogus": 1}}); !errors.As(err, &inv) {
		t.Error(err)
	}
	if res := r.s.test(ctx, config.Instance{Key: "odd", Type: "odd"}, false); res.Class != "config" {
		t.Errorf("non-indexer: %+v", res)
	}
	if res := r.s.test(ctx, config.Instance{Key: "x", Type: "gone"}, false); res.Class != "config" {
		t.Errorf("missing plugin: %+v", res)
	}
	if f := failed(context.Canceled); f.Class != "timeout" {
		t.Error(f)
	}
	if f := failed(errors.New("dial")); f.Class != "unreachable" {
		t.Error(f)
	}
	if (&InvalidError{}).Error() == "" {
		t.Error("empty message")
	}
}

func TestReloadDefinitionsRebuilds(t *testing.T) {
	r := newRig(t)
	before := r.applied
	entries := r.s.ReloadDefinitions()
	if len(entries) != 1 || r.applied != before+1 {
		t.Error("reload did not re-apply")
	}
	if r.s.Merged(r.cfg).Instances[Section]["fromcfg"].Settings[RevisionKey] != "2" {
		t.Error("revision not carried to declarative instances")
	}
}

// ── HTTP API ─────────────────────────────────────────────────────────────────

type mux struct{ *http.ServeMux }

func (m mux) Handle(p string, h http.HandlerFunc) { m.ServeMux.HandleFunc(p, h) }
func (m mux) HandleAPIKey(p string, h, deny http.HandlerFunc) {
	m.ServeMux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apikey") == "key" {
			h(w, r)
			return
		}
		deny(w, r)
	})
}

func call(t *testing.T, h http.Handler, method, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestHTTPAPI(t *testing.T) {
	r := newRig(t)
	m := mux{http.NewServeMux()}
	r.s.Mount(m)
	steps := []struct {
		method, path, body string
		code               int
		has                string
	}{
		{"GET", "/api/v1/indexers", "", 200, `"fromcfg"`},
		{"POST", "/api/v1/indexers", `{"key":"site","plugin":"fake","settings":{"baseUrl":"https://x.test/"}}`, 201, `"source":"ui"`},
		{"POST", "/api/v1/indexers", `{"key":"site","plugin":"fake"}`, 409, "exists"},
		{"POST", "/api/v1/indexers", `{"key":"bad","plugin":"fake","settings":{"baseUrl":"https://x.test/","fail":"auth"}}`, 422, "test_failed"},
		{"POST", "/api/v1/indexers", `{"key":"bad","plugin":"fake","settings":{"priority":"x"}}`, 422, `"field":"baseUrl"`},
		{"POST", "/api/v1/indexers", `{nope`, 400, "bad_request"},
		{"GET", "/api/v1/indexers/site", "", 200, `"key":"site"`},
		{"GET", "/api/v1/indexers/nope", "", 404, "not_found"},
		{"PUT", "/api/v1/indexers/site", `{"enabled":false}`, 200, `"enabled":false`},
		{"PUT", "/api/v1/indexers/fromcfg", `{}`, 409, "managed"},
		{"PUT", "/api/v1/indexers/site", `{nope`, 400, "bad_request"},
		{"POST", "/api/v1/indexers/site/test", "", 200, `"ok":true`},
		{"POST", "/api/v1/indexers/nope/test", "", 404, "not_found"},
		{"POST", "/api/v1/indexers/test", `{"plugin":"fake","settings":{"baseUrl":"https://x.test/"}}`, 200, `"ok":true`},
		{"POST", "/api/v1/indexers/test", `{"plugin":"zzz"}`, 422, "invalid"},
		{"POST", "/api/v1/indexers/test", `{nope`, 400, "bad_request"},
		{"GET", "/api/v1/indexers/definitions", "", 200, `"directory"`},
		{"POST", "/api/v1/indexers/definitions/reload", "", 200, `"items"`},
		{"GET", "/api/v1/indexers/torznab", "", 200, `"enabled":false`},
		{"PUT", "/api/v1/indexers/torznab", `{"enabled":true}`, 200, `"all":"/base/torznab/all/api"`},
		{"PUT", "/api/v1/indexers/torznab", `{nope`, 400, "bad_request"},
		{"DELETE", "/api/v1/indexers/site", "", 204, ""},
		{"DELETE", "/api/v1/indexers/site", "", 404, "not_found"},
	}
	for _, s := range steps {
		code, body := call(t, m, s.method, s.path, s.body)
		if code != s.code || !strings.Contains(body, s.has) {
			t.Errorf("%s %s = %d %s", s.method, s.path, code, body)
		}
	}
	rec := httptest.NewRecorder()
	writeErr(rec, errors.New("db down"))
	if rec.Code != 500 {
		t.Error(rec.Code)
	}
}

// ── Torznab endpoint ─────────────────────────────────────────────────────────

func TestTorznabEndpoint(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if _, err := r.s.Create(ctx, Input{Key: "second", Plugin: "fake", Settings: map[string]any{"baseUrl": "https://two.test/"}}); err != nil {
		t.Fatal(err)
	}
	m := mux{http.NewServeMux()}
	r.s.Mount(m)
	if code, body := call(t, m, "GET", "/torznab/all/api?t=caps&apikey=key", ""); code != 404 || !strings.Contains(body, `code="202"`) {
		t.Errorf("off by default: %d %s", code, body)
	}
	_ = r.s.SetTorznab(ctx, true)
	if code, body := call(t, m, "GET", "/torznab/all/api?t=caps", ""); code != 401 || !strings.Contains(body, `code="100"`) {
		t.Errorf("no key: %d %s", code, body)
	}
	code, body := call(t, m, "GET", "/torznab/fromcfg/api?t=caps&apikey=key", "")
	if code != 200 || !strings.Contains(body, `<movie-search available="yes" supportedParams="q,imdbid">`) || !strings.Contains(body, `<subcat id="2040" name="Movies/HD">`) {
		t.Errorf("caps %d %s", code, body)
	}
	code, body = call(t, m, "GET", "/torznab/all/api?t=movie&imdbid=0111161&cat=2000&apikey=key", "")
	var doc struct {
		Items []struct {
			Title string `xml:"title"`
			Link  string `xml:"link"`
			Attrs []struct {
				Name  string `xml:"name,attr"`
				Value string `xml:"value,attr"`
			} `xml:"attr"`
		} `xml:"channel>item"`
	}
	if err := xml.Unmarshal([]byte(body), &doc); err != nil || code != 200 {
		t.Fatalf("%d %v %s", code, err, body)
	}
	// r2 shares an id across both indexers; r1 differs per indexer but shares the hash.
	if len(doc.Items) != 2 || r.queries.n["fromcfg"] != 1 || r.queries.n["second"] != 1 {
		t.Fatalf("items %d queries %v", len(doc.Items), r.queries.n)
	}
	if doc.Items[0].Title != "Release r2" || !strings.HasPrefix(doc.Items[0].Link, "http://example.com/base/torznab/all/dl?") {
		t.Errorf("first item %+v", doc.Items[0])
	}
	link, _ := url.Parse(doc.Items[0].Link)
	code, body = call(t, m, "GET", strings.TrimPrefix(link.RequestURI(), "/base"), "")
	if code != 200 || body != "d4:infoe" {
		t.Errorf("torrent download %d %q", code, body)
	}
	link2, _ := url.Parse(doc.Items[1].Link)
	req := httptest.NewRequest("GET", strings.TrimPrefix(link2.RequestURI(), "/base"), nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	if rec.Code != 302 || !strings.HasPrefix(rec.Header().Get("Location"), "magnet:") {
		t.Errorf("magnet download %d", rec.Code)
	}
	for path, want := range map[string]int{
		"/torznab/all/dl?id=nope&apikey=key":           404,
		"/torznab/nope/api?t=search&apikey=key":        404,
		"/torznab/all/api?t=bogus&apikey=key":          400,
		"/torznab/fromcfg/api?t=search&q=x&apikey=key": 200,
	} {
		if code, body := call(t, m, "GET", path, ""); code != want {
			t.Errorf("%s = %d %s", path, code, body)
		}
	}
	_ = r.s.SetTorznab(ctx, false)
	if code, _ := call(t, m, "GET", strings.TrimPrefix(link.RequestURI(), "/base"), ""); code != 404 {
		t.Error("download while off", code)
	}
}

func TestTorznabSkipsBlockedLimitedAndReportsParseFailures(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_, _ = r.s.Create(ctx, Input{Key: "limited", Plugin: "fake", Settings: map[string]any{"baseUrl": "https://l.test/", "queryLimit": 1, "queryLimitPer": "hour"}})
	_, _ = r.s.Create(ctx, Input{Key: "broken", Plugin: "fake", Enabled: boolp(false), Settings: map[string]any{"baseUrl": "https://b.test/", "fail": "parse"}})
	_, _ = r.s.Update(ctx, "broken", Input{Enabled: boolp(true), Settings: map[string]any{"baseUrl": "https://b.test/"}})
	r.queries.n["limited"] = 1
	r.health.blocked["fromcfg"] = true
	// make "broken" fail at search time without failing its test
	for i, run := range r.running {
		if run.Key == "broken" {
			r.running[i].Impl = fakeIndexer{fail: "parse"}
		}
	}
	rels := r.s.fanOut(ctx, r.s.targets(AllID), Request{Search: domain.SearchRequest{Mode: domain.SearchText, Text: "x"}, Limit: 100})
	if len(rels) != 0 {
		t.Errorf("blocked or limited indexer searched: %d", len(rels))
	}
	if h, _ := r.health.Get(ref("broken")); h.LastErrorClass != "parse" {
		t.Errorf("parse failure not reported: %+v", h)
	}
	r.running = append(r.running, instances.Running{Key: "gone", Impl: struct{}{}})
	if len(r.s.targets("gone")) != 0 {
		t.Error("non-indexer targeted")
	}
	g := grab{indexer: "broken", rel: domain.Release{ID: "x"}}
	r.s.grabs.put("tok", g)
	_ = r.s.SetTorznab(ctx, true)
	m := mux{http.NewServeMux()}
	r.s.Mount(m)
	if code, _ := call(t, m, "GET", "/torznab/broken/dl?id=tok&apikey=key", ""); code != 502 {
		t.Errorf("resolve failure = %d", code)
	}
}

func TestParseRequest(t *testing.T) {
	q := url.Values{"t": {"tvsearch"}, "q": {" Show "}, "season": {"2"}, "ep": {"5"}, "tvdbid": {"77"}, "imdbid": {"tt9"}, "cat": {"5000, 5040"}, "limit": {"9999"}, "offset": {"10"}}
	r, err := ParseRequest(q)
	if err != nil || r.Search.Kind != domain.KindTV || r.Search.Season != 2 || r.Search.Episodes[0] != 5 || r.Search.IDs["tvdb"] != "77" ||
		r.Search.IDs["imdb"] != "tt9" || r.Limit != 500 || r.Offset != 10 || len(r.Cats) != 2 || r.Search.Text != "Show" {
		t.Fatalf("%+v %v", r, err)
	}
	if r, _ := ParseRequest(url.Values{"t": {"search"}, "cat": {"3000"}, "year": {"1999"}}); r.Search.Kind != domain.KindMusic || r.Search.Year != 1999 {
		t.Errorf("%+v", r)
	}
	for _, c := range []string{"2000", "7000", "5000", "8000"} {
		if r, _ := ParseRequest(url.Values{"t": {"search"}, "cat": {c}}); c != "8000" && r.Search.Kind == "" {
			t.Errorf("cat %s", c)
		}
	}
	for _, bad := range []url.Values{{"t": {"x"}}, {"t": {"search"}, "cat": {"a"}}, {"t": {"search"}, "limit": {"-1"}}, {"t": {"search"}, "offset": {"x"}},
		{"t": {"search"}, "season": {"x"}}, {"t": {"search"}, "ep": {"x"}}, {"t": {"search"}, "year": {"x"}}} {
		if _, err := ParseRequest(bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

func TestDocsAndHelpers(t *testing.T) {
	caps := CapsDoc("x", []plugin.Caps{{Modes: []domain.SearchMode{domain.SearchTV}, IDParams: []string{"tvdbid"}, Categories: map[int]string{5000: "", 9999: "Odd"}}})
	for _, want := range []string{`<tv-search available="yes" supportedParams="q,season,ep,tvdbid">`, `<search available="no"`, `name="TV"`, `id="9000"`} {
		if !strings.Contains(string(caps), want) {
			t.Errorf("caps missing %s:\n%s", want, caps)
		}
	}
	doc := ResultsDoc("x", []domain.Release{{ID: "1", Title: "T", DownloadURL: "magnet:?a"}}, func(domain.Release) string { return "L" })
	if !strings.Contains(string(doc), `name="magneturl"`) || strings.Contains(string(doc), "pubDate>Mon") {
		t.Errorf("%s", doc)
	}
	if safeName(`a/b:"c`) != "a_b__c" || safeName("") != "download" {
		t.Error("safeName")
	}
	rec := httptest.NewRecorder()
	denyTorznab(rec, nil)
	if rec.Code != 401 {
		t.Error(rec.Code)
	}
	c := newGrabCache(1)
	c.put("a", grab{})
	c.put("b", grab{})
	c.put("b", grab{})
	if _, ok := c.get("a"); ok {
		t.Error("cache unbounded")
	}
	if first(nil) != "" {
		t.Error("first")
	}
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "media.example")
	if got := (&Service{d: Deps{BasePath: func() string { return "/b/" }}}).publicBase(req, "all"); got != "https://media.example/b/torznab/all" {
		t.Error(got)
	}
}
