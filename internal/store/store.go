// Package store owns corsarr's one SQLite file `<data>/corsarr.db`: WAL while running and checkpointed into the
// single file on close, forward migrations with a pre-migration backup, one
// writer goroutine through which every write is serialised, concurrent
// read-only connections, a lock file against a second process, and
// `secret.key` encryption for secret fields.
//
// All SQL lives in this package's repositories; other packages depend on
// small interfaces, never on database/sql.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite" // pure-Go driver
)

// ErrClosed is returned by Write after Close.
var ErrClosed = errors.New("store closed")

// Store is the opened database. Reads use R(); writes go through Write.
type Store struct {
	dir    string
	path   string
	read   *sql.DB // query_only connections: writing through them fails
	write  *sql.DB // exactly one connection, owned by the writer goroutine
	queue  chan writeReq
	done   chan struct{}
	mu     sync.RWMutex
	closed bool
	lock   *os.File
	aead   *cipherBox
}

// writeReq is one unit of work for the writer goroutine.
type writeReq struct {
	run  func() error
	resp chan error
}

// File names inside the data directory.
const (
	DBFile     = "corsarr.db"
	KeyFile    = "secret.key"
	LockFile   = "corsarr.lock"
	BackupsDir = "backups"
)

// Open creates or opens the database in dir, takes the lock, loads or
// creates secret.key, backs up and migrates, and starts the writer.
func Open(ctx context.Context, dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("data dir %s is not writable: %w", dir, err)
	}
	lock, err := acquireLock(filepath.Join(dir, LockFile))
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir, path: filepath.Join(dir, DBFile), lock: lock, queue: make(chan writeReq), done: make(chan struct{})}
	if err := s.open(ctx); err != nil {
		s.closeHandles()
		releaseLock(lock)
		return nil, err
	}
	go s.writer()
	return s, nil
}

func (s *Store) open(ctx context.Context) error {
	var err error
	if s.aead, err = loadKey(filepath.Join(s.dir, KeyFile)); err != nil {
		return err
	}
	if s.write, err = openDB(s.path, false, 1); err != nil {
		return err
	}
	if err = migrate(ctx, s.write, s.path, filepath.Join(s.dir, BackupsDir)); err != nil {
		return err
	}
	s.read, err = openDB(s.path, true, 4)
	return err
}

// openDB opens a handle with the kernel pragmas. Read handles are
// query_only, so the single-writer rule is enforced by structure.
func openDB(path string, readOnly bool, conns int) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)&_pragma=cache_size(-1024)&_txlock=immediate"
	if readOnly {
		dsn += "&_pragma=query_only(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(conns)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return db, nil
}

func (s *Store) writer() {
	defer close(s.done)
	for req := range s.queue {
		req.resp <- req.run()
	}
}

// runTx executes one write; a panicking fn rolls back and becomes an error
// so the writer goroutine survives.
func (s *Store) runTx(ctx context.Context, fn func(*sql.Tx) error) (err error) {
	tx, err := s.write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			err = fmt.Errorf("write panicked: %v", p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Write runs fn in one transaction on the single writer goroutine; writes
// from all callers are serialised. fn must not call Write (it would wait on
// itself).
func (s *Store) Write(ctx context.Context, fn func(*sql.Tx) error) error {
	return s.onWriter(ctx, func() error { return s.runTx(ctx, fn) })
}

// onWriter runs fn on the writer goroutine (serialised with every write).
func (s *Store) onWriter(ctx context.Context, fn func() error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrClosed
	}
	req := writeReq{run: fn, resp: make(chan error, 1)}
	select {
	case s.queue <- req:
	case <-ctx.Done():
		return ctx.Err()
	}
	return <-req.resp
}

// R returns the concurrent read-only handle.
func (s *Store) R() *sql.DB { return s.read }

// Dir is the data directory.
func (s *Store) Dir() string { return s.dir }

// Size is the database file size in bytes (WAL excluded).
func (s *Store) Size() int64 {
	st, err := os.Stat(s.path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// Close drains the writer, checkpoints the WAL into the single file
// (TRUNCATE), closes the handles and releases the lock. Idempotent.
func (s *Store) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.queue)
	s.mu.Unlock()
	<-s.done
	s.read.Close() // readers first: an open snapshot would make TRUNCATE wait for busy_timeout
	_, err := s.write.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	s.closeHandles()
	releaseLock(s.lock)
	return err
}

func (s *Store) closeHandles() {
	if s.read != nil {
		s.read.Close()
	}
	if s.write != nil {
		s.write.Close()
	}
}
