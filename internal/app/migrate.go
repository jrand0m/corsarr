package app

import (
	"context"
	"net/http"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/migrate"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/rules"
	"github.com/jrand0m/corsarr/internal/store"
)

// Mode overrides: a mode a migration or the cutover panel chose, used when
// the config file does not set that mode itself.

// loadOverrides reads the overrides from the store into memory.
func (a *App) loadOverrides(ctx context.Context) error {
	list, err := a.d.Store.ModeOverrides().List(ctx)
	if err != nil {
		return err
	}
	ov := map[string]string{}
	for _, o := range list {
		ov[o.Scope+":"+o.Name] = o.Mode
	}
	a.ov.Store(&ov)
	return nil
}

func (a *App) overrides() map[string]string {
	if p := a.ov.Load(); p != nil {
		return *p
	}
	return nil
}

// EffectiveMode is a kind's mode: the config file's when it sets one, else
// an override, else the config default (pure).
func EffectiveMode(cfg *config.Config, ov map[string]string, k domain.MediaKind) domain.KindMode {
	if m, ok := ov[store.ScopeKind+":"+string(k)]; ok && !migrate.ConfigSetsKind(cfg, k) {
		return domain.KindMode(m)
	}
	return cfg.MediaKinds[k]
}

// RuleModes merges rule overrides under the config's rule modes (pure): a
// rule the config names keeps its mode, a rule-specific override beats the
// "*" override, and a rule with neither keeps its default.
func RuleModes(cfg map[string]string, ov map[string]string, ids []string) map[string]rules.Mode {
	out := map[string]rules.Mode{}
	for _, id := range ids {
		if m, ok := ov[store.ScopeRule+":*"]; ok {
			out[id] = rules.Mode(m)
		}
		if m, ok := ov[store.ScopeRule+":"+id]; ok {
			out[id] = rules.Mode(m)
		}
	}
	for id, m := range cfg {
		out[id] = rules.Mode(m)
	}
	return out
}

// ReloadOverrides re-reads the overrides and applies them now: rule modes
// change and each kind whose mode changed publishes mediakind.mode_changed.
func (a *App) ReloadOverrides(ctx context.Context) error {
	if err := a.loadOverrides(ctx); err != nil {
		return err
	}
	cfg := a.d.Config()
	a.applyRuleModes(cfg)
	a.publishModeChanges(cfg, "user")
	return nil
}

// afterMigration applies the new overrides and reloads the wanted indexes
// so adopted items are known without a restart.
func (a *App) afterMigration(ctx context.Context) {
	if err := a.Profiles.Load(ctx); err != nil {
		a.d.Log.Warn("reload custom formats and profiles", "err", err)
	}
	if err := a.ReloadOverrides(ctx); err != nil {
		a.d.Log.Warn("reload mode overrides", "err", err)
	}
	if err := a.Automation.Rebuild(ctx); err != nil {
		a.d.Log.Warn("rebuild movie wanted index", "err", err)
	}
	if err := a.TVAuto.Rebuild(ctx); err != nil {
		a.d.Log.Warn("rebuild tv wanted index", "err", err)
	}
}

// newMigrate builds the migration service on the shared outbound client.
func (a *App) newMigrate() *migrate.Service {
	doer, ok := a.d.HTTP.(migrate.Doer)
	if !ok {
		doer = &http.Client{Timeout: 60 * time.Second}
	}
	return migrate.New(migrate.Deps{Store: a.d.Store, HTTP: doer, Config: a.d.Config, Downloaders: a.migrationDownloaders,
		Publish: func(ev domain.Event) { _, _ = a.d.Bus.Publish(ev) }, AfterApply: a.afterMigration, Mode: a.mode, Log: a.d.Log})
}

func (a *App) migrationDownloaders() []migrate.Downloader {
	var out []migrate.Downloader
	for _, n := range usable[plugin.Downloader](a, plugin.KindDownloader) {
		out = append(out, migrate.Downloader{Key: string(n.Key), List: n.Impl})
	}
	return out
}
