package plugin_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/sdk/go/abi"
	"github.com/jrand0m/corsarr/sdk/go/plugin"
	"github.com/jrand0m/corsarr/sdk/go/plugintest"
)

func TestHostHelpers(t *testing.T) {
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	h := plugintest.New(plugintest.Host{Settings: map[string]any{"url": "http://x"}, Secrets: map[string]string{"key": "v"},
		Now: func() time.Time { return now }})
	plugin.Info("hello", "k", 1)
	plugin.Debug("d")
	plugin.Warn("w")
	if logs := h.Logs(); len(logs) != 3 || logs[0].Attrs["k"] != 1.0 || logs[2].Level != "warn" {
		t.Errorf("%+v", logs)
	}
	if plugin.SettingString("url") != "http://x" || plugin.SettingString("missing") != "" {
		t.Error("settings")
	}
	if v, err := plugin.Secret("key"); err != nil || v != "v" {
		t.Error("secret")
	}
	if _, err := plugin.Secret("nope"); plugin.ClassOf(err) != abi.ClassDenied {
		t.Errorf("undeclared secret: %v", err)
	}
	if !plugin.Now().Equal(now) || plugin.Sleep(time.Millisecond) != nil {
		t.Error("clock")
	}
	if err := plugin.Emit("download.progress", map[string]any{"a": 1}); err != nil || len(h.Events()) != 1 {
		t.Error("emit")
	}
}

func TestClientJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_ = json.NewEncoder(w).Encode(map[string]string{"got": r.URL.Query().Get("q") + r.Header.Get("X-K")})
		case "/bad":
			_, _ = w.Write([]byte("not json"))
		default:
			w.WriteHeader(map[string]int{"/auth": 401, "/missing": 404, "/slow": 429, "/boom": 500, "/teapot": 418}[r.URL.Path])
		}
	}))
	defer srv.Close()
	plugintest.New(plugintest.Host{Hosts: []string{"127.0.0.1"}})
	c := plugin.Client{Base: srv.URL, Headers: map[string][]string{"X-K": {"k"}}}
	var out map[string]string
	if err := c.JSON("POST", "/ok", map[string][]string{"q": {"v"}}, map[string]int{"a": 1}, &out); err != nil || out["got"] != "vk" {
		t.Fatalf("%v %v", out, err)
	}
	for path, class := range map[string]string{"/auth": abi.ClassAuth, "/missing": abi.ClassNotFound, "/slow": abi.ClassRateLimited,
		"/boom": abi.ClassServerError, "/teapot": abi.ClassParse, "/bad": abi.ClassParse} {
		if err := c.JSON("GET", path, nil, nil, &out); plugin.ClassOf(err) != class {
			t.Errorf("%s: %v", path, err)
		}
	}
	plugintest.New(plugintest.Host{Hosts: []string{"*.example.org"}})
	if _, err := plugin.Do(plugin.Request{Method: "GET", URL: srv.URL}); plugin.ClassOf(err) != abi.ClassDenied {
		t.Errorf("undeclared host: %v", err)
	}
	if err := c.JSON("GET", "/ok", nil, func() {}, nil); err == nil {
		t.Error("unencodable body")
	}
}

func TestDispatchAndErrors(t *testing.T) {
	h := plugintest.New(plugintest.Host{})
	plugin.Op("demo.echo", func(r map[string]string) (map[string]string, error) { return r, nil })
	plugin.Op("demo.fail", func(plugin.Empty) (plugin.Empty, error) { return plugin.Empty{}, errors.New("plain") })
	plugin.Op("demo.panic", func(plugin.Empty) (plugin.Empty, error) { panic("oops") })
	plugin.Handle("demo.bad", func([]byte) (any, error) { return func() {}, nil })
	plugin.OnTest(func() error { return plugin.Unsupported("x") })
	var echo map[string]string
	if err := h.Call("demo.echo", map[string]string{"a": "b"}, &echo); err != nil || echo["a"] != "b" {
		t.Fatal(err)
	}
	for op, class := range map[string]string{"demo.fail": abi.ClassServerError, "demo.panic": abi.ClassServerError,
		"demo.bad": abi.ClassServerError, "demo.none": abi.ClassUnsupported, abi.OpTest: abi.ClassUnsupported} {
		if err := h.Call(op, struct{}{}, nil); plugin.ClassOf(err) != class {
			t.Errorf("%s: %v", op, err)
		}
	}
	if err := h.Call("demo.echo", "not an object", nil); plugin.ClassOf(err) != abi.ClassParse {
		t.Errorf("malformed: %v", err)
	}
	if plugin.ClassOf(plugin.NotFound("x")) != abi.ClassNotFound || plugin.ClassOf(errors.New("x")) != "" {
		t.Error("ClassOf")
	}
}

func TestSchemaBuilder(t *testing.T) {
	s := plugin.Schema(
		plugin.String("url", "Address").Pattern(abi.PatternURLHTTP).Required().Describe("d").Hint("h").Length(1, 200),
		plugin.SecretField("key", "Key"),
		plugin.Bool("on", "On").Default(true),
		plugin.Int("port", "Port").Range(1, 65535).Step(1),
		plugin.Number("ratio", "Ratio").Decimals(2),
		plugin.Duration("every", "Every").Between("1s", "1h"),
		plugin.StringList("tags", "Tags").MaxItems(5),
		plugin.IntList("ids", "Ids"),
		plugin.Map("paths", "Paths").Entries(3, abi.TextRule{}, abi.TextRule{}),
		plugin.Enum("mode", "Mode", plugin.Opt("a", "A")),
	)
	s.Groups = []abi.Group{plugin.Group("G", "url")}
	s.Actions = []abi.Action{plugin.Action("test", "Test")}
	if len(s.Fields) != 10 || !s.Fields[0].Required || *s.Fields[3].Max != 65535 || s.Fields[8].MaxEntries != 3 || s.Fields[9].Options[0].Value != "a" {
		t.Fatalf("%+v", s)
	}
	m := plugin.Manifest{ID: "x", Settings: s}
	b, err := m.JSON()
	if err != nil || !json.Valid(b) {
		t.Fatal(err)
	}
	if e := plugin.EmbedManifest([]byte("\x00asm\x01\x00\x00\x00"), b); len(e) <= len(b) {
		t.Error("embed")
	}
}
