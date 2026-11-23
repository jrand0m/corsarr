// Package notify delivers notifications and media-server refreshes:
// event → event groups → per-connection toggles → rendered short message →
// outbox row (idempotent per event and connection) → delivery with jittered
// exponential backoff; import and file events queue partial library
// refreshes merged over a window and persisted across restarts.
package notify

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/msgtemplate"
)

// DefaultGroups are the toggles of a connection that names none.
var DefaultGroups = []string{"grab", "import", "upgrade", "downloadFailed", "importFailed", "manualAction"}

var groupsByType = map[domain.EventType][]string{
	domain.EvGrabSent: {"grab"}, domain.EvImportCompleted: {"import"}, domain.EvFileRenamed: {"rename"},
	domain.EvFileDeleted: {"delete"}, domain.EvItemAdded: {"itemAdded"}, domain.EvItemDeleted: {"itemDeleted"},
	domain.EvDownloadFailed: {"downloadFailed"}, domain.EvGrabFailed: {"downloadFailed"},
	domain.EvImportFailed: {"importFailed"}, domain.EvPayloadRejected: {"importFailed"},
	domain.EvImportBlocked: {"manualAction"}, domain.EvHealthChanged: {"health"}, domain.EvRefreshAbandoned: {"health"},
	domain.EvRuleRunCompleted: {"automation"}, domain.EvConfigApplied: {"config"},
	domain.EvConfigApplyFailed: {"config"}, domain.EvConfigDrifted: {"config"},
}

// Groups maps an event onto notification groups.
func Groups(ev domain.Event) []string {
	out := append([]string(nil), groupsByType[ev.Type]...)
	if d, ok := ev.Data.(domain.ImportEventData); ok && upgraded(d.Outcome) {
		out = append(out, "upgrade")
	}
	return out
}

func upgraded(o domain.ImportOutcome) bool {
	for _, it := range o.Items {
		if it.Outcome == "upgraded" {
			return true
		}
	}
	return false
}

// Wants reports whether a connection with these toggles takes the event.
func Wants(toggles []string, ev domain.Event) bool {
	if len(toggles) == 0 {
		toggles = DefaultGroups
	}
	on := map[string]bool{}
	for _, g := range toggles {
		on[g] = true
	}
	for _, g := range Groups(ev) {
		if on[g] {
			return true
		}
	}
	return false
}

// templates are the short default title/body per event.
var templates = map[domain.EventType][2]string{
	domain.EvGrabSent:        {"Grabbed: {{.Items}}", "{{.Grab.Release.Title}} ({{.Grab.Decision.Quality}}) sent to {{.Grab.Downloader}}."},
	domain.EvImportCompleted: {"{{if .Upgraded}}Upgraded{{else}}Imported{{end}}: {{.Items}}", "{{range .Import.Outcome.Items}}{{range .NewPaths}}{{.}}{{end}}{{end}}"},
	domain.EvImportBlocked:   {"Manual action needed: {{.Items}}", "Import blocked ({{.Import.Outcome.ReasonCode}})."},
	domain.EvImportFailed:    {"Import failed: {{.Items}}", "Import failed ({{.Import.Outcome.ReasonCode}})."},
	domain.EvDownloadFailed:  {"Download failed: {{.Items}}", "{{.Download.Title}}: {{.Download.Reason}}"},
	domain.EvGrabFailed:      {"Grab failed: {{.Items}}", "{{.Grab.Release.Title}}: {{.Grab.Reason}}"},
	domain.EvPayloadRejected: {"Unsafe download removed: {{.Items}}", "Dangerous files: {{join .Payload.Files}}"},
}

var parsed = func() map[domain.EventType][2]*template.Template {
	out := map[domain.EventType][2]*template.Template{}
	for t, pair := range templates {
		title, err1 := msgtemplate.Parse("t", pair[0])
		body, err2 := msgtemplate.Parse("b", pair[1])
		if err1 != nil || err2 != nil {
			panic(fmt.Sprint("built-in template: ", err1, err2))
		}
		out[t] = [2]*template.Template{title, body}
	}
	return out
}()

// view is what templates see. Title and Body are the built-in rendering, so
// a connection's own template can wrap or replace it.
type view struct {
	Event    domain.EventType
	Items    string
	Upgraded bool
	Title    string
	Body     string
	Grab     domain.GrabEventData
	Import   domain.ImportEventData
	Download domain.DownloadEventData
	Payload  domain.PayloadEventData
}

func viewOf(ev domain.Event) view {
	v := view{Event: ev.Type, Items: domain.ItemsTitle(ev.Items)}
	switch d := ev.Data.(type) {
	case domain.GrabEventData:
		v.Grab = d
	case domain.ImportEventData:
		v.Import, v.Upgraded = d, upgraded(d.Outcome)
	case domain.DownloadEventData:
		v.Download = d
	case domain.PayloadEventData:
		v.Payload = d
	}
	return v
}

func titles(items []domain.EventItem) string {
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Title)
	}
	return strings.Join(names, ", ")
}

// Render builds the short built-in title and body for an event.
func Render(ev domain.Event) (string, string) {
	tpl, ok := parsed[ev.Type]
	if !ok {
		return string(ev.Type), titles(ev.Items)
	}
	v := viewOf(ev)
	return msgtemplate.Exec(tpl[0], v), msgtemplate.Exec(tpl[1], v)
}

// Template is a connection's own title/body template; nil parts keep the
// built-in text.
type Template struct {
	Title *template.Template
	Body  *template.Template
}

// ParseTemplate parses a connection's template override (either part may be empty).
func ParseTemplate(title, body string) (Template, error) {
	var t Template
	var err error
	if title != "" {
		if t.Title, err = msgtemplate.Parse("title", title); err != nil {
			return t, err
		}
	}
	if body != "" {
		t.Body, err = msgtemplate.Parse("body", body)
	}
	return t, err
}

// RenderWith renders an event through a connection's template; the built-in
// title and body are available to it as .Title and .Body.
func RenderWith(t Template, ev domain.Event) (string, string) {
	title, body := Render(ev)
	v := viewOf(ev)
	v.Title, v.Body = title, body
	if t.Title != nil {
		title = msgtemplate.Exec(t.Title, v)
	}
	if t.Body != nil {
		body = msgtemplate.Exec(t.Body, v)
	}
	return title, body
}
