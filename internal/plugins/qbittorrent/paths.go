package qbittorrent

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// dirExists reports whether p is a folder corsarr can see.
func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// local turns a path qBittorrent reported into corsarr's view of it.
func (c *Client) local(p string) string {
	out, _ := c.paths.ToLocal(p)
	return out
}

// remote turns a local path into qBittorrent's view of it.
func (c *Client) remote(p string) string {
	out, _ := c.paths.ToRemote(p)
	return out
}

// checkPaths verifies that the folders qBittorrent saves into (the default
// save path and the categories corsarr uses) exist locally after mapping.
// Settings that cannot be read are skipped: the version check already
// proved the connection.
func (c *Client) checkPaths(ctx context.Context) error {
	for _, p := range c.savePaths(ctx) {
		if err := pathProblem(p, c.paths, c.dirExists, runtime.GOOS); err != nil {
			return err
		}
	}
	return nil
}

type preferences struct {
	SavePath string `json:"save_path"`
}

// savePaths lists the default save path and the save path of every
// configured category, as qBittorrent reports them.
func (c *Client) savePaths(ctx context.Context) []string {
	var prefs preferences
	if resp, err := c.get(ctx, "app/preferences", nil, maxSmall); err == nil {
		_ = json.Unmarshal(resp.Body, &prefs)
	}
	if prefs.SavePath == "" {
		return nil
	}
	cats := map[string]categoryInfo{}
	if resp, err := c.get(ctx, "torrents/categories", nil, maxSmall); err == nil {
		_ = json.Unmarshal(resp.Body, &cats)
	}
	set := map[string]bool{prefs.SavePath: true}
	for _, name := range c.categories {
		if cat, ok := cats[name]; ok {
			set[resolve(prefs.SavePath, cat.SavePath)] = true
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// resolve gives a category's save path: empty means the default save path,
// a relative one lies below it.
func resolve(def, cat string) string {
	switch {
	case cat == "":
		return def
	case strings.HasPrefix(cat, "/") || plugin.WindowsStyle(cat):
		return cat
	}
	return strings.TrimRight(def, `/\`) + "/" + cat
}

// pathProblem explains why a folder qBittorrent reported cannot be found
// locally, or returns nil when it can.
func pathProblem(reported string, m plugin.PathMap, exists func(string) bool, goos string) error {
	local, mapped := m.ToLocal(reported)
	switch {
	case exists(local):
		return nil
	case mapped:
		return plugin.Errorf(plugin.ClassPath, "qBittorrent saves to %s, mapped to %s, which does not exist here; check the pathMap entry and the mounted folders", reported, local)
	case plugin.WindowsStyle(reported) && goos != "windows":
		return plugin.Errorf(plugin.ClassPath, "qBittorrent saves to %s, a Windows path; add a pathMap entry such as %s=/downloads naming where corsarr sees that folder", reported, reported)
	}
	return plugin.Errorf(plugin.ClassPath, "qBittorrent saves to %s, which does not exist here and no pathMap entry covers it; mount that folder at the same path (for example in a container) or add a pathMap entry remote=local", reported)
}
