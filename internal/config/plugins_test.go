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
	if Live("plugins") {
		t.Error("plugins must apply at start")
	}
}

func TestPeek(t *testing.T) {
	b, err := Peek(inputs([]string{"--data.dir=/d", "--plugins.disabledBuiltins.0=x", "--indexers.a.type=nope"}, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if b.DataDir != "/d" || b.ModulesDir() != filepath.Join("/d", "plugins") || b.Plugins.DisabledBuiltins[0] != "x" {
		t.Errorf("%+v", b)
	}
	if _, err := Peek(inputs([]string{"--plugins.dir=relative"}, nil, nil)); err == nil || !strings.Contains(err.Error(), "plugins.dir") {
		t.Errorf("bad dir: %v", err)
	}
}
