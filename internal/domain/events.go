package domain

import "time"

// EventType names a domain event: `<entity>.<verb>`, past tense, media kind
// as a field.
type EventType string

const (
	EvItemAdded         EventType = "item.added"
	EvItemUpdated       EventType = "item.updated"
	EvItemDeleted       EventType = "item.deleted"
	EvItemAvailable     EventType = "item.available"
	EvSearchCompleted   EventType = "search.completed"
	EvGrabSent          EventType = "grab.sent"
	EvGrabFailed        EventType = "grab.failed"
	EvDecisionShadowed  EventType = "decision.shadowed"
	EvDownloadProgress  EventType = "download.progress"
	EvDownloadState     EventType = "download.state_changed"
	EvDownloadCompleted EventType = "download.completed"
	EvDownloadFailed    EventType = "download.failed"
	EvDownloadRemoved   EventType = "download.removed"
	EvImportCompleted   EventType = "import.completed"
	EvImportBlocked     EventType = "import.blocked"
	EvImportFailed      EventType = "import.failed"
	EvPayloadRejected   EventType = "payload.rejected"
	EvFileRenamed       EventType = "file.renamed"
	EvFileDeleted       EventType = "file.deleted"
	EvBlocklistAdded    EventType = "blocklist.added"
	EvBlocklistRemoved  EventType = "blocklist.removed"
	EvRuleActed         EventType = "rule.acted"
	EvRuleRunCompleted  EventType = "rule.run_completed"
	EvRuleUndone        EventType = "rule.undone"
	EvHealthChanged     EventType = "health.changed"
	EvRefreshAbandoned  EventType = "refresh.abandoned" // a media-server refresh gave up after its retries
	EvConfigApplied     EventType = "config.applied"
	EvConfigApplyFailed EventType = "config.apply_failed"
	EvConfigDrifted     EventType = "config.drifted"
	EvMediaKindMode     EventType = "mediakind.mode_changed"
	EvPluginLoaded      EventType = "plugin.loaded"
	EvJobUpdated        EventType = "job.updated"
	EvCommandUpdated    EventType = "command.updated"
	EvMigrationProgress EventType = "migration.progress"
	EvMigrationComplete EventType = "migration.run_completed"
)

// eventFlags records which events go to History (H) and SSE (S).
var eventFlags = map[EventType]struct{ History, Stream bool }{
	EvItemAdded: {true, true}, EvItemUpdated: {false, true}, EvItemDeleted: {true, true},
	EvItemAvailable: {false, true}, EvSearchCompleted: {false, true},
	EvGrabSent: {true, true}, EvGrabFailed: {true, true}, EvDecisionShadowed: {true, true},
	EvDownloadProgress: {false, true}, EvDownloadState: {false, true},
	EvDownloadCompleted: {false, true}, EvDownloadFailed: {true, true}, EvDownloadRemoved: {true, true},
	EvImportCompleted: {true, true}, EvImportBlocked: {true, true}, EvImportFailed: {true, true},
	EvPayloadRejected: {true, true}, EvFileRenamed: {true, true}, EvFileDeleted: {true, true},
	EvBlocklistAdded: {true, true}, EvBlocklistRemoved: {true, true},
	EvRuleActed: {true, true}, EvRuleRunCompleted: {false, true}, EvRuleUndone: {true, true},
	EvHealthChanged: {true, true}, EvRefreshAbandoned: {true, true}, EvConfigApplied: {true, true}, EvConfigApplyFailed: {true, true},
	EvConfigDrifted: {true, true}, EvMediaKindMode: {true, true}, EvPluginLoaded: {false, true},
	EvJobUpdated: {false, true}, EvCommandUpdated: {false, true},
	EvMigrationProgress: {false, true}, EvMigrationComplete: {true, true},
}

// Persisted reports whether the event belongs in History.
func (t EventType) Persisted() bool { return eventFlags[t].History }

// Streamed reports whether the event is sent on the SSE stream.
func (t EventType) Streamed() bool { return eventFlags[t].Stream }

// Known reports whether the type is in the catalogue.
func (t EventType) Known() bool { _, ok := eventFlags[t]; return ok }

// EventItem is an item snapshot carried by an event.
type EventItem struct {
	Ref    ItemRef  `json:"ref"`
	Title  string   `json:"title"`
	Tags   []string `json:"tags,omitempty"`
	Series string   `json:"series,omitempty"` // TV: the show an episode belongs to
}

// Event is the envelope shared by History, SSE and the webhook notifier.
type Event struct {
	SchemaVersion int         `json:"schemaVersion"`
	EventID       string      `json:"eventId"`
	Type          EventType   `json:"type"`
	OccurredAt    time.Time   `json:"occurredAt"`
	Actor         string      `json:"actor"` // user|automation|plugin:<id>|system
	Imported      bool        `json:"imported,omitempty"`
	Kind          MediaKind   `json:"mediaKind,omitempty"`
	Items         []EventItem `json:"items,omitempty"`
	Data          any         `json:"data,omitempty"`
}

// HealthStatus is an integration's health state.
type HealthStatus string

const (
	HealthOK           HealthStatus = "ok"
	HealthDegraded     HealthStatus = "degraded"
	HealthBackedOff    HealthStatus = "backed_off"
	HealthFailing      HealthStatus = "failing"
	HealthActionNeeded HealthStatus = "action_needed"
	HealthDisabled     HealthStatus = "disabled"
)

// IntegrationRef identifies something whose health is tracked.
type IntegrationRef struct {
	Type string    `json:"type"` // indexer|downloader|metadata|mediaserver|notifier|rootFolder|disk|config|plugin|scheduler
	Key  ObjectKey `json:"key"`
}

// IntegrationHealth is the shared health record.
type IntegrationHealth struct {
	Ref                 IntegrationRef `json:"ref"`
	Status              HealthStatus   `json:"status"`
	Since               time.Time      `json:"since"`
	LastOKAt            *time.Time     `json:"lastOkAt,omitempty"`
	LastErrorClass      string         `json:"lastErrorClass,omitempty"`
	LastErrorMessage    string         `json:"lastErrorMessage,omitempty"`
	ConsecutiveFailures int            `json:"consecutiveFailures"`
	NextRetryAt         *time.Time     `json:"nextRetryAt,omitempty"`
	FixHint             string         `json:"fixHint,omitempty"`
}
