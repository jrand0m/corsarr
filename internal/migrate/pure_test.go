package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"

	_ "github.com/jrand0m/corsarr/internal/plugins" // registers the plugins the snippet names
)

func TestParseEndpointValidatesAtTheBoundary(t *testing.T) {
	cases := []struct{ app, url, key, err string }{
		{"Sonarr", "http://h:8989/base/", "k", ""},
		{"readarr", "http://h", "k", "must be sonarr, radarr, lidarr or prowlarr"},
		{"Lidarr", "http://h:8989/base/", "k", ""},
		{"radarr", "ftp://h", "k", "http or https"},
		{"radarr", "http://", "k", "http or https"},
		{"prowlarr", "http://h", " ", "needs an API key"},
	}
	for _, c := range cases {
		ep, err := ParseEndpoint(c.app, c.url, NewSecret(c.key))
		if c.err == "" && (err != nil || ep.URL.String() != "http://h:8989/base" || ep.Instance() == "") {
			t.Errorf("%+v: %v %v", c, ep, err)
		}
		if c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("%+v: %v", c, err)
		}
	}
}

func TestSecretNeverPrints(t *testing.T) {
	s := NewSecret(" hunter2 ")
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("x", "key", s)
	j, _ := json.Marshal(struct{ K Secret }{s})
	for _, out := range []string{fmt.Sprint(s), fmt.Sprintf("%#v", s), buf.String(), string(j)} {
		if strings.Contains(out, "hunter2") {
			t.Fatalf("secret leaked: %s", out)
		}
	}
	var back Secret
	if err := json.Unmarshal([]byte(`"abc"`), &back); err != nil || back.Reveal() != "abc" {
		t.Fatal(back.Reveal(), err)
	}
}

func TestCheckVersion(t *testing.T) {
	ok := map[App]string{Sonarr: "4.0.20", Radarr: "v6.4.4", Prowlarr: "1.0"}
	for a, v := range ok {
		if err := CheckVersion(a, v); err != nil {
			t.Error(a, err)
		}
	}
	for a, v := range map[App]string{Sonarr: "5.0", Radarr: "2.9", Prowlarr: "x"} {
		if err := CheckVersion(a, v); !errors.Is(err, ErrUnsupportedVersion) {
			t.Error(a, v, err)
		}
	}
}

type countingDoer struct{ n int }

func (c *countingDoer) Do(*http.Request) (*http.Response, error) {
	c.n++
	return nil, errors.New("sent")
}

func TestReadOnlyRefusesWrites(t *testing.T) {
	next := &countingDoer{}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req, _ := http.NewRequest(m, "http://h/api/v3/series", nil)
		if _, err := (ReadOnly{Next: next}).Do(req); !errors.Is(err, ErrWriteRefused) {
			t.Fatal(m, err)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "http://h/", nil)
	_, _ = ReadOnly{Next: next}.Do(req)
	if next.n != 1 {
		t.Fatalf("forwarded %d", next.n)
	}
}

func TestClientReportsHTTPFailures(t *testing.T) {
	f := newFake(t, Sonarr, map[string]any{"/broken": "not-an-object"})
	c := NewClient(f.endpoint(t, Sonarr), http.DefaultClient)
	var out []wSeries
	if err := c.get(context.Background(), "/broken", nil, &out); err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Fatal(err)
	}
	f.srv.Close()
	if err := c.get(context.Background(), "/series", nil, &out); err == nil {
		t.Fatal("closed server answered")
	}
}

func TestPagedReadIsBounded(t *testing.T) {
	rows := make([]any, 0, pageSize)
	for i := 0; i < pageSize; i++ {
		rows = append(rows, map[string]any{"id": i})
	}
	f := newFake(t, Sonarr, map[string]any{"/history": map[string]any{"page": 1, "totalRecords": 10 * pageSize, "records": rows}})
	c := NewClient(f.endpoint(t, Sonarr), http.DefaultClient)
	got, err := paged(context.Background(), c, "/history", nil, pageSize+10, func(wHistory) bool { return true })
	if err != nil || len(got) != pageSize+10 {
		t.Fatal(len(got), err)
	}
	got, _ = paged(context.Background(), c, "/history", nil, 1000, func(h wHistory) bool { return h.ID < 5 })
	if len(got) != 5 {
		t.Fatal(len(got))
	}
}

func TestRewrite(t *testing.T) {
	rw, err := ParseRewrite("/data/media/:/mnt/media")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{"/data/media/tv/x": "/mnt/media/tv/x", "/data/media": "/mnt/media", "/data/mediax/a": "/data/mediax/a"}
	for in, want := range cases {
		if got := rw.Apply(in); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
	if (Rewrite{}).Apply("/a") != "/a" {
		t.Error("empty rewrite changed a path")
	}
	for _, bad := range []string{"nocolon", "rel:/x", "/x:rel"} {
		if _, err := ParseRewrite(bad); err == nil {
			t.Error(bad)
		}
	}
	if r, err := ParseRewrite(""); err != nil || r.From != "" {
		t.Error(r, err)
	}
}

func TestParseScope(t *testing.T) {
	s, err := ParseScope("library, downloads")
	if err != nil || !s.Has(CatFiles) || !s.Has(CatModes) || s.Has(CatHistory) || !s.Has(CatDownloads) {
		t.Fatal(s, err)
	}
	if _, err := ParseScope("library,bogus"); err == nil {
		t.Fatal("unknown category accepted")
	}
	if d, _ := ParseScope(""); d.Has(CatHistory) || !d.Has(CatLibrary) {
		t.Fatal(d)
	}
}

func TestQualityMapping(t *testing.T) {
	cases := map[string]string{"WEBDL-1080p": "webdl-1080p", "Bluray-2160p Remux": "remux-2160p", "Remux-1080p": "remux-1080p",
		"SDTV": "sdtv", "Raw-HD": "unknown"}
	for in, want := range cases {
		if got, _ := qualityID(in); string(got) != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
	if _, exact := qualityID("Raw-HD"); exact {
		t.Error("unknown quality reported exact")
	}
}

func TestMinAvailabilityMapping(t *testing.T) {
	cases := map[string]domain.MinAvailability{"announced": domain.AvailAnnounced, "inCinemas": domain.AvailInCinemas,
		"released": domain.AvailReleased, "tba": domain.AvailAnnounced, "preDB": domain.AvailReleased, "odd": domain.AvailReleased}
	for in, want := range cases {
		if got, _ := minAvailability(in); got != want {
			t.Errorf("%s → %s", in, got)
		}
	}
}

func TestProfileMapping(t *testing.T) {
	if m := mapProfile(SrcProfile{Name: "Any"}, domain.KindTV); !m.Exact || m.Key != "any" {
		t.Fatal(m)
	}
	if m := mapProfile(SrcProfile{Name: "Low", Cutoff: "HDTV-720p"}, domain.KindMovie); m.Exact || m.Key != "hd1080p" {
		t.Fatal(m)
	}
	if k := profileKey(nil, 9, domain.KindTV); k != "hd1080p" {
		t.Fatal(k)
	}
}

func TestSortTitle(t *testing.T) {
	for in, want := range map[string]string{"The Show": "show", "An Owl": "owl", "Zed": "zed"} {
		if got := sortTitle(in); got != want {
			t.Error(in, got)
		}
	}
}

func TestConfigSnippetIsAValidConfigFile(t *testing.T) {
	r := newRig(t, "aaaa")
	rep := r.plan()
	if rep.Config == "" || len(rep.Env) < 3 {
		t.Fatalf("snippet %q env %v", rep.Config, rep.Env)
	}
	env := []string{}
	for _, v := range rep.Env {
		env = append(env, v+"=x")
	}
	in := config.Inputs{Environ: env, ReadFile: func(string) ([]byte, error) { return []byte(rep.Config), nil },
		Resolver: config.Resolver{Getenv: func(k string) (string, bool) { return "x", true }}}
	cfg, err := config.LoadFrom(config.Source{Kind: config.SourceFile, Path: "migrated.yml"}, in, plugin.Default)
	if err != nil {
		t.Fatalf("%v\n%s", err, rep.Config)
	}
	qb := cfg.Instances["downloadClients"]["qbittorrent"]
	if qb.Enabled || qb.Settings.String("url") != "http://qbit:8080" || len(cfg.Instances["indexers"]) != 2 || len(cfg.RootFolders) != 2 {
		t.Fatalf("%+v %+v", qb, cfg.Instances["indexers"])
	}
	cats := qb.Settings["categories"].(map[string]any)
	if cats["tv"] != "tv" || cats["movie"] != "movies" {
		t.Fatalf("categories %v", cats)
	}
}

func TestConfigManagedObjectsAreNotRepeated(t *testing.T) {
	r := newRig(t)
	view := ConfigView{RootFolders: map[string]bool{r.lib.root + "/tv": true}, Clients: map[string]bool{"http://qbit:8080": true},
		Indexers: map[string]bool{"http://jackett:9117/torznab": true}, KindExplicit: map[string]bool{"tv": true}}
	r.svc.d.Config = func() *config.Config { return nil }
	snaps := r.svc.read(context.Background(), r.req, func(Progress) {})
	st, err := r.svc.state(context.Background(), snaps, r.req.Options)
	if err != nil {
		t.Fatal(err)
	}
	st.Config = view
	rep := BuildPlan(snaps, st, r.req.Options)
	for _, id := range []string{"sonarr:downloadclient:1", "prowlarr:indexer:5", "corsarr:mode:tv"} {
		if e := entry(t, rep, id); e.Outcome != OutManaged {
			t.Errorf("%s: %+v", id, e)
		}
	}
	if strings.Contains(rep.Config, "qbit:8080") {
		t.Fatalf("managed client repeated:\n%s", rep.Config)
	}
}

func TestConfigViewReadsWhatTheConfigSets(t *testing.T) {
	yml := `mediaKinds:
  tv:
    mode: active
rootFolders:
  - path: /media/tv/
    kind: tv
indexers:
  one:
    type: torznab
    url: http://idx/torznab
downloadClients:
  qb:
    type: qbittorrent
    url: http://qbit:8080/
`
	in := config.Inputs{ReadFile: func(string) ([]byte, error) { return []byte(yml), nil }}
	cfg, err := config.LoadFrom(config.Source{Kind: config.SourceFile, Path: "c.yml"}, in, plugin.Default)
	if err != nil {
		t.Fatal(err)
	}
	v := configView(cfg)
	if !v.RootFolders["/media/tv"] || !v.Indexers["http://idx/torznab"] || !v.Clients["http://qbit:8080"] ||
		!v.KindExplicit["tv"] || v.KindExplicit["movie"] {
		t.Fatalf("%+v", v)
	}
	if e := configView(nil); len(e.RootFolders) != 0 {
		t.Fatal(e)
	}
}

func TestKeysAreUniqueAndWellFormed(t *testing.T) {
	c := newConfigDoc()
	got := []string{c.key("My Tracker"), c.key("my-tracker"), c.key("1337"), c.key("")}
	want := []string{"mytracker", "mytrackerx", "x1337", "x"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%v", got)
		}
	}
}
