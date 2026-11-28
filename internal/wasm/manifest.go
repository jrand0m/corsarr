package wasm

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jrand0m/corsarr/internal/plaintext"
	"github.com/jrand0m/corsarr/internal/plugin/schema"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

var (
	idRe     = regexp.MustCompile(`^[a-z][a-z0-9]{1,31}$`)
	semverRe = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]{1,32})?(\+[0-9A-Za-z.-]{1,32})?$`)
	licRe    = regexp.MustCompile(`^[A-Za-z0-9.+-]{1,64}( (AND|OR|WITH) [A-Za-z0-9.+-]{1,64}){0,4}$`)
)

// ManifestError lists every manifest problem.
type ManifestError struct{ Problems schema.Problems }

func (e *ManifestError) Error() string { return "invalid manifest: " + e.Problems.Error() }

// ErrNoManifest is returned when a module has neither an embedded manifest
// section nor a sidecar plugin.json.
var ErrNoManifest = errors.New("no manifest: embed plugin.json as the " + abi.ManifestSection + " custom section or put it next to the module")

// ManifestBytes returns the embedded manifest section, or sidecar when the
// module has none.
func ManifestBytes(module, sidecar []byte) ([]byte, error) {
	if b, ok := CustomSection(module, abi.ManifestSection); ok {
		return b, nil
	}
	if len(sidecar) > 0 {
		return sidecar, nil
	}
	return nil, ErrNoManifest
}

// ParseManifest decodes and validates plugin.json strictly.
func ParseManifest(data []byte) (abi.Manifest, error) {
	var m abi.Manifest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, &ManifestError{schema.Problems{{Path: "manifest", Msg: "is not valid JSON for plugin.json: " + err.Error()}}}
	}
	if ps := CheckManifest(m); len(ps) > 0 {
		return m, &ManifestError{ps}
	}
	return m, nil
}

type mcheck struct{ ps schema.Problems }

func (c *mcheck) add(path, format string, a ...any) {
	c.ps = append(c.ps, schema.Problem{Path: path, Msg: fmt.Sprintf(format, a...)})
}

func (c *mcheck) text(path, s string, kind plaintext.Kind) {
	if _, err := plaintext.Check(s, kind); err != nil {
		c.add(path, "%v", err)
	}
}

// CheckManifest returns every problem of a decoded manifest.
func CheckManifest(m abi.Manifest) schema.Problems {
	c := &mcheck{}
	checkIdentity(c, m)
	checkKind(c, m)
	for _, p := range schema.Check(m.Settings) {
		c.ps = append(c.ps, schema.Problem{Path: "settings." + p.Path, Msg: p.Msg})
	}
	checkPermissions(c, m)
	checkLimits(c, m.Limits)
	return c.ps
}

func checkIdentity(c *mcheck, m abi.Manifest) {
	if !idRe.MatchString(m.ID) {
		c.add("id", "must be 2 to 32 lower-case letters and digits, starting with a letter")
	}
	if !semverRe.MatchString(m.Version) {
		c.add("version", "must be a semantic version such as 1.2.3")
	}
	c.text("name", m.Name, plaintext.Label)
	if strings.TrimSpace(m.Name) == "" {
		c.add("name", "is required")
	}
	c.text("description", m.Description, plaintext.Description)
	c.text("author", m.Author, plaintext.Label)
	if m.License != "" && !licRe.MatchString(m.License) {
		c.add("license", "must be an SPDX licence expression such as MIT or Apache-2.0")
	}
	if m.Homepage != "" && (!strings.HasPrefix(m.Homepage, "https://") || schema.CheckPattern(abi.PatternURLHTTP, m.Homepage) != nil) {
		c.add("homepage", "must be an https:// address")
	}
}

func checkKind(c *mcheck, m abi.Manifest) {
	if m.ABIVersion != abi.Version {
		c.add("abiVersion", "module speaks ABI v%d, this corsarr supports v%d", m.ABIVersion, abi.Version)
	}
	if len(m.Kinds) > 1 || (len(m.Kinds) == 1 && m.Kind != "" && m.Kinds[0] != m.Kind) {
		c.add("kinds", "a module implements exactly one kind; split it into one module per kind")
		return
	}
	kind := m.Kind
	if kind == "" && len(m.Kinds) == 1 {
		kind = m.Kinds[0]
	}
	if _, ok := kinds[kind]; !ok {
		c.add("kind", "%q is not a kind this corsarr can load from a module (supported: %s)", kind, strings.Join(KindNames(), ", "))
	}
}

func checkPermissions(c *mcheck, m abi.Manifest) {
	for i, h := range m.Permissions.HTTP.Hosts {
		if msg := hostPatternProblem(h, m.Settings); msg != "" {
			c.add(fmt.Sprintf("permissions.http.hosts[%d]", i), "%s", msg)
		}
	}
	secrets := schema.SecretFields(m.Settings)
	for i, s := range m.Permissions.Secrets {
		if !secrets[s] {
			c.add(fmt.Sprintf("permissions.secrets[%d]", i), "%q is not a secret field of the settings", s)
		}
	}
}

func hostPatternProblem(h string, s abi.Schema) string {
	if key, ok := strings.CutPrefix(h, abi.SettingHostPrefix); ok {
		f, found := schema.FieldOf(s, key)
		if !found || f.Type != abi.TypeString || (f.Pattern != abi.PatternURLHTTP && f.Pattern != abi.PatternHostname) {
			return "must name a string setting with the url-http or hostname pattern"
		}
		return ""
	}
	host, wild := strings.CutPrefix(h, "*.")
	if schema.CheckPattern(abi.PatternHostname, host) != nil || (wild && !strings.Contains(host, ".")) {
		return "must be a host name, *.domain or $setting:<key>"
	}
	return ""
}

func checkLimits(c *mcheck, l abi.Limits) {
	if l.MemoryMiB < 0 || l.CallTimeoutMs < 0 || l.Concurrency < 0 {
		c.add("limits", "must not be negative")
	}
}

// ManifestKind is the single kind a validated manifest declares.
func ManifestKind(m abi.Manifest) string {
	if m.Kind != "" {
		return m.Kind
	}
	if len(m.Kinds) == 1 {
		return m.Kinds[0]
	}
	return ""
}

// ── wasm custom sections ─────────────────────────────────────────────────────

var wasmMagic = []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

// CustomSection returns the payload of the first custom section called name.
func CustomSection(module []byte, name string) ([]byte, bool) {
	if !bytes.HasPrefix(module, wasmMagic) {
		return nil, false
	}
	rest := module[len(wasmMagic):]
	for len(rest) > 0 {
		id := rest[0]
		size, n := binary.Uvarint(rest[1:])
		if n <= 0 || uint64(len(rest)-1-n) < size {
			return nil, false
		}
		body := rest[1+n : 1+n+int(size)]
		rest = rest[1+n+int(size):]
		if id != 0 {
			continue
		}
		nameLen, m := binary.Uvarint(body)
		if m <= 0 || uint64(len(body)-m) < nameLen {
			return nil, false
		}
		if string(body[m:m+int(nameLen)]) == name {
			return body[m+int(nameLen):], true
		}
	}
	return nil, false
}

// AppendCustomSection returns module with a custom section added at the end.
func AppendCustomSection(module []byte, name string, payload []byte) []byte {
	var body []byte
	body = binary.AppendUvarint(body, uint64(len(name)))
	body = append(body, name...)
	body = append(body, payload...)
	out := append([]byte{}, module...)
	out = append(out, 0)
	out = binary.AppendUvarint(out, uint64(len(body)))
	return append(out, body...)
}
