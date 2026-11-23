package importer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/rootfolders"
	"github.com/jrand0m/corsarr/internal/rules"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/testkit"
)

var ctx = context.Background()

type fakeQueue struct {
	d        store.StoredDownload
	title    string
	moves    []domain.DownloadState
	outcomes []domain.ImportOutcome
}

func (q *fakeQueue) Get(context.Context, int64) (store.StoredDownload, error) { return q.d, nil }
func (q *fakeQueue) GrabOf(context.Context, store.StoredDownload) (domain.Grab, bool, error) {
	return domain.Grab{Release: domain.Release{Title: q.title}}, q.title != "", nil
}
func (q *fakeQueue) Transition(_ context.Context, _ int64, to domain.DownloadState, by domain.Actor, _ string) error {
	if err := domain.CheckTransition(q.d.State, to, by); err != nil {
		return err
	}
	q.d.State = to
	q.moves = append(q.moves, to)
	return nil
}
func (q *fakeQueue) ReportImport(_ context.Context, o domain.ImportOutcome) error {
	q.outcomes = append(q.outcomes, o)
	return nil
}

type fakeMovies struct{ m domain.Movie }

func (f *fakeMovies) Get(_ context.Context, id int64) (domain.Movie, error) {
	if id != f.m.ID {
		return domain.Movie{}, errors.New("no movie")
	}
	return f.m, nil
}
func (f *fakeMovies) SetFile(_ context.Context, _, fileID int64) error {
	f.m.FileID = fileID
	return nil
}

type env struct {
	svc    *Service
	q      *fakeQueue
	mv     *fakeMovies
	st     *store.Store
	rec    *testkit.Recorder
	runner *rules.Runner
	dl     string // downloads dir
	lib    string // library root
	data   string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	base := t.TempDir()
	e := &env{st: testkit.Store(t), rec: &testkit.Recorder{}, dl: filepath.Join(base, "downloads"), lib: filepath.Join(base, "movies"), data: filepath.Join(base, "data")}
	e.mv = &fakeMovies{m: domain.Movie{ID: 1, Title: "Example Movie", Year: 2021, ProfileKey: "any", Path: filepath.Join(e.lib, "Example Movie (2021)")}}
	e.q = &fakeQueue{title: "Example.Movie.2021.1080p.WEB-DL.x264-GRP", d: store.StoredDownload{Download: domain.Download{ID: 7,
		State: domain.StateCompleted, Title: "Example.Movie.2021.1080p.WEB-DL.x264-GRP", SavePath: e.dl,
		Links: []domain.DownloadLink{{Item: domain.ItemRef{Kind: domain.KindMovie, ID: 1}, Status: domain.LinkPending}}}}}
	e.runner = rules.New(e.st.Rules(), e.rec, nil)
	e.svc = New(Deps{Queue: e.q, Movies: e.mv, Library: e.st.Library(), Rules: e.runner, RuleStore: e.st.Rules(), Pub: e.rec,
		DataDir: e.data, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err := e.svc.RegisterRules(e.runner, rules.ModeLive); err != nil {
		t.Fatal(err)
	}
	return e
}

func write(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func (e *env) payload(t *testing.T, files map[string]string) {
	root := filepath.Join(e.dl, e.q.d.Title)
	for name, body := range files {
		write(t, filepath.Join(root, name), body)
	}
	e.q.d.ContentPath = root
}

func (e *env) completed(t *testing.T, id int64) {
	e.q.d.ID, e.q.d.State = id, domain.StateCompleted
	e.svc.OnDownloadCompleted(ctx, domain.Event{Type: domain.EvDownloadCompleted, Data: domain.DownloadEventData{DownloadID: id}})
	e.svc.Wait()
}

func TestFreeSpaceIsCheckedBeforeEveryImport(t *testing.T) {
	e := newEnv(t)
	var short []string
	e.svc.d.NoSpace = func(root string) { short = append(short, root) }
	e.mv.m.RootFolder = e.lib
	e.svc.d.FreeSpace = func(string) int64 { return rootfolders.Reserve - 1 } // a hardlink would fit, the reserve not
	e.payload(t, map[string]string{"movie.mkv": "x"})
	e.completed(t, 7)
	o := e.q.outcomes[0]
	if o.Status != domain.ImportBlocked || o.ReasonCode != "no-space" || !strings.Contains(o.Items[0].Reason, "reserve") {
		t.Fatalf("%+v", o)
	}
	if len(short) != 1 || short[0] != e.lib || exists(e.mv.m.Path) {
		t.Fatalf("root folder told %v", short)
	}
	if ev := e.rec.Of(domain.EvImportBlocked); len(ev) != 1 {
		t.Fatal("import.blocked published")
	}
	if err := EnoughSpace("/x", rootfolders.Reserve+10, 10); err != nil {
		t.Fatal(err)
	}
	if err := EnoughSpace("/x", rootfolders.Reserve+9, 10); !errors.Is(err, ErrNoSpace) {
		t.Fatal(err)
	}
}

func TestImportsRunOneAtATimePerRootFolder(t *testing.T) {
	l := newLanes()
	var mu sync.Mutex
	var once sync.Once
	running, most := map[string]int{}, map[string]int{}
	release := make(chan struct{})
	run := func(key string) func(context.Context, int64) {
		return func(context.Context, int64) {
			mu.Lock()
			running[key]++
			most[key] = max(most[key], running[key])
			both := running["a"] > 0 && running["b"] > 0
			mu.Unlock()
			if both {
				once.Do(func() { close(release) })
			}
			select {
			case <-release:
			case <-time.After(2 * time.Second):
			}
			mu.Lock()
			running[key]--
			mu.Unlock()
		}
	}
	for i := int64(1); i <= 3; i++ {
		l.add(ctx, "a", i, run("a"))
	}
	l.add(ctx, "a", 3, run("a")) // queued twice while waiting: runs once
	l.add(ctx, "b", 9, run("b"))
	l.wait()
	select {
	case <-release:
	default:
		t.Fatal("two root folders did not import in parallel")
	}
	if most["a"] != 1 || most["b"] != 1 {
		t.Fatalf("one import at a time per root folder: %v", most)
	}
}

func TestImportHardlinksIntoNamedFolder(t *testing.T) {
	e := newEnv(t)
	e.payload(t, map[string]string{"movie.mkv": strings.Repeat("x", 1000), "sample/sample.mkv": "s", "movie.nfo": "n", "movie.srt": "sub"})
	e.completed(t, 7)
	if len(e.q.moves) != 2 || e.q.moves[0] != domain.StateImportPending || e.q.moves[1] != domain.StateImporting {
		t.Fatalf("transitions %v", e.q.moves)
	}
	o := e.q.outcomes[0]
	want := filepath.Join(e.mv.m.Path, "Example Movie (2021) [WEBDL-1080p] - GRP.mkv")
	if o.Status != domain.ImportImported || o.Items[0].Outcome != "imported" || o.Items[0].NewPaths[0] != want {
		t.Fatalf("%+v", o)
	}
	src, _ := os.Stat(filepath.Join(e.q.d.ContentPath, "movie.mkv"))
	dst, err := os.Stat(want)
	if err != nil || !os.SameFile(src, dst) {
		t.Fatal("not hardlinked", err)
	}
	ev := e.rec.Of(domain.EvImportCompleted)
	if len(ev) != 1 || ev[0].Data.(domain.ImportEventData).Folders[0] != e.mv.m.Path || ev[0].Items[0].Title != "Example Movie" {
		t.Fatalf("%+v", ev)
	}
	f, ok, _ := e.st.Library().File(ctx, e.mv.m.FileID)
	if !ok || f.Quality != "webdl-1080p" || f.Group != "GRP" {
		t.Fatalf("library file %+v", f)
	}
	// Re-running the same import (crash, duplicate event) ends in the same state.
	e.q.d.State = domain.StateImporting
	if err := e.svc.Import(ctx, 7); err != nil || len(e.q.outcomes) != 2 || e.q.outcomes[1].Items[0].NewPaths[0] != want {
		t.Fatalf("idempotent re-run: %v %+v", err, e.q.outcomes)
	}
	e.q.d.State = domain.StateImported
	if err := e.svc.Import(ctx, 7); err != nil || len(e.q.outcomes) != 2 {
		t.Fatal("finished downloads are not imported again")
	}
}

func TestUpgradeRecyclesOldFileAndUndoRestores(t *testing.T) {
	e := newEnv(t)
	e.payload(t, map[string]string{"movie.mkv": "old"})
	e.q.title = "Example.Movie.2021.720p.HDTV.x264-OLD"
	e.completed(t, 7)
	oldPath := e.q.outcomes[0].Items[0].NewPaths[0]
	// a better release arrives
	e.q.d.Title, e.q.title = "Example.Movie.2021.1080p.BluRay.x264-NEW", "Example.Movie.2021.1080p.BluRay.x264-NEW"
	e.payload(t, map[string]string{"movie.mkv": "new and better"})
	e.completed(t, 8)
	o := e.q.outcomes[1]
	if o.Status != domain.ImportImported || o.Items[0].Outcome != "upgraded" || o.Items[0].ReplacedPaths[0] != oldPath {
		t.Fatalf("%+v", o)
	}
	if exists(oldPath) {
		t.Fatal("old file must be recycled")
	}
	acts, _, _ := e.runner.Actions(ctx, 0, 10)
	if len(acts) != 1 || acts[0].RuleID != RuleUpgradeReplace || !acts[0].UndoAvailable {
		t.Fatalf("%+v", acts)
	}
	if len(e.rec.Of(domain.EvFileDeleted)) != 1 || len(e.rec.Of(domain.EvRuleActed)) != 1 {
		t.Fatal("file.deleted and rule.acted")
	}
	if _, err := e.runner.Undo(ctx, acts[0].ID); err != nil || !exists(oldPath) {
		t.Fatalf("undo restores: %v", err)
	}
	// a worse release is blocked as not an upgrade
	e.q.d.Title, e.q.title = "Example.Movie.2021.720p.HDTV.x264-OLD2", "Example.Movie.2021.720p.HDTV.x264-OLD2"
	e.payload(t, map[string]string{"movie.mkv": "worse"})
	e.completed(t, 9)
	if o := e.q.outcomes[2]; o.Status != domain.ImportBlocked || o.ReasonCode != "not-upgrade" {
		t.Fatalf("%+v", o)
	}
	if len(e.rec.Of(domain.EvImportBlocked)) != 1 {
		t.Fatal("import.blocked")
	}
}

func TestUnsafePayloadFailsAndIsRejected(t *testing.T) {
	e := newEnv(t)
	e.payload(t, map[string]string{"movie.mkv": "x", "movie.mkv.scr": "evil"})
	e.completed(t, 7)
	o := e.q.outcomes[0]
	if o.Status != domain.ImportFailed || o.ReasonCode != "payload-unsafe" || e.q.d.State != domain.StateImporting {
		t.Fatalf("%+v", o)
	}
	rej := e.rec.Of(domain.EvPayloadRejected)
	if len(rej) != 1 || rej[0].Data.(domain.PayloadEventData).Stage != domain.StagePostDownload || len(e.rec.Of(domain.EvImportFailed)) != 1 {
		t.Fatalf("%+v", rej)
	}
	if exists(e.mv.m.Path) {
		t.Fatal("nothing imported")
	}
}

func TestHeldDownloadIsNotImported(t *testing.T) {
	e := newEnv(t)
	e.payload(t, map[string]string{"movie.mkv": "x"})
	e.q.d.Category = domain.HoldCategory
	e.completed(t, 7)
	if len(e.q.moves) != 0 || len(e.q.outcomes) != 0 || exists(e.mv.m.Path) {
		t.Fatalf("held download imported: %v %v", e.q.moves, e.q.outcomes)
	}
	e.q.d.Category = "movies" // released
	e.completed(t, 7)
	if len(e.q.outcomes) != 1 || e.q.outcomes[0].Status != domain.ImportImported {
		t.Fatalf("released download imports: %+v", e.q.outcomes)
	}
}

func TestBlockedAndFailedOutcomes(t *testing.T) {
	e := newEnv(t)
	e.q.d.ContentPath = filepath.Join(e.dl, "missing")
	e.completed(t, 7)
	if o := e.q.outcomes[0]; o.Status != domain.ImportFailed || o.ReasonCode != "path-not-found" {
		t.Fatalf("%+v", o)
	}
	e.payload(t, map[string]string{"readme.txt": "no video"})
	e.completed(t, 8)
	if o := e.q.outcomes[1]; o.Status != domain.ImportBlocked || o.ReasonCode != "no-match" {
		t.Fatalf("%+v", o)
	}
	e.q.d.Links = []domain.DownloadLink{{Item: domain.ItemRef{Kind: domain.KindMovie, ID: 5}}}
	e.completed(t, 9)
	if o := e.q.outcomes[2]; o.Status != domain.ImportBlocked || o.ReasonCode != "no-match" || o.Items[0].Item.ID != 5 {
		t.Fatalf("%+v", o)
	}
	e.q.d.Links = nil
	e.completed(t, 10)
	if o := e.q.outcomes[3]; o.ReasonCode != "no-match" {
		t.Fatalf("%+v", o)
	}
	e.q.d.State = domain.StateDownloading
	if err := e.svc.Import(ctx, 11); err != nil || len(e.q.outcomes) != 4 {
		t.Fatal("not completed: no import")
	}
	e.svc.OnDownloadCompleted(ctx, domain.Event{Data: "junk"})
}

func TestConflictWithForeignFile(t *testing.T) {
	e := newEnv(t)
	e.payload(t, map[string]string{"movie.mkv": "x"})
	write(t, filepath.Join(e.mv.m.Path, "Example Movie (2021) [WEBDL-1080p] - GRP.mkv"), "someone else's")
	e.completed(t, 7)
	if o := e.q.outcomes[0]; o.Status != domain.ImportBlocked || o.ReasonCode != "conflict" {
		t.Fatalf("%+v", o)
	}
}

func TestCopyFallbackAcrossFilesystems(t *testing.T) {
	e := newEnv(t)
	link = func(string, string) error { return errors.New("cross-device link") }
	defer func() { link = os.Link }()
	e.payload(t, map[string]string{"movie.mkv": "copied bytes"})
	e.completed(t, 7)
	dest := e.q.outcomes[0].Items[0].NewPaths[0]
	src, _ := os.Stat(filepath.Join(e.q.d.ContentPath, "movie.mkv"))
	dst, err := os.Stat(dest)
	if err != nil || os.SameFile(src, dst) || dst.Size() != src.Size() {
		t.Fatal("copied, not linked", err)
	}
	e.svc.d.FreeSpace = func(string) int64 { return 0 }
	e.q.d.Title, e.q.title = "Example.Movie.2021.2160p.WEB-DL.x264-UHD", "Example.Movie.2021.2160p.WEB-DL.x264-UHD"
	e.mv.m.ProfileKey = "ultrahd"
	e.mv.m.FileID = 0
	e.payload(t, map[string]string{"movie.mkv": "big"})
	e.completed(t, 8)
	if o := e.q.outcomes[1]; o.Status != domain.ImportBlocked || o.ReasonCode != "no-space" {
		t.Fatalf("%+v", o)
	}
}

func TestDeletedItemFilesAreRecycledAndPurged(t *testing.T) {
	e := newEnv(t)
	e.payload(t, map[string]string{"movie.mkv": "x"})
	e.completed(t, 7)
	path := e.q.outcomes[0].Items[0].NewPaths[0]
	e.svc.OnItemDeleted(ctx, domain.Event{Items: []domain.EventItem{e.mv.m.EventItem()}, Data: domain.ItemEventData{FilesDeleted: false}})
	if !exists(path) {
		t.Fatal("files kept unless asked")
	}
	e.svc.OnItemDeleted(ctx, domain.Event{Items: []domain.EventItem{e.mv.m.EventItem()}, Data: domain.ItemEventData{FilesDeleted: true}})
	if exists(path) {
		t.Fatal("recycled")
	}
	acts, _, _ := e.runner.Actions(ctx, 0, 10)
	if len(acts) == 0 || acts[0].RuleID != RuleItemDelete || !acts[0].UndoAvailable {
		t.Fatalf("a movie delete recycles under its own rule: %+v", acts)
	}
	if _, err := e.runner.Undo(ctx, acts[0].ID); err != nil || !exists(path) {
		t.Fatal("undo restores the recycled file", err)
	}
	if _, err := e.runner.Apply(ctx, RuleItemDelete, rules.Candidate{Key: path}); err != nil || exists(path) {
		t.Fatal("recycled again", err)
	}
	// purge: nothing is old enough yet, then a week later it goes
	if counts, err := e.runner.Run(ctx, RuleRecyclePurge); err != nil || counts[rules.ClassActed] != 0 {
		t.Fatal(counts, err)
	}
	e.svc.d.Now = func() time.Time { return time.Now().Add(RecycleKeep + time.Hour) }
	counts, err := e.runner.Run(ctx, RuleRecyclePurge)
	if err != nil || counts[rules.ClassActed] != 1 {
		t.Fatal(counts, err)
	}
	entries, _ := os.ReadDir(filepath.Join(e.data, "recycle"))
	if len(entries) != 0 {
		t.Fatalf("recycle not purged: %v", entries)
	}
}

func TestRestoreRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	from := write(t, filepath.Join(dir, "a"), "a")
	if err := restore(`{"from":"` + from + `","to":"x"}`); err == nil {
		t.Fatal("restore over an existing file")
	}
	if err := restore("{"); err == nil {
		t.Fatal("bad payload")
	}
}
