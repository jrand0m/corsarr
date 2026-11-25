package qbittorrent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

func TestErrorStatesWarnWithoutFailing(t *testing.T) {
	f := newFake(t, "v5.0.1")
	f.put("e1", map[string]any{"name": "Disk", "category": "movies", "state": "error"})
	f.put("e2", map[string]any{"name": "Gone", "category": "movies", "state": "missingFiles"})
	snaps, err := newClient(t, f, nil).List(t.Context(), []string{"movies"})
	if err != nil || len(snaps) != 2 {
		t.Fatal(err, snaps)
	}
	for _, s := range snaps {
		if s.State != domain.StateStalled || s.Error == "" {
			t.Errorf("%s: %+v", s.DownloadID, s)
		}
	}
}

func TestBadPathMapIsAConfigError(t *testing.T) {
	f := newFake(t, "v5.0.1")
	_, err := New(plugin.Instance{Key: "qb", PluginID: ID, Settings: plugin.Settings{"url": f.srv.URL, "pathMap": []any{"nope"}}})
	if plugin.ClassOf(err) != plugin.ClassConfig {
		t.Fatalf("%v", err)
	}
}

func TestReportedPathsAreMappedToLocal(t *testing.T) {
	f := newFake(t, "v5.0.1")
	f.put("h1", map[string]any{"name": "Done", "category": "movies", "state": "uploading",
		"save_path": "/downloads/movies", "content_path": "/downloads/movies/Done"})
	f.put("h2", map[string]any{"name": "Win", "category": "movies", "state": "uploading",
		"save_path": `D:\Torrents`, "content_path": `D:\Torrents\Win.mkv`})
	f.put("h3", map[string]any{"name": "Else", "category": "movies", "state": "uploading",
		"save_path": "/elsewhere", "content_path": "/elsewhere/Else"})
	c := newClient(t, f, plugin.Settings{"pathMap": []any{"/downloads=/mnt/dl", `D:\Torrents=/mnt/win`}})
	snaps, err := c.List(t.Context(), []string{"movies"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{
		"h1": {"/mnt/dl/movies", "/mnt/dl/movies/Done"},
		"h2": {"/mnt/win", "/mnt/win/Win.mkv"},
		"h3": {"/elsewhere", "/elsewhere/Else"},
	}
	for _, s := range snaps {
		w := want[s.DownloadID]
		if s.SavePath != w[0] || s.ContentPath != w[1] || s.Files[0] != w[1] {
			t.Errorf("%s: %q %q %v, want %v", s.DownloadID, s.SavePath, s.ContentPath, s.Files, w)
		}
	}
}

func TestLocalPathsAreSentInTheClientsView(t *testing.T) {
	f := newFake(t, "v5.0.1")
	c := newClient(t, f, plugin.Settings{"pathMap": []any{"/downloads=/mnt/dl"}})
	if _, err := c.Add(t.Context(), plugin.AddRequest{Payload: magnetPayload(), Category: "movies", SavePath: "/mnt/dl/movies"}); err != nil {
		t.Fatal(err)
	}
	if got := f.forms[len(f.forms)-1]["savepath"]; got != "/downloads/movies" {
		t.Errorf("add savepath %q", got)
	}
	if err := c.EnsureCategory(t.Context(), "tv", "/mnt/dl/tv"); err != nil {
		t.Fatal(err)
	}
	if got := f.categories["tv"]; got != "/downloads/tv" {
		t.Errorf("category save path %q", got)
	}
	if err := c.EnsureCategory(t.Context(), "tv", "/mnt/dl/tv"); err != nil || f.called("torrents/editCategory") {
		t.Errorf("an aligned category is left alone: %v", err)
	}
}

func TestHealthChecksTheSavePathsExistLocally(t *testing.T) {
	local := t.TempDir()
	if err := os.MkdirAll(filepath.Join(local, "movies"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := newFake(t, "v5.0.1")
	f.prefs = map[string]any{"save_path": "/downloads"}
	f.categories["movies"] = "movies" // relative to the default save path
	if err := newClient(t, f, plugin.Settings{"pathMap": []any{"/downloads=" + local}}).Test(t.Context()); err != nil {
		t.Fatalf("mapped paths exist: %v", err)
	}
	f.categories["tv"] = "/downloads/tv" // mapped, but the folder is missing
	err := newClient(t, f, plugin.Settings{"pathMap": []any{"/downloads=" + local}}).Test(t.Context())
	if plugin.ClassOf(err) != plugin.ClassPath || !strings.Contains(err.Error(), filepath.ToSlash(filepath.Join(local, "tv"))) {
		t.Fatalf("missing mapped folder: %v", err)
	}
	delete(f.categories, "tv")
	err = newClient(t, f, nil).Test(t.Context())
	if plugin.ClassOf(err) != plugin.ClassPath || !strings.Contains(err.Error(), "no pathMap entry") {
		t.Fatalf("unmapped missing folder: %v", err)
	}
}

func TestHealthIgnoresUnreadablePreferences(t *testing.T) {
	f := newFake(t, "v4.6.2") // answers 404 for app/preferences
	if err := newClient(t, f, nil).Test(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestPathProblemMessages(t *testing.T) {
	m, _ := plugin.ParsePathMap([]string{"/downloads=/mnt/dl"})
	none := func(string) bool { return false }
	all := func(string) bool { return true }
	cases := []struct {
		reported, goos string
		exists         func(string) bool
		want           string
	}{
		{"/downloads/x", "linux", all, ""},
		{"/downloads/x", "linux", none, "mapped to /mnt/dl/x"},
		{`C:\dl`, "linux", none, "Windows path"},
		{`C:\dl`, "windows", none, "no pathMap entry"},
		{"/data", "linux", none, "no pathMap entry"},
	}
	for _, c := range cases {
		err := pathProblem(c.reported, m, c.exists, c.goos)
		if (c.want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s/%s: %v, want %q", c.reported, c.goos, err, c.want)
		}
	}
}
