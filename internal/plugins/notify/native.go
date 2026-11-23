package notify

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// NativeID is the id of the dependency-free notifier.
const NativeID = "native"

const maxReply = 64 << 10

var nativeManifest = plugin.Manifest{
	ID: NativeID, Kind: plugin.KindNotifier, Version: "1.0.0", APIVersion: plugin.APIVersion,
	Name: "Built-in notifications", Delivery: "builtin",
	Capabilities: []string{"ntfy", "pushover", "generic"},
	Settings:     []plugin.Field{urlsField()},
}

func init() {
	plugin.Default.MustRegister(plugin.Registration{Manifest: nativeManifest, New: NewNative})
}

// target is one parsed service URL.
type target interface {
	scheme() string
	send(ctx context.Context, d plugin.HTTPDoer, ev domain.Event, msg plugin.Message) error
}

// Native delivers to ntfy, pushover and generic webhook URLs.
type Native struct {
	inst    plugin.Instance
	targets []target
}

// NewNative is the plugin factory: every URL is parsed once, here.
func NewNative(inst plugin.Instance) (any, error) {
	raw := inst.Settings.Strings("urls")
	if len(raw) == 0 {
		return nil, plugin.Errorf(plugin.ClassConfig, "notifier needs at least one service URL")
	}
	n := &Native{inst: inst}
	for i, r := range raw {
		t, err := parseTarget(r)
		if err != nil {
			return nil, plugin.Errorf(plugin.ClassConfig, "urls[%d]: %v", i, err)
		}
		n.targets = append(n.targets, t)
	}
	return n, nil
}

// Schemes lists the service schemes the native notifier implements.
func (n *Native) Schemes() []string { return []string{"ntfy", "pushover", "generic"} }

// Test sends a test notification to every URL.
func (n *Native) Test(ctx context.Context) error {
	return n.Send(ctx, testEvent(n.inst.Env.Now()), testMessage)
}

// Send delivers to every URL; one failing URL does not stop the others.
func (n *Native) Send(ctx context.Context, ev domain.Event, msg plugin.Message) error {
	return sendAll(ctx, n.targets, func(ctx context.Context, t target) error {
		return t.send(ctx, n.inst.Env.HTTP(), ev, msg)
	})
}

func parseTarget(raw string) (target, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, plugin.Errorf(plugin.ClassConfig, "not a valid service URL")
	}
	base, _, _ := strings.Cut(u.Scheme, "+")
	switch base {
	case "ntfy":
		return parseNtfy(u)
	case "pushover":
		return parsePushover(u)
	case "generic":
		return parseWebhook(u)
	}
	return nil, plugin.Errorf(plugin.ClassConfig, "unsupported scheme %q (native supports ntfy, pushover, generic)", u.Scheme)
}

// deliver runs a request and maps the outcome to a failure class. The error
// text names the scheme only, never the URL.
func deliver(d plugin.HTTPDoer, scheme string, req *http.Request) error {
	resp, err := plugin.Do(d, req, maxReply)
	if err != nil {
		return sanitize(scheme, err)
	}
	if c := plugin.ClassifyStatus(resp.Status); c != "" {
		return plugin.Errorf(c, "%s service answered HTTP %d", scheme, resp.Status)
	}
	if resp.Status >= 300 {
		return plugin.Errorf(plugin.ClassServerError, "%s service rejected the message (HTTP %d)", scheme, resp.Status)
	}
	return nil
}

// ntfyPriority maps Message.Priority (1 min … 5 urgent, 0 unset) to ntfy's scale.
func ntfyPriority(p int) (string, bool) {
	if p < 1 || p > 5 {
		return "", false
	}
	return string(rune('0' + p)), true
}

// pushoverPriority maps Message.Priority onto pushover's -2…2 around the normal level 3.
func pushoverPriority(p int) (int, bool) {
	if p < 1 || p > 5 {
		return 0, false
	}
	return p - 3, true
}
