// Package conformance holds the reusable per-extension-point checks every
// plugin must pass. Built-ins call them from
// their tests; `corsarr plugin test` can reuse them.
package conformance

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/jrand0m/corsarr/internal/plugin"
)

var (
	fieldKeyRe  = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
	secretKeyRe = regexp.MustCompile(`(?i)(apikey|token|password|secret|urls)$`)
)

var validTypes = map[plugin.FieldType]bool{
	plugin.FieldText: true, plugin.FieldSecret: true, plugin.FieldNumber: true, plugin.FieldBool: true,
	plugin.FieldEnum: true, plugin.FieldMultiselect: true, plugin.FieldDuration: true, plugin.FieldSize: true,
	plugin.FieldURL: true, plugin.FieldPath: true, plugin.FieldRegex: true, plugin.FieldList: true, plugin.FieldMap: true,
}

// ManifestProblems lists every way a manifest violates the plugin contract.
func ManifestProblems(m plugin.Manifest, kind plugin.Kind) []string {
	var p []string
	add := func(format string, a ...any) { p = append(p, fmt.Sprintf(format, a...)) }
	if m.ID == "" || strings.ToLower(m.ID) != m.ID {
		add("id %q must be non-empty lower case", m.ID)
	}
	if m.Kind != kind {
		add("kind %q, want %q", m.Kind, kind)
	}
	if m.Version == "" || m.Name == "" {
		add("version and name are required")
	}
	if m.APIVersion != plugin.APIVersion {
		add("apiVersion %d, host %d", m.APIVersion, plugin.APIVersion)
	}
	if m.Delivery != "builtin" {
		add("built-in delivery must be %q, got %q", "builtin", m.Delivery)
	}
	seen := map[string]bool{}
	for _, f := range m.Settings {
		p = append(p, fieldProblems(f, seen)...)
	}
	return p
}

type fieldRule struct {
	msg string
	bad func(plugin.Field) bool
}

var fieldRules = []fieldRule{
	{"key must be camelCase", func(f plugin.Field) bool { return !fieldKeyRe.MatchString(f.Key) }},
	{"unknown type", func(f plugin.Field) bool { return !validTypes[f.Type] }},
	{"label and help are required", func(f plugin.Field) bool { return f.Label == "" || f.Help == "" }},
	{"looks secret but is not flagged secret", func(f plugin.Field) bool { return secretKeyRe.MatchString(f.Key) && !f.IsSecret() }},
	{"enum needs options", func(f plugin.Field) bool {
		return (f.Type == plugin.FieldEnum || f.Type == plugin.FieldMultiselect) && len(f.Options) == 0
	}},
	{"a required field cannot have a default", func(f plugin.Field) bool { return f.Required && f.Default != nil }},
	{"secret fields cannot have defaults", func(f plugin.Field) bool { return f.IsSecret() && f.Default != nil }},
}

func fieldProblems(f plugin.Field, seen map[string]bool) []string {
	var p []string
	if seen[f.Key] {
		p = append(p, fmt.Sprintf("field %q: duplicate key", f.Key))
	}
	seen[f.Key] = true
	for _, r := range fieldRules {
		if r.bad(f) {
			p = append(p, fmt.Sprintf("field %q: %s", f.Key, r.msg))
		}
	}
	return p
}

// Manifest fails the test for each manifest problem.
func Manifest(t *testing.T, m plugin.Manifest, kind plugin.Kind) {
	t.Helper()
	for _, problem := range ManifestProblems(m, kind) {
		t.Error(problem)
	}
}
