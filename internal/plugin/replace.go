package plugin

import "fmt"

// Put registers a plugin, replacing one with the same id. Declarative
// plugins use it when definitions are reloaded; built-ins use Register.
func (r *Registry) Put(reg Registration) error {
	m := reg.Manifest
	if m.ID == "" || m.Kind == "" || reg.New == nil {
		return fmt.Errorf("plugin registration needs id, kind and factory")
	}
	if m.APIVersion != APIVersion {
		return fmt.Errorf("plugin %s targets API %d, host is %d", m.ID, m.APIVersion, APIVersion)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[m.ID] = reg
	return nil
}

// Remove drops a plugin (a deleted user definition); unknown ids are ignored.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
}
