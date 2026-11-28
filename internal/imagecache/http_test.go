package imagecache

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mux struct{ *http.ServeMux }

func (m mux) Handle(pattern string, h http.HandlerFunc) { m.ServeMux.Handle(pattern, h) }

func handler(e *env) http.Handler {
	m := mux{http.NewServeMux()}
	e.cache.Mount(m)
	return m
}

func do(h http.Handler, method, target string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestImageRoute(t *testing.T) {
	e := newEnv(t, nil)
	h := handler(e)
	tok := e.poster("https://img.example/p.jpg", noisy(300, 450, 1))
	rec := do(h, "GET", "/api/v1/images/"+tok, nil)
	etag := rec.Header().Get("ETag")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" || etag == "" ||
		rec.Header().Get("Cache-Control") != "private, max-age=604800" || rec.Body.Len() == 0 {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	if rec := do(h, "GET", "/api/v1/images/"+tok, map[string]string{"If-None-Match": etag}); rec.Code != http.StatusNotModified {
		t.Fatal(rec.Code)
	}
	if rec := do(h, "GET", "/api/v1/images/"+tok+"?w=100", nil); rec.Code != 200 || rec.Header().Get("ETag") == etag {
		t.Fatal("variant", rec.Code)
	}
	for target, code := range map[string]int{
		"/api/v1/images/" + tok + "?w=abc":                                      400,
		"/api/v1/images/" + tok + "?w=-1":                                       400,
		"/api/v1/images/forged":                                                 404,
		"/api/v1/images/" + e.token(RolePoster, "https://img.example/none.jpg"): 404,
	} {
		if rec := do(h, "GET", target, nil); rec.Code != code {
			t.Errorf("%s: %d, want %d", target, rec.Code, code)
		}
	}
	e.doer.down = true
	if rec := do(h, "GET", "/api/v1/images/"+e.token(RolePoster, "https://img.example/x.jpg"), nil); rec.Code != http.StatusBadGateway {
		t.Fatal(rec.Code)
	}
}

func TestPassThroughRoute(t *testing.T) {
	e := newEnv(t, func(d *Deps) { d.Settings.Enabled = false })
	rec := do(handler(e), "GET", "/api/v1/images/"+e.poster("https://img.example/p.jpg", noisy(100, 150, 1)), nil)
	if rec.Code != 200 || rec.Header().Get("ETag") != "" || rec.Header().Get("Cache-Control") != "private, max-age=3600" {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
}

func TestStatusAndClearRoutes(t *testing.T) {
	e := newEnv(t, func(d *Deps) { d.Metadata = &fakeMeta{bytes: 10} })
	h := handler(e)
	do(h, "GET", "/api/v1/images/"+e.poster("https://img.example/p.jpg", noisy(100, 150, 1)), nil)
	rec := do(h, "GET", "/api/v1/system/cache", nil)
	var st Status
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil || rec.Code != 200 || st.Images != 1 || st.MetadataBytes != 10 ||
		st.Usage != st.ImageBytes+10 || !st.Enabled || st.MaxSize != 1<<30 || st.LastRun != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "DELETE", "/api/v1/system/cache", nil); rec.Code != http.StatusNoContent {
		t.Fatal(rec.Code)
	}
	if st, _ := e.cache.Status(ctx); st.Images != 0 {
		t.Fatalf("%+v", st)
	}
}

// BenchmarkServeCached measures a cached hit end to end through the route.
func BenchmarkServeCached(b *testing.B) {
	e := newEnv(b, nil)
	h := handler(e)
	target := "/api/v1/images/" + e.poster("https://img.example/p.jpg", noisy(1000, 1500, 1))
	do(h, "GET", target, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if rec := do(h, "GET", target, nil); rec.Code != 200 {
			b.Fatal(rec.Code)
		}
	}
}
