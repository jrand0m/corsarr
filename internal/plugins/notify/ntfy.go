package notify

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// ntfyTarget publishes to ntfy: ntfy://[user:pass@]host[:port]/topic
// with optional ?priority=1-5&tags=a,b&token=…&scheme=http.
type ntfyTarget struct {
	endpoint string // https://host[:port]/topic
	user     *url.Userinfo
	token    string
	priority string
	tags     string
}

func (*ntfyTarget) scheme() string { return "ntfy" }

func parseNtfy(u *url.URL) (target, error) {
	topic := strings.Trim(u.Path, "/")
	if u.Host == "" || topic == "" || strings.Contains(topic, "/") {
		return nil, plugin.Errorf(plugin.ClassConfig, "ntfy URL must look like ntfy://host/topic")
	}
	q := u.Query()
	proto := "https"
	if q.Get("scheme") == "http" || q.Get("disabletls") == "yes" {
		proto = "http"
	}
	t := &ntfyTarget{endpoint: proto + "://" + u.Host + "/" + url.PathEscape(topic), user: u.User,
		token: q.Get("token"), priority: q.Get("priority"), tags: q.Get("tags")}
	return t, nil
}

func (t *ntfyTarget) send(ctx context.Context, d plugin.HTTPDoer, _ domain.Event, msg plugin.Message) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, strings.NewReader(msg.Body))
	if err != nil {
		return plugin.Errorf(plugin.ClassConfig, "ntfy request: %s", cause(err))
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	if msg.Title != "" {
		req.Header.Set("Title", msg.Title)
	}
	if p, ok := ntfyPriority(msg.Priority); ok {
		req.Header.Set("Priority", p)
	} else if t.priority != "" {
		req.Header.Set("Priority", t.priority)
	}
	if t.tags != "" {
		req.Header.Set("Tags", t.tags)
	}
	t.authorize(req)
	return deliver(d, "ntfy", req)
}

func (t *ntfyTarget) authorize(req *http.Request) {
	switch {
	case t.token != "":
		req.Header.Set("Authorization", "Bearer "+t.token)
	case t.user != nil:
		pw, _ := t.user.Password()
		req.SetBasicAuth(t.user.Username(), pw)
	}
}
