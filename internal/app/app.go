// Package app wires the media pipeline onto the kernel: plugin instances,
// movies, queue, importer, automation, notify and the rule runner, their
// event subscriptions, jobs and API mounts. cmd/corsarr's kernel and the
// end-to-end test build it the same way.
package app

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jrand0m/corsarr/internal/api"
	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/catalogue"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/events"
	"github.com/jrand0m/corsarr/internal/health"
	"github.com/jrand0m/corsarr/internal/imagecache"
	"github.com/jrand0m/corsarr/internal/importer"
	"github.com/jrand0m/corsarr/internal/indexers"
	"github.com/jrand0m/corsarr/internal/instances"
	"github.com/jrand0m/corsarr/internal/legacy"
	"github.com/jrand0m/corsarr/internal/mcp"
	"github.com/jrand0m/corsarr/internal/migrate"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/music"
	"github.com/jrand0m/corsarr/internal/musicauto"
	"github.com/jrand0m/corsarr/internal/notify"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/profiles"
	"github.com/jrand0m/corsarr/internal/queue"
	"github.com/jrand0m/corsarr/internal/rootfolders"
	"github.com/jrand0m/corsarr/internal/rules"
	"github.com/jrand0m/corsarr/internal/scheduler"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/tv"
	"github.com/jrand0m/corsarr/internal/tvauto"
)

// Job names owned by the app wiring.
const (
	TestJob  = "instances.test"
	PurgeJob = "rules.recyclepurge"
)

// Options tune timings (tests shorten them); zero values take the defaults.
type Options struct {
	FastPoll     time.Duration
	SlowPoll     time.Duration
	MergeWindow  time.Duration
	TestInterval time.Duration
	PurgeMode    rules.Mode // recycle purge; destructive rules ship dry-run
}

// Deps are the kernel parts the pipeline runs on.
type Deps struct {
	Config    func() *config.Config
	Store     *store.Store
	Bus       *events.Bus
	Health    *health.Registry
	Jobs      *scheduler.Scheduler
	HTTP      instances.HTTPFor
	Plugins   *plugin.Registry
	UIClients func() int
	Log       *slog.Logger
	Options   Options
	// Modules is the plugin module store; nil when modules are off.
	Modules Modules
	// CatalogueKeys verify the plugin catalogue (the build's trusted keys
	// when nil).
	CatalogueKeys []catalogue.Key
	// Version is the running corsarr version.
	Version string
}

// App is the running pipeline.
type App struct {
	d          Deps
	Instances  *instances.Manager
	Rules      *rules.Runner
	Movies     *movies.Service
	Queue      *queue.Service
	Importer   *importer.Service
	Automation *automation.Service
	Notify     *notify.Service
	Roots      *rootfolders.Checker
	TV         *tv.Service
	TVAuto     *tvauto.Service
	// IndexerAdmin manages indexers added from the UI and the Torznab endpoint.
	IndexerAdmin *indexers.Service
	Migrate      *migrate.Service
	Music        *music.Service
	MusicAuto    *musicauto.Service
	// Legacy is the compatibility API for third-party tools.
	Legacy *legacy.Service
	// MCP is the endpoint for AI assistants.
	MCP *mcp.Service
	// Profiles is the one writer of custom formats, custom profiles and the
	// decision catalog.
	Profiles *profiles.Service
	// Images is the artwork cache.
	Images *imagecache.Cache
	// Plugins installs and manages plugin modules from the catalogue.
	Plugins *PluginAdmin
	wg      sync.WaitGroup
	cancel  context.CancelFunc
	ctx     context.Context // the running pipeline's context (set by Start)
	modesMu sync.Mutex
	modes   map[domain.MediaKind]domain.KindMode // last applied, for mode_changed
	ov      atomic.Pointer[map[string]string]    // mode overrides (kind:<k>, rule:<id>)
}

// New builds every module and registers the jobs; call Start after the
// scheduler has started.
func New(ctx context.Context, d Deps) (*App, error) {
	if d.Options.TestInterval <= 0 {
		d.Options.TestInterval = 10 * time.Minute
	}
	if d.Options.PurgeMode == "" {
		d.Options.PurgeMode = rules.ModeDryRun
	}
	a := &App{d: d, Instances: instances.New(d.Plugins, d.HTTP, d.Health, d.Log)}
	if err := a.loadOverrides(ctx); err != nil {
		return nil, err
	}
	a.modes = modesWith(d.Config(), a.overrides())
	if err := a.newIndexerAdmin(ctx); err != nil {
		return nil, err
	}
	if err := a.newProfiles(ctx); err != nil {
		return nil, err
	}
	a.applyInstances(d.Config())
	a.build()
	if err := a.Importer.RegisterRules(a.Rules, d.Options.PurgeMode); err != nil {
		return nil, err
	}
	a.applyRuleModes(d.Config())
	return a, a.registerJobs(ctx)
}

func (a *App) build() {
	st, d, modes := a.d.Store, a.d, domain.Modes(a.mode)
	a.Images = a.newImages()
	a.Rules = rules.New(st.Rules(), d.Bus, nil)
	a.Roots = rootfolders.New(rootfolders.Deps{Roots: a.roots, Repo: st.RootFolders(), Health: d.Health, Log: d.Log})
	a.Movies = movies.New(movies.Deps{Repo: st.Movies(), Pub: d.Bus, Metadata: a.metadata, Cache: st.Metadata(), Folders: folders{a}, RootFolders: a.movieRoots,
		Naming: a.namingOf(domain.KindMovie), Modes: modes, Art: a.art, Log: d.Log})
	a.Queue = queue.New(queue.Deps{Repo: st.Queue(), Plugins: plugins{a}, Pub: d.Bus, Jobs: d.Jobs, UIClients: d.UIClients,
		ItemTitles: a.itemTitles, Payload: decide.PayloadPolicy{}, Modes: modes, Clients: a.downloadClients, TestClient: a.testDownloader,
		FastPoll: d.Options.FastPoll, SlowPoll: d.Options.SlowPoll, Log: d.Log})
	a.TV = a.newTV()
	a.Music = a.newMusic()
	a.Importer = importer.New(importer.Deps{Queue: a.Queue, Movies: a.Movies, TV: a.TV, Music: a.Music, Library: st.Library(), Rules: a.Rules,
		RuleStore: st.Rules(), Pub: d.Bus, DataDir: st.Dir(), NoSpace: a.recheckRoots, Files: a.Queue.PayloadFiles, Naming: a.naming, Modes: modes, Log: d.Log})
	a.Automation = automation.New(automation.Deps{Movies: a.Movies, Files: st.Library(), Queue: a.Queue, Blocklist: st.Blocklist(),
		Cursors: st.FeedCursors(), Searches: st.Searches(), Recent: st.RecentReleases(), Indexers: a.indexers, Pub: d.Bus, Modes: modes, Log: d.Log})
	a.TVAuto = a.newTVAuto()
	a.Migrate = a.newMigrate()
	a.MusicAuto = a.newMusicAuto()
	a.Legacy = a.newLegacy()
	a.Plugins = a.newPluginAdmin()
	a.MCP = a.newMCP()
	a.Notify = notify.New(notify.Deps{Outbox: st.Outbox(), Refreshes: st.Refreshes(), Connections: a.connections, Health: d.Health, Pub: d.Bus,
		MediaServers: a.mediaServers, MergeWindow: d.Options.MergeWindow, Modes: modes, Log: d.Log})
}

// mode reads a kind's mode from the current config and overrides on every
// call, so a config change or a cutover applies without a restart.
func (a *App) mode(k domain.MediaKind) domain.KindMode {
	return EffectiveMode(a.d.Config(), a.overrides(), k)
}

var allKinds = []domain.MediaKind{domain.KindTV, domain.KindMovie, domain.KindMusic, domain.KindBook}

func modesOf(cfg *config.Config) map[domain.MediaKind]domain.KindMode { return modesWith(cfg, nil) }

// modesWith normalises every kind's effective mode under the overrides.
func modesWith(cfg *config.Config, ov map[string]string) map[domain.MediaKind]domain.KindMode {
	look := domain.Modes(func(k domain.MediaKind) domain.KindMode { return EffectiveMode(cfg, ov, k) })
	out := map[domain.MediaKind]domain.KindMode{}
	for _, k := range allKinds {
		out[k] = look.Of(k)
	}
	return out
}

// ModeChanges lists the kinds whose mode differs between two normalised
// snapshots (pure).
func ModeChanges(from, to map[domain.MediaKind]domain.KindMode) []domain.ModeEventData {
	var out []domain.ModeEventData
	for _, k := range allKinds {
		if was, now := from[k], to[k]; was != now {
			out = append(out, domain.ModeEventData{Kind: k, From: from[k], To: to[k]})
		}
	}
	return out
}

func (a *App) registerJobs(ctx context.Context) error {
	slow := a.d.Options.SlowPoll
	if slow <= 0 {
		slow = queue.SlowPoll
	}
	jobs := []scheduler.Job{
		{Name: queue.PollJob, Interval: slow, Run: a.Queue.Poll},
		{Name: queue.SweepJob, Interval: queue.SweepInterval, Run: a.Queue.Sweep},
		{Name: automation.RSSJob, Interval: automation.RSSInterval, Heavy: true, Run: a.Automation.SyncRSS},
		{Name: movies.RefreshJob, Interval: movies.RefreshInterval, Heavy: true, Run: a.Movies.Refresh},
		{Name: automation.BacklogJob, Interval: automation.BacklogInterval, Run: a.Automation.Backlog},
		{Name: TestJob, Interval: a.d.Options.TestInterval, Run: a.Instances.TestAll},
		{Name: rootfolders.CheckJob, Interval: rootfolders.CheckInterval, Run: a.Roots.Check},
		{Name: imagecache.DegradeJob, Interval: imagecache.DegradeInterval, Heavy: true, Run: a.Images.Degrade},
		{Name: AutoUpdateJob, Interval: 24 * time.Hour, Run: a.Plugins.AutoUpdate},
		{Name: PurgeJob, Interval: 24 * time.Hour, Run: func(ctx context.Context) error {
			_, err := a.Rules.Run(ctx, importer.RuleRecyclePurge)
			return err
		}},
	}
	for _, j := range append(append(jobs, a.tvJobs()...), a.musicJobs()...) {
		if err := a.d.Jobs.Register(ctx, j); err != nil {
			return err
		}
	}
	return nil
}

// Start loads the wanted index, subscribes the modules to the bus, arms the
// availability timer and the notification loop, and reconciles the queue.
func (a *App) Start(ctx context.Context) error {
	ctx, a.cancel = context.WithCancel(ctx)
	a.modesMu.Lock()
	a.ctx = ctx
	a.modesMu.Unlock()
	if err := a.Automation.Rebuild(ctx); err != nil {
		return err
	}
	if err := a.Roots.Check(ctx); err != nil {
		return err
	}
	a.subscribe(ctx)
	a.Movies.Start(ctx)
	if err := a.startTV(ctx); err != nil {
		return err
	}
	if err := a.startMusic(ctx); err != nil {
		return err
	}
	a.goRun(func() { a.Notify.Run(ctx) })
	a.goRun(func() { a.MCP.Run(ctx) })
	a.goRun(func() { _ = a.Instances.TestAll(ctx) })
	a.goRun(func() { a.autoUpdateSoon(ctx) })
	a.Queue.Adjust(ctx)
	return a.d.Jobs.RunNow(queue.SweepJob) // reconciliation at start
}

func (a *App) goRun(fn func()) {
	a.wg.Add(1)
	go func() { defer a.wg.Done(); fn() }()
}

// Close stops subscriptions and loops and waits for them.
func (a *App) Close() {
	if a.cancel != nil {
		a.cancel()
	}
	a.Movies.Stop()
	a.TV.Stop()
	a.Music.Stop()
	a.wg.Wait()
	a.Importer.Wait()
	a.Automation.Wait()
}

// applyRuleModes hands the configured rule modes to the runner (validated by
// the config schema); ids that name no rule are logged.
func (a *App) applyRuleModes(cfg *config.Config) {
	if unknown := a.Rules.SetModes(RuleModes(cfg.RuleModes, a.overrides(), a.Rules.IDs())); len(unknown) > 0 {
		a.d.Log.Warn("automation.rules names unknown rules", "ids", unknown)
	}
}

// recheckRoots re-checks the root folders now: after a config change or when an
// import found one short of space.
func (a *App) recheckRoots(string) {
	if err := a.d.Jobs.RunNow(rootfolders.CheckJob); err != nil {
		a.d.Log.Warn("root folder check", "err", err)
	}
}

// OnConfig rebuilds the plugin instances from a changed config and publishes
// mediakind.mode_changed for each kind whose mode changed.
func (a *App) OnConfig(cfg *config.Config) {
	a.applyInstances(cfg)
	a.applyRuleModes(cfg)
	a.Profiles.OnConfig()
	a.recheckRoots("")
	a.publishModeChanges(cfg, "config")
}

// publishModeChanges publishes mediakind.mode_changed for each kind whose
// effective mode changed and imports what waited for a kind now active.
func (a *App) publishModeChanges(cfg *config.Config, actor string) {
	next := modesWith(cfg, a.overrides())
	a.modesMu.Lock()
	changes, ctx := ModeChanges(a.modes, next), a.ctx
	a.modes = next
	a.modesMu.Unlock()
	for _, c := range changes {
		_, _ = a.d.Bus.Publish(domain.Event{Type: domain.EvMediaKindMode, Actor: actor, Kind: c.Kind, Data: c})
		if c.To == domain.ModeActive && ctx != nil && ctx.Err() == nil {
			a.goRun(func() { a.importWaiting(ctx, c.Kind) })
		}
	}
}

// importWaiting imports the completed downloads that waited while the kind
// was in shadow or disabled mode.
func (a *App) importWaiting(ctx context.Context, k domain.MediaKind) {
	list, err := a.d.Store.Queue().Downloads(ctx, domain.StateCompleted)
	if err != nil {
		a.d.Log.Warn("list waiting imports", "err", err)
		return
	}
	for _, d := range list {
		if d.Kind == k {
			a.Importer.Enqueue(ctx, d.ID)
		}
	}
}

// Mounts are the module API routes.
func (a *App) Mounts() []func(api.Mounter) {
	return []func(api.Mounter){a.Movies.Mount, a.Queue.Mount, a.Automation.Mount, a.Rules.Mount,
		a.Roots.Mount, a.Notify.Mount, a.TV.Mount, a.TVAuto.Mount, a.IndexerAdmin.Mount, a.Migrate.Mount,
		a.Music.Mount, a.MusicAuto.Mount, a.Legacy.Mount, a.MCP.Mount, a.Images.Mount, a.Profiles.Mount, a.Plugins.Mount}
}

func typeIn(types ...domain.EventType) func(domain.EventType) bool {
	set := map[domain.EventType]bool{}
	for _, t := range types {
		set[t] = true
	}
	return func(t domain.EventType) bool { return set[t] }
}

// subscribe gives each module one ordered event stream.
func (a *App) subscribe(ctx context.Context) {
	a.listen(ctx, typeIn(domain.EvPayloadRejected), a.Queue.OnPayloadRejected)
	a.listen(ctx, typeIn(domain.EvDownloadCompleted, domain.EvItemDeleted), a.importerEvent)
	a.listen(ctx, typeIn(domain.EvItemAdded, domain.EvItemUpdated, domain.EvItemDeleted, domain.EvItemAvailable,
		domain.EvImportCompleted, domain.EvFileDeleted, domain.EvGrabSent, domain.EvDownloadFailed, domain.EvGrabFailed,
		domain.EvPayloadRejected, domain.EvDownloadRemoved), a.automationEvent)
	a.listen(ctx, notifyFilter, a.Notify.OnEvent)
}

func (a *App) importerEvent(ctx context.Context, ev domain.Event) {
	if ev.Type == domain.EvItemDeleted {
		a.Importer.OnItemDeleted(ctx, ev)
		return
	}
	a.Importer.OnDownloadCompleted(ctx, ev)
}

func (a *App) automationEvent(ctx context.Context, ev domain.Event) {
	a.Automation.OnFailure(ctx, ev)
	a.Automation.OnEvent(ctx, ev)
}

func notifyFilter(t domain.EventType) bool {
	return len(notify.Groups(domain.Event{Type: t})) > 0
}

func (a *App) listen(ctx context.Context, filter func(domain.EventType) bool, fn func(context.Context, domain.Event)) {
	sub := a.d.Bus.Subscribe(256, filter)
	a.goRun(func() {
		defer sub.Close()
		for {
			ev, err := sub.Next(ctx)
			if err != nil {
				return
			}
			fn(ctx, ev)
		}
	})
}
