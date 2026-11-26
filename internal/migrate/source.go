// Package migrate imports an existing TV, movie, music and indexer-manager
// installation through its public HTTP API, read-only. A run reads every
// source into a Snapshot, builds a Plan (one outcome and reason per source
// object) without writing anything, and on apply adopts library items and
// their files in place, adopts in-flight downloads, imports blocklist and
// recent history, emits a config snippet for config-managed objects, puts
// the migrated media kinds into shadow mode and every automation rule into
// dry-run. Provenance makes re-runs converge.
//
// Snapshot and Plan are pure; the client, the store target and the service
// are the I/O edges.
package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
)

// App is a source application type.
type App string

const (
	Sonarr   App = "sonarr"
	Radarr   App = "radarr"
	Lidarr   App = "lidarr"
	Prowlarr App = "prowlarr"
)

// ParseApp validates a source type at the boundary.
func ParseApp(s string) (App, error) {
	switch a := App(strings.ToLower(strings.TrimSpace(s))); a {
	case Sonarr, Radarr, Lidarr, Prowlarr:
		return a, nil
	}
	return "", fmt.Errorf("source %q must be sonarr, radarr, lidarr or prowlarr", s)
}

// apiBase is the API root of the app.
func (a App) apiBase() string {
	if a == Prowlarr || a == Lidarr {
		return "/api/v1"
	}
	return "/api/v3"
}

// Kind is the media kind whose library the app holds ("" for none).
func (a App) Kind() domain.MediaKind {
	switch a {
	case Sonarr:
		return domain.KindTV
	case Radarr:
		return domain.KindMovie
	case Lidarr:
		return domain.KindMusic
	}
	return ""
}

// Secret is an API key or password: it never prints, logs or serialises.
type Secret struct{ v string }

// NewSecret wraps a secret value; surrounding whitespace is dropped.
func NewSecret(s string) Secret { return Secret{strings.TrimSpace(s)} }

// Reveal returns the value for the one place that sends it.
func (s Secret) Reveal() string { return s.v }

// Empty reports whether no value is set.
func (s Secret) Empty() bool { return s.v == "" }

const redacted = "[redacted]"

func (s Secret) String() string                { return redacted }
func (s Secret) GoString() string              { return redacted }
func (s Secret) LogValue() slog.Value          { return slog.StringValue(redacted) }
func (s Secret) MarshalJSON() ([]byte, error)  { return []byte(`"` + redacted + `"`), nil }
func (s Secret) MarshalText() ([]byte, error)  { return []byte(redacted), nil }
func (s *Secret) UnmarshalText(b []byte) error { *s = NewSecret(string(b)); return nil }
func (s *Secret) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = NewSecret(v)
	return nil
}

// Endpoint is one source to read: its type, base address and API key.
type Endpoint struct {
	App App
	URL *url.URL
	Key Secret
}

// ParseEndpoint validates a source at the boundary.
func ParseEndpoint(app, rawURL string, key Secret) (Endpoint, error) {
	a, err := ParseApp(app)
	if err != nil {
		return Endpoint{}, err
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Endpoint{}, fmt.Errorf("%s address %q must be an http or https URL", a, rawURL)
	}
	if key.Empty() {
		return Endpoint{}, fmt.Errorf("%s needs an API key", a)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery, u.Fragment, u.User = "", "", nil
	return Endpoint{App: a, URL: u, Key: key}, nil
}

// Instance is the stable id of the source: its type and canonical address.
func (e Endpoint) Instance() string {
	sum := sha256.Sum256([]byte(string(e.App) + "|" + strings.ToLower(e.URL.String())))
	return hex.EncodeToString(sum[:8])
}

// Errors a source check reports.
var (
	ErrUnauthorised       = errors.New("unauthorised: the API key was refused")
	ErrUnsupportedVersion = errors.New("unsupported version")
	ErrWriteRefused       = errors.New("migration only reads from sources; a non-GET request was refused")
)

// supported is the major-version range per app: [min, below).
var supported = map[App][2]int{Sonarr: {3, 5}, Radarr: {3, 7}, Lidarr: {1, 4}, Prowlarr: {1, 3}}

// SupportedRange renders the supported versions of an app.
func SupportedRange(a App) string {
	r := supported[a]
	return fmt.Sprintf("%d.x to %d.x", r[0], r[1]-1)
}

// CheckVersion refuses a version outside the supported range.
func CheckVersion(a App, version string) error {
	major, err := strconv.Atoi(strings.SplitN(strings.TrimPrefix(version, "v"), ".", 2)[0])
	r := supported[a]
	if err != nil || major < r[0] || major >= r[1] {
		return fmt.Errorf("%w: %s %q detected, supported %s", ErrUnsupportedVersion, a, version, SupportedRange(a))
	}
	return nil
}
