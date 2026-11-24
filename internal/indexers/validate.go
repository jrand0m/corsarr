package indexers

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// FieldError names the settings field a value was refused for.
type FieldError struct {
	Field string `json:"field"`
	Msg   string `json:"message"`
}

func (e FieldError) Error() string { return e.Field + ": " + e.Msg }

// Common instance settings every indexer accepts besides its plugin's own.
var commonFields = []plugin.Field{
	{Key: "queryLimit", Type: plugin.FieldNumber, Label: "Query limit"},
	{Key: "queryLimitPer", Type: plugin.FieldEnum, Label: "Query limit window", Options: []string{"hour", "day"}},
}

// Values are the parsed settings of one instance.
type Values struct {
	Settings map[string]any
	Secrets  map[string]string
}

// Validate parses submitted settings against a manifest (pure). A secret left
// empty keeps the previous value; keys the manifest does not know are refused.
func Validate(m plugin.Manifest, in map[string]any, prevSecrets map[string]string) (Values, []FieldError) {
	v := Values{Settings: map[string]any{}, Secrets: map[string]string{}}
	var errs []FieldError
	fields := append(append([]plugin.Field{}, m.Settings...), commonFields...)
	known := map[string]bool{}
	for _, f := range fields {
		known[f.Key] = true
		var err error
		if f.IsSecret() {
			err = v.secret(f, text(in[f.Key]), prevSecrets[f.Key])
		} else {
			err = v.plain(f, in[f.Key])
		}
		if err != nil {
			errs = append(errs, FieldError{f.Key, err.Error()})
		}
	}
	for k := range in {
		if !known[k] {
			errs = append(errs, FieldError{k, "is not a setting of " + m.Name})
		}
	}
	return v, errs
}

var errRequired = fmt.Errorf("is required")

// secret keeps a new value, else the previous one.
func (v Values) secret(f plugin.Field, given, prev string) error {
	if s := firstNonEmpty(given, prev); s != "" {
		v.Secrets[f.Key] = s
		return nil
	}
	if f.Required {
		return errRequired
	}
	return nil
}

func (v Values) plain(f plugin.Field, raw any) error {
	if raw == nil || raw == "" {
		if f.Required && f.Default == nil {
			return errRequired
		}
		return nil
	}
	val, err := parseField(f, raw)
	if err == nil {
		v.Settings[f.Key] = val
	}
	return err
}

func text(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func parseField(f plugin.Field, raw any) (any, error) {
	switch f.Type {
	case plugin.FieldNumber:
		return parseNumber(f, raw)
	case plugin.FieldBool:
		return parseBool(raw)
	case plugin.FieldEnum:
		return parseEnum(f, text(raw))
	case plugin.FieldURL:
		return parseURL(text(raw))
	case plugin.FieldList, plugin.FieldMap, plugin.FieldMultiselect:
		return raw, nil
	}
	return text(raw), nil
}

func parseNumber(f plugin.Field, raw any) (any, error) {
	n, err := strconv.ParseFloat(text(raw), 64)
	switch {
	case err != nil:
		return nil, fmt.Errorf("must be a number")
	case f.Min != nil && n < *f.Min:
		return nil, fmt.Errorf("must be at least %v", *f.Min)
	case f.Max != nil && n > *f.Max:
		return nil, fmt.Errorf("must be at most %v", *f.Max)
	}
	return n, nil
}

func parseBool(raw any) (any, error) {
	if b, ok := raw.(bool); ok {
		return b, nil
	}
	b, err := strconv.ParseBool(text(raw))
	if err != nil {
		return nil, fmt.Errorf("must be true or false")
	}
	return b, nil
}

func parseEnum(f plugin.Field, s string) (any, error) {
	for _, o := range f.Options {
		if o == s {
			return s, nil
		}
	}
	return nil, fmt.Errorf("must be one of %s", strings.Join(f.Options, ", "))
}

func parseURL(s string) (any, error) {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("must be an http(s) address")
	}
	return s, nil
}
