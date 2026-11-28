package app

import (
	"github.com/jrand0m/corsarr/internal/catalogue"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/mcp"
	"github.com/jrand0m/corsarr/internal/modules"
)

// mcpPlugins answers list_plugins from the registry, the module store, the
// config and the cached catalogue.
type mcpPlugins struct{ a *App }

func (p mcpPlugins) PluginList() []mcp.PluginInfo {
	cfg := p.a.d.Config()
	mods := map[string]modules.Info{}
	if list, err := p.a.Plugins.List(); err == nil {
		for _, m := range list {
			mods[m.ID] = m.Info
		}
	}
	offers := p.a.Plugins.CachedOffers()
	out := []mcp.PluginInfo{}
	seen := map[string]bool{}
	for _, m := range p.a.d.Plugins.List("") {
		info := mcp.PluginInfo{ID: m.ID, Name: m.Name, Kind: string(m.Kind), Version: m.Version, Delivery: m.Delivery}
		if mod, ok := mods[m.ID]; ok {
			withModule(&info, mod)
		}
		out = append(out, finish(info, cfg, offers))
		seen[m.ID] = true
	}
	for id, mod := range mods { // modules not registered (failed, unloaded)
		if !seen[id] {
			info := mcp.PluginInfo{ID: id, Name: mod.Manifest.Name, Kind: mod.Kind, Version: mod.Version, Delivery: "module"}
			withModule(&info, mod)
			out = append(out, finish(info, cfg, offers))
		}
	}
	return out
}

func withModule(info *mcp.PluginInfo, mod modules.Info) {
	info.Delivery, info.Status, info.Error = "module", string(mod.Status), mod.Error
	info.Hosts, info.Secrets = mod.Manifest.Permissions.HTTP.Hosts, mod.Manifest.Permissions.Secrets
}

func finish(info mcp.PluginInfo, cfg *config.Config, offers map[string]catalogue.Offer) mcp.PluginInfo {
	info.Instances = []string{}
	for _, r := range usedBy(cfg, info.ID) {
		info.Instances = append(info.Instances, r.Section+"."+r.Key)
	}
	if o, ok := offers[info.ID]; ok && (o.State == catalogue.StateUpdate || o.State == catalogue.StateNeedsApproval) {
		info.Update = o.Entry.Version
	}
	return info
}
