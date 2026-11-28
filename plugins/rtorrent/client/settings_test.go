package client

import (
	"testing"
	"time"
)

func TestValueHelpers(t *testing.T) {
	nums := map[any]int64{int64(3): 3, true: 1, false: 0, " 12 ": 12, "x": 0, 1.5: 0, nil: 0}
	for in, want := range nums {
		if got := num(in); got != want {
			t.Errorf("num(%v) = %d, want %d", in, got, want)
		}
	}
	if decodeLabel("100%") != "100%" || decodeLabel("a%20b") != "a b" {
		t.Error("decodeLabel")
	}
	if got := truncate(string(make([]byte, 50))); len(got) != 40+len("…") {
		t.Errorf("truncate kept %d bytes", len(got))
	}
}

func TestFields(t *testing.T) {
	keys := map[string]string{}
	for _, f := range Fields() {
		keys[f.Key] = f.Type
	}
	want := map[string]string{"url": "url", "username": "text", "password": "secret", "pathMap": "map",
		"labelField": "enum", "timeout": "duration"}
	for k, typ := range want {
		if keys[k] != typ {
			t.Errorf("field %s: %q, want %q", k, keys[k], typ)
		}
	}
}

func TestConfigFrom(t *testing.T) {
	cfg, err := ConfigFrom(map[string]any{"url": " http://h/RPC2 ", "username": "u", "password": "p",
		"labelField": "custom2", "timeout": "45s", "pathMap": map[string]any{"/data/": "/mnt/media"}})
	must(t, "ConfigFrom", err)
	if cfg.URL != "http://h/RPC2" || cfg.Username != "u" || cfg.Timeout != 45*time.Second || cfg.LabelField != "custom2" {
		t.Errorf("%+v", cfg)
	}
	if len(cfg.PathMap) != 1 || cfg.PathMap[0] != (PathRule{Remote: "/data", Local: "/mnt/media"}) {
		t.Errorf("path map %+v", cfg.PathMap)
	}
	timeouts := map[any]time.Duration{nil: 0, "": 0, "10": 10 * time.Second, 5: 5 * time.Second, int64(6): 6 * time.Second, 1.5: 1500 * time.Millisecond}
	for in, want := range timeouts {
		cfg, err := ConfigFrom(map[string]any{"timeout": in})
		if err != nil || cfg.Timeout != want {
			t.Errorf("timeout %v → %v %v", in, cfg.Timeout, err)
		}
	}
	maps := []any{map[string]string{"/a": "/b"}, []any{"/a=/b"}, []string{"/a=/b"}}
	for _, m := range maps {
		cfg, err := ConfigFrom(map[string]any{"pathMap": m})
		if err != nil || cfg.PathMap.ToLocal("/a/x") != "/b/x" {
			t.Errorf("path map %v: %+v %v", m, cfg.PathMap, err)
		}
	}
	bad := []map[string]any{
		{"timeout": "soon"}, {"timeout": "-1s"}, {"timeout": true},
		{"pathMap": 5}, {"pathMap": map[string]any{"/a": 1}}, {"pathMap": []any{"/a"}}, {"pathMap": []any{"/a=/b=/c"}},
		{"pathMap": map[string]string{"rel": "/b"}}, {"pathMap": []string{"/a/=/b", "/a=/c"}},
	}
	for _, s := range bad {
		if _, err := ConfigFrom(s); KindOf(err) != KindConfig {
			t.Errorf("%v accepted: %v", s, err)
		}
	}
}

func TestPathMap(t *testing.T) {
	m, err := ParsePathMap(map[string]string{"/data": "/mnt/media", "/data/tv": "/srv/tv", "/": "/root", `C:\dl`: "/c"})
	must(t, "parse", err)
	cases := map[string]string{
		"/data/movies/x.mkv": "/mnt/media/movies/x.mkv",
		"/data/tv/show":      "/srv/tv/show",
		"/data":              "/mnt/media",
		"/database":          "/root/database",
		"":                   "",
	}
	for in, want := range cases {
		if got := m.ToLocal(in); got != want {
			t.Errorf("ToLocal(%s) = %s, want %s", in, got, want)
		}
	}
	if got := m.ToRemote("/srv/tv/a"); got != "/data/tv/a" {
		t.Errorf("ToRemote = %s", got)
	}
	if got := m.ToRemote("/c/x"); got != `C:\dl\x` {
		t.Errorf("windows ToRemote = %s", got)
	}
	if got := (PathMap{{Remote: "/a", Local: "/b"}}).ToLocal("relative"); got != "relative" {
		t.Errorf("uncovered = %s", got)
	}
	if !windowsStyle(`\\nas\share`) || windowsStyle("/x") {
		t.Error("windowsStyle")
	}
}
