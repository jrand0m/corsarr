// Package notify holds the built-in notifier plugins: `native`
// (ntfy, pushover, generic webhook, no dependencies) and `shoutrrr` (many
// services through shoutrrr service URLs; excluded by build tag
// nonotify_shoutrrr). Service URLs carry secrets: they never appear in errors.
package notify

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// testEvent is what Test delivers: a harmless, clearly marked event.
func testEvent(now time.Time) domain.Event {
	return domain.Event{SchemaVersion: 1, EventID: "test", Type: domain.EvHealthChanged, OccurredAt: now,
		Actor: "system", Data: map[string]any{"test": true}}
}

var testMessage = plugin.Message{Title: "corsarr", Body: "Test notification from corsarr."}

func urlsField() plugin.Field {
	return plugin.Field{Key: "urls", Type: plugin.FieldList, Secret: true, Required: true, Label: "Service URLs",
		Help: "One service URL per entry. URLs usually contain keys or tokens, so they are stored as secrets and shown masked."}
}

// sendAll delivers to every target; a failing target never blocks the others.
// Errors name the scheme, never the URL.
func sendAll[T any](ctx context.Context, targets []T, send func(context.Context, T) error) error {
	var errs []error
	for _, t := range targets {
		if err := send(ctx, t); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

var urlLike = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://\S+`)

// scrubText removes anything URL-shaped from an error text.
func scrubText(s string) string { return urlLike.ReplaceAllString(s, "<url>") }

// cause returns the innermost error text without the request URL.
func cause(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return scrubText(err.Error())
}

// sanitize keeps a plugin error's class but rebuilds its message so it names
// the scheme and no URL.
func sanitize(scheme string, err error) error {
	class := plugin.ClassOf(err)
	if class == "" {
		class = plugin.ClassServerError
	}
	return plugin.Errorf(class, "%s delivery failed: %s", scheme, cause(err))
}
