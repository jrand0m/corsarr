package definition

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Path is a JSON path: dot-separated keys with [n] indexes (data.items[0].name).
// The empty path is the value itself.
type Path []pathStep

type pathStep struct {
	key   string
	index int
	isIdx bool
}

// ParsePath parses a path once at load.
func ParsePath(s string) (Path, error) {
	var p Path
	for _, seg := range strings.Split(s, ".") {
		if seg == "" {
			continue
		}
		key, rest, _ := strings.Cut(seg, "[")
		if key != "" {
			p = append(p, pathStep{key: key})
		}
		idx, err := indexes(rest)
		if err != nil {
			return nil, fmt.Errorf("path %q: %w", s, err)
		}
		p = append(p, idx...)
	}
	return p, nil
}

// indexes reads "0][1]" (the part after a segment's first "[").
func indexes(rest string) ([]pathStep, error) {
	var out []pathStep
	for rest != "" {
		idx, after, ok := strings.Cut(rest, "]")
		n, err := strconv.Atoi(idx)
		if !ok || err != nil || n < 0 {
			return nil, fmt.Errorf("bad index")
		}
		out = append(out, pathStep{index: n, isIdx: true})
		if after != "" && !strings.HasPrefix(after, "[") {
			return nil, fmt.Errorf("unexpected %q", after)
		}
		rest = strings.TrimPrefix(after, "[")
	}
	return out, nil
}

// Get walks v (decoded with UseNumber) along the path.
func (p Path) Get(v any) (any, bool) {
	for _, st := range p {
		switch t := v.(type) {
		case map[string]any:
			if st.isIdx {
				return nil, false
			}
			var ok bool
			if v, ok = t[st.key]; !ok {
				return nil, false
			}
		case []any:
			if !st.isIdx || st.index >= len(t) {
				return nil, false
			}
			v = t[st.index]
		default:
			return nil, false
		}
	}
	return v, true
}

// scalar renders a JSON value as text; arrays join their scalars with commas.
func scalar(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		return strconv.FormatBool(t)
	case []any:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			if s := scalar(e); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	}
	b, _ := json.Marshal(v)
	return string(b)
}
