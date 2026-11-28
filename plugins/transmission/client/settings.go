package client

import (
	"fmt"
	"strings"
	"time"
)

// FieldType is the kind of value a setting holds.
type FieldType string

const (
	FieldURL      FieldType = "url"
	FieldText     FieldType = "text"
	FieldSecret   FieldType = "secret"
	FieldBool     FieldType = "bool"
	FieldDuration FieldType = "duration"
	FieldMap      FieldType = "map" // string keys to string values
)

// Field describes one setting of an instance; it is plain data for a
// settings schema builder.
type Field struct {
	Key      string
	Type     FieldType
	Label    string
	Help     string
	Required bool
	Default  any
}

// Setting keys.
const (
	KeyURL          = "url"
	KeyUsername     = "username"
	KeyPassword     = "password"
	KeyCategoryDirs = "categoryDirs"
	KeyPathMap      = "pathMap"
	KeyAddPaused    = "addPaused"
	KeyTimeout      = "timeout"
)

// Settings describes every setting of a Transmission instance.
func Settings() []Field {
	return []Field{
		{Key: KeyURL, Type: FieldURL, Label: "Address", Required: true,
			Help: "Address of Transmission's web interface, for example http://transmission:9091. A reverse-proxy sub-path is allowed; an address ending in /rpc is used as is."},
		{Key: KeyUsername, Type: FieldText, Label: "Username",
			Help: "RPC user name. Leave empty when authentication is off."},
		{Key: KeyPassword, Type: FieldSecret, Label: "Password",
			Help: "RPC password."},
		{Key: KeyCategoryDirs, Type: FieldMap, Label: "Download directory per category", Default: map[string]string{},
			Help: "Directory, as Transmission sees it, for the torrents of each category. Optional where Transmission has labels (3.00 and later): the category is stored as a label. Needed on older releases, where the directory is how a category is recognised."},
		{Key: KeyPathMap, Type: FieldMap, Label: "Path mapping", Default: map[string]string{},
			Help: "Needed when Transmission sees its download folders under other paths than corsarr (another host, or containers with different mounts). Each entry maps a folder as Transmission sees it to the same folder here, for example /downloads to /mnt/media/downloads. The longest matching prefix wins."},
		{Key: KeyAddPaused, Type: FieldBool, Label: "Add paused", Default: false,
			Help: "Add every torrent stopped, to be started in Transmission."},
		{Key: KeyTimeout, Type: FieldDuration, Label: "Request timeout", Default: DefaultTimeout.String(),
			Help: "How long to wait for one answer from Transmission."},
	}
}

// ConfigFromSettings reads a Config from setting values keyed as Settings
// describes: strings, a bool, a duration string (or seconds as a number) and
// string maps.
func ConfigFromSettings(v map[string]any) (Config, error) {
	cfg := Config{
		URL: str(v[KeyURL]), Username: str(v[KeyUsername]), Password: str(v[KeyPassword]),
	}
	var err error
	if cfg.CategoryDirs, err = strMap(KeyCategoryDirs, v[KeyCategoryDirs]); err != nil {
		return Config{}, err
	}
	if cfg.PathMap, err = strMap(KeyPathMap, v[KeyPathMap]); err != nil {
		return Config{}, err
	}
	cfg.AddPaused, _ = v[KeyAddPaused].(bool)
	if cfg.Timeout, err = duration(v[KeyTimeout]); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func str(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func strMap(key string, v any) (map[string]string, error) {
	out := map[string]string{}
	switch m := v.(type) {
	case nil:
	case map[string]string:
		for k, s := range m {
			out[k] = s
		}
	case map[string]any:
		for k, raw := range m {
			s, ok := raw.(string)
			if !ok {
				return nil, errorf(ClassConfig, "%s.%s must be text", key, k)
			}
			out[k] = s
		}
	default:
		return nil, errorf(ClassConfig, "%s must be a map of text values", key)
	}
	return out, nil
}

func duration(v any) (time.Duration, error) {
	switch d := v.(type) {
	case nil:
		return 0, nil
	case string:
		return parseDuration(d)
	case float64:
		return time.Duration(d * float64(time.Second)), nil
	case int:
		return time.Duration(d) * time.Second, nil
	case time.Duration:
		return d, nil
	}
	return 0, errorf(ClassConfig, "%s must be a duration", KeyTimeout)
}

func parseDuration(s string) (time.Duration, error) {
	if strings.TrimSpace(s) == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil || d < 0 {
		return 0, wrap(ClassConfig, KeyTimeout, fmt.Errorf("%q is not a duration such as 30s", s))
	}
	return d, nil
}
