package config

import (
	"slices"
	"testing"
)

func TestMCPDefaultsAndFileValues(t *testing.T) {
	def := mustLoad(t, inputs(nil, nil, nil)).MCP
	if def.Enabled || def.Scope != MCPScopeRead || def.RateLimit != 60 || len(def.Tools) != 0 {
		t.Fatalf("defaults: %+v", def)
	}
	yml := "mcp:\n  enabled: true\n  scope: write\n  rateLimit: 0\n  tools: [find_media, request_status]\n  allowedOrigins: [\"http://localhost:6274\"]\n"
	m := mustLoad(t, inputs([]string{"--configFile=/c.yml"}, nil, map[string]string{"/c.yml": yml})).MCP
	if !m.Enabled || m.Scope != MCPScopeWrite || m.RateLimit != 0 || !slices.Equal(m.Tools, []string{"find_media", "request_status"}) ||
		m.AllowedOrigins[0] != "http://localhost:6274" {
		t.Fatalf("%+v", m)
	}
	for _, bad := range []string{"CORSARR_MCP_SCOPE=admin", "CORSARR_MCP_RATELIMIT=-1"} {
		if _, err := Load(inputs(nil, []string{bad}, nil), testRegistry(t)); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	if got := stringList([]any{"a", 1}); !slices.Equal(got, []string{"a"}) {
		t.Fatal(got)
	}
}
