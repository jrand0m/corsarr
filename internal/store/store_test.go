package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

func open(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenCreatesOneFileAndCheckpointsOnClose(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	ctx := context.Background()
	if err := s.Settings().Set(ctx, "a", "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, DBFile+"-wal")); err != nil {
		t.Fatalf("WAL expected while running: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(dir, DBFile+"-wal")); err == nil && st.Size() > 0 {
		t.Fatalf("WAL not truncated on close: %d bytes", st.Size())
	}
	if err := s.Close(); err != nil {
		t.Fatal("close must be idempotent")
	}
	if err := s.Settings().Set(ctx, "b", "2"); !errors.Is(err, ErrClosed) {
		t.Fatalf("write after close: %v", err)
	}
	// reopen: data survived, key is stable, no migration rerun
	s2 := open(t, dir)
	if v, ok, err := s2.Settings().Get(ctx, "a"); v != "1" || !ok || err != nil {
		t.Fatalf("reopen: %q %v %v", v, ok, err)
	}
	if _, ok, _ := s2.Settings().Get(ctx, "missing"); ok {
		t.Fatal("missing key found")
	}
	if s2.Size() == 0 || s2.Dir() != dir {
		t.Fatal("size/dir")
	}
}

func TestSecondProcessIsRefused(t *testing.T) {
	dir := t.TempDir()
	open(t, dir)
	if _, err := Open(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "another corsarr process") {
		t.Fatalf("want lock refusal, got %v", err)
	}
}

func TestUnwritableDataDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not restrict root (CI job containers run as root)")
	}
	dir := t.TempDir()
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o500); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), filepath.Join(ro, "data")); err == nil || !strings.Contains(err.Error(), "not writable") {
		t.Fatalf("want not writable, got %v", err)
	}
	if _, err := Open(context.Background(), ro); err == nil {
		t.Fatal("lock file in read-only dir must fail")
	}
}

func TestReadsAreQueryOnly(t *testing.T) {
	s := open(t, t.TempDir())
	if _, err := s.R().Exec("INSERT INTO settings (key, value, updated_at) VALUES ('x','y',0)"); err == nil {
		t.Fatal("read handle accepted a write")
	}
}

func TestWritesAreSerialisedAndAtomic(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.Write(ctx, func(tx *sql.Tx) error {
				var n int
				if err := tx.QueryRow("SELECT COUNT(*) FROM settings").Scan(&n); err != nil {
					return err
				}
				_, err := tx.Exec("INSERT INTO settings (key, value, updated_at) VALUES (?, 'v', 0)", n)
				return err
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var n int
	_ = s.R().QueryRow("SELECT COUNT(*) FROM settings").Scan(&n)
	if n != 50 {
		t.Fatalf("lost writes: %d", n)
	}
	// a failing or panicking write rolls back and the writer survives
	boom := errors.New("boom")
	if err := s.Write(ctx, func(tx *sql.Tx) error { _, _ = tx.Exec("DELETE FROM settings"); return boom }); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error { _, _ = tx.Exec("DELETE FROM settings"); panic("x") }); err == nil {
		t.Fatal("panic not reported")
	}
	_ = s.R().QueryRow("SELECT COUNT(*) FROM settings").Scan(&n)
	if n != 50 {
		t.Fatalf("rollback failed: %d", n)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.Write(cctx, func(*sql.Tx) error { return nil }); err == nil {
		t.Fatal("cancelled ctx")
	}
}

func TestForeignKeysOn(t *testing.T) {
	s := open(t, t.TempDir())
	var on int
	_ = s.R().QueryRow("PRAGMA foreign_keys").Scan(&on)
	if on != 1 {
		t.Fatal("foreign keys off")
	}
}

func TestSecretKeyAndEncryption(t *testing.T) {
	dir := t.TempDir()
	s := open(t, dir)
	key, err := os.ReadFile(filepath.Join(dir, KeyFile))
	if err != nil || len(key) != KeySize {
		t.Fatalf("key: %d %v", len(key), err)
	}
	if st, _ := os.Stat(filepath.Join(dir, KeyFile)); st.Mode().Perm() != 0o600 {
		t.Fatalf("key mode %v", st.Mode())
	}
	a, _ := s.Encrypt("hunter2")
	b, _ := s.Encrypt("hunter2")
	if a == b || strings.Contains(a, "hunter2") {
		t.Fatal("ciphertext must be randomised and opaque")
	}
	if p, err := s.Decrypt(a); p != "hunter2" || err != nil {
		t.Fatalf("decrypt: %q %v", p, err)
	}
	for _, bad := range []string{"", "v1:!!", "v1:AAAA", "nope", a[:len(a)-2] + "AA"} {
		if _, err := s.Decrypt(bad); !errors.Is(err, ErrBadCiphertext) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if _, err := newBox([]byte("short")); err == nil {
		t.Fatal("short key accepted")
	}
}

func TestBadSecretKeyRefusesStart(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, KeyFile), []byte("short"), 0o600)
	if _, err := Open(context.Background(), dir); err == nil || !strings.Contains(err.Error(), "secret.key") {
		t.Fatalf("got %v", err)
	}
	// lock must be released after a failed open
	_ = os.Remove(filepath.Join(dir, KeyFile))
	s := open(t, dir)
	_ = s
}

func TestMigrationsBackupAndOrder(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	db, err := openDB(filepath.Join(dir, DBFile), false, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	v1 := []migration{{1, "0001_a.sql", "CREATE TABLE a (x INTEGER);"}}
	if err := applyMigrations(ctx, db, v1, "", filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "b")); err == nil {
		t.Fatal("fresh database must not be backed up")
	}
	v2 := append(v1, migration{2, "0002_b.sql", "CREATE TABLE b (y INTEGER);"})
	if err := applyMigrations(ctx, db, v2, "", filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "b", "corsarr-pre-v2-*.db"))
	if len(backups) != 1 {
		t.Fatalf("pre-migration backup: %v", backups)
	}
	if err := applyMigrations(ctx, db, v1, "", filepath.Join(dir, "b")); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("downgrade: %v", err)
	}
	broken := append(v2, migration{3, "0003_bad.sql", "CREATE TABLE a (dup);"})
	if err := applyMigrations(ctx, db, broken, "", filepath.Join(dir, "b")); err == nil || !strings.Contains(err.Error(), "0003_bad.sql") {
		t.Fatalf("broken: %v", err)
	}
	var v int
	_ = db.QueryRow("PRAGMA user_version").Scan(&v)
	if v != 2 {
		t.Fatalf("failed migration must not bump version: %d", v)
	}
}

func TestMigrationMergedBelowTheNewestStillRuns(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	db, err := openDB(filepath.Join(dir, DBFile), false, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := migration{1, "0001_a.sql", "CREATE TABLE a (x INTEGER);"}
	c := migration{20, "0020_c.sql", "CREATE TABLE c (x INTEGER);"}
	if err := applyMigrations(ctx, db, []migration{a, c}, "", filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}
	b := migration{8, "0008_b.sql", "CREATE TABLE b (x INTEGER);"}
	if err := applyMigrations(ctx, db, []migration{a, b, c}, "", filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO b (x) VALUES (1)"); err != nil {
		t.Fatalf("lower-numbered migration merged later did not run: %v", err)
	}
	var v int
	_ = db.QueryRow("PRAGMA user_version").Scan(&v)
	if v != 20 {
		t.Fatalf("user_version %d", v)
	}
}

func TestLegacyDatabaseWithoutLedger(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	db, err := openDB(filepath.Join(dir, DBFile), false, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE a (x INTEGER); PRAGMA user_version = 1"); err != nil {
		t.Fatal(err)
	}
	ms := []migration{{1, "0001_a.sql", "CREATE TABLE a (x INTEGER);"}, {2, "0002_b.sql", "CREATE TABLE b (y INTEGER);"}}
	if err := applyMigrations(ctx, db, ms, "", filepath.Join(dir, "b")); err != nil {
		t.Fatalf("legacy v1 must only run v2: %v", err)
	}
}

func TestLoadMigrations(t *testing.T) {
	ms, err := loadMigrations(migrationFS)
	if err != nil || len(ms) == 0 || ms[0].version != 1 {
		t.Fatalf("%v %v", ms, err)
	}
	gap := fstest.MapFS{"migrations/0001_a.sql": {}, "migrations/0003_c.sql": {}}
	if got, err := loadMigrations(gap); err != nil || len(got) != 2 || got[1].version != 3 {
		t.Fatalf("reserved gaps must load: %v %v", got, err)
	}
	dup := fstest.MapFS{"migrations/0002_a.sql": {}, "migrations/0002_b.sql": {}}
	if _, err := loadMigrations(dup); err == nil {
		t.Fatal("duplicate version accepted")
	}
	bad := fstest.MapFS{"migrations/x_a.sql": {}}
	if _, err := loadMigrations(bad); err == nil {
		t.Fatal("bad name accepted")
	}
}

func TestBackup(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	_ = s.Settings().Set(ctx, "k", "v")
	p, err := s.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+p+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var v string
	if err := db.QueryRow("SELECT value FROM settings WHERE key='k'").Scan(&v); err != nil || v != "v" {
		t.Fatalf("backup content: %q %v", v, err)
	}
}

func TestAuthRepo(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	a := s.Auth()
	if _, ok, err := a.Admin(ctx); ok || err != nil {
		t.Fatal("first run should have no admin")
	}
	if err := a.SetAdmin(ctx, "admin", "hash"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetAdmin(ctx, "other", "h2"); !errors.Is(err, ErrExists) {
		t.Fatalf("second admin: %v", err)
	}
	if u, ok, _ := a.Admin(ctx); !ok || u.Username != "admin" || u.PasswordHash != "hash" {
		t.Fatalf("admin: %+v", u)
	}
	now := time.Now()
	_ = a.CreateSession(ctx, "t1", now.Add(time.Hour))
	_ = a.CreateSession(ctx, "t2", now.Add(-time.Hour))
	if ok, _ := a.SessionValid(ctx, "t1", now); !ok {
		t.Fatal("t1 valid")
	}
	if ok, _ := a.SessionValid(ctx, "t2", now); ok {
		t.Fatal("t2 expired")
	}
	_ = a.PruneSessions(ctx, now)
	_ = a.DeleteSession(ctx, "t1")
	if ok, _ := a.SessionValid(ctx, "t1", now); ok {
		t.Fatal("t1 deleted")
	}
	if _, ok, _ := a.APIKey(ctx); ok {
		t.Fatal("no key yet")
	}
	// A long, distinctive key: a two-letter one can appear in the encoded ciphertext by chance.
	const second = "second-api-key-in-clear-would-show-this-marker"
	_ = a.SetAPIKey(ctx, "k1")
	_ = a.SetAPIKey(ctx, second)
	if k, ok, err := a.APIKey(ctx); k != second || !ok || err != nil {
		t.Fatalf("key: %q %v %v", k, ok, err)
	}
	var sealed string
	_ = s.R().QueryRow("SELECT key_sealed FROM api_key").Scan(&sealed)
	if strings.Contains(sealed, second) {
		t.Fatal("api key stored in clear")
	}
}

func TestHistoryRepo(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	h := s.History()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		ev := domain.Event{SchemaVersion: 1, EventID: string(rune('a' + i)), Type: domain.EvGrabSent, OccurredAt: base.Add(time.Duration(i) * time.Hour), Actor: "system", Kind: domain.KindMovie}
		if err := h.Append(ctx, ev); err != nil {
			t.Fatal(err)
		}
		_ = h.Append(ctx, ev) // idempotent
	}
	page, next, err := h.List(ctx, 0, 2)
	if err != nil || len(page) != 2 || page[0].Event.EventID != "e" || next == 0 {
		t.Fatalf("page1: %v %d %v", page, next, err)
	}
	page, next, _ = h.List(ctx, next, 10)
	if len(page) != 3 || page[2].Event.EventID != "a" || next != 0 {
		t.Fatalf("page2: %v %d", page, next)
	}
	_ = h.Prune(ctx, base.Add(150*time.Minute))
	page, _, _ = h.List(ctx, 0, 10)
	if len(page) != 2 {
		t.Fatalf("prune: %d", len(page))
	}
}

func TestJobsHealthRootFoldersRepos(t *testing.T) {
	s := open(t, t.TempDir())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	j := domain.JobState{Name: "rss", Enabled: true, Interval: 15 * time.Minute, LastStart: &now, LastResult: domain.JobFailed, LastError: "x", Failures: 2}
	_ = s.Jobs().Save(ctx, j)
	j.Failures = 3
	_ = s.Jobs().Save(ctx, j)
	js, err := s.Jobs().List(ctx)
	if err != nil || len(js) != 1 || js[0].Failures != 3 || js[0].Interval != 15*time.Minute || !js[0].LastStart.Equal(now) || js[0].NextRun != nil {
		t.Fatalf("jobs: %+v %v", js, err)
	}
	hr := domain.IntegrationHealth{Ref: domain.IntegrationRef{Type: "indexer", Key: "knaben"}, Status: domain.HealthFailing, Since: now, LastOKAt: &now, LastErrorClass: "timeout", ConsecutiveFailures: 2}
	_ = s.Health().Save(ctx, hr)
	hs, err := s.Health().List(ctx)
	if err != nil || len(hs) != 1 || hs[0].Status != domain.HealthFailing || !hs[0].LastOKAt.Equal(now) || hs[0].Ref.Key != "knaben" {
		t.Fatalf("health: %+v %v", hs, err)
	}
	rf := RootFolder{Path: "/m", Kind: domain.KindMovie, FreeSpace: 10, Accessible: true, LastChecked: &now}
	_ = s.RootFolders().Upsert(ctx, rf)
	rf.FreeSpace = 20
	_ = s.RootFolders().Upsert(ctx, rf)
	if err := s.RootFolders().Upsert(ctx, RootFolder{Path: "/x", Kind: "comic"}); err == nil {
		t.Fatal("bad kind accepted")
	}
	fs, _ := s.RootFolders().List(ctx)
	if len(fs) != 1 || fs[0].FreeSpace != 20 || fs[0].Kind != domain.KindMovie {
		t.Fatalf("roots: %+v", fs)
	}
	_ = s.RootFolders().Delete(ctx, "/m")
	if fs, _ := s.RootFolders().List(ctx); len(fs) != 0 {
		t.Fatal("delete")
	}
	// Replace keeps exactly the managed list and leaves UI-created folders alone
	_ = s.RootFolders().Upsert(ctx, RootFolder{Path: "/ui", Kind: domain.KindMovie})
	_ = s.RootFolders().Replace(ctx, []RootFolder{{Path: "/a", Kind: domain.KindMovie, Managed: true}, {Path: "/b", Kind: domain.KindTV, Managed: true}})
	if err := s.RootFolders().Replace(ctx, []RootFolder{{Path: "/b", Kind: domain.KindTV, Managed: true, Writable: true, Problem: domain.RootFull}}); err != nil {
		t.Fatal(err)
	}
	fs, _ = s.RootFolders().List(ctx)
	if len(fs) != 2 || fs[0].Path != "/b" || !fs[0].Writable || fs[0].Problem != domain.RootFull || fs[1].Path != "/ui" {
		t.Fatalf("replace: %+v", fs)
	}
	if err := s.RootFolders().Replace(ctx, []RootFolder{{Path: "/x", Kind: "comic", Managed: true}}); err == nil {
		t.Fatal("bad kind accepted")
	}
}
