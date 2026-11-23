package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// DefaultSignatureHeader carries the HMAC-SHA256 of the request body.
const DefaultSignatureHeader = "X-Corsarr-Signature"

// webhookTarget posts a JSON envelope: generic[+http|+https]://host/path with
// optional ?secret=KEY (adds a signature header) and ?signatureHeader=Name.
// Both options are removed from the URL that is called.
type webhookTarget struct {
	endpoint string
	secret   string
	header   string
}

func (*webhookTarget) scheme() string { return "generic" }

func parseWebhook(u *url.URL) (target, error) {
	if u.Host == "" {
		return nil, plugin.Errorf(plugin.ClassConfig, "generic URL must look like generic+https://host/path")
	}
	_, proto, _ := strings.Cut(u.Scheme, "+")
	if proto == "" {
		proto = "https"
	}
	if proto != "http" && proto != "https" {
		return nil, plugin.Errorf(plugin.ClassConfig, "generic supports +http and +https, not +%s", proto)
	}
	q := u.Query()
	t := &webhookTarget{secret: q.Get("secret"), header: q.Get("signatureHeader")}
	if t.header == "" {
		t.header = DefaultSignatureHeader
	}
	q.Del("secret")
	q.Del("signatureHeader")
	out := *u
	out.Scheme, out.RawQuery = proto, q.Encode()
	t.endpoint = out.String()
	return t, nil
}

// Envelope is the JSON body a webhook receives.
type Envelope struct {
	SchemaVersion int          `json:"schemaVersion"`
	Title         string       `json:"title"`
	Body          string       `json:"body"`
	Priority      int          `json:"priority"`
	Event         domain.Event `json:"event"`
}

// Sign returns the signature header value for a body.
func Sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func (t *webhookTarget) send(ctx context.Context, d plugin.HTTPDoer, ev domain.Event, msg plugin.Message) error {
	body, err := json.Marshal(Envelope{SchemaVersion: 1, Title: msg.Title, Body: msg.Body, Priority: msg.Priority, Event: ev})
	if err != nil {
		return plugin.Errorf(plugin.ClassParse, "generic envelope: %s", cause(err))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return plugin.Errorf(plugin.ClassConfig, "generic request: %s", cause(err))
	}
	req.Header.Set("Content-Type", "application/json")
	if t.secret != "" {
		req.Header.Set(t.header, Sign(t.secret, body))
	}
	return deliver(d, "generic", req)
}
