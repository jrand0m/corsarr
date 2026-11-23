package queue

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/testkit"
)

var ctx = context.Background()

type fakeIndexer struct{ err error }

func (f *fakeIndexer) Test(context.Context) error                { return nil }
func (f *fakeIndexer) Caps(context.Context) (plugin.Caps, error) { return plugin.Caps{}, nil }
func (f *fakeIndexer) Search(context.Context, domain.SearchRequest) ([]domain.Release, error) {
	return nil, nil
}
func (f *fakeIndexer) Feed(context.Context, domain.MediaKind, string) ([]domain.Release, string, error) {
	return nil, "", nil
}
func (f *fakeIndexer) Resolve(_ context.Context, r domain.Release) (plugin.Payload, error) {
	return plugin.Payload{Kind: "magnet", URL: r.DownloadURL}, f.err
}

type fakeDL struct {
	mu       sync.Mutex
	snaps    map[string]plugin.Snapshot
	files    map[string][]plugin.FileEntry
	removed  map[string]bool // hash → deleteFiles
	addErr   error
	listErr  error
	opErr    error
	ops      []string
	ensured  int
	category string
}

func newDL() *fakeDL {
	return &fakeDL{snaps: map[string]plugin.Snapshot{}, files: map[string][]plugin.FileEntry{}, removed: map[string]bool{}}
}

func (f *fakeDL) Test(context.Context) error          { return nil }
func (f *fakeDL) Protocols() []domain.Protocol        { return []domain.Protocol{domain.ProtoTorrent} }
func (f *fakeDL) CategoryFor(domain.MediaKind) string { return f.category }
func (f *fakeDL) Add(_ context.Context, r plugin.AddRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.addErr != nil {
		return "", f.addErr
	}
	id := strings.TrimPrefix(r.Payload.URL, "magnet:?xt=urn:btih:")
	f.snaps[id] = plugin.Snapshot{DownloadID: strings.ToUpper(id), Category: r.Category, State: domain.StateQueued}
	return strings.ToUpper(id), nil
}
func (f *fakeDL) List(_ context.Context, cats []string) ([]plugin.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []plugin.Snapshot
	for _, s := range f.snaps {
		out = append(out, s)
	}
	return out, f.listErr
}
func (f *fakeDL) Files(_ context.Context, id string) ([]plugin.FileEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.files[id], nil
}
func (f *fakeDL) Remove(_ context.Context, id string, del bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed[id] = del
	delete(f.snaps, id)
	return f.opErr
}
func (f *fakeDL) op(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ops = append(f.ops, name)
	return f.opErr
}
func (f *fakeDL) Pause(context.Context, string) error  { return f.op("pause") }
func (f *fakeDL) Resume(context.Context, string) error { return f.op("resume") }
func (f *fakeDL) SetPriority(_ context.Context, _ string, p plugin.Priority) error {
	return f.op("prio:" + string(p))
}
func (f *fakeDL) EnsureCategory(context.Context, string, string) error {
	f.ensured++
	return errors.New("cannot create")
}
func (f *fakeDL) set(id string, mut func(*plugin.Snapshot)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.snaps[id]
	mut(&s)
	f.snaps[id] = s
}

type plugins struct {
	ix  *fakeIndexer
	dls []Downloader
}

func (p *plugins) Indexer(key domain.ObjectKey) (plugin.Indexer, bool) { return p.ix, key == "ix" }
func (p *plugins) Downloaders() []Downloader                           { return p.dls }

type fakeJobs struct {
	mu       sync.Mutex
	interval time.Duration
	enabled  *bool
	runs     int
}

func (j *fakeJobs) SetInterval(_ string, d time.Duration) error {
	j.mu.Lock()
	j.interval = d
	j.mu.Unlock()
	return nil
}
func (j *fakeJobs) SetEnabled(_ string, on bool) error {
	j.mu.Lock()
	j.enabled = &on
	j.mu.Unlock()
	return nil
}
func (j *fakeJobs) RunNow(string) error { j.mu.Lock(); j.runs++; j.mu.Unlock(); return nil }

type env struct {
	svc  *Service
	st   *store.Store
	rec  *testkit.Recorder
	dl   *fakeDL
	ix   *fakeIndexer
	jobs *fakeJobs
	ui   int
	now  time.Time
}

func newEnv(t *testing.T) *env {
	e := &env{st: testkit.Store(t), rec: &testkit.Recorder{}, dl: newDL(), ix: &fakeIndexer{}, jobs: &fakeJobs{}, now: time.Now()}
	e.svc = New(Deps{Repo: e.st.Queue(), Plugins: &plugins{ix: e.ix, dls: []Downloader{{Key: "qb", Impl: e.dl}}},
		Pub: e.rec, Jobs: e.jobs, UIClients: func() int { return e.ui }, Now: func() time.Time { return e.now },
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	return e
}

var movie = domain.EventItem{Ref: domain.ItemRef{Kind: domain.KindMovie, ID: 1}, Title: "Example"}

func release(hash string) domain.Release {
	return domain.Release{ID: "r-" + hash, Title: "Example.2021.1080p.WEB-DL-GRP", Protocol: domain.ProtoTorrent, Size: 4e9,
		Indexers: []domain.IndexerRef{{Key: "ix"}}, GUID: "g-" + hash, DownloadURL: "magnet:?xt=urn:btih:" + hash}
}

func (e *env) grab(t *testing.T, hash string) store.StoredDownload {
	t.Helper()
	g, err := e.svc.Grab(ctx, GrabRequest{Kind: domain.KindMovie, Items: []domain.EventItem{movie}, Release: release(hash),
		Decision: domain.Decision{Accepted: true, Quality: "WEBDL-1080p"}, Origin: domain.OriginAutomatic})
	if err != nil {
		t.Fatal(err)
	}
	list, _ := e.st.Queue().Downloads(ctx, domain.StateQueued)
	for _, d := range list {
		if d.GrabID == g.ID {
			return d
		}
	}
	t.Fatal("download not created")
	return store.StoredDownload{}
}

func states(rec *testkit.Recorder) []string {
	var out []string
	for _, ev := range rec.Of(domain.EvDownloadState) {
		d := ev.Data.(domain.DownloadEventData)
		out = append(out, string(d.From)+">"+string(d.To))
	}
	return out
}
