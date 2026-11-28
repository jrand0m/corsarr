package mcp

import (
	"context"
	"fmt"
)

// Plugins lists built-in plugins and installed plugin modules (read-only:
// modules are never installed through MCP).
type Plugins interface {
	PluginList() []PluginInfo
}

// PluginInfo is one plugin as list_plugins shows it.
type PluginInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Version  string `json:"version"`
	Delivery string `json:"delivery"`         // builtin | declarative | module | …
	Status   string `json:"status,omitempty"` // modules: active | failed | unloaded | uninstalled
	Error    string `json:"error,omitempty"`
	// Hosts and Secrets are the permissions a module asked for.
	Hosts     []string `json:"hosts,omitempty"`
	Secrets   []string `json:"secrets,omitempty"`
	Instances []string `json:"instances"` // configured instances (section.key)
	// Update is the newer version the cached catalogue offers, if any.
	Update string `json:"update,omitempty"`
}

// PluginsOut is list_plugins.
type PluginsOut struct {
	Plugins []PluginInfo `json:"plugins"`
}

func (s *Service) pluginTools() []Tool {
	if s.d.Plugins == nil {
		return nil
	}
	return []Tool{{Name: "list_plugins", Title: "List plugins",
		Description: "Built-in plugins and installed plugin modules with kind, version, status, the hosts and secrets a module may use, the instances using each and a newer catalogue version when one is known. Read-only.",
		Input:       object(map[string]schema{}), Run: bind(s.listPlugins)}}
}

func (s *Service) listPlugins(_ context.Context, _ struct{}) (Result, error) {
	list := s.d.Plugins.PluginList()
	if list == nil {
		list = []PluginInfo{}
	}
	return Result{Text: fmt.Sprintf("%d plugins", len(list)), Data: PluginsOut{Plugins: list}}, nil
}
