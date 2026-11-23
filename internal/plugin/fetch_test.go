package plugin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func get(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestFetchClasses(t *testing.T) {
	cases := map[int]FailureClass{401: ClassAuth, 403: ClassAuth, 429: ClassRateLimited, 500: ClassServerError, 503: ClassServerError}
	for status, want := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
		_, err := Fetch(srv.Client(), get(t, srv.URL), 100)
		srv.Close()
		if ClassOf(err) != want {
			t.Errorf("status %d: class %q, want %q", status, ClassOf(err), want)
		}
	}
}

func TestFetchOKAndNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(404)
		}
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()
	r, err := Fetch(srv.Client(), get(t, srv.URL), 100)
	if err != nil || string(r.Body) != "hello" || r.Status != 200 {
		t.Fatalf("%v %+v", err, r)
	}
	r, err = Fetch(srv.Client(), get(t, srv.URL+"/missing"), 100)
	if err != nil || r.Status != 404 {
		t.Fatalf("404 is the caller's: %v %+v", err, r)
	}
}

func TestDoBounds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 50)))
	}))
	defer srv.Close()
	if _, err := Do(srv.Client(), get(t, srv.URL), 10); ClassOf(err) != ClassParse {
		t.Fatalf("oversize must be parse, got %v", err)
	}
	if _, err := Do(srv.Client(), get(t, srv.URL), 50); err != nil {
		t.Fatalf("exact limit must pass: %v", err)
	}
}

func TestTransportClasses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	_, err := Fetch(srv.Client(), req, 10)
	srv.CloseClientConnections()
	srv.Close()
	if ClassOf(err) != ClassTimeout {
		t.Errorf("timeout: %v", err)
	}
	if _, err := Fetch(http.DefaultClient, get(t, srv.URL), 10); ClassOf(err) != ClassUnreachable {
		t.Errorf("closed server: %v", err)
	}
}

func TestErrorsNeverLeakQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	_, err := Fetch(srv.Client(), get(t, srv.URL+"/api?apikey=SECRET"), 10)
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("status error leaks: %v", err)
	}
	url := srv.URL + "/api?apikey=SECRET"
	srv.Close()
	_, err = Fetch(http.DefaultClient, get(t, url), 10)
	if err == nil || strings.Contains(err.Error(), "SECRET") || ClassOf(err) != ClassUnreachable {
		t.Fatalf("transport error leaks or misclassified: %v", err)
	}
	bad := &neturlErr{}
	if scrub(bad) != error(bad) {
		t.Error("non-url errors pass through")
	}
}

type neturlErr struct{}

func (*neturlErr) Error() string { return "x" }

func TestErrorShape(t *testing.T) {
	base := errors.New("boom")
	e := Wrap(ClassParse, "bad xml", base)
	if !errors.Is(e, base) || !strings.Contains(e.Error(), "parse: bad xml: boom") {
		t.Fatal(e)
	}
	if got := Errorf(ClassAuth, "no %s", "key").Error(); got != "auth: no key" {
		t.Fatal(got)
	}
	if ClassOf(errors.New("x")) != "" {
		t.Fatal("untyped error has no class")
	}
}

func TestSettingsAccessors(t *testing.T) {
	s := Settings{"a": 3, "b": float64(4), "c": "5", "d": "x", "l": []any{"p", 1, "q"}, "one": "z", "m": map[string]string{"k": "v"}}
	if s.Int("a", 0) != 3 || s.Int("b", 0) != 4 || s.Int("c", 0) != 5 || s.Int("d", 9) != 9 || s.Int("zz", 7) != 7 || s.Int("i64", 1) != 1 {
		t.Error("Int")
	}
	s["i64"] = int64(8)
	if s.Int("i64", 1) != 8 {
		t.Error("int64")
	}
	if got := s.Strings("l"); len(got) != 2 || got[1] != "q" {
		t.Error("Strings any", got)
	}
	if got := s.Strings("one"); len(got) != 1 {
		t.Error("Strings single")
	}
	if s.Strings("none") != nil || s.Map("none") != nil {
		t.Error("absent")
	}
	if s.Map("m")["k"] != "v" {
		t.Error("Map")
	}
	s["m2"] = map[string]any{"a": 1}
	if len(s.Map("m2")) != 1 {
		t.Error("Map any")
	}
	s["ss"] = []string{"a"}
	if len(s.Strings("ss")) != 1 {
		t.Error("Strings []string")
	}
	if !(Field{Type: FieldSecret}).IsSecret() || !(Field{Type: FieldList, Secret: true}).IsSecret() || (Field{Type: FieldText}).IsSecret() {
		t.Error("IsSecret")
	}
}
