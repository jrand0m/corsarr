package schema

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// Validate checks saved values against the schema: unknown keys, missing
// required fields and every field rule. Messages are fixed text; a field's
// plain-text hint is appended when it has one.
func Validate(s Schema, values map[string]any) Problems {
	c := &collector{}
	known := map[string]bool{}
	for _, f := range s.Fields {
		known[f.Key] = true
		if msg := fieldValueProblem(f, values); msg != "" {
			c.add(f.Key, "%s", withHint(f, msg))
		}
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !known[k] {
			c.add(k, "is not a setting of this plugin")
		}
	}
	return c.ps
}

// fieldValueProblem checks one field's value ("" when fine or unset).
func fieldValueProblem(f Field, values map[string]any) string {
	v, set := values[f.Key]
	if !set || v == nil || v == "" {
		if f.Required && f.Default == nil {
			return "is required"
		}
		return ""
	}
	return valueProblem(f, v)
}

func withHint(f Field, msg string) string {
	if f.Hint == "" {
		return msg
	}
	return msg + " (" + f.Hint + ")"
}

// valueProblem returns "" when v is valid for f.
func valueProblem(f Field, v any) string {
	switch f.Type {
	case abi.TypeString, abi.TypeSecret:
		return stringProblem(abi.TextRule{MinLength: f.MinLength, MaxLength: f.MaxLength, Pattern: f.Pattern}, v)
	case abi.TypeBool:
		if _, ok := v.(bool); !ok {
			return "must be true or false"
		}
	case abi.TypeInt, abi.TypeNumber:
		return numberProblem(f, v)
	case abi.TypeEnum:
		return enumProblem(f, v)
	case abi.TypeDuration:
		return durationProblem(f, v)
	case abi.TypeListString, abi.TypeListInt:
		return listProblem(f, v)
	case abi.TypeMap:
		return mapProblem(f, v)
	}
	return ""
}

func stringProblem(r abi.TextRule, v any) string {
	s, ok := v.(string)
	if !ok {
		return "must be text"
	}
	return textProblem(r, s)
}

func textProblem(r abi.TextRule, s string) string {
	n := utf8.RuneCountInString(s)
	if lo := intOr(r.MinLength, 0); n < lo {
		return fmt.Sprintf("must be at least %d characters", lo)
	}
	if hi := intOr(r.MaxLength, MaxText); n > hi {
		return fmt.Sprintf("must be at most %d characters", hi)
	}
	if strings.ContainsAny(s, "\x00") {
		return "must not contain a NUL character"
	}
	if err := CheckPattern(r.Pattern, s); err != nil {
		return err.Error()
	}
	return ""
}

// AsNumber reads a JSON or config number (or numeric text).
func AsNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

func numberProblem(f Field, v any) string {
	n, ok := AsNumber(v)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
		return "must be a number"
	}
	return boundsProblem(f, n)
}

func boundsProblem(f Field, n float64) string {
	if (f.Type == abi.TypeInt || f.Type == abi.TypeListInt) && n != math.Trunc(n) {
		return "must be a whole number"
	}
	if msg := rangeProblem(f, n); msg != "" {
		return msg
	}
	if f.Step != nil && !onStep(n, *f.Step, f.Min) {
		return "must be a multiple of " + num(*f.Step)
	}
	if f.Decimals != nil && !withinDecimals(n, *f.Decimals) {
		return fmt.Sprintf("must have at most %d decimals", *f.Decimals)
	}
	return ""
}

func rangeProblem(f Field, n float64) string {
	if f.Min != nil && n < *f.Min {
		return "must be at least " + num(*f.Min)
	}
	if f.Max != nil && n > *f.Max {
		return "must be at most " + num(*f.Max)
	}
	return ""
}

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func onStep(n, step float64, min *float64) bool {
	base := 0.0
	if min != nil {
		base = *min
	}
	q := (n - base) / step
	return math.Abs(q-math.Round(q)) < 1e-9
}

func withinDecimals(n float64, d int) bool {
	scaled := n * math.Pow10(d)
	return math.Abs(scaled-math.Round(scaled)) < 1e-6
}

func enumProblem(f Field, v any) string {
	s, _ := v.(string)
	for _, o := range f.Options {
		if o.Value == s {
			return ""
		}
	}
	return "must be one of the listed options"
}

// AsDuration reads a duration value (Go duration text or a config duration).
func AsDuration(v any) (time.Duration, bool) {
	switch d := v.(type) {
	case time.Duration:
		return d, true
	case string:
		p, err := time.ParseDuration(d)
		return p, err == nil
	}
	return 0, false
}

func durationProblem(f Field, v any) string {
	d, ok := AsDuration(v)
	if !ok || d < 0 {
		return "must be a duration like 90s, 15m or 2h"
	}
	if lo, _ := parseDur(f.MinDuration); f.MinDuration != "" && d < lo {
		return "must be at least " + f.MinDuration
	}
	if hi, _ := parseDur(f.MaxDuration); f.MaxDuration != "" && d > hi {
		return "must be at most " + f.MaxDuration
	}
	return ""
}

func listProblem(f Field, v any) string {
	items, ok := asList(v)
	if !ok {
		return "must be a list"
	}
	if limit := orDefault(f.MaxItems, MaxItems); len(items) > limit {
		return fmt.Sprintf("must have at most %d entries", limit)
	}
	for i, it := range items {
		var msg string
		if f.Type == abi.TypeListString {
			msg = stringProblem(abi.TextRule{MinLength: f.MinLength, MaxLength: f.MaxLength, Pattern: f.Pattern}, it)
		} else {
			msg = numberProblem(f, it)
		}
		if msg != "" {
			return fmt.Sprintf("entry %d %s", i+1, msg)
		}
	}
	return ""
}

func asList(v any) ([]any, bool) {
	switch l := v.(type) {
	case []any:
		return l, true
	case []string:
		out := make([]any, len(l))
		for i, s := range l {
			out[i] = s
		}
		return out, true
	}
	return nil, false
}

func orDefault(n, def int) int {
	if n <= 0 {
		return def
	}
	return n
}

// AsMap reads a map value: a JSON object of strings, or a list of
// "key=value" entries (the form config files use).
func AsMap(v any) (map[string]string, bool) {
	switch m := v.(type) {
	case map[string]string:
		return m, true
	case map[string]any:
		out := make(map[string]string, len(m))
		for k, e := range m {
			s, ok := e.(string)
			if !ok {
				return nil, false
			}
			out[k] = s
		}
		return out, true
	}
	items, ok := asList(v)
	if !ok {
		return nil, false
	}
	out := make(map[string]string, len(items))
	for _, it := range items {
		s, _ := it.(string)
		k, val, found := strings.Cut(s, "=")
		if !found {
			return nil, false
		}
		out[k] = val
	}
	return out, true
}

func mapProblem(f Field, v any) string {
	m, ok := AsMap(v)
	if !ok {
		return "must be a set of key=value pairs"
	}
	if limit := orDefault(f.MaxEntries, MaxItems); len(m) > limit {
		return fmt.Sprintf("must have at most %d entries", limit)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if msg := entryProblem(f, k, m[k]); msg != "" {
			return msg
		}
	}
	return ""
}

func entryProblem(f Field, k, val string) string {
	kr, vr := abi.TextRule{MinLength: ptr(1)}, abi.TextRule{}
	if f.KeyRule != nil {
		kr = *f.KeyRule
	}
	if f.ValueRule != nil {
		vr = *f.ValueRule
	}
	if msg := textProblem(kr, k); msg != "" {
		return fmt.Sprintf("key %q %s", k, msg)
	}
	if msg := textProblem(vr, val); msg != "" {
		return fmt.Sprintf("value of %q %s", k, msg)
	}
	return ""
}

func ptr[T any](v T) *T { return &v }
