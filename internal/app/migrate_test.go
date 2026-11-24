package app

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/events"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/importer"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/rules"
	"github.com/jrand0m/corsarr/internal/scheduler"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/testkit"
)

func loaded(t *testing.T, yml string) *config.Config {
	cfg, err := config.LoadFrom(config.Source{Kind: config.SourceFile, Path: "c.yml"},
		config.Inputs{ReadFile: func(string) ([]byte, error) { return []byte(yml), nil }}, plugin.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestEffectiveModeLetsTheConfigFileWin(t *testing.T) {
	ov := map[string]string{"kind:tv": "shadow", "kind:movie": "shadow"}
	cfg := loaded(t, "mediaKinds:\n  movie:\n    mode: active\n")
	if m := EffectiveMode(cfg, ov, domain.KindTV); m != domain.ModeShadow {
		t.Fatalf("override under a default: %s", m)
	}
	if m := EffectiveMode(cfg, ov, domain.KindMovie); m != domain.ModeActive {
		t.Fatalf("config file set the mode: %s", m)
	}
	if m := EffectiveMode(cfg, nil, domain.KindTV); m != domain.ModeActive {
		t.Fatalf("no override: %s", m)
	}
}

func TestRuleModesMerge(t *testing.T) {
	got := RuleModes(map[string]string{"a": "live"}, map[string]string{"rule:*": "dry-run", "rule:b": "off"}, []string{"a", "b", "c"})
	want := map[string]rules.Mode{"a": rules.ModeLive, "b": rules.ModeOff, "c": rules.ModeDryRun}
	for id, m := range want {
		if got[id] != m {
			t.Fatalf("%s: %s, want %s (%v)", id, got[id], m, got)
		}
	}
	if len(RuleModes(nil, nil, []string{"a"})) != 0 {
		t.Fatal("no overrides, no modes")
	}
}

func TestOverridesApplyWithoutRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := testkit.Store(t)
	bus := events.New()
	defer bus.Close()
	rec := bus.Subscribe(16, func(t domain.EventType) bool { return t == domain.EvMediaKindMode })
	defer rec.Close()
	cfg := testConfig(t.TempDir())
	cfg.RuleModes = map[string]string{importer.RuleRecyclePurge: "live"}
	a, err := New(ctx, Deps{Config: func() *config.Config { return cfg }, Store: st, Bus: bus, Health: health.New(bus, nil, nil),
		Jobs: scheduler.New(scheduler.Options{}), HTTP: httpFor{}, Plugins: registry(), UIClients: func() int { return 0 },
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.ModeOverrides().Set(ctx, store.ModeOverride{Scope: store.ScopeKind, Name: "tv", Mode: "shadow"})
	_ = st.ModeOverrides().Set(ctx, store.ModeOverride{Scope: store.ScopeRule, Name: "*", Mode: "off"})
	a.afterMigration(ctx)
	ev, err := rec.Next(ctx)
	if err != nil || ev.Kind != domain.KindTV || ev.Actor != "user" || ev.Data.(domain.ModeEventData).To != domain.ModeShadow {
		t.Fatalf("%v %+v", err, ev)
	}
	if a.mode(domain.KindTV) != domain.ModeShadow {
		t.Fatal("tv not shadow")
	}
	if m, _ := a.Rules.Mode(importer.RuleRecyclePurge); m != rules.ModeLive {
		t.Fatalf("config rule mode must win: %s", m)
	}
	if states := a.Migrate.Modes(); states[0].Kind != domain.KindTV || states[0].Mode != domain.ModeShadow {
		t.Fatalf("%+v", states)
	}
	if len(a.migrationDownloaders()) != 1 {
		t.Fatal("configured downloader not offered to the migration")
	}
}
