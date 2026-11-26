package config

import (
	"strings"
	"testing"
)

func TestLegacyAppsAreOffUntilEnabled(t *testing.T) {
	def := mustLoad(t, inputs(nil, nil, nil))
	for _, app := range []string{LegacySonarr, LegacyRadarr, LegacyLidarr, LegacyProwlarr} {
		if def.LegacyEnabled(app) {
			t.Errorf("%s on by default", app)
		}
	}
	file := mustLoad(t, inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": "legacy:\n  sonarr: { enabled: true }\n"}))
	args := mustLoad(t, inputs([]string{"--legacy.sonarr.enabled=true"}, nil, nil))
	env := mustLoad(t, inputs(nil, []string{"CORSARR_LEGACY_SONARR_ENABLED=true"}, nil))
	for name, c := range map[string]*Config{"file": file, "args": args, "env": env} {
		if !c.LegacyEnabled(LegacySonarr) || c.LegacyEnabled(LegacyRadarr) {
			t.Errorf("%s: %v", name, c.Legacy)
		}
	}
	if _, err := Load(inputs(nil, []string{"CORSARR_LEGACY_READARR_ENABLED=true"}, nil), testRegistry(t)); err == nil {
		t.Fatal("an app without a shim is refused")
	}
	if y := string(env.ExportYAML(ExportOptions{})); !strings.Contains(y, "legacy:") {
		t.Fatalf("export keeps the legacy section:\n%s", y)
	}
	if !Live("legacy") {
		t.Fatal("compatibility apps switch live")
	}
	var nilCfg *Config
	if nilCfg.LegacyEnabled(LegacySonarr) {
		t.Fatal("no config, nothing enabled")
	}
}
