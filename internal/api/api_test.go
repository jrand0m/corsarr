package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/events"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/scheduler"
	"github.com/jrand0m/corsarr/internal/store"
)

type fakeJobs struct{ ran []string }

func (f *fakeJobs) List() []domain.JobState {
	return []domain.JobState{{Name: "rss", Enabled: true, Interval: 15 * time.Minute}}
}

func (f *fakeJobs) RunNow(name string) error {
	if name != "rss" {
		return scheduler.ErrUnknownJob
	}
	f.ran = append(f.ran, name)
	return nil
}

type env struct {
	srv  *httptest.Server
	st   *store.Store
	bus  *events.Bus
	jobs *fakeJobs
	s    *Server
	c    *http.Client
}

var ui = fstest.MapFS{
	"index.html":    {Data: []byte("<html><head><title>corsarr</title></head></html>")},
	"assets/app.js": {Data: []byte("console.log(1)")},
}

func newEnv(t *testing.T, mutate func(*Deps)) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := EnsureAPIKey(ctx, st.Auth()); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAPIKey(ctx, st.Auth()); err != nil { // idempotent
		t.Fatal(err)
	}
	cfg, err := config.Load(config.Inputs{Environ: []string{"CORSARR_MEDIAKINDS_BOOK_MODE=shadow"}, Exists: func(string) bool { return false }}, plugin.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	bus := events.New(events.WithHistory(st.History(), nil))
	t.Cleanup(bus.Close)
	jobs := &fakeJobs{}
	reg := health.New(bus, nil, nil)
	reg.Fail(domain.IntegrationRef{Type: "indexer", Key: "knaben"}, health.ClassTimeout, "slow")
	d := Deps{
		Version: "test", Started: time.Now().Add(-time.Minute), AuthMode: "password",
		Auth: st.Auth(), History: st.History(), Jobs: jobs, Health: reg, Plugins: plugin.NewRegistry(),
		Config: func() *config.Config { return cfg }, Events: events.NewHub(bus, time.Hour), UI: ui,
		DBSize: st.Size, Ready: func(context.Context) error { return nil },
	}
	if mutate != nil {
		mutate(&d)
	}
	s, err := New(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return &env{srv: srv, st: st, bus: bus, jobs: jobs, s: s, c: &http.Client{Jar: jar}}
}

func (e *env) do(t *testing.T, method, path, body string, hdr ...string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := e.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, m
}

func errCode(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func (e *env) key(t *testing.T) string {
	k, _, _ := e.st.Auth().APIKey(context.Background())
	return k
}

func TestHealthIsOpenAndReflectsReadiness(t *testing.T) {
	e := newEnv(t, nil)
	if code, m := e.do(t, "GET", "/api/v1/health", ""); code != 200 || m["status"] != "ok" || m["version"] != "test" {
		t.Fatalf("%d %v", code, m)
	}
	e2 := newEnv(t, func(d *Deps) { d.Ready = func(context.Context) error { return errors.New("db locked") } })
	if code, m := e2.do(t, "GET", "/api/v1/health", ""); code != 503 || m["status"] != "unavailable" {
		t.Fatalf("%d %v", code, m)
	}
}

func TestFirstRunFlowAndSessionCookie(t *testing.T) {
	e := newEnv(t, nil)
	if _, m := e.do(t, "GET", "/api/v1/firstrun", ""); m["required"] != true {
		t.Fatalf("%v", m)
	}
	if code, m := e.do(t, "GET", "/api/v1/system/status", ""); code != 401 || errCode(m) != "setup_required" {
		t.Fatalf("before setup: %d %v", code, m)
	}
	if code, m := e.do(t, "POST", "/api/v1/auth/setup", `{"username":"admin","password":"short"}`); code != 400 || m["error"].(map[string]any)["field"] != "password" {
		t.Fatalf("short password: %d %v", code, m)
	}
	if code, m := e.do(t, "POST", "/api/v1/auth/setup", `{"username":" ","password":"longenough"}`); code != 400 || errCode(m) != "invalid" {
		t.Fatalf("blank user: %d %v", code, m)
	}
	if code, _ := e.do(t, "POST", "/api/v1/auth/setup", `{"username":"admin","password":"correct horse"}`); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	if code, m := e.do(t, "POST", "/api/v1/auth/setup", `{"username":"x","password":"correct horse"}`); code != 409 || errCode(m) != "already_set_up" {
		t.Fatalf("second setup: %d %v", code, m)
	}
	if code, _ := e.do(t, "GET", "/api/v1/system/status", ""); code != 200 {
		t.Fatalf("session from setup: %d", code)
	}
	if _, m := e.do(t, "GET", "/api/v1/auth/me", ""); m["method"] != "session" {
		t.Fatalf("me: %v", m)
	}
	e.do(t, "POST", "/api/v1/auth/logout", "")
	if code, m := e.do(t, "GET", "/api/v1/system/status", ""); code != 401 || errCode(m) != "unauthorized" {
		t.Fatalf("after logout: %d %v", code, m)
	}
	if code, m := e.do(t, "POST", "/api/v1/auth/login", `{"username":"admin","password":"wrong pass"}`); code != 401 || errCode(m) != "invalid_credentials" {
		t.Fatalf("bad login: %d %v", code, m)
	}
	if code, _ := e.do(t, "POST", "/api/v1/auth/login", `{"username":"admin","password":"correct horse"}`); code != 200 {
		t.Fatal("login")
	}
	if code, _ := e.do(t, "GET", "/api/v1/jobs", ""); code != 200 {
		t.Fatal("session after login")
	}
	if code, m := e.do(t, "POST", "/api/v1/auth/login", `{"user":"x"}`); code != 400 || errCode(m) != "bad_request" {
		t.Fatalf("unknown field: %d %v", code, m)
	}
}

func TestCookieAttributes(t *testing.T) {
	e := newEnv(t, nil)
	resp, err := http.Post(e.srv.URL+"/api/v1/auth/setup", "application/json", strings.NewReader(`{"username":"a","password":"12345678"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	c := resp.Cookies()[0]
	if c.Name != SessionCookie || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Secure {
		t.Fatalf("cookie: %+v", c)
	}
}

func TestAPIKeyHeaderQueryAndRotate(t *testing.T) {
	e := newEnv(t, nil)
	k := e.key(t)
	if code, _ := e.do(t, "GET", "/api/v1/plugins", "", "X-Api-Key", k); code != 200 {
		t.Fatal("header key")
	}
	if code, _ := e.do(t, "GET", "/api/v1/plugins?apikey="+k, ""); code != 200 {
		t.Fatal("query key")
	}
	if code, _ := e.do(t, "GET", "/api/v1/plugins", "", "X-Api-Key", "nope"); code != 401 {
		t.Fatal("bad key accepted")
	}
	_, m := e.do(t, "GET", "/api/v1/auth/apikey", "", "X-Api-Key", k)
	if m["apiKey"] != k {
		t.Fatal("get key")
	}
	_, m = e.do(t, "POST", "/api/v1/auth/apikey/rotate", "", "X-Api-Key", k)
	nk, _ := m["apiKey"].(string)
	if nk == "" || nk == k {
		t.Fatal("rotate")
	}
	if code, _ := e.do(t, "GET", "/api/v1/plugins", "", "X-Api-Key", k); code != 401 {
		t.Fatal("old key still valid")
	}
	if code, _ := e.do(t, "GET", "/api/v1/plugins", "", "X-Api-Key", nk); code != 200 {
		t.Fatal("new key")
	}
}

func TestAuthModesLocalAndProxy(t *testing.T) {
	local := newEnv(t, func(d *Deps) { d.AuthMode = "local" })
	if code, _ := local.do(t, "GET", "/api/v1/jobs", ""); code != 200 {
		t.Fatal("loopback must be trusted in local mode")
	}
	proxy := newEnv(t, func(d *Deps) { d.AuthMode = "proxy" })
	if code, _ := proxy.do(t, "GET", "/api/v1/jobs", ""); code != 401 {
		t.Fatal("proxy mode without header")
	}
	if code, _ := proxy.do(t, "GET", "/api/v1/jobs", "", "Remote-User", "mike"); code != 200 {
		t.Fatal("proxy header")
	}
	if code, _ := proxy.do(t, "GET", "/api/v1/jobs", "", "X-Forwarded-User", "mike"); code != 200 {
		t.Fatal("forwarded user header")
	}
	if privateRemote("8.8.8.8:1") || !privateRemote("10.0.0.2:5") || !privateRemote("::1") {
		t.Fatal("privateRemote")
	}
}

func TestKernelEndpoints(t *testing.T) {
	e := newEnv(t, nil)
	h := []string{"X-Api-Key", e.key(t)}
	code, m := e.do(t, "GET", "/api/v1/system/status", "", h...)
	if code != 200 || m["version"] != "test" || m["uptimeSeconds"].(float64) < 59 || m["rssBytes"].(float64) <= 0 ||
		m["goroutines"].(float64) < 1 || m["dbSizeBytes"].(float64) <= 0 || m["configSource"].(map[string]any)["kind"] != "env" {
		t.Fatalf("status: %v", m)
	}
	if kinds := m["enabledKinds"].([]any); len(kinds) != 4 || kinds[0] != "book" {
		t.Fatalf("kinds: %v", kinds)
	}
	if _, m := e.do(t, "GET", "/api/v1/health/integrations", "", h...); len(m["items"].([]any)) != 1 {
		t.Fatalf("integrations: %v", m)
	}
	if _, m := e.do(t, "GET", "/api/v1/jobs", "", h...); m["items"].([]any)[0].(map[string]any)["intervalSeconds"] != 900.0 {
		t.Fatalf("jobs: %v", m)
	}
	if code, _ := e.do(t, "POST", "/api/v1/jobs/rss/run", "", h...); code != 202 || len(e.jobs.ran) != 1 {
		t.Fatal("run job")
	}
	if code, m := e.do(t, "POST", "/api/v1/jobs/nope/run", "", h...); code != 404 || errCode(m) != "not_found" {
		t.Fatal("unknown job")
	}
	if code, m := e.do(t, "GET", "/api/v1/nope", "", h...); code != 404 || errCode(m) != "not_found" {
		t.Fatalf("unknown endpoint: %d %v", code, m)
	}
	if code, m := e.do(t, "GET", "/legacy/sonarr/api/v3/series", "", h...); code != 404 || errCode(m) != "not_found" {
		t.Fatalf("legacy reserved: %d %v", code, m)
	}
	if _, m := e.do(t, "GET", "/api/v1/openapi.json", "", h...); m["openapi"] != "3.1.0" {
		t.Fatal("openapi")
	}
	if _, m := e.do(t, "GET", "/api/v1/plugins", "", h...); m["items"] != nil {
		t.Fatalf("plugins: %v", m)
	}
}

func get(t *testing.T, e *env, path string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", e.srv.URL+path, nil)
	req.Header.Set("X-Api-Key", e.key(t))
	resp, err := e.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestConfigSchemaAndExport(t *testing.T) {
	e := newEnv(t, nil)
	resp, body := get(t, e, "/api/v1/config/schema")
	if resp.Header.Get("Content-Type") != "application/schema+json" || !strings.Contains(body, `"$schema"`) {
		t.Fatalf("schema: %s", body[:80])
	}
	resp, body = get(t, e, "/api/v1/config/export?format=yaml")
	if !strings.Contains(resp.Header.Get("Content-Type"), "yaml") || !strings.Contains(body, "mode: shadow") {
		t.Fatalf("yaml: %s", body)
	}
	_, body = get(t, e, "/api/v1/config/export?format=env")
	if !strings.Contains(body, "CORSARR_MEDIAKINDS_BOOK_MODE=shadow") {
		t.Fatalf("env: %s", body)
	}
	_, body = get(t, e, "/api/v1/config/export?full=true")
	if !strings.Contains(body, "address:") {
		t.Fatalf("full: %s", body)
	}
	if resp, _ := get(t, e, "/api/v1/config/export?format=xml"); resp.StatusCode != 400 {
		t.Fatal("bad format")
	}
}

func TestHistoryCursorPaging(t *testing.T) {
	e := newEnv(t, nil)
	for i := 0; i < 3; i++ {
		_ = e.st.History().Append(context.Background(), domain.Event{EventID: string(rune('a' + i)), Type: domain.EvGrabSent, OccurredAt: time.Now(), Actor: "system"})
	}
	e.bus.Close() // flush the health.changed event persisted during setup: 4 entries in all
	h := []string{"X-Api-Key", e.key(t)}
	_, m := e.do(t, "GET", "/api/v1/history?limit=3", "", h...)
	cur, _ := m["nextCursor"].(string)
	if len(m["items"].([]any)) != 3 || cur == "" {
		t.Fatalf("page1: %v", m)
	}
	_, m = e.do(t, "GET", "/api/v1/history?limit=3&cursor="+cur, "", h...)
	if len(m["items"].([]any)) != 1 || m["nextCursor"] != nil {
		t.Fatalf("page2: %v", m)
	}
	for _, q := range []string{"limit=0", "limit=x", "cursor=-1", "limit=500"} {
		if code, _ := e.do(t, "GET", "/api/v1/history?"+q, "", h...); code != 400 {
			t.Errorf("%s accepted", q)
		}
	}
}

func TestSSERequiresAuthAndStreams(t *testing.T) {
	e := newEnv(t, nil)
	if code, _ := e.do(t, "GET", "/api/v1/events", ""); code != 401 {
		t.Fatal("events open")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", e.srv.URL+"/api/v1/events", nil)
	req.Header.Set("X-Api-Key", e.key(t))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	rd := bufio.NewReader(resp.Body)
	_, _ = rd.ReadString('\n') // retry line
	for e.bus.Subscribers() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	_, _ = e.bus.Publish(domain.Event{Type: domain.EvJobUpdated})
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(line, "event: job.updated") {
			return
		}
	}
}

func TestStaticSPAWithBasePath(t *testing.T) {
	e := newEnv(t, func(d *Deps) { d.BasePath = "/corsarr" })
	resp, body := get(t, e, "/corsarr/movies/42")
	if resp.StatusCode != 200 || !strings.Contains(body, `<base href="/corsarr/">`) {
		t.Fatalf("spa fallback: %d %s", resp.StatusCode, body)
	}
	resp, body = get(t, e, "/corsarr/assets/app.js")
	if body != "console.log(1)" || !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %q %v", body, resp.Header)
	}
	if resp, _ := get(t, e, "/corsarr/api/v1/health"); resp.StatusCode != 200 {
		t.Fatal("api under base path")
	}
	if resp, _ := get(t, e, "/other"); resp.StatusCode != 404 {
		t.Fatal("outside base path")
	}
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := noRedirect.Get(e.srv.URL + "/corsarr")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 301 || r.Header.Get("Location") != "/corsarr/" {
		t.Fatalf("redirect: %d %s", r.StatusCode, r.Header.Get("Location"))
	}
}

func TestSPAWithoutBuiltUI(t *testing.T) {
	e := newEnv(t, func(d *Deps) { d.UI = fstest.MapFS{} })
	if resp, body := get(t, e, "/"); resp.StatusCode != 404 || !strings.Contains(body, "just build ui") {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	e2 := newEnv(t, nil)
	if resp, body := get(t, e2, "/"); resp.StatusCode != 200 || !strings.Contains(body, `<base href="/">`) {
		t.Fatalf("root index: %s", body)
	}
}

func TestNewNeedsAPIKey(t *testing.T) {
	st, _ := store.Open(context.Background(), t.TempDir())
	defer st.Close()
	if _, err := New(context.Background(), Deps{Auth: st.Auth()}); err == nil {
		t.Fatal("want error")
	}
}

func TestPasswordHashing(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil || !strings.HasPrefix(h, "$argon2id$v=19$m=12288,t=3,p=1$") {
		t.Fatalf("%s %v", h, err)
	}
	if !VerifyPassword(h, "correct horse") || VerifyPassword(h, "wrong") {
		t.Fatal("verify")
	}
	for _, bad := range []string{"", "$bcrypt$x", "$argon2id$v=19$m=x$a$b", "$argon2id$v=19$m=1,t=1,p=1$!!$b", "$argon2id$v=19$m=1,t=1,p=1$YQ$"} {
		if VerifyPassword(bad, "x") {
			t.Errorf("%q verified", bad)
		}
	}
}

type flushRec struct {
	*httptest.ResponseRecorder
	flushes int
}

func (f *flushRec) Flush() { f.flushes++ }

func TestNDJSON(t *testing.T) {
	rec := &flushRec{ResponseRecorder: httptest.NewRecorder()}
	s := NewNDJSON(rec)
	_ = s.Record(map[string]int{"a": 1})
	_ = s.Error("knaben", errors.New("timeout"))
	_ = s.Record(map[string]int{"a": 2})
	_ = s.Done(map[string]any{"indexers": 2})
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	if rec.Header().Get("Content-Type") != "application/x-ndjson" || len(lines) != 4 || rec.flushes != 4 {
		t.Fatalf("%v %d", lines, rec.flushes)
	}
	var sum map[string]map[string]any
	_ = json.Unmarshal([]byte(lines[3]), &sum)
	if sum["summary"]["count"] != 2.0 || sum["summary"]["errors"] != 1.0 || sum["summary"]["done"] != true || sum["summary"]["indexers"] != 2.0 {
		t.Fatalf("summary: %v", sum)
	}
	plain := NewNDJSON(httptest.NewRecorder()) // no flusher: still works
	if plain.Record(1) != nil {
		t.Fatal("record")
	}
	broken := NewNDJSON(failWriter{httptest.NewRecorder()})
	if broken.Record(1) == nil || broken.Error("x", errors.New("y")) == nil {
		t.Fatal("write errors must surface")
	}
}

type failWriter struct{ *httptest.ResponseRecorder }

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("gone") }

func TestRSSHelpers(t *testing.T) {
	if statmRSS("100 25 3", 4096) != 25*4096 || statmRSS("", 4096) != 0 {
		t.Fatal("statm")
	}
	if rusageRSS("darwin") <= 0 || rusageRSS("linux") <= 0 {
		t.Fatal("rusage")
	}
}
