package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jrand0m/corsarr/internal/app"
	"github.com/jrand0m/corsarr/internal/catalogue"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/httpx"
	"github.com/jrand0m/corsarr/internal/modules"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/plugin/conformance"
	"github.com/jrand0m/corsarr/internal/wasm"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// Plugin module commands:
//
//	corsarr plugin list                              built-in plugins and installed modules
//	corsarr plugin test <file.wasm> [--settings=f]   verify a module and run its kind's conformance suite
//	corsarr plugin load <file.wasm> [--sha256=hex]   install a module into the plugins folder
//	corsarr plugin unload <id>                       deactivate a module (files are kept)
//	corsarr plugin rollback <id>                     make the previous version active again
//
// A running server picks up load, unload and rollback within seconds.

func init() {
	for _, f := range []string{"settings", "sha256"} {
		commandFlags[f] = true
	}
}

// moduleState is the process's module engine and manager, created on first
// use from the bootstrap part of the configuration.
type moduleState struct {
	eng *wasm.Engine
	mgr *modules.Manager
	dir string
	// provisioned are the plugins.install changes made at start, recorded
	// in History once it is open.
	provisioned []app.PluginChange
}

// registerModules registers installed modules before the configuration is
// validated (instances name modules as their type). A missing plugins folder
// means no modules.
func (c cli) registerModules() {
	if c.mods.mgr != nil {
		return
	}
	b, err := config.Peek(c.in)
	if err != nil {
		return // the full load reports it
	}
	if _, err := os.Stat(b.ModulesDir()); err != nil {
		c.mods.dir = b.ModulesDir()
		return
	}
	c.openModules(b)
}

func (c cli) openModules(b config.Bootstrap) error {
	dir := b.ModulesDir()
	eng, err := wasm.NewEngine(wasm.Options{Limits: limitsOf(b.Plugins), CacheDir: modules.Store{Dir: dir}.CacheDir(), Compiler: true})
	if err != nil {
		return err
	}
	c.mods.eng, c.mods.dir = eng, dir
	c.mods.mgr = modules.New(modules.Options{Dir: dir, Engine: eng, Registry: plugin.Default, DisabledBuiltins: b.Plugins.DisabledBuiltins})
	c.mods.mgr.Register()
	c.provision(b)
	return nil
}

// catalogueKeys are the keys a catalogue must be signed with.
var catalogueKeys = catalogue.Trusted

// provisionTimeout bounds the catalogue installs made at start.
const provisionTimeout = 60 * time.Second

// provision installs the plugins.install entries from the catalogue before
// the configuration is validated, so instances of those modules validate
// in the same start. Failures are logged; the start goes on.
func (c cli) provision(b config.Bootstrap) {
	if len(b.Plugins.Install) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), provisionTimeout)
	defer cancel()
	cfg := &config.Config{Plugins: b.Plugins}
	cfg.Data.Dir = b.DataDir
	client := httpx.New(httpx.Options{Version: version, Allow: b.EgressAllow, Transport: wasm.GuardedTransport()})
	admin := app.NewPluginAdmin(app.PluginAdminDeps{Modules: c.mods.mgr, Config: func() *config.Config { return cfg },
		HTTP: client, Keys: catalogueKeys(), Version: version})
	changes, errs := admin.Provision(ctx, b.Plugins.Install)
	c.mods.provisioned = changes
	for _, err := range errs {
		fmt.Fprintln(c.stderr, "corsarr: plugin module not installed:", err)
	}
}

// recordProvisioned writes the start's plugins.install changes to History.
func (c cli) recordProvisioned(bus interface {
	Publish(domain.Event) (domain.Event, error)
}) {
	if c.mods == nil {
		return
	}
	for _, ch := range c.mods.provisioned {
		_, _ = bus.Publish(domain.Event{Type: domain.EvPluginChanged, Actor: "system", Data: ch})
	}
	c.mods.provisioned = nil
}

// moduleStore is the module manager for the plugin admin; nil when modules
// are off.
func (c cli) moduleStore() app.Modules {
	if c.mods == nil || c.mods.mgr == nil {
		return nil
	}
	return c.mods.mgr
}

func limitsOf(p config.Plugins) wasm.Limits {
	return wasm.Limits{MemoryMiB: p.MemoryMiB, CallTimeout: p.CallTimeout, Concurrency: p.Concurrency,
		MaxModuleBytes: int64(p.MaxModuleMiB) << 20}
}

// ensureModules opens the manager, creating the plugins folder.
func (c cli) ensureModules() (*modules.Manager, error) {
	c.registerModules()
	if c.mods.mgr != nil {
		return c.mods.mgr, nil
	}
	b, err := config.Peek(c.in)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(b.ModulesDir(), 0o755); err != nil {
		return nil, fmt.Errorf("plugins folder: %w", err)
	}
	err = c.openModules(b)
	return c.mods.mgr, err
}

// prepareModules opens the module manager for the server before plugin
// instances are built: the plugins folder exists and module logs go to the
// server log.
func (c cli) prepareModules(log *slog.Logger) {
	mgr, err := c.ensureModules()
	if err != nil {
		log.Warn("plugin modules disabled", "err", err)
		return
	}
	c.mods.eng.SetLog(log)
	mgr.SetLog(log)
}

// runModules lets modules emit events, compiles them in the background and
// watches the folder for drop-ins and changes made by the CLI.
func (c cli) runModules(ctx context.Context, events wasm.EventSink) {
	if c.mods.mgr == nil {
		return
	}
	c.mods.eng.SetEvents(events)
	mgr := c.mods.mgr
	go func() {
		mgr.Sync(ctx)
		mgr.Warm()
		mgr.Run(ctx)
	}()
}

func (c cli) closeModules() {
	if c.mods.mgr != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c.mods.mgr.Close(ctx)
		_ = c.mods.eng.Close(ctx)
	}
}

var pluginCommands = map[string]func(c cli, ctx context.Context) int{
	"list":     func(c cli, _ context.Context) int { return c.pluginList() },
	"test":     func(c cli, ctx context.Context) int { return c.pluginTest(ctx) },
	"load":     func(c cli, ctx context.Context) int { return c.pluginLoad(ctx) },
	"unload":   func(c cli, _ context.Context) int { return c.pluginUnload() },
	"rollback": func(c cli, _ context.Context) int { return c.pluginRollback() },
}

func (c cli) plugin(ctx context.Context) int {
	if len(c.cmd) >= 2 {
		if h, ok := pluginCommands[c.cmd[1]]; ok {
			c.args = c.cmd[2:]
			return h(c, ctx)
		}
	}
	fmt.Fprintln(c.stderr, "usage: corsarr plugin list | test <file.wasm> [--settings=file.json] | load <file.wasm> [--sha256=hex] | unload <id> | rollback <id>")
	return 2
}

func (c cli) arg() (string, bool) {
	if len(c.args) != 1 {
		fmt.Fprintf(c.stderr, "corsarr plugin %s needs exactly one argument\n", c.cmd[1])
		return "", false
	}
	return c.args[0], true
}

func (c cli) fail(err error) int {
	fmt.Fprintln(c.stderr, "corsarr:", err)
	return 1
}

func (c cli) pluginList() int {
	c.registerModules()
	w := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tKIND\tVERSION\tDELIVERY\tSTATUS")
	for _, m := range plugin.Default.List("") {
		if m.Delivery != "module" {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\tok\n", m.ID, m.Kind, orDash(m.Version), orDash(m.Delivery))
		}
	}
	if c.mods.mgr != nil {
		for _, info := range c.mods.mgr.List() {
			fmt.Fprintf(w, "%s\t%s\t%s\tmodule\t%s\n", info.ID, orDash(info.Kind), orDash(info.Version), moduleStatus(info))
		}
	}
	_ = w.Flush()
	return 0
}

func moduleStatus(info modules.Info) string {
	if info.Error != "" {
		return string(info.Status) + ": " + info.Error
	}
	return string(info.Status)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// readModule reads a module file and its optional sidecar <name>.json.
func readModule(path string) (module, sidecar []byte, err error) {
	module, err = os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	sidecar, _ = os.ReadFile(strings.TrimSuffix(path, ".wasm") + ".json")
	return module, sidecar, nil
}

func (c cli) pluginLoad(ctx context.Context) int {
	path, ok := c.arg()
	if !ok {
		return 2
	}
	module, sidecar, err := readModule(path)
	if err != nil {
		return c.fail(err)
	}
	mgr, err := c.ensureModules()
	if err != nil {
		return c.fail(err)
	}
	info, err := mgr.Install(ctx, module, sidecar, strings.ToLower(c.flags["sha256"]))
	if err != nil {
		return c.fail(err)
	}
	fmt.Fprintf(c.stdout, "installed %s %s (%s) into %s\nsha256 %s\n", info.ID, info.Version, info.Kind, c.mods.dir, info.SHA256)
	printPermissions(c.stdout, info.Manifest)
	return 0
}

func printPermissions(w io.Writer, m abi.Manifest) {
	fmt.Fprintf(w, "may contact: %s\n", orDash(strings.Join(m.Permissions.HTTP.Hosts, ", ")))
	fmt.Fprintf(w, "may read secrets: %s\n", orDash(strings.Join(m.Permissions.Secrets, ", ")))
}

func (c cli) pluginUnload() int {
	id, ok := c.arg()
	if !ok {
		return 2
	}
	mgr, err := c.ensureModules()
	if err != nil {
		return c.fail(err)
	}
	if err := mgr.Unload(id); err != nil {
		return c.fail(err)
	}
	fmt.Fprintf(c.stdout, "unloaded %s (files kept; load it again or roll back)\n", id)
	return 0
}

func (c cli) pluginRollback() int {
	id, ok := c.arg()
	if !ok {
		return 2
	}
	mgr, err := c.ensureModules()
	if err != nil {
		return c.fail(err)
	}
	info, err := mgr.Rollback(id)
	if err != nil {
		return c.fail(err)
	}
	fmt.Fprintf(c.stdout, "%s is back on %s\n", info.ID, info.Version)
	return 0
}

// ── plugin test ──────────────────────────────────────────────────────────────

// conformanceAdd is the payload the live downloader checks add (paused).
var conformanceAdd = plugin.AddRequest{Payload: plugin.Payload{Kind: "magnet", Name: "corsarr-conformance",
	URL: "magnet:?xt=urn:btih:c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0&dn=corsarr-conformance"},
	Category: "corsarr-conformance", Paused: true}

func (c cli) pluginTest(ctx context.Context) int {
	path, ok := c.arg()
	if !ok {
		return 2
	}
	module, sidecar, err := readModule(path)
	if err != nil {
		return c.fail(err)
	}
	eng, err := wasm.NewEngine(wasm.Options{}) // the interpreter: no compile wait, same behaviour
	if err != nil {
		return c.fail(err)
	}
	defer func() { _ = eng.Close(ctx) }()
	var checks []conformance.Check
	start := time.Now()
	mod, err := eng.Load(ctx, module, sidecar)
	checks = append(checks, check("load: manifest, imports, exports, start, capabilities", time.Since(start), err))
	if err == nil {
		defer func() { _ = mod.Close(ctx) }()
		checks = append(checks, c.describe(mod)...)
		checks = append(checks, c.liveChecks(ctx, mod)...)
	}
	return report(c.stdout, checks)
}

func check(name string, took time.Duration, err error) conformance.Check {
	ch := conformance.Check{Name: name, Passed: err == nil, Took: took}
	if err != nil {
		ch.Problems = []string{err.Error()}
	}
	return ch
}

// startBudget is the longest a module instance may take to start.
const startBudget = 200 * time.Millisecond

func (c cli) describe(mod *wasm.Module) []conformance.Check {
	m := mod.Manifest
	fmt.Fprintf(c.stdout, "%s %s (%s), sha256 %s\n", m.ID, m.Version, mod.Kind(), mod.SHA256)
	printPermissions(c.stdout, m)
	fmt.Fprintf(c.stdout, "operations: %s\n", strings.Join(mod.Caps.Ops, ", "))
	var err error
	if mod.StartTime > startBudget {
		err = fmt.Errorf("started in %s; the budget is %s", mod.StartTime.Round(time.Millisecond), startBudget)
	}
	return []conformance.Check{check("start within "+startBudget.String(), mod.StartTime, err)}
}

func (c cli) liveChecks(ctx context.Context, mod *wasm.Module) []conformance.Check {
	file := c.flags["settings"]
	if file == "" {
		fmt.Fprintln(c.stdout, "live checks skipped: pass --settings=<file.json> with the settings of a test instance")
		return nil
	}
	settings, err := readSettings(file)
	if err != nil {
		return []conformance.Check{check("settings", 0, err)}
	}
	p, err := mod.NewPool(wasm.Binding{Key: "conformance", Settings: settings, HTTP: http.DefaultClient,
		Log: slog.New(slog.NewTextHandler(c.stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))})
	if err != nil {
		return []conformance.Check{check("instance", 0, err)}
	}
	defer p.Drain(ctx)
	spec, _ := wasm.Spec(mod.Kind())
	impl := spec.Adapter(wasm.NewHandle(p))
	return []conformance.Check{suite(mod.Kind(), impl)}
}

func readSettings(file string) (map[string]any, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var s map[string]any
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return s, nil
}

// suite runs the conformance suite of a kind against an adapter.
func suite(kind string, impl any) conformance.Check {
	switch d := impl.(type) {
	case plugin.Downloader:
		return conformance.Run(kind+" conformance", func(t conformance.T) { conformance.Downloader(t, d, conformanceAdd) })
	}
	return check(kind+" conformance", 0, errors.New("no conformance suite for this kind yet"))
}

func report(w io.Writer, checks []conformance.Check) int {
	failed := 0
	for _, ch := range checks {
		mark := "PASS"
		if !ch.Passed {
			mark, failed = "FAIL", failed+1
		}
		fmt.Fprintf(w, "%s  %s (%s)\n", mark, ch.Name, ch.Took.Round(time.Millisecond))
		for _, p := range ch.Problems {
			fmt.Fprintf(w, "      %s\n", p)
		}
	}
	if failed > 0 {
		fmt.Fprintf(w, "%d of %d checks failed\n", failed, len(checks))
		return 1
	}
	fmt.Fprintf(w, "all %d checks passed\n", len(checks))
	return 0
}
