package api

import (
	"net/http"
	"testing"
)

func TestMountedModuleRoutesRequireAuthAndPage(t *testing.T) {
	e := newEnv(t, func(d *Deps) {
		d.Mounts = append(d.Mounts, func(m Mounter) {
			m.Handle("GET /api/v1/things", func(w http.ResponseWriter, r *http.Request) {
				cursor, limit, ok := Page(w, r)
				if !ok {
					return
				}
				WritePage(w, []int64{cursor, int64(limit)}, cursor+1)
			})
		})
	})
	if code, _ := e.do(t, "GET", "/api/v1/things", ""); code != 401 {
		t.Fatalf("unauthenticated: %d", code)
	}
	h := []string{"X-Api-Key", e.key(t)}
	if code, m := e.do(t, "GET", "/api/v1/things?cursor=4&limit=7", "", h...); code != 200 || m["nextCursor"] != "5" {
		t.Fatalf("%d %v", code, m)
	}
	if code, _ := e.do(t, "GET", "/api/v1/things?limit=0", "", h...); code != 400 {
		t.Fatalf("bad limit: %d", code)
	}
	_, m := e.do(t, "GET", "/api/v1/openapi.json", "", h...)
	if paths, _ := m["paths"].(map[string]any); paths["/things"] == nil {
		t.Fatalf("module path missing from openapi: %v", m["paths"])
	}
}
