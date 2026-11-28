package release

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/wasm/wasmtest"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

func moduleFile(t *testing.T, id, version string, mutate ...func(*abi.Manifest)) ModuleFile {
	t.Helper()
	man := wasmtest.HostileManifest(append([]func(*abi.Manifest){func(m *abi.Manifest) {
		m.ID, m.Version, m.Name = id, version, strings.ToUpper(id)
	}}, mutate...)...)
	return ModuleFile{Name: id + "-" + version + ".wasm", Data: wasmtest.Embed(wasmtest.Assemble(nil, nil), man)}
}

var sourceDate = time.Unix(1764320400, 0)

func TestBuildIndex(t *testing.T) {
	b, err := BuildIndex("v1.4.0", sourceDate, []ModuleFile{
		moduleFile(t, "zeta", "2.0.0"),
		moduleFile(t, "alpha", "1.0.0", func(m *abi.Manifest) { m.Description, m.License = "Does things.", "MIT" }),
	})
	if err != nil {
		t.Fatal(err)
	}
	var idx PluginIndex
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&idx); err != nil {
		t.Fatal(err)
	}
	if idx.Schema != 1 || idx.Release != "v1.4.0" || idx.Generated != "2025-11-28T09:00:00Z" || len(idx.Plugins) != 2 {
		t.Fatalf("%+v", idx)
	}
	a := idx.Plugins[0]
	if a.ID != "alpha" || a.URL != "alpha-1.0.0.wasm" || a.MinCorsarr != "1.4.0" || a.Kind != "downloader" ||
		a.ABIVersion != 1 || len(a.SHA256) != 64 || a.Size == 0 || a.License != "MIT" || a.Permissions.HTTP.Hosts[0] != "*.example.org" {
		t.Errorf("%+v", a)
	}
	if strings.Contains(string(b), `"author"`) || strings.Contains(string(b), `"attestation"`) || !strings.HasSuffix(string(b), "}\n") {
		t.Errorf("empty fields must be left out:\n%s", b)
	}
	again, _ := BuildIndex("v1.4.0", sourceDate, []ModuleFile{moduleFile(t, "alpha", "1.0.0", func(m *abi.Manifest) {
		m.Description, m.License = "Does things.", "MIT"
	}), moduleFile(t, "zeta", "2.0.0")})
	if !bytes.Equal(b, again) {
		t.Error("index depends on the order of the files")
	}
}

func TestBuildIndexRefuses(t *testing.T) {
	dev, err := BuildIndex("v1.4.0-3-gabc-dirty", sourceDate, nil)
	if err != nil || strings.Contains(string(dev), "minCorsarr") || !strings.Contains(string(dev), `"plugins": []`) {
		t.Errorf("dev build: %s %v", dev, err)
	}
	renamed := moduleFile(t, "alpha", "1.0.0")
	renamed.Name = "alpha.wasm"
	bad := moduleFile(t, "alpha", "1.0.0", func(m *abi.Manifest) { m.ABIVersion = 9 })
	for name, files := range map[string][]ModuleFile{
		"wrong name":   {renamed},
		"twice":        {moduleFile(t, "alpha", "1.0.0"), moduleFile(t, "alpha", "1.0.0")},
		"bad manifest": {bad},
		"no manifest":  {{Name: "x-1.0.0.wasm", Data: wasmtest.Assemble(nil, nil)}},
		"too many":     make([]ModuleFile, MaxIndexPlugins+1),
	} {
		if _, err := BuildIndex("v1.0.0", sourceDate, files); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSignAndVerify(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	pemBytes, err := MarshalPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePrivateKey(pemBytes)
	if err != nil || !parsed.Equal(key) {
		t.Fatal(err)
	}
	trusted, err := ParsePublicKey([]byte(PublicKeyLine(pub)))
	if err != nil || len(KeyID(pub)) != 16 {
		t.Fatal(err)
	}
	keys := map[string]ed25519.PublicKey{KeyID(pub): trusted}
	index := []byte("{\"schema\":1}\n")
	sig, err := SignIndex(index, key)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(sig), `{"schema":1,"alg":"ed25519","keyId":"`+KeyID(pub)+`","signature":"`) {
		t.Errorf("%s", sig)
	}
	if err := VerifyIndex(index, sig, keys); err != nil {
		t.Fatal(err)
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	for name, c := range map[string]struct {
		index, sig []byte
		keys       map[string]ed25519.PublicKey
	}{
		"changed index": {[]byte("{\"schema\":2}\n"), sig, keys},
		"unknown key":   {index, sig, map[string]ed25519.PublicKey{KeyID(other): other}},
		"extra field":   {index, []byte(`{"schema":1,"alg":"ed25519","keyId":"x","signature":"","x":1}`), keys},
		"other alg":     {index, []byte(`{"schema":1,"alg":"rsa","keyId":"x","signature":""}`), keys},
		"short sig":     {index, []byte(`{"schema":1,"alg":"ed25519","keyId":"` + KeyID(pub) + `","signature":"AAAA"}`), keys},
	} {
		if err := VerifyIndex(c.index, c.sig, c.keys); err == nil {
			t.Errorf("%s: verified", name)
		}
	}
}

func TestKeyParsing(t *testing.T) {
	if _, err := ParsePrivateKey([]byte("nope")); err == nil {
		t.Error("no PEM")
	}
	if _, err := ParsePrivateKey([]byte("-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n")); err == nil {
		t.Error("bad DER")
	}
	if _, err := ParsePublicKey([]byte("AAAA")); err == nil {
		t.Error("short public key")
	}
	if _, err := SignIndex(nil, ed25519.PrivateKey(make([]byte, 10))); err == nil {
		t.Error("short private key")
	}
}
