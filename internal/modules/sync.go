package modules

import (
	"context"
	"time"
)

// SyncInterval is how often the server looks for drop-ins and store changes.
const SyncInterval = 10 * time.Second

// Sync installs drop-ins and follows store changes made outside this process
// (corsarr plugin load|unload on the same folder): a new active version is
// loaded and swapped in, a cleared active pointer unloads the module.
func (m *Manager) Sync(ctx context.Context) {
	for _, d := range m.store.DropIns() {
		if info, err := m.Install(ctx, d.Module, d.Sidecar, ""); err != nil {
			m.logger().Warn("plugin module drop-in refused", "file", d.Path, "err", err)
			m.store.Reject(d, err)
		} else {
			m.logger().Info("plugin module installed", "plugin", info.ID, "version", info.Version, "file", d.Path)
			m.store.Consumed(d)
		}
	}
	m.followStore()
}

func (m *Manager) followStore() {
	stored := map[string]bool{}
	for _, id := range m.store.IDs() {
		stored[id] = true
		m.follow(id)
	}
	for _, info := range m.List() {
		if !stored[info.ID] && info.Status == StatusActive {
			_ = m.Unload(info.ID)
		}
	}
}

// follow brings one id in line with its active pointer.
func (m *Manager) follow(id string) {
	active, _ := m.store.Active(id)
	cur, loaded := m.current(id)
	switch {
	case active == "" && loaded:
		_ = m.Unload(id)
	case active != "" && active != cur:
		if err := m.registerStored(id, active); err != nil {
			m.fail(id, active, err)
		}
	}
}

// current is the loaded or failed version of id.
func (m *Manager) current(id string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e, ok := m.entries[id]; ok {
		return e.info.Version, true
	}
	if f, ok := m.failed[id]; ok {
		return f.Version, false
	}
	return "", false
}

// Run syncs every SyncInterval until ctx ends.
func (m *Manager) Run(ctx context.Context) {
	t := time.NewTicker(SyncInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.Sync(ctx)
		}
	}
}
