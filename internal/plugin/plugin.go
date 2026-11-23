// Package plugin is corsarr's extension system: one
// manifest model with a typed settings schema, a registry, and the extension
// point interfaces every integration implements. Built-in plugins register
// themselves from init() in their own packages.
package plugin

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// APIVersion is the extension-point API version built-ins are compiled against.
const APIVersion = 1

// Kind is an extension point.
type Kind string

const (
	KindIndexer     Kind = "indexer"
	KindSource      Kind = "source"
	KindDownloader  Kind = "downloader"
	KindMediaServer Kind = "mediaserver"
	KindNotifier    Kind = "notifier"
	KindMetadata    Kind = "metadata"
	KindArtwork     Kind = "artwork"
	KindChatbot     Kind = "chatbot"
)

// Section returns the config section that holds instances of this kind.
func (k Kind) Section() string {
	switch k {
	case KindIndexer:
		return "indexers"
	case KindSource:
		return "sources"
	case KindDownloader:
		return "downloadClients"
	case KindMediaServer:
		return "mediaServers"
	case KindNotifier:
		return "notifications"
	case KindChatbot:
		return "chatbots"
	default:
		return "metadata"
	}
}

// FieldType is a settings-schema field type; each maps to a UI field molecule.
type FieldType string

const (
	FieldText        FieldType = "text"
	FieldSecret      FieldType = "secret"
	FieldNumber      FieldType = "number"
	FieldBool        FieldType = "bool"
	FieldEnum        FieldType = "enum"
	FieldMultiselect FieldType = "multiselect"
	FieldDuration    FieldType = "duration"
	FieldSize        FieldType = "size"
	FieldURL         FieldType = "url"
	FieldPath        FieldType = "path"
	FieldRegex       FieldType = "regex"
	FieldList        FieldType = "list"
	FieldMap         FieldType = "map"
)

// Field is one typed setting. Key follows camelCase; it becomes the config
// key segment (file/args) and the upper-cased env segment.
type Field struct {
	Key      string    `json:"key"`
	Type     FieldType `json:"type"`
	Label    string    `json:"label"`
	Help     string    `json:"help,omitempty"`
	Required bool      `json:"required,omitempty"`
	Default  any       `json:"default,omitempty"`
	Options  []string  `json:"options,omitempty"` // enum / multiselect
	ShowIf   string    `json:"showIf,omitempty"`  // "field=value" conditional visibility
	Min      *float64  `json:"min,omitempty"`
	Max      *float64  `json:"max,omitempty"`
	// Secret marks list/map fields whose values are secret (secret-typed fields always are).
	Secret bool `json:"secret,omitempty"`
}

// SetupFlow is one of the generic interactive setup flows.
type SetupFlow string

const (
	FlowPinPairing  SetupFlow = "pinPairing"
	FlowCookiePaste SetupFlow = "cookiePaste"
	FlowLoginForm   SetupFlow = "loginForm"
	FlowOAuthDevice SetupFlow = "oauthDevice"
)

// Manifest describes a plugin; the settings schema drives config keys and the
// generated UI form, so a plugin needs no UI code.
type Manifest struct {
	ID           string      `json:"id"`
	Kind         Kind        `json:"kind"`
	Version      string      `json:"version"`
	APIVersion   int         `json:"apiVersion"`
	Name         string      `json:"name"`
	Delivery     string      `json:"delivery"` // builtin|declarative|external
	Capabilities []string    `json:"capabilities,omitempty"`
	Settings     []Field     `json:"settings"`
	SetupFlows   []SetupFlow `json:"setupFlows,omitempty"`
}

// Settings are the validated values for one plugin instance.
type Settings map[string]any

// String returns a string setting or "".
func (s Settings) String(key string) string { v, _ := s[key].(string); return v }

// Instance is one configured plugin instance (e.g. indexers.knaben).
type Instance struct {
	Key      domain.ObjectKey
	Name     string
	PluginID string
	Settings Settings
	Env      Env
}

// Env is what the host gives every plugin: the shared outbound HTTP client
// (timeouts, retries, rate limits, health) and a logger-like reporter.
type Env interface {
	HTTP() HTTPDoer
	Now() time.Time
}

// Factory builds a running plugin from an instance.
type Factory func(Instance) (any, error)

// Registration pairs a manifest with its factory.
type Registration struct {
	Manifest Manifest
	New      Factory
}

// Registry holds every available plugin.
type Registry struct {
	mu   sync.RWMutex
	byID map[string]Registration
}

// Default is the process-wide registry built-ins register into.
var Default = NewRegistry()

// NewRegistry returns an empty registry (tests use their own).
func NewRegistry() *Registry { return &Registry{byID: map[string]Registration{}} }

// Register adds a plugin; duplicate ids or a wrong API version are bugs.
func (r *Registry) Register(reg Registration) error {
	m := reg.Manifest
	if m.ID == "" || m.Kind == "" || reg.New == nil {
		return fmt.Errorf("plugin registration needs id, kind and factory")
	}
	if m.APIVersion != APIVersion {
		return fmt.Errorf("plugin %s targets API %d, host is %d", m.ID, m.APIVersion, APIVersion)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byID[m.ID]; dup {
		return fmt.Errorf("plugin %s registered twice", m.ID)
	}
	r.byID[m.ID] = reg
	return nil
}

// MustRegister is Register for init() of built-ins.
func (r *Registry) MustRegister(reg Registration) {
	if err := r.Register(reg); err != nil {
		panic(err)
	}
}

// Get finds a plugin by id.
func (r *Registry) Get(id string) (Registration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.byID[id]
	return reg, ok
}

// List returns manifests of one kind ("" = all), sorted by id.
func (r *Registry) List(kind Kind) []Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Manifest
	for _, reg := range r.byID {
		if kind == "" || reg.Manifest.Kind == kind {
			out = append(out, reg.Manifest)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Tester is implemented by every plugin: a cheap self-test for health and
// test-on-save.
type Tester interface {
	Test(ctx context.Context) error
}
