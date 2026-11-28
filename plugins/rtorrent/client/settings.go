package client

import (
	"strconv"
	"strings"
	"time"
)

// Field describes one setting for a settings form; it is plain data.
type Field struct {
	Key      string
	Type     string // text | url | secret | enum | duration | map
	Label    string
	Help     string
	Required bool
	Default  any
	Options  []string // enum values
}

// Fields describes the connection settings ConfigFrom reads.
func Fields() []Field {
	return []Field{
		{Key: "url", Type: "url", Label: "XML-RPC URL", Required: true,
			Help: "The rTorrent XML-RPC endpoint over HTTP, usually …/RPC2 behind a web front end or reverse proxy. SCGI sockets cannot be reached directly; expose them over HTTP."},
		{Key: "username", Type: "text", Label: "Username", Help: "Basic auth user of the endpoint, if any."},
		{Key: "password", Type: "secret", Label: "Password"},
		{Key: "pathMap", Type: "map", Label: "Path mapping",
			Help: "Folders as rTorrent sees them mapped to the same folders here (remote → local)."},
		{Key: "labelField", Type: "enum", Label: "Label field", Default: "custom1",
			Options: []string{"custom1", "custom2", "custom3", "custom4"},
			Help:    "The custom field that holds the label; web front ends use custom1."},
		{Key: "timeout", Type: "duration", Label: "Request timeout", Default: DefaultTimeout.String()},
	}
}

// ConfigFrom reads settings values (as decoded from YAML or JSON) into a
// Config. The path mapping is a remote → local map; a list of
// "remote=local" entries is accepted too.
func ConfigFrom(s map[string]any) (Config, error) {
	cfg := Config{
		URL: strSetting(s, "url"), Username: strSetting(s, "username"), Password: strSetting(s, "password"),
		LabelField: strSetting(s, "labelField"),
	}
	timeout, err := durationSetting(s["timeout"])
	if err != nil {
		return Config{}, err
	}
	cfg.Timeout = timeout
	pairs, err := pairsSetting(s["pathMap"])
	if err != nil {
		return Config{}, err
	}
	cfg.PathMap, err = ParsePathMap(pairs)
	return cfg, err
}

func strSetting(s map[string]any, key string) string {
	v, _ := s[key].(string)
	return strings.TrimSpace(v)
}

// durationSetting reads "30s"-style strings or plain seconds.
func durationSetting(v any) (time.Duration, error) {
	switch x := v.(type) {
	case nil:
		return 0, nil
	case string:
		return parseDuration(x)
	case int:
		return time.Duration(x) * time.Second, nil
	case int64:
		return time.Duration(x) * time.Second, nil
	case float64:
		return time.Duration(x * float64(time.Second)), nil
	}
	return 0, errorf(KindConfig, "timeout must be a duration such as 30s")
}

func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Second, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, errorf(KindConfig, "timeout %q must be a duration such as 30s", s)
	}
	return d, nil
}

// pairsSetting reads a remote → local map or a list of "remote=local".
func pairsSetting(v any) (map[string]string, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case map[string]string:
		return x, nil
	case map[string]any:
		return anyPairs(x)
	case []any:
		return listPairs(x)
	case []string:
		items := make([]any, len(x))
		for i, s := range x {
			items[i] = s
		}
		return listPairs(items)
	}
	return nil, errorf(KindConfig, "path mapping must map remote folders to local folders")
}

func anyPairs(m map[string]any) (map[string]string, error) {
	out := make(map[string]string, len(m))
	for k, v := range m {
		s, ok := v.(string)
		if !ok {
			return nil, errorf(KindConfig, "path mapping for %s must be a folder", k)
		}
		out[k] = s
	}
	return out, nil
}

func listPairs(items []any) (map[string]string, error) {
	out := make(map[string]string, len(items))
	for _, it := range items {
		s, _ := it.(string)
		remote, local, ok := strings.Cut(s, "=")
		if !ok || strings.Contains(local, "=") {
			return nil, errorf(KindConfig, "path mapping %q must be one remote=local pair", s)
		}
		out[remote] = local
	}
	return out, nil
}
