package torznab

import (
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

var kindAlias = map[string]string{"movies": "movie", "audio": "music", "books": "book"}

// parseCategories validates the per-kind category setting once, merged over the defaults.
func parseCategories(raw map[string]any) (map[domain.MediaKind][]int, error) {
	out := map[domain.MediaKind][]int{}
	for k, v := range defaultCategories {
		kind, _ := domain.ParseMediaKind(k)
		out[kind] = v.([]int)
	}
	for k, v := range raw {
		if a, ok := kindAlias[k]; ok {
			k = a
		}
		kind, err := domain.ParseMediaKind(k)
		if err != nil {
			return nil, plugin.Wrap(plugin.ClassConfig, "categories", err)
		}
		ids, err := toInts(v)
		if err != nil {
			return nil, plugin.Wrap(plugin.ClassConfig, "categories."+k, err)
		}
		out[kind] = ids
	}
	return out, nil
}

func toInts(v any) ([]int, error) {
	switch t := v.(type) {
	case []int:
		return t, nil
	case string:
		return splitInts(strings.Split(t, ","))
	case []any:
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = toString(e)
		}
		return splitInts(parts)
	}
	return nil, plugin.Errorf(plugin.ClassConfig, "want a list of category ids, got %T", v)
}

func toString(e any) string {
	switch n := e.(type) {
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		return strconv.Itoa(int(n))
	case string:
		return n
	}
	return ""
}

func splitInts(parts []string) ([]int, error) {
	var ids []int
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 {
			return nil, plugin.Errorf(plugin.ClassConfig, "%q is not a category id", p)
		}
		ids = append(ids, n)
	}
	return ids, nil
}

func joinInts(ids []int) string {
	s := make([]string, len(ids))
	for i, n := range ids {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ",")
}
