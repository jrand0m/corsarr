package plugin

import (
	"bytes"
	"net/http"
	"strings"
)

// ChallengeScan is how much of a body is searched for challenge markers.
const ChallengeScan = 64 << 10

// vendors map a Server header token to the protection's display name.
var vendors = []struct{ token, name string }{
	{"cloudflare", "Cloudflare"},
	{"ddos-guard", "DDoS-Guard"},
}

// pageMarkers appear on interstitial and block pages; any one is enough when
// the Server header names a protection vendor.
var pageMarkers = []string{
	"<title>just a moment",
	"<title>attention required! | cloudflare",
	"<title>ddos-guard</title>",
	"error code: 1020",
	"cf-browser-verification",
	"checking your browser before accessing",
}

// scriptMarkers are the challenge scripts themselves; they identify a
// challenge even when a proxy has rewritten the Server header.
var scriptMarkers = []string{
	"/cdn-cgi/challenge-platform/",
	"_cf_chl_opt",
	"ddos-guard.net/",
}

// vendor returns the protection named by the Server header ("" when none).
func vendor(h http.Header) string {
	server := strings.ToLower(h.Get("Server"))
	for _, v := range vendors {
		if strings.Contains(server, v.token) {
			return v.name
		}
	}
	return ""
}

// anyIn reports whether the lowercased head of body contains one of markers.
func anyIn(body []byte, markers []string) bool {
	head := bytes.ToLower(body[:min(len(body), ChallengeScan)])
	for _, m := range markers {
		if bytes.Contains(head, []byte(m)) {
			return true
		}
	}
	return false
}

// Challenged reports whether a response is an anti-bot interstitial or block
// page (Cloudflare, DDoS-Guard) rather than the site's own answer: the
// protection says so in a header, or a 403/503 carries a challenge page.
func Challenged(status int, h http.Header, body []byte) bool {
	if h.Get("Cf-Mitigated") == "challenge" {
		return true
	}
	if status != http.StatusForbidden && status != http.StatusServiceUnavailable {
		return false
	}
	if vendor(h) != "" && anyIn(body, pageMarkers) {
		return true
	}
	return anyIn(body, scriptMarkers)
}

// ClassifyResponse is ClassifyStatus that recognises anti-bot pages first,
// so a challenge is never mistaken for rejected credentials.
func ClassifyResponse(status int, h http.Header, body []byte) FailureClass {
	if Challenged(status, h, body) {
		return ClassChallenge
	}
	return ClassifyStatus(status)
}

// ChallengeError describes a response blocked by anti-bot protection.
func ChallengeError(status int, h http.Header) *Error {
	name := vendor(h)
	if name == "" {
		name = "anti-bot protection"
	}
	return Errorf(ClassChallenge, "the site is blocked by %s (HTTP %d); this is not a credentials problem", name, status)
}

// StatusError returns the classified error for a response that is not a
// success at this level (nil otherwise); what names the request in the message.
func StatusError(what string, status int, h http.Header, body []byte) error {
	switch c := ClassifyResponse(status, h, body); c {
	case "":
		return nil
	case ClassChallenge:
		return ChallengeError(status, h)
	default:
		return Errorf(c, "%s returned HTTP %d", what, status)
	}
}
