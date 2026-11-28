package release

import (
	"encoding/json"
	"strings"
	"testing"
)

func fakeModule(info string) []byte {
	b := []byte("\x00asm junk go1.27.4 runtime go1.27.4 go1.2 ")
	b = append(b, modInfoStart...)
	b = append(b, info...)
	return append(append(b, modInfoEnd...), " tail"...)
}

const modInfo = "path\texample.org/plugin\nmod\texample.org/plugin\t(devel)\t\n" +
	"dep\tgolang.org/x/text\tv0.30.0\th1:abc=\n" +
	"dep\told.example/lib\tv1.0.0\th1:def=\n=>\tnew.example/lib\tv1.1.0\th1:ghi=\n" +
	"build\tGOOS=wasip1\n"

func TestModuleBuildInfo(t *testing.T) {
	info, err := ModuleBuildInfo(fakeModule(modInfo))
	if err != nil || info.GoVersion != "go1.27.4" || info.Main.Path != "example.org/plugin" || len(info.Deps) != 2 {
		t.Fatalf("%+v %v", info, err)
	}
	if _, err := ModuleBuildInfo([]byte("plain")); err == nil {
		t.Error("no build info accepted")
	}
	if _, err := ModuleBuildInfo(fakeModule("dep\tonly-a-path\n")); err == nil {
		t.Error("malformed build info accepted")
	}
}

func TestModuleSBOM(t *testing.T) {
	b, err := ModuleSBOM("x-1.0.0.wasm", fakeModule(modInfo), sourceDate, "https://example.org/spdx/x-1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	var doc spdxDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.SPDXVersion != "SPDX-2.3" || doc.CreationInfo.Created != "2025-11-28T09:00:00Z" || len(doc.Packages) != 5 ||
		len(doc.Relationships) != 5 || len(doc.Packages[0].Checksums) != 1 {
		t.Fatalf("%s", b)
	}
	for _, want := range []string{"pkg:golang/stdlib@go1.27.4", "pkg:golang/golang.org/x/text@v0.30.0", "pkg:golang/new.example/lib@v1.1.0"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(string(b), "pkg:golang/example.org/plugin@") {
		t.Error("a development version has no purl")
	}
	if _, err := ModuleSBOM("x", []byte("no info"), sourceDate, "ns"); err == nil {
		t.Error("module without build info")
	}
	if mostFrequent(nil) != "" || mostFrequent([]string{"b", "a"}) != "a" {
		t.Error("mostFrequent")
	}
}
