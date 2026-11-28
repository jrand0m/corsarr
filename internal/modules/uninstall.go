package modules

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jrand0m/corsarr/internal/plugin"
	"github.com/jrand0m/corsarr/internal/wasm"
	"github.com/jrand0m/corsarr/sdk/go/abi"
)

// StatusUninstalled marks a module whose files are gone but whose manifest
// is kept, so instances configured for it still validate and report that the
// module is missing.
const StatusUninstalled Status = "uninstalled"

// fileUninstalled holds the manifest of an uninstalled module.
const fileUninstalled = "uninstalled.json"

// ErrUninstalled is what an instance of an uninstalled module fails with.
var ErrUninstalled = errors.New("module not installed")

func (s Store) recordPath(id string) string { return filepath.Join(s.idDir(id), fileUninstalled) }

// Record returns the kept manifest of an uninstalled module.
func (s Store) Record(id string) ([]byte, bool) {
	b, err := os.ReadFile(s.recordPath(id))
	return b, err == nil
}

// writeRecord keeps the manifest of a module after its files are deleted.
func (s Store) writeRecord(id string, manifest []byte) error {
	if err := os.MkdirAll(s.idDir(id), 0o755); err != nil {
		return err
	}
	return WriteAtomic(s.recordPath(id), manifest)
}

func (s Store) dropRecord(id string) {
	_ = os.Remove(s.recordPath(id)) // absent when the module was never uninstalled
}

// Uninstall unloads a module, deletes every version and keeps its manifest:
// the id stays registered as a placeholder whose instances fail with
// ErrUninstalled until the module is installed again.
func (m *Manager) Uninstall(id string) (Info, error) {
	info, ok := m.Get(id)
	if !ok || info.Status == StatusUninstalled {
		return Info{}, fmt.Errorf("%s: %w", id, ErrNotInstalled)
	}
	raw, err := m.manifestBytes(id, info.Version)
	if err != nil {
		return Info{}, err
	}
	if err := m.Remove(id); err != nil {
		return Info{}, err
	}
	if err := m.store.writeRecord(id, raw); err != nil {
		return Info{}, err
	}
	m.registerRecord(id)
	out, _ := m.Get(id)
	return out, nil
}

func (m *Manager) manifestBytes(id, version string) ([]byte, error) {
	v, err := m.store.Read(id, version, wasm.SHA256)
	if err != nil {
		return nil, err
	}
	return wasm.ManifestBytes(v.Module, v.Manifest)
}

// recordInfo describes an uninstalled module from its kept manifest.
func (m *Manager) recordInfo(id string) (Info, abi.Manifest, bool) {
	raw, _ := m.store.Record(id) // no record: nil, which does not parse
	man, err := wasm.ParseManifest(raw)
	if err != nil || man.ID != id {
		return Info{}, abi.Manifest{}, false
	}
	return Info{ID: id, Version: man.Version, Kind: wasm.ManifestKind(man), Status: StatusUninstalled,
		Error: ErrUninstalled.Error(), Manifest: man}, man, true
}

// registerRecord registers the placeholder of an uninstalled module; a
// built-in with the same id (enabled, or disabled and now back) keeps it.
func (m *Manager) registerRecord(id string) {
	_, man, ok := m.recordInfo(id)
	if _, disabled := m.disabled[id]; !ok || m.builtins[id] || disabled {
		return
	}
	spec, ok := wasm.Spec(wasm.ManifestKind(man))
	if !ok {
		return
	}
	msg := "plugin module " + id + " is not installed; install it again to run this instance"
	_ = m.o.Registry.Replace(plugin.Registration{Manifest: registryManifest(man, spec, nil),
		New: func(plugin.Instance) (any, error) { return nil, plugin.Wrap(plugin.ClassConfig, msg, ErrUninstalled) }})
}

// isRecordOnly reports whether id has a kept manifest and no versions.
func (m *Manager) isRecordOnly(id string) bool {
	_, ok := m.store.Record(id)
	return ok && len(m.store.versions(id)) == 0
}
