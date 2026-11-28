// Package wasmtest builds the test modules (the example downloader and the
// hostile module) once per test process and hand-assembles tiny modules for
// link checks. Tests only.
package wasmtest

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/jrand0m/corsarr/sdk/go/abi"
)

var (
	once   sync.Once
	dir    string
	errB   error
	mods   = map[string][]byte{}
	rootFn = func() string {
		_, file, _, _ := runtime.Caller(0)
		return filepath.Join(filepath.Dir(file), "..", "..", "..")
	}
)

func build() {
	dir, errB = os.MkdirTemp("", "corsarr-wasmtest-")
	if errB != nil {
		return
	}
	root := rootFn()
	for name, pkg := range map[string]string{"example": "./plugins/example-downloader", "hostile": "./internal/wasm/testdata/hostile"} {
		out := filepath.Join(dir, name+".wasm")
		cmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", out, pkg)
		cmd.Dir, cmd.Env = root, append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
		if b, err := cmd.CombinedOutput(); err != nil {
			errB = fmt.Errorf("build %s: %v\n%s", pkg, err, b)
			return
		}
		mods[name], errB = os.ReadFile(out)
		if errB != nil {
			return
		}
	}
	cmd := exec.Command("go", "run", "./plugins/example-downloader")
	cmd.Dir = root
	manifest, err := cmd.Output()
	if err != nil {
		errB = fmt.Errorf("example manifest: %v", err)
		return
	}
	mods["example-manifest"] = manifest
}

func get(t testing.TB, name string) []byte {
	t.Helper()
	once.Do(build)
	if errB != nil {
		t.Fatal(errB)
	}
	return mods[name]
}

// Cleanup removes the build directory; call it from TestMain.
func Cleanup() {
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
}

// CacheDir is a compilation cache shared by the test processes of one run.
func CacheDir() string { return filepath.Join(os.TempDir(), "corsarr-wasmtest-cache") }

// ExampleRaw is the example downloader without an embedded manifest.
func ExampleRaw(t testing.TB) []byte { return get(t, "example") }

// ExampleManifest is the example's plugin.json.
func ExampleManifest(t testing.TB) []byte { return get(t, "example-manifest") }

// Example is the example downloader with its manifest embedded.
func Example(t testing.TB) []byte { return Embed(ExampleRaw(t), ExampleManifest(t)) }

// Hostile is the hostile module (its manifest comes from HostileManifest).
func Hostile(t testing.TB) []byte { return get(t, "hostile") }

// HostileManifest is plugin.json for the hostile module.
func HostileManifest(mutate ...func(*abi.Manifest)) []byte {
	m := abi.Manifest{ID: "hostile", Name: "Hostile", Version: "0.0.1", Kind: abi.KindDownloader, ABIVersion: abi.Version,
		Permissions: abi.Permissions{HTTP: abi.HTTPPermission{Hosts: []string{"*.example.org", "$setting:target"}}},
		Limits:      abi.Limits{MemoryMiB: 48, CallTimeoutMs: 1500, Concurrency: 2},
		Settings: abi.Schema{Fields: []abi.Field{
			{Key: "attack", Type: abi.TypeString, Label: "Attack"},
			{Key: "target", Type: abi.TypeString, Label: "Target", Pattern: abi.PatternURLHTTP},
			{Key: "other", Type: abi.TypeSecret, Label: "Other"},
		}}}
	for _, fn := range mutate {
		fn(&m)
	}
	b, _ := json.Marshal(m)
	return b
}

// Embed appends plugin.json as the manifest custom section.
func Embed(module, manifest []byte) []byte {
	var body []byte
	body = binary.AppendUvarint(body, uint64(len(abi.ManifestSection)))
	body = append(body, abi.ManifestSection...)
	body = append(body, manifest...)
	out := append([]byte{}, module...)
	out = append(out, 0)
	out = binary.AppendUvarint(out, uint64(len(body)))
	return append(out, body...)
}

// Import is one function import of an assembled module.
type Import struct{ Module, Name string }

// Assemble builds a minimal valid module whose functions all have type
// () -> (): the given imports and one empty exported function per export.
func Assemble(imports []Import, exports []string) []byte {
	out := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	out = section(out, 1, []byte{1, 0x60, 0, 0})
	imp := binary.AppendUvarint(nil, uint64(len(imports)))
	for _, i := range imports {
		imp = name(imp, i.Module)
		imp = name(imp, i.Name)
		imp = append(imp, 0, 0)
	}
	out = section(out, 2, imp)
	fn := binary.AppendUvarint(nil, uint64(len(exports)))
	code := binary.AppendUvarint(nil, uint64(len(exports)))
	for range exports {
		fn = append(fn, 0)
		code = append(code, 2, 0, 0x0b)
	}
	out = section(out, 3, fn)
	exp := binary.AppendUvarint(nil, uint64(len(exports)))
	for i, e := range exports {
		exp = name(exp, e)
		exp = append(exp, 0)
		exp = binary.AppendUvarint(exp, uint64(len(imports)+i))
	}
	out = section(out, 7, exp)
	return section(out, 10, code)
}

func section(out []byte, id byte, body []byte) []byte {
	out = append(out, id)
	out = binary.AppendUvarint(out, uint64(len(body)))
	return append(out, body...)
}

func name(out []byte, s string) []byte {
	out = binary.AppendUvarint(out, uint64(len(s)))
	return append(out, s...)
}
