package conformance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// Env is a fixed-clock plugin.Env around a test HTTP client.
func Env(h plugin.HTTPDoer) plugin.Env { return testEnv{h} }

type testEnv struct{ h plugin.HTTPDoer }

func (e testEnv) HTTP() plugin.HTTPDoer { return e.h }
func (testEnv) Now() time.Time          { return time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC) }

// ErrorCase is one server behaviour and the failure class Test must report.
type ErrorCase struct {
	Name    string
	Handler http.HandlerFunc
	Want    plugin.FailureClass
}

// Status returns a case where every request answers with the status.
func Status(status int, want plugin.FailureClass) ErrorCase {
	return ErrorCase{Name: http.StatusText(status), Want: want,
		Handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }}
}

// StandardCases are the HTTP failures every network plugin maps identically.
func StandardCases() []ErrorCase {
	return []ErrorCase{
		Status(401, plugin.ClassAuth),
		Status(403, plugin.ClassAuth),
		Status(429, plugin.ClassRateLimited),
		Status(500, plugin.ClassServerError),
		Status(503, plugin.ClassServerError),
		{Name: "hang", Want: plugin.ClassTimeout, Handler: func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
		}},
	}
}

// ErrorMapping runs Test against misbehaving servers and a closed port and
// requires the typed failure class. extra adds plugin-specific cases (parse).
func ErrorMapping(t *testing.T, build func(url string) plugin.Tester, extra ...ErrorCase) {
	t.Helper()
	for _, c := range append(StandardCases(), extra...) {
		t.Run(c.Name, func(t *testing.T) {
			srv := httptest.NewServer(c.Handler)
			defer srv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			if got := plugin.ClassOf(build(srv.URL).Test(ctx)); got != c.Want {
				t.Errorf("class %q, want %q", got, c.Want)
			}
		})
	}
	t.Run("unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if got := plugin.ClassOf(build(url).Test(ctx)); got != plugin.ClassUnreachable {
			t.Errorf("class %q, want unreachable", got)
		}
	})
}
