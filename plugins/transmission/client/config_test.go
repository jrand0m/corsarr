package client

import (
	"errors"
	"testing"
	"time"
)

func TestEndpoint(t *testing.T) {
	cases := map[string]string{
		"http://host:9091":                    "http://host:9091/transmission/rpc",
		"http://host:9091/":                   "http://host:9091/transmission/rpc",
		" https://proxy/torrents ":            "https://proxy/torrents/transmission/rpc",
		"http://host:9091/transmission/rpc":   "http://host:9091/transmission/rpc",
		"http://host/custom/rpc?x=1#frag":     "http://host/custom/rpc",
		"http://user@host:9091/transmission/": "http://user@host:9091/transmission/transmission/rpc",
	}
	for in, want := range cases {
		if got, err := Endpoint(in); err != nil || got != want {
			t.Errorf("%q: %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "host:9091", "ftp://host", "http://"} {
		if _, err := Endpoint(bad); ClassOf(err) != ClassConfig {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestNewValidates(t *testing.T) {
	bad := []Config{
		{URL: "nope"},
		{URL: "http://h", PathMap: map[string]string{"relative": "/x"}},
		{URL: "http://h", CategoryDirs: map[string]string{"tv": " "}},
	}
	for _, cfg := range bad {
		if _, err := New(nil, cfg); ClassOf(err) != ClassConfig {
			t.Errorf("%+v: %v", cfg, err)
		}
	}
	c, err := New(nil, Config{URL: "http://h"})
	if err != nil || c.timeout != DefaultTimeout || c.auth != "" {
		t.Errorf("defaults: %+v %v", c, err)
	}
}

func TestPathMap(t *testing.T) {
	m, err := ParsePathMap(map[string]string{"/data": "/mnt/data", "/data/tv": "/srv/tv", `D:\Torrents`: "/mnt/d", "/": "/root"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ remote, local string }{
		{"/data/movies/x", "/mnt/data/movies/x"},
		{"/data/tv/show", "/srv/tv/show"},
		{"/data", "/mnt/data"},
		{"/database", "/root/database"},
		{`d:\torrents\film`, "/mnt/d/film"},
	}
	for _, tc := range cases {
		if got, ok := m.ToLocal(tc.remote); !ok || got != tc.local {
			t.Errorf("ToLocal(%q) = %q %v, want %q", tc.remote, got, ok, tc.local)
		}
	}
	if got, ok := m.ToRemote("/mnt/d/film"); !ok || got != `D:\Torrents\film` {
		t.Errorf("ToRemote windows = %q", got)
	}
	if got, ok := (PathMap{}).ToLocal("/x"); ok || got != "/x" {
		t.Errorf("no rules: %q %v", got, ok)
	}
	if got, ok := m.ToLocal(""); ok || got != "" {
		t.Errorf("empty: %q", got)
	}
	for _, bad := range []map[string]string{{"a": "/b"}, {"/a": "b"}, {"/a": "/b", "/a/": "/c"}} {
		if _, err := ParsePathMap(bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	if joinPath(`C:\dl\`, "x") != `C:\dl\x` || joinPath("", "x") != "x" || joinPath("/dl/", "x") != "/dl/x" {
		t.Error("joinPath")
	}
}

func TestSettingsDescribeEveryConfigField(t *testing.T) {
	keys := map[string]FieldType{}
	for _, f := range Settings() {
		if f.Label == "" || f.Help == "" {
			t.Errorf("%s lacks label or help", f.Key)
		}
		keys[f.Key] = f.Type
	}
	want := map[string]FieldType{KeyURL: FieldURL, KeyUsername: FieldText, KeyPassword: FieldSecret,
		KeyCategoryDirs: FieldMap, KeyPathMap: FieldMap, KeyAddPaused: FieldBool, KeyTimeout: FieldDuration}
	for k, typ := range want {
		if keys[k] != typ {
			t.Errorf("%s: %q, want %q", k, keys[k], typ)
		}
	}
}

func TestConfigFromSettings(t *testing.T) {
	cfg, err := ConfigFromSettings(map[string]any{
		KeyURL: " http://h:9091 ", KeyUsername: "u", KeyPassword: "p", KeyAddPaused: true, KeyTimeout: "45s",
		KeyCategoryDirs: map[string]any{"tv": "/dl/tv"}, KeyPathMap: map[string]string{"/dl": "/mnt/dl"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "http://h:9091" || cfg.Username != "u" || cfg.Password != "p" || !cfg.AddPaused ||
		cfg.Timeout != 45*time.Second || cfg.CategoryDirs["tv"] != "/dl/tv" || cfg.PathMap["/dl"] != "/mnt/dl" {
		t.Errorf("cfg = %+v", cfg)
	}
	for _, v := range []any{float64(2), 2, 2 * time.Second, "2s"} {
		cfg, err := ConfigFromSettings(map[string]any{KeyTimeout: v})
		if err != nil || cfg.Timeout != 2*time.Second {
			t.Errorf("timeout %v: %v %v", v, cfg.Timeout, err)
		}
	}
	if cfg, err := ConfigFromSettings(map[string]any{KeyTimeout: " "}); err != nil || cfg.Timeout != 0 {
		t.Errorf("blank timeout: %v", err)
	}
	for _, bad := range []map[string]any{
		{KeyTimeout: "soon"}, {KeyTimeout: "-1s"}, {KeyTimeout: true},
		{KeyCategoryDirs: "tv=/x"}, {KeyPathMap: map[string]any{"/a": 1}},
	} {
		if _, err := ConfigFromSettings(bad); ClassOf(err) != ClassConfig {
			t.Errorf("%v: %v", bad, err)
		}
	}
}

func TestErrorFormatting(t *testing.T) {
	inner := errors.New("boom")
	e := wrap(ClassParse, "reading", inner)
	if e.Error() != "parse: reading: boom" || !errors.Is(e, inner) {
		t.Errorf("wrap: %v", e)
	}
	if errorf(ClassAuth, "no %s", "entry").Error() != "auth: no entry" {
		t.Error("errorf")
	}
	if ClassOf(inner) != "" {
		t.Error("plain error has a class")
	}
}
