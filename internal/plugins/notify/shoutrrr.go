//go:build !nonotify_shoutrrr

package notify

import (
	"context"
	"errors"
	"net"
	"regexp"

	"github.com/nicholas-fedor/shoutrrr"
	"github.com/nicholas-fedor/shoutrrr/pkg/router"
	"github.com/nicholas-fedor/shoutrrr/pkg/services"
	"github.com/nicholas-fedor/shoutrrr/pkg/types"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// ShoutrrrID is the id of the shoutrrr-backed notifier. It is compiled in by
// default; lean builds drop it with `-tags nonotify_shoutrrr`.
const ShoutrrrID = "shoutrrr"

func init() {
	plugin.Default.MustRegister(plugin.Registration{Manifest: shoutrrrManifest(), New: NewShoutrrr})
}

func shoutrrrManifest() plugin.Manifest {
	return plugin.Manifest{
		ID: ShoutrrrID, Kind: plugin.KindNotifier, Version: "1.0.0", APIVersion: plugin.APIVersion,
		Name: "Notification services (shoutrrr)", Delivery: "builtin",
		Capabilities: services.SupportedSchemas(),
		Settings:     []plugin.Field{urlsField()},
	}
}

// Shoutrrr delivers through shoutrrr service URLs over the shared HTTP client.
type Shoutrrr struct {
	inst   plugin.Instance
	router *router.ServiceRouter
}

// NewShoutrrr is the plugin factory: the URLs are parsed and validated once, here.
func NewShoutrrr(inst plugin.Instance) (any, error) {
	urls := inst.Settings.Strings("urls")
	if len(urls) == 0 {
		return nil, plugin.Errorf(plugin.ClassConfig, "notifier needs at least one service URL")
	}
	r, err := shoutrrr.CreateSenderWithOptions(types.SenderOptions{HTTPClient: inst.Env.HTTP()}, urls...)
	if err != nil {
		return nil, plugin.Errorf(plugin.ClassConfig, "service URLs: %s", scrubText(err.Error()))
	}
	return &Shoutrrr{inst: inst, router: r}, nil
}

// Schemes lists every service scheme this build supports.
func (*Shoutrrr) Schemes() []string { return services.SupportedSchemas() }

// Test sends a test notification to every URL.
func (s *Shoutrrr) Test(ctx context.Context) error {
	return s.Send(ctx, testEvent(s.inst.Env.Now()), testMessage)
}

// Send fans out to every URL; shoutrrr delivers to the others when one fails.
func (s *Shoutrrr) Send(ctx context.Context, _ domain.Event, msg plugin.Message) error {
	params := types.Params{}
	if msg.Title != "" {
		params.SetTitle(msg.Title)
	}
	var errs []error
	for _, err := range s.router.SendContext(ctx, msg.Body, &params) {
		if err != nil {
			errs = append(errs, classifyShoutrrr(err))
		}
	}
	return errors.Join(errs...)
}

var (
	reAuth    = regexp.MustCompile(`\b(401|403)\b|unauthori[sz]ed|forbidden|invalid (token|credentials|api key)`)
	reLimited = regexp.MustCompile(`\b429\b|too many requests|rate limit`)
	reServer  = regexp.MustCompile(`\b5\d\d\b`)
)

// classifyShoutrrr maps a shoutrrr error to a failure class without leaking
// the service URL it may contain.
func classifyShoutrrr(err error) error {
	var ne net.Error
	class := plugin.ClassServerError
	text := err.Error()
	switch {
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		class = plugin.ClassTimeout
	case errors.As(err, &ne):
		class = plugin.ClassUnreachable
	case reAuth.MatchString(text):
		class = plugin.ClassAuth
	case reLimited.MatchString(text):
		class = plugin.ClassRateLimited
	case reServer.MatchString(text):
		class = plugin.ClassServerError
	}
	return plugin.Errorf(class, "shoutrrr delivery failed: %s", scrubText(text))
}
