package migrate

import (
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// configDoc collects the config-managed objects a migration found, written
// as a config file fragment. Secrets are always references to environment
// variables, never values.
type configDoc struct {
	Version         int                       `yaml:"version"`
	RootFolders     []map[string]string       `yaml:"rootFolders,omitempty"`
	Indexers        map[string]map[string]any `yaml:"indexers,omitempty"`
	DownloadClients map[string]map[string]any `yaml:"downloadClients,omitempty"`

	keys  map[string]bool
	byURL map[string]string // client url → key
	env   []string
}

func newConfigDoc() *configDoc {
	return &configDoc{Version: 1, Indexers: map[string]map[string]any{}, DownloadClients: map[string]map[string]any{},
		keys: map[string]bool{}, byURL: map[string]string{}}
}

func (c *configDoc) root(path string, k domain.MediaKind) {
	for _, r := range c.RootFolders {
		if r["path"] == path {
			return
		}
	}
	c.RootFolders = append(c.RootFolders, map[string]string{"path": path, "kind": string(k)})
}

// secretEnv names the variable a secret field reads, in the config's own
// convention for secret references.
func (c *configDoc) secretEnv(section, key, field string) string {
	name := "CORSARR_SECRET_" + strings.ToUpper(section+"_"+key+"_"+field)
	c.env = append(c.env, name)
	return name
}

func ref(env string) string { return "${env:" + env + "}" }

// key derives a unique object key from a display name.
func (c *configDoc) key(name string) string {
	k := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, strings.ToLower(name))
	if k == "" || k[0] < 'a' {
		k = "x" + k
	}
	base := k
	for i := 2; c.keys[k]; i++ {
		k = base + strings.Repeat("x", i-1)
	}
	c.keys[k] = true
	return k
}

func (c *configDoc) client(u string, cl SrcClient, k domain.MediaKind) string {
	key := c.key(cl.Name)
	c.byURL[u] = key
	env := c.secretEnv(plugin.KindDownloader.Section(), key, "password")
	inst := map[string]any{"type": "qbittorrent", "name": cl.Name, "enabled": cl.Enabled && !cl.PasswordMasked, "url": u,
		"password": ref(env), "categories": map[string]string{}}
	if cl.Username != "" {
		inst["username"] = cl.Username
	}
	c.DownloadClients[key] = inst
	c.category(u, k, cl.Category)
	return env
}

func (c *configDoc) category(u string, k domain.MediaKind, cat string) {
	inst, ok := c.DownloadClients[c.byURL[u]]
	if !ok || k == "" || cat == "" {
		return
	}
	inst["categories"].(map[string]string)[string(k)] = cat
}

func (c *configDoc) indexer(u, apiPath string, ix SrcIndexer, needKey bool) string {
	key := c.key(ix.Name)
	env := c.secretEnv(plugin.KindIndexer.Section(), key, "apiKey")
	inst := map[string]any{"type": "torznab", "name": ix.Name, "enabled": ix.Enabled && !needKey, "url": u, "apiKey": ref(env)}
	if apiPath != "" {
		inst["apiPath"] = apiPath
	}
	if ix.Priority > 0 {
		inst["priority"] = ix.Priority
	}
	c.Indexers[key] = inst
	return env
}

// render returns the YAML fragment and the variables it references; empty
// when nothing config-managed was found.
func (c *configDoc) render() (string, []string) {
	if len(c.RootFolders) == 0 && len(c.Indexers) == 0 && len(c.DownloadClients) == 0 {
		return "", nil
	}
	out, _ := yaml.Marshal(c) // plain maps and slices of strings always encode
	env := append([]string(nil), c.env...)
	sort.Strings(env)
	return string(out), env
}
