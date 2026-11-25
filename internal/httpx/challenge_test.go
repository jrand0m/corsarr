package httpx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/health"
)

// challengePage is an interstitial written for this test.
const challengePage = `<html><head><title>Just a moment...</title></head><body>wait</body></html>`

func TestAntiBotChallengeIsNotRetriedAndNotAuth(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n.Add(1)
		w.Header().Set("Server", "cloudflare")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, challengePage)
	}))
	defer srv.Close()
	reg := health.New(nil, nil, nil)
	ref := domain.IntegrationRef{Type: "indexer", Key: "cf"}
	resp, err := client(t, reg, &sleeps{}).For(ref).Do(mustReq(t, "GET", srv.URL, ""))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != challengePage {
		t.Errorf("caller lost the body: %q", body)
	}
	if n.Load() != 1 {
		t.Errorf("challenge retried: %d attempts", n.Load())
	}
	if h, _ := reg.Get(ref); h.LastErrorClass != health.ClassChallenge || h.Status != domain.HealthActionNeeded {
		t.Errorf("health %+v", h)
	}
}

func TestPlainForbiddenBehindCloudflareStaysAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Server", "cloudflare")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":"bad api key"}`)
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if o := Classify(resp, nil, time.Now()); o.Class != health.ClassAuth {
		t.Errorf("%+v", o)
	}
	if b, _ := io.ReadAll(resp.Body); !strings.Contains(string(b), "bad api key") {
		t.Errorf("body %q", b)
	}
}
