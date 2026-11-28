package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/app"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/events"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/httpx"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/preflight"
	"github.com/jrand0m/corsarr/internal/scheduler"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/wasm"
	"github.com/jrand0m/corsarr/internal/web"
)

// ShutdownGrace bounds graceful shutdown.
const ShutdownGrace = 10 * time.Second

// HistoryRetention is the default History period.
const HistoryRetention = 365 * 24 * time.Hour

// kernel holds the running parts; new modules are wired in here.
type kernel struct {
	cfg    atomic.Pointer[config.Config]
	log    *slog.Logger
	store  *store.Store
	bus    *events.Bus
	health *health.Registry
	jobs   *scheduler.Scheduler
	http   *httpx.Client // shared outbound client for plugins (Env.HTTP)
	hub    *events.Hub   // SSE stream; its client count is the "UI connected" signal
	app    *app.App      // the media pipeline (instances, movies, queue, import, automation, notify)
}

var configRef = domain.IntegrationRef{Type: "config", Key: "source"}

// serve runs until ctx ends; ready (tests) receives the listen address.
func (c cli) serve(ctx context.Context, ready func(addr string)) int {
	cfg, ok := c.load()
	if !ok {
		return 2 // invalid bootstrap config refuses start
	}
	applyRuntimeDefaults(c.in.Environ)
	log := newLogger(c, cfg.Log.Level)
	if err := c.preflight(cfg, log); err != nil {
		fmt.Fprintln(c.stderr, "corsarr:", err)
		return 1
	}
	c.prepareModules(log)
	defer c.closeModules()
	k, err := startKernel(ctx, cfg, log, c.moduleStore())
	if err != nil {
		fmt.Fprintln(c.stderr, "corsarr:", err)
		return 1
	}
	defer k.close()
	c.recordProvisioned(k.bus)
	c.runModules(ctx, k.bus)
	srv, ln, err := k.listen(ctx, cfg)
	if err != nil {
		k.log.Error("cannot start HTTP server", "err", err)
		return 1
	}
	k.log.Info("corsarr listening", "addr", ln.Addr().String(), "version", version,
		"configSource", cfg.Source.Kind, "configFile", cfg.Source.Path, "dataDir", cfg.Data.Dir)
	go config.WatchFile(ctx, cfg, c.in, plugin.Default, config.WatchInterval, k.onConfigChange)
	if ready != nil {
		ready(ln.Addr().String())
	}
	return k.wait(ctx, srv, ln)
}

// preflight refuses root without --allow-root, applies the configured umask
// and proves the data and temp dirs are writable before anything is opened.
func (c cli) preflight(cfg *config.Config, log *slog.Logger) error {
	warning, err := preflight.Run(preflight.Options{
		ID: preflight.Current(), DataDir: cfg.Data.Dir, TempDir: os.TempDir(), Umask: cfg.Data.Umask,
		AllowRoot: c.has("allow-root"), Container: preflight.InContainer(preflight.FileExists, os.Getenv),
	})
	if warning != "" {
		log.Warn(warning)
	}
	return err
}

func newLogger(c cli, level string) *slog.Logger {
	var lv slog.Level
	_ = lv.UnmarshalText([]byte(level)) // validated by config
	return slog.New(slog.NewJSONHandler(c.stderr, &slog.HandlerOptions{Level: lv}))
}

// applyRuntimeDefaults sets GOMEMLIMIT=96MiB and GOGC=50 unless the
// environment chose (the runtime already honours the env vars).
func applyRuntimeDefaults(environ []string) {
	if !hasEnv(environ, "GOMEMLIMIT") {
		debug.SetMemoryLimit(96 << 20)
	}
	if !hasEnv(environ, "GOGC") {
		debug.SetGCPercent(50)
	}
}

func hasEnv(environ []string, name string) bool {
	for _, kv := range environ {
		if len(kv) > len(name) && kv[:len(name)+1] == name+"=" {
			return true
		}
	}
	return false
}

func startKernel(ctx context.Context, cfg *config.Config, log *slog.Logger, mods app.Modules) (*kernel, error) {
	st, err := store.Open(ctx, cfg.Data.Dir)
	if err != nil {
		return nil, err
	}
	k := &kernel{log: log, store: st}
	k.cfg.Store(cfg)
	for _, w := range cfg.Warnings {
		log.Warn(w)
	}
	k.bus = events.New(events.WithHistory(st.History(), func(err error) { log.Error("history write failed", "err", err) }))
	k.health = health.New(k.bus, st.Health(), nil)
	if list, err := st.Health().List(ctx); err == nil {
		k.health.Load(list)
	}
	k.http = newOutbound(ctx, cfg, st, k.health, log)
	k.health.OK(configRef)
	k.jobs = scheduler.New(scheduler.Options{Store: st.Jobs(), Events: k.bus,
		OnError: func(job string, err error) { log.Warn("job failed", "job", job, "err", err) }})
	if err := k.registerJobs(ctx); err != nil {
		k.close()
		return nil, err
	}
	k.hub = events.NewHub(k.bus, 15*time.Second)
	if k.app, err = app.New(ctx, app.Deps{Config: k.cfg.Load, Store: st, Bus: k.bus, Health: k.health, Jobs: k.jobs,
		HTTP: k.http, Plugins: plugin.Default, UIClients: k.hub.Clients, Log: log, Modules: mods, Version: version}); err != nil {
		k.close()
		return nil, err
	}
	k.jobs.Start(ctx)
	if err := k.app.Start(ctx); err != nil {
		k.close()
		return nil, err
	}
	return k, nil
}

// newOutbound builds the shared outbound client and loads the rate limits
// it learned before the last restart.
func newOutbound(ctx context.Context, cfg *config.Config, st *store.Store, rep httpx.Reporter, log *slog.Logger) *httpx.Client {
	rl := cfg.Server.RateLearning
	c := httpx.New(httpx.Options{Version: version, Reporter: rep, Allow: cfg.Server.EgressAllow,
		Learning: httpx.Learning{Enabled: rl.Enabled, Headroom: rl.Headroom, ProbeEvery: rl.ProbeEvery, Store: st.RateLimits()},
		// Checks every dialled address again, so a name that resolves to a private address cannot
		// reach it unless a plugin setting names that host.
		Transport: wasm.GuardedTransport()})
	if err := c.RestoreLimits(ctx); err != nil {
		log.Warn("learned rate limits not loaded; they are learned again", "err", err)
	}
	return c
}

// registerJobs adds the kernel's own idempotent housekeeping jobs.
func (k *kernel) registerJobs(ctx context.Context) error {
	jobs := []scheduler.Job{
		{Name: "sessions.prune", Interval: time.Hour, Run: func(ctx context.Context) error {
			return k.store.Auth().PruneSessions(ctx, time.Now())
		}},
		{Name: "history.prune", Interval: 24 * time.Hour, Run: func(ctx context.Context) error {
			return k.store.History().Prune(ctx, time.Now().Add(-HistoryRetention))
		}},
	}
	for _, j := range jobs {
		if err := k.jobs.Register(ctx, j); err != nil {
			return err
		}
	}
	return nil
}

func (k *kernel) listen(ctx context.Context, cfg *config.Config) (*http.Server, net.Listener, error) {
	if err := api.EnsureAPIKey(ctx, k.store.Auth()); err != nil {
		return nil, nil, err
	}
	a, err := api.New(ctx, api.Deps{
		Version: version, Started: time.Now(), BasePath: cfg.Server.BasePath, AuthMode: cfg.Auth.Mode,
		Auth: k.store.Auth(), History: k.store.History(), Jobs: k.jobs, Health: k.health, Plugins: plugin.Default,
		Config: k.cfg.Load, Events: k.hub, UI: web.Assets(),
		DBSize: k.store.Size, Ready: k.ready, Log: k.log, Mounts: append(k.app.Mounts(), k.mountRateLimits),
	})
	if err != nil {
		return nil, nil, err
	}
	ln, err := net.Listen("tcp", cfg.Server.Address)
	if err != nil {
		return nil, nil, err
	}
	srv := &http.Server{Handler: a.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	return srv, ln, nil
}

// mountRateLimits serves what the outbound client learned about each
// service's rate limit, read-only.
func (k *kernel) mountRateLimits(m api.Mounter) {
	m.Handle("GET /api/v1/system/ratelimits", func(w http.ResponseWriter, _ *http.Request) {
		api.WriteJSON(w, http.StatusOK, map[string]any{"enabled": k.http.LearningEnabled(), "items": k.http.RateLimits()})
	})
}

// ready is the readiness probe: the database answers.
func (k *kernel) ready(ctx context.Context) error {
	return k.store.R().QueryRowContext(ctx, "SELECT 1").Scan(new(int))
}

// wait serves until ctx ends, then shuts down within ShutdownGrace.
func (k *kernel) wait(ctx context.Context, srv *http.Server, ln net.Listener) int {
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		k.log.Error("HTTP server stopped", "err", err)
		return 1
	case <-ctx.Done():
	}
	k.log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), ShutdownGrace)
	defer cancel()
	err := errors.Join(srv.Shutdown(sctx), k.jobs.Stop(sctx))
	if err != nil {
		k.log.Warn("unclean shutdown", "err", err)
	}
	return 0
}

func (k *kernel) close() {
	if k.app != nil {
		k.app.Close()
	}
	if k.jobs != nil {
		_ = k.jobs.Stop(context.Background())
	}
	if k.bus != nil {
		k.bus.Close()
	}
	if err := k.store.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
		k.log.Warn("database close", "err", err)
	}
}

// onConfigChange handles a watched file: live sections are swapped in for
// the API (the config apply pipeline consumes them), bootstrap changes ask for
// a restart, invalid files keep the last good config and fail the health.
func (k *kernel) onConfigChange(ch config.Change) {
	switch {
	case ch.Missing:
		k.health.Fail(configRef, health.ClassConfig, "config file missing; keeping the current configuration")
	case ch.Err != nil:
		k.health.Fail(configRef, health.ClassConfig, ch.Err.Error())
	default:
		k.cfg.Store(ch.Config)
		k.app.OnConfig(ch.Config)
		k.health.OK(configRef)
		k.log.Info("config file changed", "live", ch.Live, "bootstrap", ch.Bootstrap)
		if len(ch.Bootstrap) > 0 {
			k.log.Warn("bootstrap keys changed; restart corsarr to apply them", "sections", ch.Bootstrap)
		}
	}
}
