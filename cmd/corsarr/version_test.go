package main

import (
	"encoding/json"
	"runtime"
	"testing"
)

func TestVersionJSON(t *testing.T) {
	code, out, _ := cliRun(t, nil, "version", "--json")
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); code != 0 || err != nil {
		t.Fatalf("%d %v %q", code, err, out)
	}
	for _, k := range []string{"version", "commit", "tag", "goVersion", "buildDate", "dirty"} {
		if _, ok := got[k]; !ok {
			t.Fatalf("missing %q in %s", k, out)
		}
	}
	if got["version"] != "dev" || got["goVersion"] != runtime.Version() {
		t.Fatalf("%s", out)
	}
}
