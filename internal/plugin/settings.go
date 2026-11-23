package plugin

import "strconv"

// Int returns an integer setting (YAML/JSON numbers or numeric strings) or def.
func (s Settings) Int(key string, def int) int {
	switch v := s[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// Strings returns a list-of-strings setting (a single string counts as one).
func (s Settings) Strings(key string) []string {
	switch v := s[key].(type) {
	case []string:
		return v
	case string:
		if v != "" {
			return []string{v}
		}
	case []any:
		var out []string
		for _, e := range v {
			if str, ok := e.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

// Map returns a map setting as string keys with raw values.
func (s Settings) Map(key string) map[string]any {
	switch v := s[key].(type) {
	case map[string]any:
		return v
	case map[string]string:
		out := make(map[string]any, len(v))
		for k, e := range v {
			out[k] = e
		}
		return out
	}
	return nil
}

// IsSecret reports whether the field's value must never be logged or exported in clear.
func (f Field) IsSecret() bool { return f.Secret || f.Type == FieldSecret }
