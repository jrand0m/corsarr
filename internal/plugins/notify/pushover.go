package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

const pushoverEndpoint = "https://api.pushover.net/1/messages.json"

// pushoverTarget is pushover://shoutrrr:APP_TOKEN@USER_KEY/?devices=a,b&priority=-2..2
// (the same layout shoutrrr uses; the user name part is ignored).
type pushoverTarget struct {
	endpoint string
	token    string
	user     string
	devices  string
	priority string
}

func (*pushoverTarget) scheme() string { return "pushover" }

func parsePushover(u *url.URL) (target, error) {
	token, _ := u.User.Password()
	if token == "" || u.Host == "" {
		return nil, plugin.Errorf(plugin.ClassConfig, "pushover URL must look like pushover://shoutrrr:APP_TOKEN@USER_KEY")
	}
	q := u.Query()
	return &pushoverTarget{endpoint: pushoverEndpoint, token: token, user: u.Host,
		devices: q.Get("devices"), priority: q.Get("priority")}, nil
}

func (t *pushoverTarget) form(msg plugin.Message) url.Values {
	f := url.Values{"token": {t.token}, "user": {t.user}, "message": {msg.Body}}
	if msg.Title != "" {
		f.Set("title", msg.Title)
	}
	if t.devices != "" {
		f.Set("device", t.devices)
	}
	if p, ok := pushoverPriority(msg.Priority); ok {
		f.Set("priority", strconv.Itoa(p))
	} else if t.priority != "" {
		f.Set("priority", t.priority)
	}
	return f
}

func (t *pushoverTarget) send(ctx context.Context, d plugin.HTTPDoer, _ domain.Event, msg plugin.Message) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, strings.NewReader(t.form(msg).Encode()))
	if err != nil {
		return plugin.Errorf(plugin.ClassConfig, "pushover request: %s", cause(err))
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := plugin.Do(d, req, maxReply)
	if err != nil {
		return sanitize("pushover", err)
	}
	return pushoverResult("pushover", resp)
}

type pushoverReply struct {
	Status int      `json:"status"`
	Errors []string `json:"errors"`
}

// pushoverResult reads Pushover's answer: a 400 about the token or user key is an auth failure.
func pushoverResult(scheme string, resp *plugin.Response) error {
	if c := plugin.ClassifyStatus(resp.Status); c != "" {
		return plugin.Errorf(c, "%s service answered HTTP %d", scheme, resp.Status)
	}
	var r pushoverReply
	_ = json.Unmarshal(resp.Body, &r)
	if resp.Status < 300 && r.Status != 0 {
		return nil
	}
	text := scrubText(strings.Join(r.Errors, "; "))
	if resp.Status == http.StatusBadRequest && mentionsCredentials(text) {
		return plugin.Errorf(plugin.ClassAuth, "%s rejected the credentials: %s", scheme, text)
	}
	return plugin.Errorf(plugin.ClassServerError, "%s rejected the message (HTTP %d): %s", scheme, resp.Status, text)
}

func mentionsCredentials(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "invalid") && (strings.Contains(s, "token") || strings.Contains(s, "user"))
}
