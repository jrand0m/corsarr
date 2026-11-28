package config

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// Secret is a secret value plus the reference it came from (if any). It
// never prints, logs or marshals its value.
type Secret struct {
	Ref   string // "${env:NAME}" or "${file:/path}"; "" when given directly
	value string
}

// NewSecret wraps a plain secret value (UI/database origin).
func NewSecret(v string) Secret { return Secret{value: v} }

// Value is the resolved secret for the code that needs it (plugins).
func (s Secret) Value() string { return s.value }

func (s Secret) String() string   { return "[redacted]" }
func (s Secret) GoString() string { return "config.Secret{[redacted]}" }

// MarshalText keeps secrets out of JSON/YAML output: only the reference shows.
func (s Secret) MarshalText() ([]byte, error) {
	if s.Ref != "" {
		return []byte(s.Ref), nil
	}
	return []byte("[redacted]"), nil
}

var refRe = regexp.MustCompile(`^\$\{(env|file):([^}]+)\}$`)

// parseRef recognises `${env:NAME}` and `${file:/path}`.
func parseRef(s string) (scheme, target string, ok bool) {
	m := refRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// Resolver looks up secret references; tests inject fakes.
type Resolver struct {
	Getenv   func(string) (string, bool)
	ReadFile func(string) ([]byte, error)
}

// OSResolver reads the process environment and the filesystem.
func OSResolver() Resolver { return Resolver{Getenv: os.LookupEnv, ReadFile: os.ReadFile} }

// resolve returns the secret for raw text: a reference is looked up, a plain
// value is taken as is.
func (r Resolver) resolve(text string) (Secret, error) {
	scheme, target, ok := parseRef(text)
	if !ok {
		return Secret{value: text}, nil
	}
	ref := "${" + scheme + ":" + target + "}"
	if scheme == "env" {
		v, found := r.Getenv(target)
		if !found {
			return Secret{}, fmt.Errorf("environment variable %s is not set", target)
		}
		return Secret{Ref: ref, value: v}, nil
	}
	b, err := r.ReadFile(target)
	if err != nil {
		return Secret{}, fmt.Errorf("secret file %s cannot be read", target)
	}
	return Secret{Ref: ref, value: strings.TrimRight(string(b), "\r\n")}, nil
}

// parseLeaf turns the one text form into a typed value. Secrets are handled
// by the decoder (they need the resolver).
func parseLeaf(n *Node, s string) (any, error) {
	v, err := parseScalar(n, s)
	if err != nil {
		return nil, err
	}
	if n.Check != nil {
		if err := n.Check(s); err != nil {
			return nil, err
		}
	}
	return v, nil
}

func parseScalar(n *Node, s string) (any, error) {
	switch n.Kind {
	case KindInt:
		v, err := strconv.ParseInt(s, 10, 64)
		return v, wrap(err, "must be an integer")
	case KindNumber:
		v, err := strconv.ParseFloat(s, 64)
		return v, wrap(err, "must be a number")
	case KindBool:
		v, err := strconv.ParseBool(s)
		return v, wrap(err, "must be true or false")
	case KindDuration:
		v, err := time.ParseDuration(s)
		return v, wrap(err, "must be a duration like 90s, 15m or 2h")
	case KindEnum:
		return s, enumCheck(n, s)
	}
	return s, nil
}

func wrap(err error, msg string) error {
	if err != nil {
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func enumCheck(n *Node, s string) error {
	if len(n.Enum) > 0 && !slices.Contains(n.Enum, s) {
		return fmt.Errorf("must be one of %s", strings.Join(n.Enum, ", "))
	}
	return nil
}

// formatLeaf is the inverse of parseLeaf: the canonical text form.
func formatLeaf(v any) string {
	switch x := v.(type) {
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case time.Duration:
		return x.String()
	case Secret:
		return x.Ref
	}
	return fmt.Sprint(v)
}

// canonicalDefault is the default in canonical form ("" when none).
func canonicalDefault(n *Node) string {
	if n.Default == "" {
		return ""
	}
	v, err := parseScalar(n, n.Default)
	if err != nil {
		return n.Default
	}
	return formatLeaf(v)
}

// rangeCheck enforces a plugin number field's min/max.
func rangeCheck(f plugin.Field) func(string) error {
	if f.Type != plugin.FieldNumber || (f.Min == nil && f.Max == nil) {
		return nil
	}
	return func(s string) error {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil // reported by parseScalar
		}
		if f.Min != nil && v < *f.Min {
			return fmt.Errorf("must be ≥ %v", *f.Min)
		}
		if f.Max != nil && v > *f.Max {
			return fmt.Errorf("must be ≤ %v", *f.Max)
		}
		return nil
	}
}

// bothChecks runs a then b; either may be nil.
func bothChecks(a, b func(string) error) func(string) error {
	if a == nil || b == nil {
		if a == nil {
			return b
		}
		return a
	}
	return func(s string) error {
		if err := a(s); err != nil {
			return err
		}
		return b(s)
	}
}
