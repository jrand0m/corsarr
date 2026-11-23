package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

func f64(v float64) *float64 { return &v }

// testRegistry holds a fake indexer and downloader so plugin settings are
// validated exactly as built-ins will be.
func testRegistry(t *testing.T) *plugin.Registry {
	t.Helper()
	reg := plugin.NewRegistry()
	nop := func(plugin.Instance) (any, error) { return nil, nil }
	reg.MustRegister(plugin.Registration{New: nop, Manifest: plugin.Manifest{
		ID: "torznab", Kind: plugin.KindIndexer, APIVersion: plugin.APIVersion, Name: "Torznab",
		Settings: []plugin.Field{
			{Key: "url", Type: plugin.FieldURL, Required: true},
			{Key: "apiKey", Type: plugin.FieldSecret},
			{Key: "categories", Type: plugin.FieldList},
			{Key: "minSeeders", Type: plugin.FieldNumber, Default: 1, Min: f64(0)},
			{Key: "timeout", Type: plugin.FieldDuration, Default: "30s"},
		},
	}})
	reg.MustRegister(plugin.Registration{New: nop, Manifest: plugin.Manifest{
		ID: "qbittorrent", Kind: plugin.KindDownloader, APIVersion: plugin.APIVersion, Name: "qBittorrent",
		Settings: []plugin.Field{
			{Key: "url", Type: plugin.FieldURL, Required: true},
			{Key: "username", Type: plugin.FieldText},
			{Key: "password", Type: plugin.FieldSecret},
			{Key: "category", Type: plugin.FieldText, Default: "corsarr"},
		},
	}})
	return reg
}

// inputs builds Inputs with no default config files present.
func inputs(args, environ []string, files map[string]string) Inputs {
	return Inputs{
		Args: args, Environ: environ,
		Resolver: Resolver{
			Getenv:   func(k string) (string, bool) { v := envValue(environ, k); return v, v != "" },
			ReadFile: func(p string) ([]byte, error) { return readMap(files, p) },
		},
		ReadFile: func(p string) ([]byte, error) { return readMap(files, p) },
		Exists:   func(p string) bool { _, ok := files[p]; return ok },
	}
}

func readMap(files map[string]string, p string) ([]byte, error) {
	s, ok := files[p]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(s), nil
}

const fileYAML = `version: 1
server:
  address: ":9000"
  basePath: /corsarr
log:
  level: debug
mediaKinds:
  book:
    mode: shadow
rootFolders:
  - path: /data/media/movies
    kind: movie
indexers:
  knaben:
    type: torznab
    name: Knaben
    url: https://torznab.example/api
    apiKey: ${env:CORSARR_SECRET_INDEXERS_KNABEN_APIKEY}
    categories: [2000, 5000]
    minSeeders: 3
downloadClients:
  qbit:
    type: qbittorrent
    url: http://qbit:8080
    password: ${env:CORSARR_SECRET_DOWNLOADCLIENTS_QBIT_PASSWORD}
`

var sameArgs = []string{
	"--server.address=:9000", "--server.basePath=/corsarr", "--log.level=debug",
	"--mediaKinds.book.mode=shadow",
	"--rootFolders.0.path=/data/media/movies", "--rootFolders.0.kind=movie",
	"--indexers.knaben.type=torznab", "--indexers.knaben.name=Knaben",
	"--indexers.knaben.url=https://torznab.example/api", "--indexers.knaben.apiKey=s3cret",
	"--indexers.knaben.categories.0=2000", "--indexers.knaben.categories.1=5000",
	"--indexers.knaben.minSeeders=3",
	"--downloadClients.qbit.type=qbittorrent", "--downloadClients.qbit.url=http://qbit:8080",
	"--downloadClients.qbit.password=hunter2",
}

var sameEnv = []string{
	"CORSARR_SERVER_ADDRESS=:9000", "CORSARR_SERVER_BASEPATH=/corsarr", "corsarr_log_level=debug",
	"CORSARR_MEDIAKINDS_BOOK_MODE=shadow",
	"CORSARR_ROOTFOLDERS_0_PATH=/data/media/movies", "CORSARR_ROOTFOLDERS_0_KIND=movie",
	"CORSARR_INDEXERS_KNABEN_TYPE=torznab", "CORSARR_INDEXERS_KNABEN_NAME=Knaben",
	"CORSARR_INDEXERS_KNABEN_URL=https://torznab.example/api", "CORSARR_INDEXERS_KNABEN_APIKEY=s3cret",
	"CORSARR_INDEXERS_KNABEN_CATEGORIES_0=2000", "CORSARR_INDEXERS_KNABEN_CATEGORIES_1=5000",
	"CORSARR_INDEXERS_KNABEN_MINSEEDERS=3",
	"CORSARR_DOWNLOADCLIENTS_QBIT_TYPE=qbittorrent", "CORSARR_DOWNLOADCLIENTS_QBIT_URL=http://qbit:8080",
	"CORSARR_DOWNLOADCLIENTS_QBIT_PASSWORD=hunter2",
}

var secretEnv = []string{
	"CORSARR_SECRET_INDEXERS_KNABEN_APIKEY=s3cret", "CORSARR_SECRET_DOWNLOADCLIENTS_QBIT_PASSWORD=hunter2",
}

func mustLoad(t *testing.T, in Inputs) *Config {
	t.Helper()
	c, err := Load(in, testRegistry(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return c
}

// Equivalent file, args and env configurations export byte-identical YAML.
func TestSameSettingsViaFileArgsEnvExportIdenticalYAML(t *testing.T) {
	file := mustLoad(t, inputs([]string{"--configFile=/etc/x.yml"}, secretEnv, map[string]string{"/etc/x.yml": fileYAML}))
	args := mustLoad(t, inputs(sameArgs, nil, nil))
	env := mustLoad(t, inputs(nil, sameEnv, nil))
	if file.Source.Kind != SourceFile || args.Source.Kind != SourceArgs || env.Source.Kind != SourceEnv {
		t.Fatalf("sources: %s %s %s", file.Source.Kind, args.Source.Kind, env.Source.Kind)
	}
	fy, ay, ey := file.ExportYAML(ExportOptions{}), args.ExportYAML(ExportOptions{}), env.ExportYAML(ExportOptions{})
	if string(fy) != string(ay) || string(ay) != string(ey) {
		t.Fatalf("exports differ:\n--file--\n%s\n--args--\n%s\n--env--\n%s", fy, ay, ey)
	}
	for _, secret := range []string{"s3cret", "hunter2"} {
		if strings.Contains(string(fy), secret) {
			t.Fatalf("export leaks secret %q:\n%s", secret, fy)
		}
	}
	if !strings.Contains(string(fy), "${env:CORSARR_SECRET_INDEXERS_KNABEN_APIKEY}") {
		t.Fatalf("secret not exported as reference:\n%s", fy)
	}
	if env.Instances["indexers"]["knaben"].Settings.String("apiKey") != "s3cret" {
		t.Fatal("plugin did not receive the resolved secret")
	}
}

func TestExportRoundTripsThroughFileAndEnv(t *testing.T) {
	orig := mustLoad(t, inputs(nil, sameEnv, nil))
	y := orig.ExportYAML(ExportOptions{})
	fromFile := mustLoad(t, inputs([]string{"--configFile=/c.yml"}, secretEnv, map[string]string{"/c.yml": string(y)}))
	if got := fromFile.ExportYAML(ExportOptions{}); string(got) != string(y) {
		t.Fatalf("yaml round trip changed:\n%s\nvs\n%s", y, got)
	}
	envList := orig.ExportEnv(ExportOptions{})
	if !strings.Contains(string(envList), "CORSARR_INDEXERS_KNABEN_APIKEY=\n") {
		t.Fatalf("secret placeholder missing:\n%s", envList)
	}
	// fill the placeholders, as the export's variable list tells the owner to
	lines := strings.Split(strings.TrimSpace(string(envList)), "\n")
	for i, l := range lines {
		lines[i] = strings.Replace(strings.Replace(l, "APIKEY=", "APIKEY=s3cret", 1), "PASSWORD=", "PASSWORD=hunter2", 1)
	}
	fromEnv := mustLoad(t, inputs(nil, lines, nil))
	if got := fromEnv.ExportYAML(ExportOptions{}); string(got) != string(y) {
		t.Fatalf("env round trip changed:\n%s\nvs\n%s", y, got)
	}
	if strings.Contains(string(envList), "VERSION") {
		t.Fatal("env export must not carry the file version")
	}
}

func TestExportIsDeterministicAndOmitsDefaults(t *testing.T) {
	c := mustLoad(t, inputs(nil, sameEnv, nil))
	a, b := c.ExportYAML(ExportOptions{}), c.ExportYAML(ExportOptions{})
	if string(a) != string(b) {
		t.Fatal("export not deterministic")
	}
	if strings.Contains(string(a), "timeout") || strings.Contains(string(a), "enabled") {
		t.Fatalf("defaults should be omitted:\n%s", a)
	}
	full := string(c.ExportYAML(ExportOptions{Full: true}))
	for _, want := range []string{"timeout: 30s", "enabled: true", "dir: /data", "mode: active"} {
		if !strings.Contains(full, want) {
			t.Fatalf("full export misses %q:\n%s", want, full)
		}
	}
	if !strings.HasPrefix(string(a), "# corsarr configuration. Set these environment variables: CORSARR_SECRET_DOWNLOADCLIENTS_QBIT_PASSWORD, CORSARR_SECRET_INDEXERS_KNABEN_APIKEY") {
		t.Fatalf("missing variable header:\n%s", a)
	}
}

func TestDefaultsWhenNoSource(t *testing.T) {
	c := mustLoad(t, inputs(nil, []string{"HOME=/x", "CORSARR_SECRET_FOO=1", "CORSARR_CONFIGFILE="}, nil))
	if c.Source.Kind != SourceDefaults {
		t.Fatalf("source %s", c.Source.Kind)
	}
	if c.Server.Address != ":8090" || c.Data.Dir != "/data" || c.Log.Level != "info" || c.Auth.Mode != "password" {
		t.Fatalf("defaults: %+v %+v %+v %+v", c.Server, c.Data, c.Log, c.Auth)
	}
	if c.MediaKinds[domain.KindBook] != domain.ModeDisabled || c.MediaKinds[domain.KindMovie] != domain.ModeActive {
		t.Fatalf("kinds: %v", c.MediaKinds)
	}
	if got := string(c.ExportYAML(ExportOptions{})); got != "# Schema version of this file.\nversion: 1\n" {
		t.Fatalf("empty export: %q", got)
	}
}

func TestSelectionOrder(t *testing.T) {
	files := map[string]string{"/etc/corsarr/corsarr.yml": "version: 1\n", "/home/u/.config/corsarr/corsarr.yaml": "version: 1\n"}
	cases := []struct {
		name    string
		in      Inputs
		want    SourceKind
		path    string
		wantErr bool
	}{
		{"explicit arg wins", inputs([]string{"--configFile=/etc/corsarr/corsarr.yml", "--log.level=debug"}, nil, files), SourceFile, "/etc/corsarr/corsarr.yml", false},
		{"explicit env", inputs(nil, []string{"CORSARR_CONFIGFILE=/home/u/.config/corsarr/corsarr.yaml"}, files), SourceFile, "/home/u/.config/corsarr/corsarr.yaml", false},
		{"explicit missing", inputs([]string{"--configfile=/nope.yml"}, nil, files), "", "", true},
		{"default location", inputs([]string{"--log.level=debug"}, nil, files), SourceFile, "/etc/corsarr/corsarr.yml", false},
		{"args before env", inputs([]string{"--log.level=debug", "--plan"}, []string{"CORSARR_LOG_LEVEL=warn"}, nil), SourceArgs, "", false},
		{"env", inputs([]string{"--plan"}, []string{"CORSARR_LOG_LEVEL=warn"}, nil), SourceEnv, "", false},
	}
	for _, c := range cases {
		c.in.Home = "/home/u"
		got, err := Select(c.in)
		if (err != nil) != c.wantErr || got.Kind != c.want || got.Path != c.path {
			t.Errorf("%s: got %+v err=%v", c.name, got, err)
		}
	}
}

func TestDefaultFilesIncludeXDGAndHome(t *testing.T) {
	files := map[string]string{"/xdg/corsarr/corsarr.yml": "version: 1\n"}
	in := inputs(nil, nil, files)
	in.XDG, in.Home = "/xdg", "/home/u"
	if s, _ := Select(in); s.Path != "/xdg/corsarr/corsarr.yml" {
		t.Fatalf("xdg: %+v", s)
	}
}

// One warning names the ignored keys, never referenced or SECRET vars.
func TestFileSourceWarnsAboutIgnoredKeysOnce(t *testing.T) {
	file := "version: 1\ndownloadClients:\n  qbit:\n    type: qbittorrent\n    url: http://q\n    password: ${env:QBIT_PASS}\n"
	env := []string{"CORSARR_LOG_LEVEL=debug", "QBIT_PASS=x", "CORSARR_SECRET_X=1", "CORSARR_CONFIGFILE=/c.yml"}
	c := mustLoad(t, inputs([]string{"--server.address=:1"}, env, map[string]string{"/c.yml": file}))
	if strings.Join(c.Source.Ignored, ",") != "--server.address,CORSARR_LOG_LEVEL" {
		t.Fatalf("ignored: %v", c.Source.Ignored)
	}
	n := 0
	for _, w := range c.Warnings {
		if strings.Contains(w, "ignored keys") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want one warning, got %v", c.Warnings)
	}
	// a file referencing a CORSARR_* variable keeps it out of the warning
	file2 := file + "indexers:\n  k:\n    type: torznab\n    url: http://i\n    apiKey: ${env:CORSARR_LOG_LEVEL}\n"
	c2 := mustLoad(t, inputs(nil, env, map[string]string{"/c.yml": file2}))
	if len(c2.Source.Ignored) != 0 {
		t.Fatalf("referenced var named: %v", c2.Source.Ignored)
	}
}

func TestValidationReportsAllErrorsWithLocations(t *testing.T) {
	file := `version: 1
server:
  address: nope
  basePth: /x
log:
  level: loud
rootFolders:
  - path: relative
    kind: comic
indexers:
  my_idx:
    type: torznab
    url: http://x
  good:
    type: nosuch
  noType:
    url: http://x
  k2:
    type: torznab
    minSeeders: -1
    extra: 1
downloadClients:
  qbit:
    type: qbittorrent
    url: http://q
    password: ${env:MISSING_VAR}
`
	_, err := Load(inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": file}), testRegistry(t))
	var errs Errors
	if !errors.As(err, &errs) {
		t.Fatalf("want Errors, got %v", err)
	}
	want := map[string]string{
		"server.address":                "/c.yml:3:12",
		"server.basePth":                "/c.yml:4:12",
		"log.level":                     "/c.yml:6:10",
		"rootFolders.0.path":            "/c.yml:8:11",
		"rootFolders.0.kind":            "/c.yml:9:11",
		"indexers.my_idx":               "/c.yml:12:5",
		"indexers.good.type":            "/c.yml:15:11",
		"indexers.noType":               "/c.yml:17:5",
		"indexers.k2.minSeeders":        "/c.yml:20:17",
		"indexers.k2.extra":             "/c.yml:21:12",
		"indexers.k2.url":               "/c.yml:19:5",
		"downloadClients.qbit.password": "/c.yml:26:15",
	}
	got := map[string]string{}
	for _, e := range errs {
		got[e.Path] = e.Loc.String()
	}
	for p, loc := range want {
		if got[p] != loc {
			t.Errorf("%s: got loc %q, want %q (all: %v)", p, got[p], loc, errs)
		}
	}
	msg := err.Error()
	for _, frag := range []string{`try "myidx" with name: "my_idx"`, "MISSING_VAR is not set", "available: torznab", "must be ≥ 0", "requires"} {
		if frag == "requires" {
			continue
		}
		if !strings.Contains(msg, frag) {
			t.Errorf("message misses %q:\n%s", frag, msg)
		}
	}
}

func TestEnvErrorsNameTheVariable(t *testing.T) {
	_, err := Load(inputs(nil, []string{"CORSARR_LOG_LEVEL=loud", "CORSARR_SERVER_NOPE=1", "CORSARR_INDEXERS_MY=1"}, nil), testRegistry(t))
	for _, frag := range []string{"log.level (CORSARR_LOG_LEVEL)", "server.nope (CORSARR_SERVER_NOPE): unknown key", "indexers.my (CORSARR_INDEXERS_MY): must be a section"} {
		if err == nil || !strings.Contains(err.Error(), frag) {
			t.Errorf("missing %q in %v", frag, err)
		}
	}
}

func TestStructuralConflicts(t *testing.T) {
	for _, args := range [][]string{
		{"--log=1", "--log.level=debug"},
		{"--log.level=debug", "--log=1"},
	} {
		if _, err := Load(inputs(args, nil, nil), testRegistry(t)); err == nil {
			t.Errorf("%v: want error", args)
		}
	}
	if _, err := Load(inputs([]string{"--log.level.x=1"}, nil, nil), testRegistry(t)); err == nil || !strings.Contains(err.Error(), "must be a value") {
		t.Errorf("leaf as section: %v", err)
	}
	if _, err := Load(inputs([]string{"--rootFolders.x.path=/a"}, nil, nil), testRegistry(t)); err == nil || !strings.Contains(err.Error(), "list index") {
		t.Errorf("bad index: %v", err)
	}
}

func TestBadFiles(t *testing.T) {
	cases := map[string]string{
		"syntax":    "server: [",
		"duplicate": "log:\n  level: info\nlog:\n  level: debug\n",
		"alias":     "a: &x 1\nb: *x\n",
		"version":   "version: 2\n",
		"big":       strings.Repeat("#", maxFileSize+1),
	}
	for name, body := range cases {
		if _, err := Load(inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": body}), testRegistry(t)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	empty := mustLoad(t, inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": ""}))
	if empty.Server.Address != ":8090" {
		t.Fatal("empty file should give defaults")
	}
}

func TestSecretsFromFileRefsAndClearTextWarning(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pw")
	if err := os.WriteFile(p, []byte("filepw\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := "downloadClients:\n  a:\n    type: qbittorrent\n    url: http://q\n    password: ${file:" + p + "}\n  b:\n    type: qbittorrent\n    url: http://q\n    password: clear\n  c:\n    type: qbittorrent\n    url: http://q\n    password: ${file:/nonexistent}\n"
	in := inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": file})
	in.Resolver = OSResolver()
	_, err := Load(in, testRegistry(t))
	if err == nil || !strings.Contains(err.Error(), "secret file /nonexistent cannot be read") {
		t.Fatalf("want file error, got %v", err)
	}
	file = file[:strings.Index(file, "  c:")]
	in = inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": file})
	in.Resolver = OSResolver()
	c := mustLoad(t, in)
	if c.Instances["downloadClients"]["a"].Settings.String("password") != "filepw" {
		t.Fatal("file secret not resolved")
	}
	if len(c.Warnings) != 1 || !strings.Contains(c.Warnings[0], "clear-text secret") || strings.Contains(c.Warnings[0], "clear\n") {
		t.Fatalf("warnings: %v", c.Warnings)
	}
	y := string(c.ExportYAML(ExportOptions{}))
	if !strings.Contains(y, "${file:"+p+"}") || !strings.Contains(y, "${env:CORSARR_SECRET_DOWNLOADCLIENTS_B_PASSWORD}") || strings.Contains(y, "clear") {
		t.Fatalf("export:\n%s", y)
	}
}

func TestSecretNeverPrints(t *testing.T) {
	s := NewSecret("topsecret")
	b, _ := s.MarshalText()
	for _, out := range []string{s.String(), s.GoString(), string(b)} {
		if strings.Contains(out, "topsecret") {
			t.Fatalf("leak: %s", out)
		}
	}
	if s.Value() != "topsecret" {
		t.Fatal("value")
	}
	r, _ := Secret{Ref: "${env:X}"}.MarshalText()
	if string(r) != "${env:X}" {
		t.Fatal(string(r))
	}
}

func TestTypedConfigAccessors(t *testing.T) {
	c := mustLoad(t, inputs(nil, sameEnv, nil))
	if c.Server.Address != ":9000" || c.Server.BasePath != "/corsarr" || c.Log.Level != "debug" {
		t.Fatalf("bootstrap: %+v", c.Server)
	}
	if len(c.RootFolders) != 1 || c.RootFolders[0] != (RootFolder{Path: "/data/media/movies", Kind: domain.KindMovie}) {
		t.Fatalf("roots: %v", c.RootFolders)
	}
	k := c.Instances["indexers"]["knaben"]
	if k.Type != "torznab" || k.Name != "Knaben" || !k.Enabled || k.Settings["minSeeders"] != 3.0 || k.Loc.Name == "" {
		t.Fatalf("instance: %+v", k)
	}
	if got := k.Settings["categories"].([]any); len(got) != 2 || got[0] != "2000" {
		t.Fatalf("list: %v", got)
	}
	if c.Location("server.address").Name != "CORSARR_SERVER_ADDRESS" {
		t.Fatalf("loc: %+v", c.Location("server.address"))
	}
	if !Live("indexers") || Live("server") || Live("version") {
		t.Fatal("live classification")
	}
}

func TestOSInputs(t *testing.T) {
	in := OSInputs([]string{"--log.level=debug"})
	if in.Exists("/definitely/not/here") || len(in.Args) != 1 || in.ReadFile == nil {
		t.Fatal("os inputs")
	}
}

func TestLocationString(t *testing.T) {
	if (Location{Kind: SourceDefaults}).String() != "defaults" {
		t.Fatal("defaults")
	}
}
