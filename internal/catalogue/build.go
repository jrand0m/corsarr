package catalogue

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/wasm"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// EntryFromModule describes a module for the index from its embedded
// manifest; the module is published as <id>-<version>.wasm next to the index.
func EntryFromModule(module []byte, minCorsarr string) (Entry, error) {
	raw, ok := wasm.CustomSection(module, abi.ManifestSection)
	if !ok {
		return Entry{}, errors.New("the module has no embedded manifest")
	}
	m, err := wasm.ParseManifest(raw)
	if err != nil {
		return Entry{}, err
	}
	return Entry{ID: m.ID, Name: m.Name, Version: m.Version, Kind: wasm.ManifestKind(m), ABIVersion: m.ABIVersion,
		Description: m.Description, Author: m.Author, License: m.License, Homepage: m.Homepage, Permissions: m.Permissions,
		MinCorsarr: minCorsarr, Size: int64(len(module)), SHA256: wasm.SHA256(module), URL: m.ID + "-" + m.Version + ".wasm"}, nil
}

// BuildIndex returns plugins-index.json for a release: entries sorted by id
// and checked as a reader would check them.
func BuildIndex(release string, generated time.Time, entries []Entry, maxModule int64) ([]byte, error) {
	sorted := append([]Entry{}, entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	ix := Index{Schema: SchemaVersion, Release: release, Generated: generated.UTC().Truncate(time.Second), Plugins: sorted}
	if ix.Plugins == nil {
		ix.Plugins = []Entry{}
	}
	if ps := Check(ix, maxModule); len(ps) > 0 {
		return nil, &FormatError{ps}
	}
	b, err := json.MarshalIndent(ix, "", "  ")
	return append(b, '\n'), err
}

// NewSigningKey returns a new private key as PKCS#8 PEM and its public key
// file content.
func NewSigningKey(rand interface{ Read([]byte) (int, error) }) (pemKey []byte, pub Key, err error) {
	pubKey, priv, err := ed25519.GenerateKey(rand)
	if err != nil {
		return nil, Key{}, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, Key{}, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), Key{ID: KeyID(pubKey), Public: pubKey}, nil
}

// PublicKeyFile is the content of trusted/<id>.pub for k.
func PublicKeyFile(k Key) string { return base64.StdEncoding.EncodeToString(k.Public) + "\n" }

// ParseSigningKey reads a PKCS#8 PEM Ed25519 private key.
func ParseSigningKey(pemKey []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(pemKey)
	if block == nil || !strings.Contains(block.Type, "PRIVATE KEY") {
		return nil, errors.New("not a PEM private key")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("private key: %w", err)
	}
	priv, ok := k.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("the private key is not an Ed25519 key")
	}
	return priv, nil
}
