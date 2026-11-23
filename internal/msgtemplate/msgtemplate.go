// Package msgtemplate is the one place notification message templates are
// parsed: the built-in templates, per-connection overrides and config
// validation use the same functions, so a template accepted at config load
// renders at delivery.
package msgtemplate

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// Funcs are the functions a message template may call.
var Funcs = template.FuncMap{"join": func(xs []string) string { return strings.Join(xs, ", ") }}

// Parse parses a message template; missing fields render as empty.
func Parse(name, text string) (*template.Template, error) {
	t, err := template.New(name).Funcs(Funcs).Option("missingkey=zero").Parse(text)
	if err != nil {
		return nil, fmt.Errorf("not a valid message template: %w", err)
	}
	return t, nil
}

// Check validates template text (config validation).
func Check(text string) error {
	_, err := Parse("check", text)
	return err
}

// Exec renders t with v, trimmed; a failing template renders "".
func Exec(t *template.Template, v any) string {
	var b bytes.Buffer
	if err := t.Execute(&b, v); err != nil {
		return ""
	}
	return strings.TrimSpace(b.String())
}
