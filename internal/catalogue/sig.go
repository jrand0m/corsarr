package catalogue

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// MessagePrefix starts every signed message, so a signature made for other
// data cannot pass as an index signature.
const MessagePrefix = "corsarr-plugin-index-v1\n"

// SigAlg is the only signature algorithm.
const SigAlg = "ed25519"

// Signature is plugins-index.json.sig.
type Signature struct {
	Schema    int    `json:"schema"`
	Alg       string `json:"alg"`
	KeyID     string `json:"keyId"`
	Signature string `json:"signature"`
}

// Key is a trusted index signing key.
type Key struct {
	ID     string
	Public ed25519.PublicKey
}

// Signature errors.
var (
	ErrUnsigned     = errors.New("the catalogue is not signed")
	ErrNoTrustedKey = errors.New("this build has no trusted catalogue key, so no catalogue can be checked")
	ErrUnknownKey   = errors.New("the catalogue is signed with a key this build does not trust")
	ErrBadSignature = errors.New("the catalogue signature does not match its content")
)

// KeyID names a public key: the first 16 hex characters of its SHA-256.
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])[:16]
}

// ParseKey reads a public key file: one line, standard base64 of the 32 raw
// key bytes.
func ParseKey(text string) (Key, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return Key{}, errors.New("a public key file holds the base64 of 32 key bytes")
	}
	pub := ed25519.PublicKey(raw)
	return Key{ID: KeyID(pub), Public: pub}, nil
}

// message is what is signed for index bytes.
func message(index []byte) []byte {
	return append([]byte(MessagePrefix), index...)
}

// Sign returns the signature file for index bytes.
func Sign(index []byte, priv ed25519.PrivateKey) []byte {
	pub, _ := priv.Public().(ed25519.PublicKey)
	sig := Signature{Schema: 1, Alg: SigAlg, KeyID: KeyID(pub),
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, message(index)))}
	out, _ := json.Marshal(sig)
	return append(out, '\n')
}

// Verify checks index bytes against their signature file with the trusted
// keys; only then may the index be decoded.
func Verify(index, sigFile []byte, keys []Key) error {
	if len(bytes.TrimSpace(sigFile)) == 0 {
		return ErrUnsigned
	}
	if len(keys) == 0 {
		return ErrNoTrustedKey
	}
	keyID, raw, err := decodeSignature(sigFile)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if k.ID != keyID {
			continue
		}
		if ed25519.Verify(k.Public, message(index), raw) {
			return nil
		}
		return ErrBadSignature
	}
	return fmt.Errorf("%w (key %s)", ErrUnknownKey, keyID)
}

// decodeSignature reads a signature file strictly.
func decodeSignature(sigFile []byte) (keyID string, raw []byte, err error) {
	var sig Signature
	dec := json.NewDecoder(bytes.NewReader(sigFile))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&sig); err != nil {
		return "", nil, fmt.Errorf("the catalogue signature file is unreadable: %w", err)
	}
	if sig.Schema != 1 || sig.Alg != SigAlg {
		return "", nil, fmt.Errorf("the catalogue signature uses format %d / %q; this build reads format 1 / %s", sig.Schema, sig.Alg, SigAlg)
	}
	raw, err = base64.StdEncoding.DecodeString(sig.Signature)
	if err != nil || len(raw) != ed25519.SignatureSize {
		return "", nil, ErrBadSignature
	}
	return sig.KeyID, raw, nil
}

//go:embed trusted
var trustedFS embed.FS

// Trusted returns the keys compiled into this build from trusted/<id>.pub;
// a file whose name does not match its key is ignored.
func Trusted() []Key { return keysIn(trustedFS, "trusted") }

func keysIn(fsys fs.FS, dir string) []Key {
	names, _ := fs.Glob(fsys, path.Join(dir, "*.pub"))
	sort.Strings(names)
	var out []Key
	for _, n := range names {
		b, err := fs.ReadFile(fsys, n)
		if err != nil {
			continue
		}
		k, err := ParseKey(string(b))
		if err == nil && strings.TrimSuffix(path.Base(n), ".pub") == k.ID {
			out = append(out, k)
		}
	}
	return out
}
