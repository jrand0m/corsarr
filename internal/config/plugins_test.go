package config

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPluginsSection(t *testing.T) {
	reg := testRegistry(t)
	c, err := Load(inputs(nil, nil, nil), reg)
	if err != nil {
		t.Fatal(err)
	}
	p := c.Plugins
	if p.MemoryMiB != 64 || p.CallTimeout != 10*time.Second || p.Concurrency != 4 || p.MaxModuleMiB != 32 || p.Dir != "" {
		t.Fatalf("defaults %+v", p)
	}
	if c.ModulesDir() != filepath.Join("/data", "plugins") {
		t.Errorf("modules dir %s", c.ModulesDir())
	}
	c, err = Load(inputs([]string{"--plugins.dir=/srv/mods", "--plugins.callTimeout=2s", "--plugins.disabledBuiltins.0=sabnzbd"}, nil, nil), reg)
	if err != nil {
		t.Fatal(err)
	}
	if c.ModulesDir() != "/srv/mods" || c.Plugins.CallTimeout != 2*time.Second || !slices.Equal(c.Plugins.DisabledBuiltins, []string{"sabnzbd"}) {
		t.Errorf("%+v", c.Plugins)
	}
	for _, bad := range []string{"--plugins.memoryMiB=1", "--plugins.callTimeout=1ms", "--plugins.dir=rel", "--plugins.concurrency=0"} {
		if _, err := Load(inputs([]string{bad}, nil, nil), reg); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	if !p.UIInstall || p.AutoUpdate != "off" || p.CatalogueURL != "" || len(p.Install) != 0 {
		t.Errorf("install defaults %+v", p)
	}
	c, err = Load(inputs([]string{"--plugins.uiInstall=false", "--plugins.autoUpdate=patch", "--plugins.catalogue.url=https://example.org/plugins-index.json",
		"--plugins.install.0.id=transmission", "--plugins.install.1.id=rtorrent", "--plugins.install.1.version=1.2.0"}, nil, nil), reg)
	if err != nil {
		t.Fatal(err)
	}
	want := []PluginInstall{{ID: "transmission"}, {ID: "rtorrent", Version: "1.2.0"}}
	if p := c.Plugins; p.UIInstall || p.AutoUpdate != "patch" || p.CatalogueURL != "https://example.org/plugins-index.json" || !slices.Equal(p.Install, want) {
		t.Errorf("install leaves %+v", p)
	}
	for _, bad := range []string{"--plugins.autoUpdate=major", "--plugins.catalogue.url=ftp://x", "--plugins.install.0.id=Bad",
		"--plugins.install.0.version=1.0.0"} {
		if _, err := Load(inputs([]string{bad}, nil, nil), reg); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	if Live("plugins") {
		t.Error("plugins must apply at start")
	}
}

func TestPeek(t *testing.T) {
	b, err := Peek(inputs([]string{"--data.dir=/d", "--plugins.disabledBuiltins.0=x", "--indexers.a.type=nope", "--server.egressAllow.0=example.org"}, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if b.DataDir != "/d" || b.ModulesDir() != filepath.Join("/d", "plugins") || b.Plugins.DisabledBuiltins[0] != "x" || !slices.Equal(b.EgressAllow, []string{"example.org"}) {
		t.Errorf("%+v", b)
	}
	if _, err := Peek(inputs([]string{"--plugins.dir=relative"}, nil, nil)); err == nil || !strings.Contains(err.Error(), "plugins.dir") {
		t.Errorf("bad dir: %v", err)
	}
}
