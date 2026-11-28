package store

import (
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
)

// KeySize is the length of secret.key.
const KeySize = 32

const sealPrefix = "v1:"

// ErrBadCiphertext is returned for tampered or foreign ciphertext.
var ErrBadCiphertext = errors.New("secret cannot be decrypted with this secret.key")

type cipherBox struct {
	aead cipher.AEAD
	key  []byte
}

// loadKey reads secret.key, creating 32 random bytes (0600) on first start.
func loadKey(path string) (*cipherBox, error) {
	key, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, KeySize)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, key, 0o600); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return newBox(key)
}

func newBox(key []byte) (*cipherBox, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("secret.key must be %d bytes, found %d", KeySize, len(key))
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	return &cipherBox{aead: aead, key: key}, nil
}

// SubKey derives a 32-byte key for one purpose from secret.key, so a purpose
// such as signing links never sees the encryption key itself.
func (s *Store) SubKey(purpose string) []byte {
	m := hmac.New(sha256.New, s.aead.key)
	m.Write([]byte(purpose))
	return m.Sum(nil)
}

// Encrypt seals a secret field value: "v1:" + base64(nonce ‖ ciphertext).
func (s *Store) Encrypt(plain string) (string, error) { return s.aead.seal(plain) }

// Decrypt opens a value produced by Encrypt.
func (s *Store) Decrypt(sealed string) (string, error) { return s.aead.open(sealed) }

func (b *cipherBox) seal(plain string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := b.aead.Seal(nonce, nonce, []byte(plain), nil)
	return sealPrefix + base64.RawStdEncoding.EncodeToString(out), nil
}

func (b *cipherBox) open(sealed string) (string, error) {
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(sealed, sealPrefix))
	if err != nil || !strings.HasPrefix(sealed, sealPrefix) || len(raw) < b.aead.NonceSize() {
		return "", ErrBadCiphertext
	}
	n := b.aead.NonceSize()
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		return "", ErrBadCiphertext
	}
	return string(plain), nil
}
