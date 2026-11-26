package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRadarr serves a one-movie library to the migrate command.
func fakeRadarr(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("X-Api-Key") != "key1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body any = []any{}
		switch {
		case r.URL.Path == "/api/v3/system/status":
			body = map[string]any{"version": "5.2.0"}
		case r.URL.Path == "/api/v3/movie":
			body = []any{map[string]any{"id": 1, "title": "Film", "year": 2020, "tmdbId": 7, "path": "/m/Film", "rootFolderPath": "/m",
				"monitored": true, "minimumAvailability": "released",
				"movieFile": map[string]any{"id": 2, "path": "/m/Film/f.mkv", "size": 1, "quality": map[string]any{"quality": map[string]any{"name": "Bluray-1080p"}}}}}
		case r.URL.Path == "/api/v3/config/naming":
			body = map[string]any{"renameMovies": true}
		case r.URL.Path == "/api/v3/rootfolder":
			body = []any{map[string]any{"id": 1, "path": "/m"}}
		case r.URL.Query().Get("page") != "":
			body = map[string]any{"records": []any{}}
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMigratePlanThenRunConverges(t *testing.T) {
	srv := fakeRadarr(t)
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	_ = os.WriteFile(keyFile, []byte("key1\n"), 0o600)
	args := []string{"--data.dir=" + filepath.Join(dir, "data"), "--from=radarr", "--url=" + srv.URL, "--api-key-file=" + keyFile}
	code, out, errs := cliRun(t, nil, append([]string{"migrate", "plan"}, args...)...)
	if code != 0 || !strings.Contains(out, "create") || !strings.Contains(out, "radarr:movie:1") || !strings.Contains(errs, "reading movies") {
		t.Fatalf("%d\n%s\n%s", code, out, errs)
	}
	if strings.Contains(out+errs, "key1") {
		t.Fatal("API key printed")
	}
	snippet := filepath.Join(dir, "migrated.yml")
	code, out, errs = cliRun(t, nil, append([]string{"migrate", "run", "--json", "--write-config=" + snippet}, args...)...)
	var rep struct{ Counts map[string]int }
	if code != 0 || json.Unmarshal([]byte(out), &rep) != nil || rep.Counts["create"] == 0 {
		t.Fatalf("%d %s %s", code, out, errs)
	}
	if b, err := os.ReadFile(snippet); err != nil || !strings.Contains(string(b), "path: /m") {
		t.Fatalf("snippet %s %v", b, err)
	}
	code, out, _ = cliRun(t, []string{"RADARR_KEY=key1"}, "migrate", "run", "--data.dir="+filepath.Join(dir, "data"),
		"--from=radarr", "--url="+srv.URL, "--api-key-env=RADARR_KEY")
	if code != 0 || strings.Contains(out, "create ") {
		t.Fatalf("second run still creates:\n%s", out)
	}
}

func TestMigrateRefusesBadInput(t *testing.T) {
	srv := fakeRadarr(t)
	data := "--data.dir=" + t.TempDir()
	cases := [][]string{
		{"--from=radarr", "--url=" + srv.URL},
		{"--from=radarr", "--url=" + srv.URL, "--api-key-env=NOPE"},
		{"--from=radarr", "--url=" + srv.URL, "--api-key-file=/does/not/exist"},
		{"--from=readarr", "--url=" + srv.URL, "--api-key-env=HOME"},
		{"--from=radarr", "--url=" + srv.URL, "--api-key-env=HOME", "--scope=bogus"},
		{"--from=radarr", "--url=" + srv.URL, "--api-key-env=HOME", "--path-rewrite=x"},
		{"--from=radarr", "--url=" + srv.URL, "--api-key-env=HOME", "--history-days=0"},
	}
	for _, c := range cases {
		if code, _, errs := cliRun(t, nil, append([]string{"migrate", "plan", data}, c...)...); code != 2 || !strings.Contains(errs, "usage") {
			t.Errorf("%v: %d %s", c, code, errs)
		}
	}
	code, _, errs := cliRun(t, []string{"K=wrong"}, "migrate", "plan", data, "--from=radarr", "--url="+srv.URL, "--api-key-env=K",
		"--history-days=30", "--exclude=a,b", "--activate")
	if code != 1 || !strings.Contains(errs, "unauthorised") {
		t.Fatalf("%d %s", code, errs)
	}
}
