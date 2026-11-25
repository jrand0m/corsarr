package plugin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func hdr(kv ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(kv); i += 2 {
		h.Set(kv[i], kv[i+1])
	}
	return h
}

// Pages written for these tests; they carry the markers real interstitials use.
const (
	waitPage    = `<!DOCTYPE html><html><head><title>Just a moment...</title></head><body><script src="/cdn-cgi/challenge-platform/h/b/orchestrate/x.js"></script></body></html>`
	deniedPage  = `<html><head><title>Access denied</title></head><body>error code: 1020</body></html>`
	guardPage   = `<html><head><title>DDoS-Guard</title></head><body>checking</body></html>`
	wafPage     = `<html><head><title>Attention Required! | Cloudflare</title></head><body>blocked</body></html>`
	loginPage   = `<html><body><form action="/login"><input name="user"></form></body></html>`
	strongNoHdr = `<html><body><script>window._cf_chl_opt={cType:'managed'};</script></body></html>`
)

func TestChallenged(t *testing.T) {
	cf := hdr("Server", "cloudflare")
	cases := []struct {
		name   string
		status int
		h      http.Header
		body   string
		want   bool
	}{
		{"cloudflare wait page", 503, cf, waitPage, true},
		{"cloudflare wait page on 403", 403, cf, waitPage, true},
		{"cloudflare access denied", 403, cf, deniedPage, true},
		{"cloudflare WAF block", 403, cf, wafPage, true},
		{"ddos-guard", 403, hdr("Server", "ddos-guard"), guardPage, true},
		{"mitigated header alone", 403, hdr("Cf-Mitigated", "challenge"), "", true},
		{"challenge script without vendor header", 403, http.Header{}, strongNoHdr, true},
		{"plain forbidden behind cloudflare", 403, cf, loginPage, false},
		{"plain forbidden", 403, http.Header{}, loginPage, false},
		{"wait page title without vendor header", 503, http.Header{}, waitPage[:60], false},
		{"ok page behind cloudflare", 200, cf, waitPage, false},
		{"server error behind cloudflare", 502, cf, "bad gateway", false},
	}
	for _, c := range cases {
		if got := Challenged(c.status, c.h, []byte(c.body)); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestClassifyResponse(t *testing.T) {
	cf := hdr("Server", "cloudflare")
	if c := ClassifyResponse(403, cf, []byte(waitPage)); c != ClassChallenge {
		t.Errorf("challenge: %s", c)
	}
	if c := ClassifyResponse(403, cf, []byte(loginPage)); c != ClassAuth {
		t.Errorf("plain 403: %s", c)
	}
	if c := ClassifyResponse(200, cf, nil); c != "" {
		t.Errorf("ok: %s", c)
	}
}

func TestChallengeErrorNamesTheVendor(t *testing.T) {
	for server, want := range map[string]string{"cloudflare": "Cloudflare", "ddos-guard": "DDoS-Guard", "nginx": "anti-bot protection"} {
		err := ChallengeError(403, hdr("Server", server))
		if ClassOf(err) != ClassChallenge || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "blocked") {
			t.Errorf("%s: %v", server, err)
		}
	}
}

func TestFetchReportsAChallengeNotAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Server", "cloudflare")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(waitPage))
	}))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api?apikey=secret", nil)
	_, err := Fetch(http.DefaultClient, req, 1<<20)
	if ClassOf(err) != ClassChallenge || strings.Contains(err.Error(), "secret") {
		t.Errorf("%v", err)
	}
}
