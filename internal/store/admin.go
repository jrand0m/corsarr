package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrNoAdmin means the first-run setup has not happened yet, so there is no
// admin password to replace.
var ErrNoAdmin = errors.New("no admin user is set up yet")

// ReadAPIKey reads the API key without taking the data dir, so a local
// tool can reach the server that holds it (the MCP stdio bridge).
func ReadAPIKey(ctx context.Context, dir string) (string, error) {
	key, err := os.ReadFile(filepath.Join(dir, KeyFile))
	if err != nil {
		return "", fmt.Errorf("no corsarr secret key in %s: %w", dir, err)
	}
	box, err := newBox(key)
	if err != nil {
		return "", err
	}
	db, err := openDB(filepath.Join(dir, DBFile), true, 1)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var sealed string
	if err := db.QueryRowContext(ctx, "SELECT key_sealed FROM api_key WHERE id = 1").Scan(&sealed); err != nil {
		return "", fmt.Errorf("no API key in %s: %w", dir, err)
	}
	return box.open(sealed)
}

// ResetAdminPassword replaces the admin password hash (and the username when
// one is given) and signs every session out. It opens the database file
// directly instead of through Open, so it works while a server holds the data
// dir: SQLite serialises this one transaction with the server's own writer,
// and the server reads credentials from the database on every login.
func ResetAdminPassword(ctx context.Context, dir, username, hash string) error {
	path := filepath.Join(dir, DBFile)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("no corsarr database in %s: %w", dir, err)
	}
	db, err := openDB(path, false, 1)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE admin_user
		SET password_hash = ?, username = COALESCE(NULLIF(?, ''), username), updated_at = ?
		WHERE id = 1`, hash, username, ms(time.Now()))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoAdmin
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions"); err != nil {
		return err
	}
	return tx.Commit()
}
