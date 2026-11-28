package release

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/catalogue"
	"github.com/jrand0m/corsarr/internal/wasm"
)

// The plugin index: plugins-index.json lists the plugin modules a release
// publishes, and plugins-index.json.sig signs its exact bytes. The shapes,
// the signed message and the checks are the catalogue reader's own
// (internal/catalogue), so what a release writes is what corsarr accepts.

// IndexSchema is the plugin index format version.
const IndexSchema = catalogue.SchemaVersion

// MaxIndexPlugins bounds the entries of one index.
const MaxIndexPlugins = catalogue.MaxEntries

// SignaturePrefix is prepended (with a newline) to the index bytes to form
// the signed message.
var SignaturePrefix = strings.TrimSuffix(catalogue.MessagePrefix, "\n")

// PluginIndex is plugins-index.json as written (generated as RFC 3339 text).
type PluginIndex struct {
	Schema    int           `json:"schema"`
	Release   string        `json:"release"`
	Generated string        `json:"generated"`
	Plugins   []PluginEntry `json:"plugins"`
}

// PluginEntry is one plugin module of an index.
type PluginEntry = catalogue.Entry

// ModuleFile is one module file given to BuildIndex.
type ModuleFile struct {
	Name string // file name, <id>-<version>.wasm
	Data []byte
}

var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)

// BuildIndex writes the index of a release: one entry per module, copied
// from the manifest embedded in it (validated as corsarr validates it on
// load), sorted by id. generated is the release's source date; minCorsarr,
// the oldest corsarr that can run the modules (a version without "v", or ""
// for none), is the version being released.
func BuildIndex(release, minCorsarr string, generated time.Time, files []ModuleFile) ([]byte, error) {
	if len(files) > MaxIndexPlugins {
		return nil, fmt.Errorf("%d modules; an index holds at most %d", len(files), MaxIndexPlugins)
	}
	idx := PluginIndex{Schema: IndexSchema, Release: release, Generated: generated.UTC().Format(time.RFC3339), Plugins: []PluginEntry{}}
	if minCorsarr != "" && !semverRe.MatchString(minCorsarr) {
		return nil, fmt.Errorf("minimum corsarr version %q is not a version such as 1.4.0", minCorsarr)
	}
	seen := map[string]bool{}
	for _, f := range files {
		e, err := indexEntry(f)
		if err != nil {
			return nil, err
		}
		if seen[e.ID] {
			return nil, fmt.Errorf("%s: plugin %s is listed twice", f.Name, e.ID)
		}
		seen[e.ID] = true
		e.MinCorsarr = minCorsarr
		idx.Plugins = append(idx.Plugins, e)
	}
	sort.Slice(idx.Plugins, func(i, j int) bool { return idx.Plugins[i].ID < idx.Plugins[j].ID })
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')
	if _, err := catalogue.Decode(b, 0); err != nil {
		return nil, fmt.Errorf("corsarr would refuse this index: %w", err)
	}
	return b, nil
}

func indexEntry(f ModuleFile) (PluginEntry, error) {
	raw, err := wasm.ManifestBytes(f.Data, nil)
	if err != nil {
		return PluginEntry{}, fmt.Errorf("%s: %w", f.Name, err)
	}
	m, err := wasm.ParseManifest(raw)
	if err != nil {
		return PluginEntry{}, fmt.Errorf("%s: %w", f.Name, err)
	}
	want := m.ID + "-" + m.Version + ".wasm"
	if f.Name != want {
		return PluginEntry{}, fmt.Errorf("%s: a module of %s %s must be named %s", f.Name, m.ID, m.Version, want)
	}
	sum := sha256.Sum256(f.Data)
	return PluginEntry{
		ID: m.ID, Name: m.Name, Version: m.Version, Kind: wasm.ManifestKind(m), ABIVersion: m.ABIVersion,
		Description: m.Description, Author: m.Author, License: m.License, Homepage: m.Homepage,
		Permissions: m.Permissions, Size: int64(len(f.Data)), SHA256: hex.EncodeToString(sum[:]), URL: want,
	}, nil
}

// Signature is plugins-index.json.sig.
type Signature = catalogue.Signature

// KeyID names a public key: the first 16 hex characters of SHA-256 over its
// 32 raw bytes.
func KeyID(pub ed25519.PublicKey) string { return catalogue.KeyID(pub) }

// SignIndex signs the exact index bytes and returns the signature file.
func SignIndex(index []byte, key ed25519.PrivateKey) ([]byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("not an Ed25519 private key")
	}
	return catalogue.Sign(index, key), nil
}

// VerifyIndex checks a signature file against the index bytes and a set of
// trusted keys by key id, as corsarr does.
func VerifyIndex(index, sigFile []byte, keys map[string]ed25519.PublicKey) error {
	list := make([]catalogue.Key, 0, len(keys))
	for id, pub := range keys {
		list = append(list, catalogue.Key{ID: id, Public: pub})
	}
	return catalogue.Verify(index, sigFile, list)
}

// ParsePrivateKey reads an Ed25519 private key in PKCS#8 PEM, as OpenSSL
// writes it.
func ParsePrivateKey(pemBytes []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := k.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("not an Ed25519 key")
	}
	return key, nil
}

// MarshalPrivateKey writes key as PKCS#8 PEM.
func MarshalPrivateKey(key ed25519.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// PublicKeyLine is the trusted-key file content: standard base64 of the 32
// raw bytes and a newline.
func PublicKeyLine(pub ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(pub) + "\n"
}

// ParsePublicKey reads a trusted-key file.
func ParsePublicKey(line []byte) (ed25519.PublicKey, error) {
	k, err := catalogue.ParseKey(string(line))
	return k.Public, err
}
