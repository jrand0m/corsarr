package migrate

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

type mux struct{ *http.ServeMux }

func (m mux) Handle(pattern string, h http.HandlerFunc) { m.ServeMux.Handle(pattern, h) }

func serve(t *testing.T, r *rig) *httptest.Server {
	m := mux{http.NewServeMux()}
	r.svc.Mount(m)
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return srv
}

func (r *rig) body(extra string) string {
	var parts []string
	for _, ep := range r.req.Sources {
		parts = append(parts, fmt.Sprintf(`{"app":%q,"url":%q,"apiKey":%q}`, ep.App, ep.URL, testKey))
	}
	return `{"sources":[` + strings.Join(parts, ",") + `],"pathRewrite":"/data/media:` + r.lib.root + `"` + extra + `}`
}

// stream posts and returns the decoded NDJSON lines.
func stream(t *testing.T, url, body string) []map[string]json.RawMessage {
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []map[string]json.RawMessage
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var line map[string]json.RawMessage
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			t.Fatalf("%v: %s", err, sc.Text())
		}
		out = append(out, line)
	}
	return out
}

func summary(t *testing.T, lines []map[string]json.RawMessage) (Report, bool) {
	var s struct {
		Report      Report `json:"report"`
		PlanChanged bool   `json:"planChanged"`
	}
	if err := json.Unmarshal(lines[len(lines)-1]["summary"], &s); err != nil {
		t.Fatal(err)
	}
	return s.Report, s.PlanChanged
}

func TestHTTPPlanStreamsProgressThenReport(t *testing.T) {
	r := newRig(t, "aaaa")
	srv := serve(t, r)
	lines := stream(t, srv.URL+"/api/v1/migration/plan", r.body(""))
	if _, ok := lines[0]["record"]; !ok || len(lines) < 5 {
		t.Fatalf("no progress lines: %d", len(lines))
	}
	rep, _ := summary(t, lines)
	if rep.Mode != "plan" || rep.Counts[OutCreate] == 0 || rep.PlanHash == "" {
		t.Fatalf("%+v", rep.Counts)
	}
	lines = stream(t, srv.URL+"/api/v1/migration/apply", r.body(`,"planHash":"old"`))
	if _, changed := summary(t, lines); !changed {
		t.Fatal("stale plan hash was not reported")
	}
	lines = stream(t, srv.URL+"/api/v1/migration/apply", r.body(`,"planHash":"`+rep.PlanHash+`"`))
	if done, _ := summary(t, lines); done.Mode != "apply" || done.Counts[OutFailed] != 0 {
		t.Fatalf("%+v", done.Counts)
	}
	resp, _ := http.Get(srv.URL + "/api/v1/migration/runs")
	var runs struct{ Items []map[string]any }
	_ = json.NewDecoder(resp.Body).Decode(&runs)
	resp.Body.Close()
	if len(runs.Items) != 3 {
		t.Fatalf("runs %+v", runs)
	}
	resp, _ = http.Get(fmt.Sprintf("%s/api/v1/migration/runs/%v/report", srv.URL, runs.Items[0]["id"]))
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Disposition"), "migration-") {
		t.Fatal(resp.Status)
	}
	resp.Body.Close()
}

func TestHTTPRejectsBadInput(t *testing.T) {
	r := newRig(t)
	srv := serve(t, r)
	for _, body := range []string{`{"sources":[{"app":"x","url":"http://h","apiKey":"k"}]}`, r.body(`,"scope":["nope"]`),
		`{"sources":[],"pathRewrite":"bad"}`} {
		resp, _ := http.Post(srv.URL+"/api/v1/migration/plan", "application/json", strings.NewReader(body))
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Errorf("%s: %d", body, resp.StatusCode)
		}
	}
	for _, path := range []string{"/api/v1/migration/runs/x/report", "/api/v1/migration/runs/9/report"} {
		resp, _ := http.Get(srv.URL + path)
		resp.Body.Close()
		if resp.StatusCode != 400 && resp.StatusCode != 404 {
			t.Errorf("%s: %d", path, resp.StatusCode)
		}
	}
	lines := stream(t, srv.URL+"/api/v1/migration/plan", `{"sources":[]}`)
	if _, ok := lines[0]["error"]; !ok {
		t.Fatalf("empty run not refused: %v", lines)
	}
}

func put(t *testing.T, url, body string) (int, string) {
	req, _ := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, fmt.Sprint(m)
}

func TestHTTPModesCutover(t *testing.T) {
	r := newRig(t)
	reloads := 0
	modes := map[domain.MediaKind]domain.KindMode{}
	r.svc.d.Mode = func(k domain.MediaKind) domain.KindMode {
		if m, ok := modes[k]; ok {
			return m
		}
		return domain.ModeActive
	}
	r.svc.d.AfterApply = func(context.Context) { reloads++ }
	srv := serve(t, r)
	if code, _ := put(t, srv.URL+"/api/v1/migration/modes/tv", `{"mode":"shadow"}`); code != 200 || reloads != 1 {
		t.Fatal(code, reloads)
	}
	ov, _ := r.st.ModeOverrides().List(t.Context())
	if len(ov) != 1 || ov[0].Mode != "shadow" {
		t.Fatalf("%+v", ov)
	}
	for _, c := range []struct{ path, body string }{{"/modes/zz", `{"mode":"active"}`}, {"/modes/tv", `{"mode":"disabled"}`}} {
		if code, _ := put(t, srv.URL+"/api/v1/migration"+c.path, c.body); code != 400 {
			t.Errorf("%s %s: %d", c.path, c.body, code)
		}
	}
	yml := "mediaKinds:\n  movie:\n    mode: shadow\n"
	cfg, err := config.LoadFrom(config.Source{Kind: config.SourceFile, Path: "c.yml"},
		config.Inputs{ReadFile: func(string) ([]byte, error) { return []byte(yml), nil }}, plugin.Default)
	if err != nil {
		t.Fatal(err)
	}
	r.svc.d.Config = func() *config.Config { return cfg }
	if code, msg := put(t, srv.URL+"/api/v1/migration/modes/movie", `{"mode":"active"}`); code != 409 || !strings.Contains(msg, "mode: active") {
		t.Fatal(code, msg)
	}
	resp, _ := http.Get(srv.URL + "/api/v1/migration/modes")
	var list struct{ Items []KindState }
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list.Items) != 4 || !list.Items[1].Managed || list.Items[0].Managed {
		t.Fatalf("%+v", list)
	}
	if by := list.Items[1].By; by == nil || by.Key != "mediaKinds.movie.mode" || by.Kind != config.SourceFile || by.File != "c.yml" || list.Items[0].By != nil {
		t.Fatalf("managed by: %+v", list.Items)
	}
}
