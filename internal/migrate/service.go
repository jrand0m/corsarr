package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
)

// Lister lists a download client's downloads (read-only).
type Lister interface {
	List(ctx context.Context, categories []string) ([]plugin.Snapshot, error)
}

// Downloader is a corsarr download client a migration may find downloads in.
type Downloader struct {
	Key  string
	List Lister
}

// Deps are the edges a migration runs on.
type Deps struct {
	Store       *store.Store
	HTTP        Doer
	Config      func() *config.Config
	Downloaders func() []Downloader
	Stat        func(path string) (int64, bool)        // nil = the local filesystem
	Probe       func(path string) Access               // folder access for path checks; nil = the local filesystem
	Publish     func(domain.Event)                     // nil = no events (offline CLI)
	AfterApply  func(context.Context)                  // nil = nothing (offline CLI); also after a mode change
	Mode        func(domain.MediaKind) domain.KindMode // effective mode; nil = active
	Now         func() time.Time
	Log         *slog.Logger
}

// Service runs migrations, one at a time.
type Service struct {
	d      Deps
	mu     sync.Mutex
	checks gate
}

// New returns the service.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Stat == nil {
		d.Stat = statFile
	}
	if d.Config == nil {
		d.Config = func() *config.Config { return nil }
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{d: d}
}

func statFile(p string) (int64, bool) {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return 0, false
	}
	return fi.Size(), true
}

// Request is one run: the sources and the options.
type Request struct {
	Sources []Endpoint
	Options Options
}

// Errors a run reports.
var (
	ErrBusy        = errors.New("a migration is already running")
	ErrPlanChanged = errors.New("the source or corsarr changed since the plan you reviewed; review the new plan")
	ErrNoSources   = errors.New("name at least one source")
)

// Plan reads the sources and builds the plan; nothing is written except
// the run record itself.
func (s *Service) Plan(ctx context.Context, req Request, progress func(Progress)) (Report, error) {
	return s.run(ctx, req, "plan", "", progress)
}

// Apply re-reads the sources, re-plans against live state and executes the
// plan. A non-empty reviewed hash must match the new plan.
func (s *Service) Apply(ctx context.Context, req Request, reviewed string, progress func(Progress)) (Report, error) {
	return s.run(ctx, req, "apply", reviewed, progress)
}

func (s *Service) run(ctx context.Context, req Request, mode, reviewed string, progress func(Progress)) (Report, error) {
	if len(req.Sources) == 0 {
		return Report{}, ErrNoSources
	}
	if !s.mu.TryLock() {
		return Report{}, ErrBusy
	}
	defer s.mu.Unlock()
	progress = s.progress(progress)
	start := s.d.Now()
	id, err := s.d.Store.Migration().StartRun(ctx, mode, appsOf(req.Sources), start)
	if err != nil {
		return Report{}, err
	}
	r, err := s.execute(ctx, req, mode, reviewed, progress)
	r.RunID, r.Started, r.Finished = id, start, s.d.Now()
	s.finish(ctx, &r, err)
	return r, err
}

func (s *Service) execute(ctx context.Context, req Request, mode, reviewed string, progress func(Progress)) (Report, error) {
	snaps := s.read(ctx, req, progress)
	if err := allFailed(snaps); err != nil {
		return Report{Mode: mode, Sources: sourcesOf(snaps)}, err
	}
	st, err := s.state(ctx, snaps, req.Options)
	if err != nil {
		return Report{Mode: mode}, err
	}
	r := BuildPlan(snaps, st, req.Options)
	if mode == "plan" {
		return r, nil
	}
	if reviewed != "" && reviewed != r.PlanHash {
		return r, ErrPlanChanged
	}
	err = Apply(ctx, &r, storeTarget{s.d.Store}, s.d.Now(), progress)
	if s.d.AfterApply != nil {
		s.d.AfterApply(context.WithoutCancel(ctx))
	}
	return r, err
}

// read reads every source; a source that fails is reported and the others
// go on.
func (s *Service) read(ctx context.Context, req Request, progress func(Progress)) []Snapshot {
	days := req.Options.HistoryDays
	if days <= 0 {
		days = DefaultHistoryDays
	}
	since := s.d.Now().Add(-time.Duration(min(days, MaxHistoryDays)) * 24 * time.Hour)
	var out []Snapshot
	for _, ep := range req.Sources {
		began := s.d.Now()
		snap, err := Read(ctx, NewClient(ep, s.d.HTTP), ReadOptions{Scope: req.Options.scope(), HistorySince: since}, progress)
		snap.Source.ReadMs = s.d.Now().Sub(began).Milliseconds()
		if err != nil {
			s.d.Log.Warn("migration source failed", "source", ep.App, "url", ep.URL.String(), "err", err)
		}
		out = append(out, snap)
	}
	return out
}

// allFailed is an error naming every source when none could be read.
func allFailed(snaps []Snapshot) error {
	var errs []error
	for _, s := range snaps {
		if s.Source.Error == "" {
			return nil
		}
		errs = append(errs, fmt.Errorf("%s: %s", s.Source.App, s.Source.Error))
	}
	return errors.Join(errs...)
}

func sourcesOf(snaps []Snapshot) []SourceInfo {
	out := make([]SourceInfo, len(snaps))
	for i, s := range snaps {
		out[i] = s.Source
	}
	return out
}

func (o Options) scope() Scope {
	if o.Scope == nil {
		return DefaultScope()
	}
	return o.Scope
}

func (s *Service) finish(ctx context.Context, r *Report, runErr error) {
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}
	counts := map[string]int{}
	for k, v := range r.Counts {
		counts[string(k)] = v
	}
	body, _ := json.Marshal(r)
	if err := s.d.Store.Migration().FinishRun(context.WithoutCancel(ctx), r.RunID, counts, body, errText, s.d.Now()); err != nil {
		s.d.Log.Warn("migration run record", "err", err)
	}
	if r.Mode == "apply" && s.d.Publish != nil {
		s.d.Publish(domain.Event{Type: domain.EvMigrationComplete, Actor: "user",
			Data: map[string]any{"runId": r.RunID, "counts": r.Counts, "error": errText}})
	}
}

func (s *Service) progress(fn func(Progress)) func(Progress) {
	return func(p Progress) {
		if fn != nil {
			fn(p)
		}
		if s.d.Publish != nil {
			s.d.Publish(domain.Event{Type: domain.EvMigrationProgress, Actor: "user", Data: p})
		}
	}
}

func appsOf(eps []Endpoint) []string {
	out := make([]string, len(eps))
	for i, e := range eps {
		out[i] = string(e.App)
	}
	return out
}

// Runs lists recent runs.
func (s *Service) Runs(ctx context.Context) ([]store.MigrationRun, error) {
	return s.d.Store.Migration().Runs(ctx, 50)
}

// RunReport returns a stored run's report as JSON.
func (s *Service) RunReport(ctx context.Context, id int64) ([]byte, bool, error) {
	return s.d.Store.Migration().Report(ctx, id)
}

// ── live state ───────────────────────────────────────────────────────────────

// state reads what corsarr holds for the plan.
func (s *Service) state(ctx context.Context, snaps []Snapshot, o Options) (State, error) {
	st := State{Prov: map[store.ProvenanceKey]store.Provenance{}, Current: map[store.ProvenanceKey]string{}, Stat: s.d.Stat,
		Now: s.d.Now(), Config: configView(s.d.Config())}
	var err error
	if st.Index, err = s.d.Store.Migration().Index(ctx); err != nil {
		return st, err
	}
	for _, step := range []func(context.Context, *State, []Snapshot) error{s.provenance, s.overrides, s.live, s.clients} {
		if err := step(ctx, &st, snaps); err != nil {
			return st, err
		}
	}
	return st, nil
}

func (s *Service) provenance(ctx context.Context, st *State, snaps []Snapshot) error {
	for _, snap := range snaps {
		prov, err := s.d.Store.Migration().Provenance(ctx, string(snap.Source.App), snap.Source.Instance)
		if err != nil {
			return err
		}
		for k, p := range prov {
			st.Prov[k] = p
			if err := s.current(ctx, st, p); err != nil {
				return err
			}
		}
	}
	return nil
}

// current records the target hash of a library object as corsarr holds it now.
func (s *Service) current(ctx context.Context, st *State, p store.Provenance) error {
	switch p.Key.ObjectType {
	case "series":
		ser, ok, err := s.d.Store.TV().Series(ctx, p.TargetID)
		if err != nil || !ok {
			return err
		}
		seasons, err1 := s.d.Store.TV().Seasons(ctx, ser.ID)
		eps, err2 := s.d.Store.TV().Episodes(ctx, ser.ID)
		st.Current[p.Key] = SeriesTargetHash(ser, seasons, eps)
		return errors.Join(err1, err2)
	case "movie":
		m, ok, err := s.d.Store.Movies().Get(ctx, p.TargetID)
		if ok {
			st.Current[p.Key] = MovieTargetHash(m)
		}
		return err
	case "artist":
		return s.currentArtist(ctx, st, p)
	}
	return nil
}

func (s *Service) currentArtist(ctx context.Context, st *State, p store.Provenance) error {
	ar, ok, err := s.d.Store.Music().Artist(ctx, p.TargetID)
	if err != nil || !ok {
		return err
	}
	albums, err := s.d.Store.Music().Albums(ctx, ar.ID)
	st.Current[p.Key] = ArtistTargetHash(ar, albums)
	return err
}

func (s *Service) overrides(ctx context.Context, st *State, _ []Snapshot) error {
	list, err := s.d.Store.ModeOverrides().List(ctx)
	st.Overrides = map[string]string{}
	for _, o := range list {
		st.Overrides[o.Scope+":"+o.Name] = o.Mode
	}
	return err
}

// liveStates are the queue states of a download corsarr still tracks.
var liveStates = []domain.DownloadState{domain.StateQueued, domain.StateDownloading, domain.StateStalled, domain.StateCompleted,
	domain.StateImportPending, domain.StateImporting, domain.StateImportBlocked}

func (s *Service) live(ctx context.Context, st *State, _ []Snapshot) error {
	list, err := s.d.Store.Queue().Downloads(ctx, liveStates...)
	st.Live = map[string]bool{}
	for _, d := range list {
		st.Live[strings.ToLower(d.DownloadID)] = true
	}
	return err
}

// clients lists the downloads in corsarr's download clients, read-only, when
// some source has a queue.
func (s *Service) clients(ctx context.Context, st *State, snaps []Snapshot) error {
	var dls []Downloader
	if s.d.Downloaders != nil {
		dls = s.d.Downloaders()
	}
	if len(dls) == 0 || !anyQueue(snaps) {
		return nil
	}
	st.Client = map[string]ClientHit{}
	for _, d := range dls {
		list, err := d.List.List(ctx, nil)
		if err != nil {
			return fmt.Errorf("download client %s: %w", d.Key, err)
		}
		for _, sn := range list {
			st.Client[strings.ToLower(sn.DownloadID)] = ClientHit{Downloader: d.Key, Category: sn.Category, SavePath: sn.SavePath}
		}
	}
	return nil
}

func anyQueue(snaps []Snapshot) bool {
	for _, s := range snaps {
		if len(s.Queue) > 0 {
			return true
		}
	}
	return false
}

// configView extracts what the active config sets.
func configView(cfg *config.Config) ConfigView {
	v := ConfigView{RootFolders: map[string]bool{}, Indexers: map[string]bool{}, Clients: map[string]bool{}, KindExplicit: map[string]bool{},
		KindMode: map[string]string{}, ManagerRefs: map[string]string{}, Naming: map[string]bool{}}
	if cfg == nil {
		return v
	}
	for _, f := range cfg.RootFolders {
		v.RootFolders[cleanPath(f.Path)] = true
	}
	for _, key := range sortedKeys(cfg.Instances[plugin.KindIndexer.Section()]) {
		inst := cfg.Instances[plugin.KindIndexer.Section()][key]
		u := inst.Settings.String("url")
		v.Indexers[u] = true
		host := strings.ToLower(hostOf(u))
		if r := inst.Refs["apiKey"]; r != "" && v.ManagerRefs[host] == "" {
			v.ManagerRefs[host] = r
		}
	}
	for _, inst := range cfg.Instances[plugin.KindDownloader.Section()] {
		v.Clients[strings.TrimRight(inst.Settings.String("url"), "/")] = true
	}
	for _, k := range []domain.MediaKind{domain.KindTV, domain.KindMovie, domain.KindMusic, domain.KindBook} {
		v.KindExplicit[string(k)] = ConfigSetsKind(cfg, k)
		v.Naming[string(k)] = configSetsNaming(cfg, k)
		if v.KindExplicit[string(k)] {
			v.KindMode[string(k)] = string(cfg.MediaKinds[k])
		}
	}
	return v
}

func sortedKeys[V any](m map[domain.ObjectKey]V) []domain.ObjectKey {
	out := make([]domain.ObjectKey, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// configSetsNaming reports whether the config source sets any of the kind's
// naming settings (defaults do not count).
func configSetsNaming(cfg *config.Config, k domain.MediaKind) bool {
	for _, f := range append([]string{"rename", "colon"}, namingFields[k]...) {
		if f != "" && cfg.ManagedBy("naming."+string(k)+"."+f) != nil {
			return true
		}
	}
	return false
}

// ConfigSetsKind reports whether the config source sets the kind's mode
// (a default does not count).
func ConfigSetsKind(cfg *config.Config, k domain.MediaKind) bool {
	return cfg.ManagedBy(kindModeKey(k)) != nil
}

// kindModeKey is the dotted config key of a kind's mode.
func kindModeKey(k domain.MediaKind) string { return "mediaKinds." + string(k) + ".mode" }

// ── store target ─────────────────────────────────────────────────────────────

type storeTarget struct{ st *store.Store }

func (t storeTarget) AdoptSeries(ctx context.Context, s domain.Series, seasons []domain.Season, eps []domain.Episode,
	files []store.EpisodeFile, p store.Provenance) (int64, error) {
	return t.st.Migration().AdoptSeries(ctx, s, seasons, eps, files, p)
}

func (t storeTarget) AdoptMovie(ctx context.Context, m domain.Movie, f *domain.LibraryFile, p store.Provenance) (int64, error) {
	return t.st.Migration().AdoptMovie(ctx, m, f, p)
}

func (t storeTarget) AdoptArtist(ctx context.Context, a domain.Artist, albums []store.AdoptAlbum, p store.Provenance) (int64, error) {
	return t.st.Migration().AdoptArtist(ctx, a, albums, p)
}

func (t storeTarget) AlbumIDs(ctx context.Context, artistMBID string) (map[string]int64, error) {
	return t.st.Migration().AlbumIDs(ctx, artistMBID)
}

func (t storeTarget) EpisodeIDs(ctx context.Context, tmdb int64) (map[store.EpisodeKey]int64, error) {
	return t.st.Migration().EpisodeIDs(ctx, tmdb)
}

func (t storeTarget) Index(ctx context.Context) (store.LibraryIndex, error) {
	return t.st.Migration().Index(ctx)
}

func (t storeTarget) AdoptDownload(ctx context.Context, g domain.Grab, d domain.Download) error {
	_, _, err := t.st.Queue().CreateGrab(ctx, g, d)
	return err
}

func (t storeTarget) Block(ctx context.Context, e domain.BlocklistEntry) error {
	_, _, err := t.st.Blocklist().Add(ctx, e)
	return err
}

func (t storeTarget) AppendHistory(ctx context.Context, ev domain.Event) error {
	return t.st.History().Append(ctx, ev)
}

func (t storeTarget) Record(ctx context.Context, p store.Provenance) error {
	return t.st.Migration().Record(ctx, p)
}

func (t storeTarget) SetOverride(ctx context.Context, o store.ModeOverride) error {
	return t.st.ModeOverrides().Set(ctx, o)
}
