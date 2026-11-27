package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jrand0m/corsarr/internal/store"
)

func scopedProbe(m Mounter) {
	m.(ScopedMounter).HandleScoped("/probe", func(w http.ResponseWriter, _ *http.Request, sc KeyScope) {
		fmt.Fprint(w, sc)
	})
}

func (e *env) probe(t *testing.T, hdr ...string) string {
	t.Helper()
	req, _ := http.NewRequest("GET", e.srv.URL+"/probe", nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b := make([]byte, 8)
	n, _ := resp.Body.Read(b)
	return string(b[:n])
}

func TestReadOnlyKeyLifecycleAndScopes(t *testing.T) {
	e := newEnv(t, func(d *Deps) { d.Mounts = []func(Mounter){scopedProbe} })
	full := e.key(t)
	if got := e.probe(t, "X-Api-Key", full); got != "2" {
		t.Fatalf("full key: %s", got)
	}
	if got := e.probe(t, "Authorization", "Bearer "+full); got != "2" {
		t.Fatalf("bearer: %s", got)
	}
	if got := e.probe(t); got != "0" {
		t.Fatalf("no key: %s", got)
	}
	code, m := e.do(t, "GET", "/api/v1/auth/apikey/readonly", "", "X-Api-Key", full)
	ro, _ := m["apiKey"].(string)
	if code != 200 || ro == "" {
		t.Fatalf("issue: %d %v", code, m)
	}
	if _, again := e.do(t, "GET", "/api/v1/auth/apikey/readonly", "", "X-Api-Key", full); again["apiKey"] != ro {
		t.Fatal("the read-only key is stable until rotated")
	}
	if got := e.probe(t, "Authorization", "bearer "+ro); got != "1" {
		t.Fatalf("read-only key: %s", got)
	}
	if code, _ := e.do(t, "GET", "/api/v1/system/status", "", "X-Api-Key", ro); code != 401 {
		t.Fatalf("the read-only key never opens the API: %d", code)
	}
	_, m = e.do(t, "POST", "/api/v1/auth/apikey/readonly/rotate", "", "X-Api-Key", full)
	if m["apiKey"] == ro || e.probe(t, "X-Api-Key", ro) != "0" {
		t.Fatal("rotation replaces the old key")
	}
	if code, _ := e.do(t, "DELETE", "/api/v1/auth/apikey/readonly", "", "X-Api-Key", full); code != 200 {
		t.Fatal(code)
	}
	if k, ok, _ := e.st.Auth().ReadOnlyAPIKey(context.Background()); ok || k != "" {
		t.Fatal("revoked")
	}
}

type plainAuth struct{ AuthStore }

func TestReadOnlyKeyNeedsSupportingStore(t *testing.T) {
	e := newEnv(t, func(d *Deps) { d.Auth = plainAuth{d.Auth} })
	if code, _ := e.do(t, "GET", "/api/v1/auth/apikey/readonly", "", "X-Api-Key", e.key(t)); code != 501 {
		t.Fatal(code)
	}
}

func TestReadOnlyKeyLoadedAtStart(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_ = EnsureAPIKey(ctx, st.Auth())
	_ = st.Auth().SetReadOnlyAPIKey(ctx, "ro-key")
	s, err := New(ctx, Deps{Auth: st.Auth()})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := http.NewRequest("GET", "/", nil)
	r.Header.Set("X-Api-Key", "ro-key")
	if s.KeyScope(r) != ScopeRead {
		t.Fatal("stored key not loaded")
	}
}
