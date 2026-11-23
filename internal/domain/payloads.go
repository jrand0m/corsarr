package domain

// Typed `data` payloads of the Event Catalogue. In-process consumers type
// assert ev.Data to these; History and SSE serialise them as JSON.

// ItemEventData is the data of item.added|updated|deleted|available.
type ItemEventData struct {
	Origin       string   `json:"origin,omitempty"` // user | list | import | migration
	Path         string   `json:"path,omitempty"`
	Monitored    bool     `json:"monitored"`
	SearchNow    bool     `json:"searchNow,omitempty"`
	Reason       string   `json:"reason,omitempty"` // item.available: min_availability
	Changed      []string `json:"changed,omitempty"`
	FilesDeleted bool     `json:"filesDeleted,omitempty"`
}

// GrabEventData is the data of grab.sent|failed.
type GrabEventData struct {
	GrabID     int64        `json:"grabId,omitempty"`
	DownloadID int64        `json:"downloadId,omitempty"`
	Release    Release      `json:"release"`
	Decision   Decision     `json:"decision"`
	Origin     SearchOrigin `json:"origin"`
	Downloader ObjectKey    `json:"downloader,omitempty"`
	ClientID   string       `json:"clientId,omitempty"`
	Override   bool         `json:"override,omitempty"`
	Reason     string       `json:"reason,omitempty"`
}

// DownloadEventData is the data of download.*.
type DownloadEventData struct {
	DownloadID   int64           `json:"downloadId"`
	GrabID       int64           `json:"grabId"`
	Downloader   ObjectKey       `json:"downloader"`
	ClientID     string          `json:"clientId"`
	Title        string          `json:"title"`
	From         DownloadState   `json:"from,omitempty"`
	To           DownloadState   `json:"to,omitempty"`
	Actor        Actor           `json:"requestedBy,omitempty"`
	Reason       string          `json:"reason,omitempty"`
	Progress     float64         `json:"progress,omitempty"`
	Speed        int64           `json:"speed,omitempty"`
	ETASeconds   int64           `json:"eta,omitempty"`
	SavePath     string          `json:"savePath,omitempty"`
	ContentPath  string          `json:"contentPath,omitempty"`
	Protocol     Protocol        `json:"protocol,omitempty"`
	Release      ReleaseIdentity `json:"release"`
	FilesDeleted bool            `json:"filesDeleted,omitempty"`
	Cause        string          `json:"cause,omitempty"` // user | external | unsafe_payload | downloader
	Blocklist    bool            `json:"blocklist,omitempty"`
	Research     bool            `json:"research,omitempty"`
	Held         bool            `json:"held,omitempty"` // on hold: automation leaves it alone
}

// Payload stages.
const (
	StagePreDownload  = "pre-download"
	StagePostDownload = "post-download"
)

// PayloadEventData is the data of payload.rejected.
type PayloadEventData struct {
	DownloadID int64           `json:"downloadId"`
	Stage      string          `json:"stage"`
	Verdict    string          `json:"verdict"`
	Files      []string        `json:"files"`
	Release    ReleaseIdentity `json:"release"`
}

// ImportEventData is the data of import.completed|blocked|failed.
type ImportEventData struct {
	Outcome ImportOutcome `json:"outcome"`
	Folders []string      `json:"folders,omitempty"` // item folders that changed (media-server refresh)
}

// SearchEventData is the data of search.completed.
type SearchEventData struct {
	Origin   SearchOrigin `json:"origin"`
	Indexers int          `json:"indexers"`
	Failed   int          `json:"failed"`
	Releases int          `json:"releases"`
	Accepted int          `json:"accepted"`
	Grabbed  string       `json:"grabbed,omitempty"` // release title
}

// FileEventData is the data of file.deleted|renamed.
type FileEventData struct {
	Path        string `json:"path"`
	OldPath     string `json:"oldPath,omitempty"`
	Reason      string `json:"reason,omitempty"`
	RecyclePath string `json:"recyclePath,omitempty"`
}

// BlocklistEventData is the data of blocklist.added|removed.
type BlocklistEventData struct {
	Entry BlocklistEntry `json:"entry"`
}

// RuleEventData is the data of rule.acted|run_completed|undone.
type RuleEventData struct {
	RuleID         string         `json:"ruleId"`
	Mode           string         `json:"mode"`
	ActionID       int64          `json:"actionId,omitempty"`
	Classification string         `json:"classification,omitempty"`
	Evidence       map[string]any `json:"evidence,omitempty"`
	Ops            []string       `json:"ops,omitempty"`
	UndoAvailable  bool           `json:"undoAvailable,omitempty"`
	Counts         map[string]int `json:"counts,omitempty"`
}

// RefreshEventData is the data of refresh.abandoned.
type RefreshEventData struct {
	Server   ObjectKey `json:"server"`
	Paths    []string  `json:"paths"`
	Attempts int       `json:"attempts"`
	Reason   string    `json:"reason"`
}

// ModeEventData is the data of mediakind.mode_changed.
type ModeEventData struct {
	Kind MediaKind `json:"kind"`
	From KindMode  `json:"from"`
	To   KindMode  `json:"to"`
}
