// Package indexers manages indexer instances from the web UI next to those
// from the config file: adding, editing, testing and enabling them, the
// definition catalogue, and an outbound Torznab endpoint so other tools can
// search through corsarr's indexers with its limits and health.
package indexers

import (
	"maps"
	"strconv"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

// Section is the config section of indexer instances.
var Section = plugin.KindIndexer.Section()

// RevisionKey is the setting carrying the definition catalogue revision to
// declarative instances, so reloading definitions rebuilds them.
const RevisionKey = "definitionRevision"

// Merge returns cfg with the UI-managed instances added to the indexers
// section (pure). An instance in the config file wins over a managed one
// with the same key. Declarative instances get the catalogue revision.
func Merge(cfg *config.Config, managed []store.ManagedIndexer, declarative func(pluginID string) bool, revision int) *config.Config {
	out := *cfg
	out.Instances = maps.Clone(cfg.Instances)
	if out.Instances == nil {
		out.Instances = map[string]map[domain.ObjectKey]config.Instance{}
	}
	section := map[domain.ObjectKey]config.Instance{}
	for _, m := range managed {
		section[m.Key] = Instance(m)
	}
	for k, inst := range cfg.Instances[Section] {
		section[k] = inst
	}
	for k, inst := range section {
		if declarative(inst.Type) {
			inst.Settings = maps.Clone(inst.Settings)
			if inst.Settings == nil {
				inst.Settings = plugin.Settings{}
			}
			inst.Settings[RevisionKey] = strconv.Itoa(revision)
			section[k] = inst
		}
	}
	out.Instances[Section] = section
	return &out
}

// Instance turns a managed row into a config instance (secrets resolved).
func Instance(m store.ManagedIndexer) config.Instance {
	s := plugin.Settings{}
	for k, v := range m.Settings {
		s[k] = v
	}
	for k, v := range m.Secrets {
		s[k] = v
	}
	return config.Instance{Section: Section, Key: m.Key, Type: m.PluginID, Name: m.Name, Enabled: m.Enabled, Settings: s}
}
