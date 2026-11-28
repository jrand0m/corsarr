package app

import (
	"context"
	"time"

	"github.com/jrand0m/corsarr/internal/catalogue"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// autoUpdateDelay is how long after start the first auto-update runs.
const autoUpdateDelay = 2 * time.Minute

func (a *App) newPluginAdmin() *PluginAdmin {
	keys := a.d.CatalogueKeys
	if keys == nil {
		keys = catalogue.Trusted()
	}
	d := PluginAdminDeps{Modules: a.d.Modules, Config: a.d.Config, Bus: a.d.Bus, Keys: keys, Version: a.d.Version, Log: a.d.Log,
		Reload: func(id string) { a.Instances.Reload(a.d.Config(), id) }}
	if a.d.HTTP != nil {
		d.HTTP = a.d.HTTP.For(domain.IntegrationRef{Type: "plugin", Key: "catalogue"})
		d.Actions = func(id string) plugin.HTTPDoer { return a.d.HTTP.For(domain.IntegrationRef{Type: "plugin", Key: domain.ObjectKey(id)}) }
	}
	return NewPluginAdmin(d)
}

// autoUpdateSoon runs the plugin auto-update once shortly after start.
func (a *App) autoUpdateSoon(ctx context.Context) {
	t := time.NewTimer(autoUpdateDelay)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
		if err := a.Plugins.AutoUpdate(ctx); err != nil {
			a.d.Log.Warn("plugin auto-update", "err", err)
		}
	}
}
