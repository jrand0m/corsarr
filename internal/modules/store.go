package modules

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Store is the on-disk layout of installed modules:
//
//	<dir>/<id>/<version>/module.wasm   the module as installed
//	<dir>/<id>/<version>/plugin.json   its manifest (embedded or sidecar)
//	<dir>/<id>/<version>/sha256        hex digest recorded at install
//	<dir>/<id>/active                  the active version
//	<dir>/<id>/previous                the version kept for rollback
//	<dir>/<name>.wasm [+ <name>.json]  drop-ins, installed on the next sync
//	<dir>/.cache/                      compiled code
type Store struct{ Dir string }

const (
	fileModule   = "module.wasm"
	fileManifest = "plugin.json"
	fileSHA      = "sha256"
	fileActive   = "active"
	filePrevious = "previous"
	dirCache     = ".cache"
	rejectedExt  = ".rejected"
)

// Version is one stored version.
type Version struct {
	ID, Version, SHA256 string
	Module, Manifest    []byte
}

func (s Store) idDir(id string) string           { return filepath.Join(s.Dir, id) }
func (s Store) verDir(id, version string) string { return filepath.Join(s.Dir, id, version) }

// CacheDir is where compiled code is cached.
func (s Store) CacheDir() string { return filepath.Join(s.Dir, dirCache) }

func readText(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// writeAtomic writes a file through a temporary name and a rename.
func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Active returns the active and previous versions of id ("" when none).
func (s Store) Active(id string) (active, previous string) {
	return readText(filepath.Join(s.idDir(id), fileActive)), readText(filepath.Join(s.idDir(id), filePrevious))
}

// IDs lists the installed module ids.
func (s Store) IDs() []string {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out
}

// ErrChecksum is returned when a stored module no longer matches its digest.
var ErrChecksum = errors.New("checksum mismatch")

// Read loads one stored version and checks its recorded SHA-256.
func (s Store) Read(id, version string, sum func([]byte) string) (Version, error) {
	dir := s.verDir(id, version)
	mod, err := os.ReadFile(filepath.Join(dir, fileModule))
	if err != nil {
		return Version{}, err
	}
	want := readText(filepath.Join(dir, fileSHA))
	if got := sum(mod); got != want {
		return Version{}, fmt.Errorf("%s %s: %w (recorded %s, file %s)", id, version, ErrChecksum, short(want), short(got))
	}
	man, _ := os.ReadFile(filepath.Join(dir, fileManifest))
	return Version{ID: id, Version: version, SHA256: want, Module: mod, Manifest: man}, nil
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// Write stores a version (idempotent for identical content).
func (s Store) Write(v Version) error {
	dir := s.verDir(v.ID, v.Version)
	if existing := readText(filepath.Join(dir, fileSHA)); existing != "" {
		if existing != v.SHA256 {
			return fmt.Errorf("%s %s is already installed with different content; publish a new version", v.ID, v.Version)
		}
		return nil
	}
	tmp := dir + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	for name, data := range map[string][]byte{fileModule: v.Module, fileManifest: v.Manifest, fileSHA: []byte(v.SHA256 + "\n")} {
		if err := os.WriteFile(filepath.Join(tmp, name), data, 0o644); err != nil {
			return err
		}
	}
	return os.Rename(tmp, dir)
}

// Activate makes version active, keeps the old active one as previous and
// deletes every other version.
func (s Store) Activate(id, version string) error {
	active, previous := s.Active(id)
	if active != version {
		previous = active
	}
	if previous != "" {
		if err := writeAtomic(filepath.Join(s.idDir(id), filePrevious), []byte(previous+"\n")); err != nil {
			return err
		}
	}
	if err := writeAtomic(filepath.Join(s.idDir(id), fileActive), []byte(version+"\n")); err != nil {
		return err
	}
	if s.prune(id, version, previous) {
		s.ClearCache()
	}
	return nil
}

func (s Store) prune(id string, keep ...string) bool {
	entries, _ := os.ReadDir(s.idDir(id))
	pruned := false
	for _, e := range entries {
		if e.IsDir() && !contains(keep, e.Name()) {
			pruned = os.RemoveAll(filepath.Join(s.idDir(id), e.Name())) == nil || pruned
		}
	}
	return pruned
}

// ClearCache deletes the cached compiled code (directories stay, so a running
// engine keeps writing there); modules still installed compile again on
// their next load. Compiled code is keyed by content, so this is the only way
// to drop the code of removed versions.
func (s Store) ClearCache() {
	_ = filepath.WalkDir(s.CacheDir(), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			_ = os.Remove(path)
		}
		return nil
	})
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Deactivate clears the active pointer (files stay for a later load).
func (s Store) Deactivate(id string) error {
	err := os.Remove(filepath.Join(s.idDir(id), fileActive))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Swap makes previous active and active previous (rollback).
func (s Store) Swap(id string) (string, error) {
	active, previous := s.Active(id)
	if previous == "" {
		return "", fmt.Errorf("%s has no previous version to roll back to", id)
	}
	if err := writeAtomic(filepath.Join(s.idDir(id), fileActive), []byte(previous+"\n")); err != nil {
		return "", err
	}
	if active == "" {
		return previous, os.Remove(filepath.Join(s.idDir(id), filePrevious))
	}
	return previous, writeAtomic(filepath.Join(s.idDir(id), filePrevious), []byte(active+"\n"))
}

// versions lists the stored versions of id.
func (s Store) versions(id string) []string {
	entries, _ := os.ReadDir(s.idDir(id))
	var out []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasSuffix(e.Name(), ".tmp") {
			out = append(out, e.Name())
		}
	}
	return out
}

// Remove deletes every version of id and the cached compiled code.
func (s Store) Remove(id string) error {
	defer s.ClearCache()
	return os.RemoveAll(s.idDir(id))
}

// DropIn is a module file put straight into the plugins folder.
type DropIn struct {
	Path            string
	Module, Sidecar []byte
}

// DropIns lists the *.wasm files waiting in the folder.
func (s Store) DropIns() []DropIn {
	paths, _ := filepath.Glob(filepath.Join(s.Dir, "*.wasm"))
	out := make([]DropIn, 0, len(paths))
	for _, p := range paths {
		mod, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		side, _ := os.ReadFile(strings.TrimSuffix(p, ".wasm") + ".json")
		out = append(out, DropIn{Path: p, Module: mod, Sidecar: side})
	}
	return out
}

// Consumed removes an installed drop-in.
func (s Store) Consumed(d DropIn) {
	_ = os.Remove(d.Path)
	_ = os.Remove(strings.TrimSuffix(d.Path, ".wasm") + ".json")
}

// Reject renames a refused drop-in out of the way and writes the reason next
// to it.
func (s Store) Reject(d DropIn, reason error) {
	_ = os.Rename(d.Path, d.Path+rejectedExt)
	_ = os.WriteFile(d.Path+rejectedExt+".txt", []byte(reason.Error()+"\n"), 0o644)
}
