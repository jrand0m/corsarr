package plugin

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunMain(t *testing.T) {
	m := Manifest{ID: "demo", Name: "Demo"}
	var out, errb bytes.Buffer
	if code := runMain(m, nil, &out, &errb); code != 0 || !strings.Contains(out.String(), `"id": "demo"`) {
		t.Fatalf("%d %s", code, out.String())
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in.wasm")
	_ = os.WriteFile(in, []byte("\x00asm\x01\x00\x00\x00"), 0o644)
	outFile := filepath.Join(dir, "out.wasm")
	if code := runMain(m, []string{"-embed", in, "-o", outFile}, &out, &errb); code != 0 {
		t.Fatalf("%d %s", code, errb.String())
	}
	if b, _ := os.ReadFile(outFile); !bytes.Contains(b, []byte("corsarr.manifest")) {
		t.Error("manifest not embedded")
	}
	if code := runMain(m, []string{"-embed", filepath.Join(dir, "missing")}, &out, &errb); code != 1 {
		t.Error("missing input")
	}
	if code := runMain(m, []string{"-bogus"}, &out, &errb); code != 2 {
		t.Error("bad flag")
	}
	if code := runMain(m, []string{"-o", filepath.Join(dir, "no", "such", "dir")}, &out, &errb); code != 1 {
		t.Error("unwritable output")
	}
	SetHost(nil)
	if _, err := Settings(); ClassOf(err) != "denied" {
		t.Errorf("no host: %v", err)
	}
}
