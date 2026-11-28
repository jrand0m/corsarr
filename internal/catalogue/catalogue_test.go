package catalogue

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin/schema"
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
	e := entryFor(mod)
	e.Attestation = "https://example.org/attestations/1"
	ix, err := Decode(indexBytes(t, e), 32<<20)
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
		"bad sha":          func(e *Entry) { e.SHA256 = strings.ToUpper(e.SHA256) },
		"zero size":        func(e *Entry) { e.Size = 0 },
		"too large":        func(e *Entry) { e.Size = 33 << 20 },
		"http url":         func(e *Entry) { e.URL = "http://example.org/x.wasm" },
		"host-only url":    func(e *Entry) { e.URL = "//example.org/x.wasm" },
		"bidi name":        func(e *Entry) { e.Name = "Fake‮client" },
		"bad id":           func(e *Entry) { e.ID = "Fake" },
		"bad host":         func(e *Entry) { e.Permissions.HTTP.Hosts = []string{"exa mple"} },
		"bad min":          func(e *Entry) { e.MinCorsarr = "one" },
		"http homepage":    func(e *Entry) { e.Homepage = "http://example.org" },
		"bad secret key":   func(e *Entry) { e.Permissions.Secrets = []string{"pass-word"} },
		"http attestation": func(e *Entry) { e.Attestation = "http://example.org/a" },
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
	other := ed25519.NewKeyFromSeed([]byte("fedcba9876543210fedcba9876543210")).Public().(ed25519.PublicKey)
	fsys := fstest.MapFS{
		"trusted/" + key.ID + ".pub": {Data: []byte(text)},
		"trusted/wrongname.pub":      {Data: []byte(string(mustB64(other)) + "\n")},
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
	notModified        atomic.Int32
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
				cs.notModified.Add(1)
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

func TestFetcherRefetchesConditionallyOnceTheCacheIsAnHourOld(t *testing.T) {
	cs := newCatalogueServer(t, true)
	_, key := testKey()
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	f := &Fetcher{Dir: t.TempDir(), HTTP: http.DefaultClient, Keys: []Key{key}, MaxModule: 32 << 20, Now: func() time.Time { return now }}
	url := cs.URL + "/plugins-index.json"
	if _, err := f.Get(context.Background(), url, false); err != nil {
		t.Fatal(err)
	}
	now = now.Add(MaxAge)
	s, err := f.Get(context.Background(), url, false)
	if err != nil || cs.indexHits.Load() != 2 || !s.FetchedAt.Equal(now) {
		t.Fatalf("an hour-old cache was not refetched: hits=%d %+v %v", cs.indexHits.Load(), s, err)
	}
	if cs.notModified.Load() != 1 {
		t.Fatalf("the refetch did not send the cached ETag (not-modified answers = %d)", cs.notModified.Load())
	}
}

func TestFetcherIgnoresACacheRecordedForAnotherAddress(t *testing.T) {
	cs := newCatalogueServer(t, true)
	_, key := testKey()
	f := &Fetcher{Dir: t.TempDir(), HTTP: http.DefaultClient, Keys: []Key{key}, MaxModule: 32 << 20}
	url := cs.URL + "/plugins-index.json"
	if _, err := f.Get(context.Background(), url, false); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(meta{URL: url + "?other", FetchedAt: time.Now()})
	if err := os.WriteFile(filepath.Join(f.dir(url), "meta.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Cached(url); ok {
		t.Fatal("a cache recorded for another address was used")
	}
	cs.down.Store(true)
	if s, err := f.Get(context.Background(), url, false); err == nil {
		t.Fatalf("offline with no usable cache: %+v", s)
	}
}

func TestFetcherBoundsTheSignatureFile(t *testing.T) {
	const limit = 64 << 10
	for _, c := range []struct {
		name string
		size int
		ok   bool
	}{{"exactly the limit", limit, true}, {"one byte over", limit + 1, false}} {
		t.Run(c.name, func(t *testing.T) {
			cs := newCatalogueServer(t, true)
			// Trailing blanks keep the signature valid, so only the size decides.
			cs.sig = append(cs.sig, []byte(strings.Repeat(" ", c.size-len(cs.sig)))...)
			_, key := testKey()
			f := &Fetcher{Dir: t.TempDir(), HTTP: http.DefaultClient, Keys: []Key{key}, MaxModule: 32 << 20}
			_, err := f.Get(context.Background(), cs.URL+"/plugins-index.json", false)
			if c.ok && err != nil {
				t.Fatalf("a %d-byte signature file was refused: %v", c.size, err)
			}
			if !c.ok && (err == nil || !strings.Contains(err.Error(), "larger than 65536 bytes")) {
				t.Fatalf("a %d-byte signature file: err = %v", c.size, err)
			}
		})
	}
}

func TestDownloadRefusesAModuleLongerThanItsEntry(t *testing.T) {
	cs := newCatalogueServer(t, true)
	e := entryFor(cs.module)
	cs.module = append(append([]byte{}, cs.module...), 0xff)
	var ve *VerifyError
	if _, err := Download(context.Background(), http.DefaultClient, cs.URL+"/plugins-index.json", e); !errors.As(err, &ve) {
		t.Fatalf("err = %v", err)
	}
}

func hasProblem(ps schema.Problems, path string) bool {
	return slices.ContainsFunc(ps, func(p schema.Problem) bool { return p.Path == path })
}

func TestCheckEntryLimits(t *testing.T) {
	good := entryFor(fakeModule(testManifest))
	if ps := CheckEntry(good, 32<<20); len(ps) != 0 {
		t.Fatalf("valid entry: %v", ps)
	}
	cases := []struct {
		name      string
		mutate    func(*Entry)
		maxModule int64
		path      string
		want      bool
	}{
		{"kind not an identifier", func(e *Entry) { e.Kind = "Downloader" }, 32 << 20, "kind", true},
		{"abi 0", func(e *Entry) { e.ABIVersion = 0 }, 32 << 20, "abiVersion", true},
		{"one byte", func(e *Entry) { e.Size = 1 }, 32 << 20, "size", false},
		{"exactly the largest", func(e *Entry) { e.Size = 32 << 20 }, 32 << 20, "size", false},
		{"over a one-byte limit", func(e *Entry) { e.Size = 2 }, 1, "size", true},
		{"no limit", func(e *Entry) { e.Size = 1 << 40 }, 0, "size", false},
		{"empty url", func(e *Entry) { e.URL = "" }, 32 << 20, "url", true},
		{"unparsable url", func(e *Entry) { e.URL = "%zz" }, 32 << 20, "url", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := good
			c.mutate(&e)
			if got := hasProblem(CheckEntry(e, c.maxModule), c.path); got != c.want {
				t.Fatalf("problem at %s = %v, want %v", c.path, got, c.want)
			}
		})
	}
}

func TestCheckEntryCountAndOrder(t *testing.T) {
	good := entryFor(fakeModule(testManifest))
	entries := func(n int) []Entry {
		out := make([]Entry, n)
		for i := range out {
			out[i] = good
			out[i].ID = "p" + string(rune('a'+i/26)) + string(rune('a'+i%26))
		}
		return out
	}
	ix := func(es []Entry) Index {
		return Index{Schema: 1, Release: "v1.4.0", Generated: time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC), Plugins: es}
	}
	if ps := Check(ix(entries(MaxEntries)), 32<<20); len(ps) != 0 {
		t.Fatalf("%d sorted entries: %v", MaxEntries, ps)
	}
	if ps := Check(ix(entries(MaxEntries+1)), 32<<20); !hasProblem(ps, "plugins") {
		t.Fatalf("%d entries: %v", MaxEntries+1, ps)
	}
	dup := entries(2)
	dup[1].ID = dup[0].ID
	if ps := Check(ix(dup), 32<<20); !hasProblem(ps, "plugins[1].id") {
		t.Fatalf("duplicate ids: %v", ps)
	}
}

func TestPolicyEdges(t *testing.T) {
	if !(Diff{AddedHosts: []string{"a.example"}}).Grew() || !(Diff{AddedSecrets: []string{"token"}}).Grew() || (Diff{RemovedHosts: []string{"a.example"}}).Grew() {
		t.Fatal("one added host or secret is growth; a removal is not")
	}
	for _, bad := range []string{"v+1.2.3", "1.x.0"} {
		if CompareVersions(bad, "2.0.0") != 0 {
			t.Fatalf("%q is not a version and compares as equal", bad)
		}
	}
	e := entryFor(fakeModule(testManifest))
	if o := OfferFor(e, Installed{}, "v"+e.MinCorsarr); o.State != StateAvailable {
		t.Fatalf("corsarr equal to minCorsarr: %s (%s)", o.State, o.Reason)
	}
}

func TestParseKeyNeeds32Bytes(t *testing.T) {
	_, key := testKey()
	if k, err := ParseKey(string(mustB64(key.Public))); err != nil || k.ID != key.ID {
		t.Fatalf("valid key: %+v %v", k, err)
	}
	if _, err := ParseKey(string(mustB64(key.Public[:16]))); err == nil {
		t.Fatal("a 16-byte key was accepted")
	}
}

func TestVerifyReadsTheSignatureFileStrictly(t *testing.T) {
	priv, key := testKey()
	index := []byte(`{"schema":1}`)
	var good Signature
	if err := json.Unmarshal(Sign(index, priv), &good); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Signature){
		"newer format":    func(s *Signature) { s.Schema = 2 },
		"other algorithm": func(s *Signature) { s.Alg = "rsa" },
		"trailing junk":   func(s *Signature) { s.Signature += "!!" },
	} {
		t.Run(name, func(t *testing.T) {
			s := good
			mutate(&s)
			b, _ := json.Marshal(s)
			if err := Verify(index, b, []Key{key}); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}
