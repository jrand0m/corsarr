package config

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/plugin"
)

func rowByKey(t *testing.T, rows []Row, key string) Row {
	t.Helper()
	for _, r := range rows {
		if r.Key() == key {
			return r
		}
	}
	t.Fatalf("no row %s", key)
	return Row{}
}

func TestReferenceRowForms(t *testing.T) {
	ref := NewSchema(testRegistry(t)).Reference()
	addr := rowByKey(t, ref.Rows, "server.address")
	if addr.Env() != "CORSARR_SERVER_ADDRESS" || addr.Flag() != "--server.address=" || addr.ShownDefault() != ":8090" || addr.Live {
		t.Fatalf("address row: %+v", addr)
	}
	kinds := rowByKey(t, ref.Rows, "mediaKinds.book.mode")
	if !kinds.Live || kinds.Default != "disabled" || !strings.Contains(kinds.Explanation(), "One of: disabled, shadow, active.") {
		t.Fatalf("enum row: %+v", kinds)
	}
	folder := rowByKey(t, ref.Rows, "rootFolders.<n>.path")
	if folder.Env() != "CORSARR_ROOTFOLDERS_<N>_PATH" || folder.ShownDefault() != "" {
		t.Fatalf("list row: %+v", folder)
	}
	rowByKey(t, ref.Rows, "indexers.<name>.<setting>")
	rowByKey(t, ref.Rows, "automation.rules.<name>.mode")
}

func TestReferenceMarksExactlyTheHotReloadedSectionsLive(t *testing.T) {
	sections := map[string]bool{}
	for _, r := range NewSchema(testRegistry(t)).Reference().Rows {
		sections[r.Path[0]] = true
		if r.Live != Live(r.Path[0]) {
			t.Errorf("%s: reference says live=%v, the watcher reloads it: %v", r.Key(), r.Live, Live(r.Path[0]))
		}
	}
	for _, s := range []string{"mediaKinds", "rootFolders", "customFormats", "qualityProfiles", "naming", "automation", "legacy", "mcp", "indexers"} {
		if !sections[s] {
			t.Errorf("no reference row under %s", s)
		}
	}
}

func TestARootFolderNeedsAPathAndAKind(t *testing.T) {
	for _, file := range []string{"rootFolders:\n  - kind: movie\n", "rootFolders:\n  - path: /media/movies\n"} {
		_, err := Load(inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": file}), testRegistry(t))
		if err == nil || !strings.Contains(err.Error(), "is required") {
			t.Errorf("%q: %v", file, err)
		}
	}
}

func TestReferencePluginTables(t *testing.T) {
	ref := NewSchema(testRegistry(t)).Reference()
	if len(ref.Plugins) != 2 || ref.Plugins[0].ID != "torznab" || ref.Plugins[1].Section != "downloadClients" {
		t.Fatalf("plugin tables: %+v", ref.Plugins)
	}
	key := rowByKey(t, ref.Plugins[0].Rows, "indexers.<name>.apiKey")
	if !key.Secret || key.ShownDefault() != "(secret, unset)" || !key.Live {
		t.Fatalf("secret row: %+v", key)
	}
	timeout := rowByKey(t, ref.Plugins[0].Rows, "indexers.<name>.timeout")
	if timeout.Default != "30s" || !strings.Contains(timeout.Note, "duration") {
		t.Fatalf("duration row: %+v", timeout)
	}
	rowByKey(t, ref.Plugins[0].Rows, "indexers.<name>.categories.<n>")
}

// A setting without an explanation is reported by its key.
func TestMissingDocsNamesTheKey(t *testing.T) {
	missing := NewSchema(testRegistry(t)).Reference().MissingDocs()
	if !slices.Contains(missing, "indexers.<name>.url") || !slices.Contains(missing, "downloadClients.<name>.category") {
		t.Fatalf("missing: %v", missing)
	}
	for _, k := range missing {
		if !strings.Contains(k, "<name>.") || strings.HasPrefix(k, "server") {
			t.Fatalf("core setting without explanation: %s", k)
		}
	}
}

func TestPluginDefaultsWithListsAndMaps(t *testing.T) {
	reg := plugin.NewRegistry()
	reg.MustRegister(plugin.Registration{New: func(plugin.Instance) (any, error) { return nil, nil }, Manifest: plugin.Manifest{
		ID: "dl", Kind: plugin.KindDownloader, APIVersion: plugin.APIVersion, Name: "DL",
		Settings: []plugin.Field{
			{Key: "categories", Type: plugin.FieldMap, Help: "Per kind.", Default: map[string][]int{"tv": {5000, 5040}, "movie": {2000}}},
			{Key: "urls", Type: plugin.FieldList, Help: "Service URLs.", Secret: true},
		},
	}})
	s := NewSchema(reg)
	rows := s.Reference().Plugins[0].Rows
	if got := rowByKey(t, rows, "downloadClients.<name>.categories.<name>").Default; got != "movie=2000 tv=5000,5040" {
		t.Fatalf("map default %q", got)
	}
	if !rowByKey(t, rows, "downloadClients.<name>.urls.<n>").Secret {
		t.Fatal("secret list not marked")
	}
	if y := string(s.DefaultsYAML()); !strings.Contains(y, "#     categories:\n#       movie:\n#         - 2000\n") {
		t.Fatalf("yaml map default:\n%s", y)
	}
}

func TestFormatAny(t *testing.T) {
	for in, want := range map[string]any{"a,b": []string{"a", "b"}, "x=1 y=2": map[string]int{"y": 2, "x": 1}, "7": 7} {
		if got := formatAny(want); got != in {
			t.Fatalf("formatAny(%v) = %q", want, got)
		}
	}
}

// The generated YAML file loads and gives exactly the built-in defaults.
func TestDefaultsYAMLLoadsAsDefaults(t *testing.T) {
	s := NewSchema(testRegistry(t))
	files := map[string]string{"/c.yml": string(s.DefaultsYAML())}
	got := mustLoad(t, inputs([]string{"--configFile=/c.yml"}, nil, files))
	want := mustLoad(t, inputs(nil, nil, nil))
	if g, w := got.ExportYAML(ExportOptions{Full: true}), want.ExportYAML(ExportOptions{Full: true}); !bytes.Equal(g, w) {
		t.Fatalf("file:\n%s\ndefaults:\n%s", g, w)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings: %v", got.Warnings)
	}
}

// The active lines of the env list load and give exactly the built-in defaults.
func TestDefaultsEnvLoadsAsDefaults(t *testing.T) {
	out := string(NewSchema(testRegistry(t)).Reference().Env())
	var environ []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "CORSARR_") {
			environ = append(environ, l)
		}
	}
	if len(environ) < 10 || !strings.Contains(out, "# CORSARR_SERVER_BASEPATH=\n") || !strings.Contains(out, "# CORSARR_INDEXERS_<NAME>_APIKEY=\n") {
		t.Fatalf("env output:\n%s", out)
	}
	got := mustLoad(t, inputs(nil, environ, nil))
	want := mustLoad(t, inputs(nil, nil, nil))
	if g, w := got.ExportYAML(ExportOptions{Full: true}), want.ExportYAML(ExportOptions{Full: true}); !bytes.Equal(g, w) {
		t.Fatalf("env:\n%s\ndefaults:\n%s", g, w)
	}
}

func TestMarkdownAndTable(t *testing.T) {
	ref := NewSchema(testRegistry(t)).Reference()
	md := string(ref.Markdown())
	for _, want := range []string{
		"# Configuration reference", "| `server.address` | `:8090` | `CORSARR_SERVER_ADDRESS` | `--server.address=` | no |",
		"| `server.basePath` | — |", "(secret, unset)", "### Indexer plugins (`indexers`)", "#### Torznab (`type: torznab`)",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown lacks %q:\n%s", want, md)
		}
	}
	if !bytes.Equal(ref.Markdown(), NewSchema(testRegistry(t)).Reference().Markdown()) {
		t.Fatal("markdown not deterministic")
	}
	table := string(ref.Table())
	if !strings.Contains(table, "KEY") || !strings.Contains(table, "Download client plugin qBittorrent (type: qbittorrent)") {
		t.Fatalf("table:\n%s", table)
	}
	var b bytes.Buffer
	mdTable(&b, nil)
	if b.String() != "No settings.\n" || mdEscape("a|b") != `a\|b` || capitalize("") != "" {
		t.Fatal("helpers")
	}
}

func TestLeafNotes(t *testing.T) {
	if leafNote(&Node{Kind: KindEnum}) != "" || leafNote(&Node{Kind: KindString}) != "" || leafNote(&Node{Kind: KindSecret}) == "" {
		t.Fatal("notes")
	}
	if got := yamlScalar(&Node{Kind: KindDuration, Default: "90m"}); got != "1h30m0s" {
		t.Fatalf("duration scalar %q", got)
	}
}
