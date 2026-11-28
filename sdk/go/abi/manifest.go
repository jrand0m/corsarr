package abi

// ManifestSection is the name of the wasm custom section holding plugin.json.
const ManifestSection = "corsarr.manifest"

// Manifest is plugin.json: who the module is, what it may touch and how its
// settings form looks.
type Manifest struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Version     string      `json:"version"`
	Kind        string      `json:"kind"`
	Kinds       []string    `json:"kinds,omitempty"` // accepted only to refuse multi-kind modules with a reason
	ABIVersion  int         `json:"abiVersion"`
	Description string      `json:"description,omitempty"`
	Author      string      `json:"author,omitempty"`
	License     string      `json:"license,omitempty"`
	Homepage    string      `json:"homepage,omitempty"`
	Permissions Permissions `json:"permissions"`
	Limits      Limits      `json:"limits"`
	Settings    Schema      `json:"settings"`
}

// Permissions are what the owner sees and approves before installing.
type Permissions struct {
	HTTP    HTTPPermission `json:"http"`
	Secrets []string       `json:"secrets,omitempty"`
}

// HTTPPermission lists the hosts a module may reach: exact names, "*.domain"
// for any subdomain, or "$setting:<key>" for the host of a URL or host-name
// setting of the instance.
type HTTPPermission struct {
	Hosts []string `json:"hosts,omitempty"`
}

// SettingHostPrefix marks a host pattern bound to an instance setting.
const SettingHostPrefix = "$setting:"

// Limits are the resources a module asks for; the host caps them.
type Limits struct {
	MemoryMiB     int `json:"memoryMiB,omitempty"`
	CallTimeoutMs int `json:"callTimeoutMs,omitempty"`
	Concurrency   int `json:"concurrency,omitempty"`
}

// ── settings schema ──────────────────────────────────────────────────────────

// Field types: a closed set.
const (
	TypeString     = "string"
	TypeSecret     = "secret"
	TypeBool       = "bool"
	TypeInt        = "int"
	TypeNumber     = "number"
	TypeEnum       = "enum"
	TypeDuration   = "duration"
	TypeListString = "list<string>"
	TypeListInt    = "list<int>"
	TypeMap        = "map<string,string>"
)

// String patterns: a fixed list.
const (
	PatternHostname     = "hostname"
	PatternURLHTTP      = "url-http"
	PatternPathAbsolute = "path-absolute"
	PatternIdentifier   = "identifier"
)

// Schema is a module's settings form as data.
type Schema struct {
	Fields  []Field  `json:"fields"`
	Groups  []Group  `json:"groups,omitempty"`
	Actions []Action `json:"actions,omitempty"`
}

// Field is one typed setting.
type Field struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Hint        string `json:"hint,omitempty"` // plain-text hint shown with validation errors
	Required    bool   `json:"required,omitempty"`
	Default     any    `json:"default,omitempty"`

	// string, list<string>
	MinLength *int   `json:"minLength,omitempty"`
	MaxLength *int   `json:"maxLength,omitempty"`
	Pattern   string `json:"pattern,omitempty"`

	// int, number, list<int>
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	Step     *float64 `json:"step,omitempty"`
	Decimals *int     `json:"decimals,omitempty"`

	// enum
	Options []Option `json:"options,omitempty"`

	// duration, as Go durations ("30s", "5m")
	MinDuration string `json:"minDuration,omitempty"`
	MaxDuration string `json:"maxDuration,omitempty"`

	// list<…>
	MaxItems int `json:"maxItems,omitempty"`

	// map<string,string>
	MaxEntries int       `json:"maxEntries,omitempty"`
	KeyRule    *TextRule `json:"keyRule,omitempty"`
	ValueRule  *TextRule `json:"valueRule,omitempty"`
}

// Option is one enum choice.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// TextRule constrains one side of a map entry.
type TextRule struct {
	MinLength *int   `json:"minLength,omitempty"`
	MaxLength *int   `json:"maxLength,omitempty"`
	Pattern   string `json:"pattern,omitempty"`
}

// Group is an ordered, titled set of field keys (one level, no nesting).
type Group struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Fields      []string `json:"fields"`
}

// Action is a button on the settings form that calls the module.
type Action struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
