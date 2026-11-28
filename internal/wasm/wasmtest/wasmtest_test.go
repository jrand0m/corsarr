package wasmtest

import (
	"bytes"
	"os"
	"testing"

	"github.com/jrand0m/corsarr/sdk/go/abi"
)

func TestMain(m *testing.M) {
	code := m.Run()
	Cleanup()
	os.Exit(code)
}

func TestBuilds(t *testing.T) {
	if !bytes.HasPrefix(Example(t), []byte("\x00asm")) || len(Hostile(t)) == 0 {
		t.Fatal("modules not built")
	}
	if !bytes.Contains(ExampleManifest(t), []byte(`"exampledl"`)) {
		t.Fatal("manifest")
	}
	if !bytes.Contains(HostileManifest(func(m *abi.Manifest) { m.ID = "zz" }), []byte(`"zz"`)) {
		t.Fatal("mutate")
	}
	if CacheDir() == "" {
		t.Fatal("cache dir")
	}
}

func TestAssemble(t *testing.T) {
	m := Assemble([]Import{{Module: "env", Name: "f"}}, []string{"a", "b"})
	if !bytes.HasPrefix(m, []byte("\x00asm\x01\x00\x00\x00")) || !bytes.Contains(m, []byte("env")) {
		t.Fatalf("%x", m)
	}
	if e := Embed(m, []byte("{}")); !bytes.HasSuffix(e, []byte(abi.ManifestSection+"{}")) {
		t.Fatalf("%q", e)
	}
}
