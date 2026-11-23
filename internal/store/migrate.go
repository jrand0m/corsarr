package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// migration is one forward step `NNNN_name.sql`; the version is NNNN.
type migration struct {
	version int
	name    string
	sql     string
}

// loadMigrations parses the embedded files, sorted. Numbers are reserved in
// blocks by parallel lines of work, so gaps are allowed; a duplicate number
// is a build-time bug.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	files, err := fs.Glob(fsys, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, f := range files {
		base := filepath.Base(f)
		num, _, _ := strings.Cut(base, "_")
		v, err := strconv.Atoi(num)
		if err != nil || v < 1 {
			return nil, fmt.Errorf("migration %s: name must start with a positive number", base)
		}
		body, _ := fs.ReadFile(fsys, f) // listed by Glob on the same FS
		out = append(out, migration{version: v, name: base, sql: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	for i := 1; i < len(out); i++ {
		if out[i].version == out[i-1].version {
			return nil, fmt.Errorf("migration %s: version %d is used twice", out[i].name, out[i].version)
		}
	}
	return out, nil
}

// migrate applies pending migrations, each in its own transaction, after
// copying an existing database to backups/.
func migrate(ctx context.Context, db *sql.DB, dbPath, backupDir string) error {
	ms, err := loadMigrations(migrationFS)
	if err != nil {
		return err
	}
	return applyMigrations(ctx, db, ms, dbPath, backupDir)
}

// ledgerDDL records every applied migration, so one merged later with a
// lower number than the newest applied one still runs.
const ledgerDDL = "CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY) STRICT"

func applyMigrations(ctx context.Context, db *sql.DB, ms []migration, dbPath, backupDir string) error {
	applied, err := appliedVersions(ctx, db, ms)
	if err != nil {
		return err
	}
	if v, ok := unknownVersion(applied, ms); ok {
		return fmt.Errorf("database schema v%d is newer than this corsarr; upgrade corsarr", v)
	}
	pending := pendingOf(applied, ms)
	if len(pending) == 0 {
		return nil
	}
	if len(applied) > 0 {
		name := fmt.Sprintf("corsarr-pre-v%d-%s.db", ms[len(ms)-1].version, time.Now().UTC().Format("20060102T150405"))
		if err := backup(ctx, db, backupDir, name); err != nil {
			return fmt.Errorf("pre-migration backup: %w", err)
		}
	}
	for _, m := range pending {
		if err := applyOne(ctx, db, m); err != nil {
			return err
		}
	}
	return nil
}

// appliedVersions reads the ledger. A database from before the ledger
// counts every known migration up to its user_version as applied.
func appliedVersions(ctx context.Context, db *sql.DB, ms []migration) (map[int]bool, error) {
	if _, err := db.ExecContext(ctx, ledgerDDL); err != nil {
		return nil, err
	}
	applied := map[int]bool{}
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil || len(applied) > 0 {
		return applied, err
	}
	var cur int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&cur); err != nil {
		return nil, err
	}
	return legacyApplied(cur), nil
}

func legacyApplied(userVersion int) map[int]bool {
	applied := map[int]bool{}
	for v := 1; v <= userVersion; v++ {
		applied[v] = true
	}
	return applied
}

func unknownVersion(applied map[int]bool, ms []migration) (int, bool) {
	known := map[int]bool{}
	for _, m := range ms {
		known[m.version] = true
	}
	for v := range applied {
		if !known[v] {
			return v, true
		}
	}
	return 0, false
}

func pendingOf(applied map[int]bool, ms []migration) []migration {
	var out []migration
	for _, m := range ms {
		if !applied[m.version] {
			out = append(out, m)
		}
	}
	return out
}

func applyOne(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("migration %s: %w", m.name, err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
		return err
	}
	var cur int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&cur); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", max(cur, m.version))); err != nil {
		return err
	}
	return tx.Commit()
}

// backup writes a consistent single-file copy (WAL folded in) via VACUUM INTO.
func backup(ctx context.Context, db *sql.DB, dir, name string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	dst := filepath.Join(dir, name)
	_, err := db.ExecContext(ctx, "VACUUM INTO ?", dst)
	return err
}

// Backup writes a consistent single-file copy of the database into backups/
// on the writer goroutine, so no write interleaves; it returns the path.
func (s *Store) Backup(ctx context.Context) (string, error) {
	name := "corsarr-" + time.Now().UTC().Format("20060102T150405.000") + ".db"
	dir := filepath.Join(s.dir, BackupsDir)
	err := s.onWriter(ctx, func() error { return backup(ctx, s.write, dir, name) })
	return filepath.Join(dir, name), err
}
