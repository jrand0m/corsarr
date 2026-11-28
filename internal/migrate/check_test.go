package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func postJSON(t *testing.T, url, body string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, m
}

func checkSvc(t *testing.T) (*Service, *httptest.Server) {
	svc := New(Deps{HTTP: http.DefaultClient})
	m := mux{http.NewServeMux()}
	svc.Mount(m)
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return svc, srv
}

func sourceJSON(app App, url, key string) string {
	b, _ := json.Marshal(map[string]string{"app": string(app), "url": url, "apiKey": key})
	return string(b)
}

func TestCheckSourceStates(t *testing.T) {
	sonarr := newFake(t, Sonarr, nil)
	radarr := newFake(t, Radarr, nil)
	old := newFake(t, Sonarr, nil)
	old.version = "2.0.0.5344"
	notArr := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(notArr.Close)
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }))
	t.Cleanup(html.Close)
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	t.Cleanup(proxy.Close)
	_, srv := checkSvc(t)
	cases := []struct {
		name, body string
		want       CheckState
	}{
		{"full check", sourceJSON(Sonarr, sonarr.srv.URL, testKey), CheckReachable},
		{"address only", sourceJSON(Sonarr, sonarr.srv.URL, ""), CheckKeyNeeded},
		{"wrong key", sourceJSON(Sonarr, sonarr.srv.URL, "nope"), CheckUnauthorised},
		{"other app", sourceJSON(Sonarr, radarr.srv.URL, testKey), CheckWrongApp},
		{"old version", sourceJSON(Sonarr, old.srv.URL, testKey), CheckUnsupported},
		{"no api", sourceJSON(Sonarr, notArr.URL, ""), CheckWrongApp},
		{"not json", sourceJSON(Sonarr, html.URL, testKey), CheckWrongApp},
		{"nothing listens", sourceJSON(Sonarr, down.URL, testKey), CheckUnreachable},
		{"proxy error", sourceJSON(Sonarr, proxy.URL, testKey), CheckUnreachable},
	}
	for _, c := range cases {
		code, m := postJSON(t, srv.URL+"/api/v1/migration/check/source", c.body)
		if code != 200 || m["state"] != string(c.want) {
			t.Errorf("%s: %d %v", c.name, code, m)
		}
		if strings.Contains(c.body, testKey) && strings.Contains(toJSON(m), testKey) {
			t.Errorf("%s: key echoed", c.name)
		}
	}
	sonarr.onlyGets(t)
	_, m := postJSON(t, srv.URL+"/api/v1/migration/check/source", sourceJSON(Sonarr, old.srv.URL, testKey))
	if m["version"] != "2.0.0.5344" || m["supported"] != "3.x to 4.x" {
		t.Errorf("version detail: %v", m)
	}
}

func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestCheckSourceSendsNoKeyHeaderWithoutKey(t *testing.T) {
	var got []string
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("X-Api-Key"))
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(src.Close)
	svc := New(Deps{HTTP: http.DefaultClient})
	ep, err := ParseTarget("sonarr", src.URL, NewSecret(""))
	if err != nil {
		t.Fatal(err)
	}
	if r := svc.Check(context.Background(), ep); r.State != CheckKeyNeeded || len(got) != 1 || got[0] != "" {
		t.Fatalf("%+v %q", r, got)
	}
}

func TestCheckSourceRejectsBadInput(t *testing.T) {
	_, srv := checkSvc(t)
	for _, body := range []string{sourceJSON("sickbeard", "http://x", ""), sourceJSON(Sonarr, "sonarr:8989", ""), `{`} {
		if code, _ := postJSON(t, srv.URL+"/api/v1/migration/check/source", body); code != 400 {
			t.Errorf("%s: %d", body, code)
		}
	}
}

func TestCheckNeverLogsTheKey(t *testing.T) {
	var buf bytes.Buffer
	src := newFake(t, Sonarr, nil)
	svc := New(Deps{HTTP: http.DefaultClient, Log: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))})
	ep, _ := ParseTarget("sonarr", src.srv.URL, NewSecret(testKey))
	r := svc.Check(context.Background(), ep)
	if strings.Contains(buf.String(), testKey) || strings.Contains(toJSON(r), testKey) {
		t.Fatal("key leaked")
	}
}

func TestCheckRateLimit(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var g gate
	for i := range gateInFlight {
		if !g.enter(now) {
			t.Fatalf("slot %d refused", i)
		}
	}
	if g.enter(now) {
		t.Fatal("more than the in-flight cap admitted")
	}
	for range gateInFlight {
		g.leave()
	}
	admitted := gateInFlight
	for g.enter(now) {
		g.leave()
		admitted++
	}
	if admitted != gateBurst {
		t.Fatalf("burst %d", admitted)
	}
	if !g.enter(now.Add(time.Second)) {
		t.Fatal("bucket did not refill")
	}
	g.leave()

	svc, srv := checkSvc(t)
	svc.d.Now = func() time.Time { return now }
	svc.checks = gate{last: now}
	if code, _ := postJSON(t, srv.URL+"/api/v1/migration/check/source", sourceJSON(Sonarr, "http://127.0.0.1:1", "")); code != http.StatusTooManyRequests {
		t.Fatalf("empty bucket answered %d", code)
	}
}

func TestNetReason(t *testing.T) {
	if got := netReason(context.DeadlineExceeded); !strings.Contains(got, "No answer within") {
		t.Fatal(got)
	}
	if got := netReason(errors.New("connection refused")); got != "connection refused" {
		t.Fatal(got)
	}
	if appLabel("") != "" || appLabel(Prowlarr) != "Prowlarr" {
		t.Fatal("label")
	}
}

func TestCheckPaths(t *testing.T) {
	local := t.TempDir()
	if err := os.MkdirAll(filepath.Join(local, "tv"), 0o755); err != nil {
		t.Fatal(err)
	}
	sonarr := newFake(t, Sonarr, map[string]any{"/rootfolder": []map[string]any{{"path": "/data/media/tv"}, {"path": "/other/tv"}}})
	prowlarr := newFake(t, Prowlarr, nil)
	broken := newFake(t, Radarr, nil)
	_, srv := checkSvc(t)
	sources := `[` + sourceJSON(Sonarr, sonarr.srv.URL, testKey) + `,` + sourceJSON(Prowlarr, prowlarr.srv.URL, testKey) + `,` +
		sourceJSON(Radarr, broken.srv.URL, "wrong") + `]`
	body := func(from, to string) string {
		return `{"sources":` + sources + `,"fromPrefix":` + toJSON(from) + `,"toPrefix":` + toJSON(to) + `}`
	}
	var pc PathCheck
	decode := func(m map[string]any) {
		pc = PathCheck{}
		if err := json.Unmarshal([]byte(toJSON(m)), &pc); err != nil {
			t.Fatal(err)
		}
	}

	code, m := postJSON(t, srv.URL+"/api/v1/migration/check/paths", body("/data/media", local))
	decode(m)
	if code != 200 || len(pc.Roots) != 2 || pc.Target == nil || !pc.Target.Exists || !pc.Target.Readable || !pc.Target.Writable {
		t.Fatalf("%d %+v", code, pc)
	}
	tv, other := pc.Roots[0], pc.Roots[1]
	if !tv.Matched || tv.Rewritten != filepath.ToSlash(local)+"/tv" && tv.Rewritten != local+"/tv" || !tv.Exists {
		t.Errorf("matched root: %+v", tv)
	}
	if other.Matched || other.Rewritten != "/other/tv" || other.Exists {
		t.Errorf("unmapped root: %+v", other)
	}
	if len(pc.Sources) != 1 || pc.Sources[0].App != Radarr || !strings.Contains(pc.Sources[0].Reason, "refused") {
		t.Errorf("source issues: %+v", pc.Sources)
	}

	_, m = postJSON(t, srv.URL+"/api/v1/migration/check/paths", body("", ""))
	decode(m)
	if pc.Target != nil || pc.Roots[0].Matched || pc.Roots[0].Rewritten != "/data/media/tv" {
		t.Errorf("no rewrite: %+v", pc)
	}

	_, m = postJSON(t, srv.URL+"/api/v1/migration/check/paths", body("", filepath.Join(local, "missing")))
	decode(m)
	if pc.Target == nil || pc.Target.Exists || pc.Target.Readable {
		t.Errorf("missing target: %+v", pc.Target)
	}
	sonarr.onlyGets(t)

	if code, _ := postJSON(t, srv.URL+"/api/v1/migration/check/paths", body("data", "")); code != 400 {
		t.Errorf("relative prefix: %d", code)
	}
	if code, _ := postJSON(t, srv.URL+"/api/v1/migration/check/paths", `{"sources":[`+sourceJSON(Sonarr, "x", "")+`]}`); code != 400 {
		t.Errorf("bad source: %d", code)
	}
	if code, _ := postJSON(t, srv.URL+"/api/v1/migration/check/paths", `{"sources":[],"toPrefix":"rel"}`); code != 400 {
		t.Errorf("relative corsarr prefix: %d", code)
	}
}

func TestCheckPathsCapsTheSample(t *testing.T) {
	roots := make([]map[string]any, maxRoots+5)
	for i := range roots {
		roots[i] = map[string]any{"path": "/r/" + string(rune('a'+i%26)) + strings.Repeat("x", i)}
	}
	src := newFake(t, Sonarr, map[string]any{"/rootfolder": roots})
	svc := New(Deps{HTTP: http.DefaultClient, Probe: func(string) Access { return Access{} }})
	pc := svc.CheckPaths(context.Background(), []Endpoint{src.endpoint(t, Sonarr)}, Rewrite{})
	if len(pc.Roots) != maxRoots {
		t.Fatal(len(pc.Roots))
	}
}
