package catalogue

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jrand0m/corsarr/internal/wasm"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

var testSeed = []byte("0123456789abcdef0123456789abcdef")

func testKey() (ed25519.PrivateKey, Key) {
	priv := ed25519.NewKeyFromSeed(testSeed)
	pub := priv.Public().(ed25519.PublicKey)
	return priv, Key{ID: KeyID(pub), Public: pub}
}

const testManifest = `{"id":"fakeclient","name":"Fake client","version":"1.2.0","kind":"downloader","abiVersion":1,
"description":"A test module.","license":"MIT","permissions":{"http":{"hosts":["$setting:url"]},"secrets":["password"]},
"limits":{},"settings":{"fields":[{"key":"url","type":"string","label":"Address","pattern":"url-http","required":true},
{"key":"password","type":"secret","label":"Password"}]}}`

// fakeModule is a wasm header with the manifest section: enough for every
// check before the engine loads it.
func fakeModule(manifest string) []byte {
	return wasm.AppendCustomSection([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}, abi.ManifestSection, []byte(manifest))
}

func entryFor(module []byte) Entry {
	return Entry{ID: "fakeclient", Name: "Fake client", Version: "1.2.0", Kind: "downloader", ABIVersion: 1,
		Description: "A test module.", License: "MIT",
		Permissions: abi.Permissions{HTTP: abi.HTTPPermission{Hosts: []string{"$setting:url"}}, Secrets: []string{"password"}},
		MinCorsarr:  "1.0.0", Size: int64(len(module)), SHA256: wasm.SHA256(module), URL: "fakeclient-1.2.0.wasm"}
}

func indexBytes(t *testing.T, entries ...Entry) []byte {
	t.Helper()
	b, err := json.Marshal(Index{Schema: 1, Release: "v1.4.0", Generated: time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC), Plugins: entries})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecodeAcceptsAValidIndex(t *testing.T) {
	mod := fakeModule(testManifest)
	ix, err := Decode(indexBytes(t, entryFor(mod)), 32<<20)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := ix.Find("fakeclient"); !ok || e.Version != "1.2.0" {
		t.Fatalf("entry = %+v", ix.Plugins)
	}
}

func TestDecodeRejects(t *testing.T) {
	mod := fakeModule(testManifest)
	good := entryFor(mod)
	cases := map[string]func(*Entry){
		"bad sha":        func(e *Entry) { e.SHA256 = strings.ToUpper(e.SHA256) },
		"zero size":      func(e *Entry) { e.Size = 0 },
		"too large":      func(e *Entry) { e.Size = 33 << 20 },
		"http url":       func(e *Entry) { e.URL = "http://example.org/x.wasm" },
		"host-only url":  func(e *Entry) { e.URL = "//example.org/x.wasm" },
		"bidi name":      func(e *Entry) { e.Name = "Fake‮client" },
		"bad id":         func(e *Entry) { e.ID = "Fake" },
		"bad host":       func(e *Entry) { e.Permissions.HTTP.Hosts = []string{"exa mple"} },
		"bad min":        func(e *Entry) { e.MinCorsarr = "one" },
		"http homepage":  func(e *Entry) { e.Homepage = "http://example.org" },
		"bad secret key": func(e *Entry) { e.Permissions.Secrets = []string{"pass-word"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			e := good
			mutate(&e)
			var fe *FormatError
			if _, err := Decode(indexBytes(t, e), 32<<20); !errors.As(err, &fe) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	t.Run("unknown field", func(t *testing.T) {
		raw := strings.Replace(string(indexBytes(t, good)), `"schema":1`, `"schema":1,"extra":true`, 1)
		if _, err := Decode([]byte(raw), 32<<20); err == nil {
			t.Fatal("unknown field accepted")
		}
	})
	t.Run("unsorted", func(t *testing.T) {
		b := good
		b.ID = "aclient"
		if _, err := Decode(indexBytes(t, good, b), 32<<20); err == nil {
			t.Fatal("unsorted ids accepted")
		}
	})
	t.Run("newer format", func(t *testing.T) {
		raw := strings.Replace(string(indexBytes(t, good)), `"schema":1`, `"schema":2`, 1)
		if _, err := Decode([]byte(raw), 32<<20); err == nil || !strings.Contains(err.Error(), "format 2") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestVerify(t *testing.T) {
	priv, key := testKey()
	index := []byte(`{"schema":1}`)
	sig := Sign(index, priv)
	if err := Verify(index, sig, []Key{key}); err != nil {
		t.Fatalf("valid signature: %v", err)
	}
	other := ed25519.NewKeyFromSeed([]byte("fedcba9876543210fedcba9876543210"))
	otherPub := other.Public().(ed25519.PublicKey)
	cases := []struct {
		name  string
		index []byte
		sig   []byte
		keys  []Key
		want  error
	}{
		{"tampered", []byte(`{"schema":2}`), sig, []Key{key}, ErrBadSignature},
		{"unsigned", index, nil, []Key{key}, ErrUnsigned},
		{"no keys", index, sig, nil, ErrNoTrustedKey},
		{"unknown key", index, sig, []Key{{ID: KeyID(otherPub), Public: otherPub}}, ErrUnknownKey},
		{"same id other key", index, sig, []Key{{ID: key.ID, Public: otherPub}}, ErrBadSignature},
		{"bad base64", index, []byte(`{"schema":1,"alg":"ed25519","keyId":"` + key.ID + `","signature":"!!"}`), []Key{key}, ErrBadSignature},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := Verify(c.index, c.sig, c.keys); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
	if err := Verify(index, []byte(`{"schema":1,"alg":"rsa","keyId":"x","signature":""}`), []Key{key}); err == nil {
		t.Fatal("other algorithm accepted")
	}
}

func TestSignedMessageHasThePrefix(t *testing.T) {
	priv, key := testKey()
	index := []byte("{}")
	var sig Signature
	_ = json.Unmarshal(Sign(index, priv), &sig)
	if sig.KeyID != key.ID || len(sig.KeyID) != 16 {
		t.Fatalf("key id %q", sig.KeyID)
	}
	raw := ed25519.Sign(priv, append([]byte("corsarr-plugin-index-v1\n"), index...))
	if !strings.Contains(string(Sign(index, priv)), string(mustB64(raw))) {
		t.Fatal("signature is not over prefix + index")
	}
}

func TestTrustedKeysFromFiles(t *testing.T) {
	_, key := testKey()
	text := string(mustB64(key.Public)) + "\n"
	fsys := fstest.MapFS{
		"trusted/" + key.ID + ".pub": {Data: []byte(text)},
		"trusted/wrongname.pub":      {Data: []byte(text)},
		"trusted/README":             {Data: []byte("x")},
	}
	keys := keysIn(fsys, "trusted")
	if len(keys) != 1 || keys[0].ID != key.ID {
		t.Fatalf("keys = %+v", keys)
	}
	_ = Trusted() // the embedded folder is readable
}

func TestPermissionDiffAndOffers(t *testing.T) {
	mod := fakeModule(testManifest)
	e := entryFor(mod)
	old := abi.Permissions{HTTP: abi.HTTPPermission{Hosts: []string{"$setting:url"}}}
	cases := []struct {
		name string
		inst Installed
		ver  string
		want State
	}{
		{"not installed", Installed{}, "v1.4.0", StateAvailable},
		{"same", Installed{Present: true, Version: "1.2.0", Permissions: e.Permissions}, "v1.4.0", StateInstalled},
		{"update", Installed{Present: true, Version: "1.1.0", Permissions: e.Permissions}, "v1.4.0", StateUpdate},
		{"grown", Installed{Present: true, Version: "1.1.0", Permissions: old}, "v1.4.0", StateNeedsApproval},
		{"older", Installed{Present: true, Version: "2.0.0", Permissions: e.Permissions}, "v1.4.0", StateOlder},
		{"needs newer corsarr", Installed{}, "v0.9.0", StateIncompatible},
		{"dev build skips the version check", Installed{}, "dev", StateAvailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := OfferFor(e, c.inst, c.ver); got.State != c.want {
				t.Fatalf("state = %s (%s), want %s", got.State, got.Reason, c.want)
			}
		})
	}
	if r := OfferFor(e, Installed{}, "v0.9.0").Reason; r != "needs corsarr ≥ 1.0.0" {
		t.Fatalf("reason %q", r)
	}
	abiTwo := e
	abiTwo.ABIVersion = 2
	if o := OfferFor(abiTwo, Installed{}, "v1.4.0"); o.State != StateIncompatible {
		t.Fatalf("abi 2 = %s", o.State)
	}
	d := PermissionDiff(old, e.Permissions)
	if !d.Grew() || len(d.AddedSecrets) != 1 || len(d.RemovedHosts) != 0 {
		t.Fatalf("diff = %+v", d)
	}
	if offers := Offers(Index{Plugins: []Entry{e}}, nil, "v1.4.0"); len(offers) != 1 {
		t.Fatal("offers")
	}
}

func TestVersionsAndPolicy(t *testing.T) {
	order := [][2]string{{"1.0.0-rc.1", "1.0.0"}, {"1.0.0", "1.0.1"}, {"1.9.0", "1.10.0"}, {"v1.2.3", "2.0.0"}}
	for _, p := range order {
		if CompareVersions(p[0], p[1]) != -1 || CompareVersions(p[1], p[0]) != 1 {
			t.Fatalf("%s < %s", p[0], p[1])
		}
	}
	if CompareVersions("1.2.3+build", "1.2.3") != 0 || CompareVersions("x", "1.0.0") != 0 {
		t.Fatal("equal versions")
	}
	cases := []struct {
		policy, from, to string
		want             bool
	}{
		{AutoPatch, "1.2.0", "1.2.5", true}, {AutoPatch, "1.2.0", "1.3.0", false},
		{AutoMinor, "1.2.0", "1.3.0", true}, {AutoMinor, "1.2.0", "2.0.0", false},
		{AutoOff, "1.2.0", "1.2.1", false}, {AutoMinor, "1.2.0", "1.2.0", false},
		{AutoMinor, "1.2.0", "1.3.0-rc.1", false}, {AutoMinor, "bad", "1.3.0", false},
	}
	for _, c := range cases {
		if got := WithinPolicy(c.policy, c.from, c.to); got != c.want {
			t.Fatalf("%s %s→%s = %v", c.policy, c.from, c.to, got)
		}
	}
}

func TestDefaultURL(t *testing.T) {
	if u := DefaultURL("v1.4.0"); !strings.HasSuffix(u, "/releases/download/v1.4.0/plugins-index.json") {
		t.Fatal(u)
	}
	if u := DefaultURL("dev"); !strings.HasSuffix(u, "/releases/latest/download/plugins-index.json") {
		t.Fatal(u)
	}
}

func TestVerifyModule(t *testing.T) {
	mod := fakeModule(testManifest)
	e := entryFor(mod)
	if err := VerifyModule(mod, e); err != nil {
		t.Fatal(err)
	}
	tampered := append([]byte{}, mod...)
	tampered[len(tampered)-3] ^= 1
	other := fakeModule(strings.Replace(testManifest, `"secrets":["password"]`, `"secrets":[]`, 1))
	oe := e
	oe.Size, oe.SHA256 = int64(len(other)), wasm.SHA256(other)
	for name, c := range map[string]struct {
		mod []byte
		e   Entry
	}{
		"hash":        {tampered, e},
		"size":        {append(append([]byte{}, mod...), 0), e},
		"permissions": {other, oe},
	} {
		t.Run(name, func(t *testing.T) {
			var ve *VerifyError
			if err := VerifyModule(c.mod, c.e); !errors.As(err, &ve) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// catalogueServer serves an index, its signature and the module.
type catalogueServer struct {
	*httptest.Server
	index, sig, module []byte
	indexHits          atomic.Int32
	down               atomic.Bool
}

func newCatalogueServer(t *testing.T, signed bool) *catalogueServer {
	t.Helper()
	priv, _ := testKey()
	cs := &catalogueServer{module: fakeModule(testManifest)}
	cs.index = indexBytes(t, entryFor(cs.module))
	if signed {
		cs.sig = Sign(cs.index, priv)
	}
	cs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cs.down.Load() {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		switch r.URL.Path {
		case "/plugins-index.json":
			cs.indexHits.Add(1)
			if r.Header.Get("If-None-Match") == `"v1"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"v1"`)
			_, _ = w.Write(cs.index)
		case "/plugins-index.json.sig":
			if cs.sig == nil {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(cs.sig)
		case "/fakeclient-1.2.0.wasm":
			_, _ = w.Write(cs.module)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(cs.Close)
	return cs
}

func TestFetcherCachesVerifiesAndFallsBack(t *testing.T) {
	cs := newCatalogueServer(t, true)
	_, key := testKey()
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	f := &Fetcher{Dir: t.TempDir(), HTTP: http.DefaultClient, Keys: []Key{key}, MaxModule: 32 << 20, Now: func() time.Time { return now }}
	url := cs.URL + "/plugins-index.json"
	ctx := context.Background()
	s, err := f.Get(ctx, url, false)
	if err != nil || len(s.Index.Plugins) != 1 || s.Stale {
		t.Fatalf("first get: %+v %v", s, err)
	}
	if _, err := f.Get(ctx, url, false); err != nil || cs.indexHits.Load() != 1 {
		t.Fatalf("fresh cache refetched: hits=%d err=%v", cs.indexHits.Load(), err)
	}
	now = now.Add(2 * time.Hour)
	if s, err := f.Get(ctx, url, false); err != nil || cs.indexHits.Load() != 2 || !s.FetchedAt.Equal(now) {
		t.Fatalf("not-modified refresh: %+v %v", s, err)
	}
	cs.down.Store(true)
	s, err = f.Get(ctx, url, true)
	if err != nil || !s.Stale || s.Err == nil || len(s.Index.Plugins) != 1 {
		t.Fatalf("offline: %+v %v", s, err)
	}
	if c, ok := f.Cached(url); !ok || len(c.Index.Plugins) != 1 {
		t.Fatal("cached copy missing")
	}
	mod, err := Download(ctx, http.DefaultClient, url, s.Index.Plugins[0])
	if err == nil || mod != nil && VerifyModule(mod, s.Index.Plugins[0]) == nil {
		t.Fatal("download from a down server succeeded")
	}
	cs.down.Store(false)
	if mod, err := Download(ctx, http.DefaultClient, url, s.Index.Plugins[0]); err != nil || len(mod) == 0 {
		t.Fatalf("download: %v", err)
	}
}

func TestFetcherRefusesUnsignedOrForeign(t *testing.T) {
	cs := newCatalogueServer(t, false)
	_, key := testKey()
	f := &Fetcher{Dir: t.TempDir(), HTTP: http.DefaultClient, Keys: []Key{key}, MaxModule: 32 << 20}
	if _, err := f.Get(context.Background(), cs.URL+"/plugins-index.json", false); !errors.Is(err, ErrUnsigned) {
		t.Fatalf("unsigned: %v", err)
	}
	signed := newCatalogueServer(t, true)
	f2 := &Fetcher{Dir: t.TempDir(), HTTP: http.DefaultClient, MaxModule: 32 << 20}
	if _, err := f2.Get(context.Background(), signed.URL+"/plugins-index.json", false); !errors.Is(err, ErrNoTrustedKey) {
		t.Fatalf("no keys: %v", err)
	}
}

func mustB64(b []byte) []byte { return []byte(base64.StdEncoding.EncodeToString(b)) }

func TestBuildSignAndReadBack(t *testing.T) {
	mod := fakeModule(testManifest)
	e, err := EntryFromModule(mod, "1.4.0")
	if err != nil || e.URL != "fakeclient-1.2.0.wasm" || e.SHA256 != wasm.SHA256(mod) {
		t.Fatalf("entry %+v %v", e, err)
	}
	index, err := BuildIndex("v1.4.0", time.Unix(1764320000, 0), []Entry{e}, 32<<20)
	if err != nil {
		t.Fatal(err)
	}
	pemKey, pub, err := NewSigningKey(strings.NewReader(strings.Repeat("k", 64)))
	if err != nil {
		t.Fatal(err)
	}
	priv, err := ParseSigningKey(pemKey)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseKey(PublicKeyFile(pub))
	if err != nil || parsed.ID != pub.ID {
		t.Fatalf("public key file: %v", err)
	}
	if err := Verify(index, Sign(index, priv), []Key{parsed}); err != nil {
		t.Fatal(err)
	}
	if ix, err := Decode(index, 32<<20); err != nil || ix.Plugins[0].ID != "fakeclient" {
		t.Fatalf("read back: %v", err)
	}
	if _, err := EntryFromModule([]byte("not wasm"), ""); err == nil {
		t.Fatal("module without manifest")
	}
	bad := e
	bad.SHA256 = "x"
	if _, err := BuildIndex("v1.4.0", time.Now(), []Entry{bad}, 32<<20); err == nil {
		t.Fatal("bad entry built")
	}
	if _, err := ParseSigningKey([]byte("nope")); err == nil {
		t.Fatal("bad key parsed")
	}
	if _, err := ParseKey("short"); err == nil {
		t.Fatal("bad public key parsed")
	}
}
