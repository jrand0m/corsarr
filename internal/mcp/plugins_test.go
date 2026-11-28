package mcp

import "testing"

type fakePlugins struct{}

func (fakePlugins) PluginList() []PluginInfo {
	return []PluginInfo{
		{ID: "qbittorrent", Name: "qBittorrent", Kind: "downloader", Version: "1.0.0", Delivery: "builtin", Instances: []string{}},
		{ID: "transmission", Name: "Transmission", Kind: "downloader", Version: "1.0.0", Delivery: "module", Status: "active",
			Hosts: []string{"$setting:url"}, Secrets: []string{"password"}, Instances: []string{"downloadClients.tr"}, Update: "1.1.0"},
	}
}

func TestListPlugins(t *testing.T) {
	h := newHarness(t)
	out := h.ok(t, "list_plugins", map[string]any{}, "2 plugins", "transmission", "1.1.0")
	if list, _ := out.Data["plugins"].([]any); len(list) != 2 {
		t.Fatalf("plugins %v", out.Data)
	}
	if tool := h.s.byName["list_plugins"]; tool.Write || tool.Destructive {
		t.Fatal("list_plugins must be read-only")
	}
	if New(Deps{Config: h.cfg.Load}).byName["list_plugins"] != nil {
		t.Fatal("no plugin source, no tool")
	}
}
