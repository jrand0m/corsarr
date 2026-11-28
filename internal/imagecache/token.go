// Package imagecache fetches artwork once through the shared outbound client,
// stores it shrunk and re-encoded under the cache folder, serves it from
// there, and keeps the folder under its size cap by lowering quality, then
// width, then removing the least recently viewed files.
//
// Clients never name a provider address: every image is reached through a
// signed token that only corsarr can produce, so the image route is not an
// open proxy.
package imagecache

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Role says how an image is used, which decides its stored width.
type Role = domain.ArtRole

const (
	RolePoster   = domain.ArtPoster
	RoleBackdrop = domain.ArtBackdrop
)

func roleName(r Role) string {
	if r == RoleBackdrop {
		return "backdrop"
	}
	return "poster"
}

func parseRole(b byte) (Role, bool) {
	switch r := Role(b); r {
	case RolePoster, RoleBackdrop:
		return r, true
	}
	return 0, false
}

// LinkPrefix is the API path of the image route, relative like every API
// path the web UI uses, so it works under any base path.
const LinkPrefix = "api/v1/images/"

const macSize = 16

// Signer makes and checks image tokens.
type Signer struct{ key []byte }

// NewSigner returns a signer for a secret key.
func NewSigner(key []byte) Signer { return Signer{key: append([]byte(nil), key...)} }

func (s Signer) mac(payload []byte) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write(payload)
	return m.Sum(nil)[:macSize]
}

// Sign returns the token of an origin address for a role.
func (s Signer) Sign(role Role, origin string) string {
	payload := append([]byte{byte(role)}, origin...)
	return base64.RawURLEncoding.EncodeToString(append(s.mac(payload), payload...))
}

// Open checks a token and returns its role and origin address.
func (s Signer) Open(token string) (Role, string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) < macSize+2 {
		return 0, "", false
	}
	sum, payload := raw[:macSize], raw[macSize:]
	role, ok := parseRole(payload[0])
	if !ok || !hmac.Equal(sum, s.mac(payload)) {
		return 0, "", false
	}
	return role, string(payload[1:]), true
}

// Link turns a provider address into the cached image path; an empty or
// non-http(s) address gives "" (the UI then shows its letter tile).
func (s Signer) Link(role Role, origin string) string {
	if !fetchable(origin) {
		return ""
	}
	return LinkPrefix + s.Sign(role, origin)
}

func fetchable(origin string) bool {
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// Key is the index key of an origin address: its SHA-256 in hex.
func Key(origin string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(origin)))
	return hex.EncodeToString(sum[:])
}
