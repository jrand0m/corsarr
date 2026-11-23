package config

import (
	"strings"
	"testing"
)

func TestRuleModesFromFileArgsAndEnv(t *testing.T) {
	file := mustLoad(t, inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": "automation:\n  rules:\n    recyclepurge: { mode: live }\n    itemdelete: { mode: \"off\" }\n"}))
	args := mustLoad(t, inputs([]string{"--automation.rules.recyclepurge.mode=live", "--automation.rules.itemdelete.mode=off"}, nil, nil))
	env := mustLoad(t, inputs(nil, []string{"CORSARR_AUTOMATION_RULES_RECYCLEPURGE_MODE=live", "CORSARR_AUTOMATION_RULES_ITEMDELETE_MODE=off"}, nil))
	for name, c := range map[string]*Config{"file": file, "args": args, "env": env} {
		if c.RuleModes["recyclepurge"] != "live" || c.RuleModes["itemdelete"] != "off" || len(c.RuleModes) != 2 {
			t.Errorf("%s: %v", name, c.RuleModes)
		}
	}
	if y := string(env.ExportYAML(ExportOptions{})); !strings.Contains(y, "recyclepurge:") || !strings.Contains(y, "mode: live") {
		t.Fatalf("export keeps the rule mode:\n%s", y)
	}
	if def := mustLoad(t, inputs(nil, nil, nil)); len(def.RuleModes) != 0 {
		t.Fatalf("no modes configured: %v", def.RuleModes)
	}
	_, err := Load(inputs(nil, []string{"CORSARR_AUTOMATION_RULES_RECYCLEPURGE_MODE=sometimes"}, nil), testRegistry(t))
	if err == nil || !strings.Contains(err.Error(), "automation.rules.recyclepurge.mode") || !strings.Contains(err.Error(), "off, dry-run, live") {
		t.Fatalf("bad mode: %v", err)
	}
	if !Live("automation") {
		t.Fatal("rule modes switch live")
	}
}
