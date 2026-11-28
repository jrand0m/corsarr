package abi

// KindDownloader is the download-client extension point.
const KindDownloader = "downloader"

// Downloader operations. The first block is required; the rest are optional
// capabilities a module lists in Capabilities.Ops.
const (
	OpDLProtocols      = "downloader.protocols"
	OpDLAdd            = "downloader.add"
	OpDLList           = "downloader.list"
	OpDLRemove         = "downloader.remove"
	OpDLPause          = "downloader.pause"
	OpDLResume         = "downloader.resume"
	OpDLSetPriority    = "downloader.set_priority"
	OpDLEnsureCategory = "downloader.ensure_category"

	OpDLFiles            = "downloader.files"
	OpDLSetPriorityLevel = "downloader.set_priority_level"
	OpDLPauseAll         = "downloader.pause_all"
	OpDLResumeAll        = "downloader.resume_all"
	OpDLSpeedLimits      = "downloader.speed_limits"
	OpDLSetSpeedLimits   = "downloader.set_speed_limits"
	OpDLAltSpeed         = "downloader.alt_speed"
	OpDLSetAltSpeed      = "downloader.set_alt_speed"
	OpDLRecheck          = "downloader.recheck"
	OpDLReannounce       = "downloader.reannounce"
	OpDLSetCategory      = "downloader.set_category"
	OpDLStats            = "downloader.stats"
	OpDLTrackers         = "downloader.trackers"
)

// DownloaderRequired are the operations every downloader module must answer.
var DownloaderRequired = []string{OpTest, OpDLProtocols, OpDLAdd, OpDLList, OpDLRemove, OpDLPause,
	OpDLResume, OpDLSetPriority, OpDLEnsureCategory}

// FlagRemoveKeepsData declares that the client cannot delete a download's
// files when removing it; remove with deleteFiles answers unsupported.
const FlagRemoveKeepsData = "removeKeepsData"

// Payload is what an indexer resolved for a grab.
type Payload struct {
	Kind    string `json:"kind"` // torrent | magnet | nzb | source
	URL     string `json:"url,omitempty"`
	Content []byte `json:"content,omitempty"`
	Name    string `json:"name,omitempty"`
}

// AddRequest starts a payload.
type AddRequest struct {
	Payload  Payload `json:"payload"`
	Category string  `json:"category,omitempty"`
	SavePath string  `json:"savePath,omitempty"`
	Paused   bool    `json:"paused,omitempty"`
}

// AddResponse names the new download.
type AddResponse struct {
	ID string `json:"id"`
}

// ListRequest asks for every download in the categories (empty = all).
type ListRequest struct {
	Categories []string `json:"categories,omitempty"`
}

// Snapshot is one download as the client reports it. State is one of queued,
// downloading, stalled, completed or failed.
type Snapshot struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Category       string   `json:"category,omitempty"`
	State          string   `json:"state"`
	Progress       float64  `json:"progress"`
	Size           int64    `json:"size"`
	Downloaded     int64    `json:"downloaded"`
	Speed          int64    `json:"speed"`
	ETASeconds     int64    `json:"etaSeconds"`
	UpSpeed        int64    `json:"upSpeed,omitempty"`
	Ratio          float64  `json:"ratio,omitempty"`
	Seeders        int      `json:"seeders,omitempty"`
	Peers          int      `json:"peers,omitempty"`
	SavePath       string   `json:"savePath,omitempty"`
	ContentPath    string   `json:"contentPath,omitempty"`
	Files          []string `json:"files,omitempty"`
	LastActivityAt int64    `json:"lastActivityAt,omitempty"` // unix seconds; 0 = unknown
	Error          string   `json:"error,omitempty"`
}

// ListResponse is one batched snapshot.
type ListResponse struct {
	Downloads []Snapshot `json:"downloads"`
}

// IDRequest names one download.
type IDRequest struct {
	ID string `json:"id"`
}

// RemoveRequest removes one download.
type RemoveRequest struct {
	ID          string `json:"id"`
	DeleteFiles bool   `json:"deleteFiles,omitempty"`
}

// PriorityRequest moves a download (top|up|down|bottom) or sets its level
// (low|normal|high|force).
type PriorityRequest struct {
	ID       string `json:"id"`
	Priority string `json:"priority"`
}

// CategoryRequest creates a category or moves a download into one.
type CategoryRequest struct {
	ID       string `json:"id,omitempty"`
	Category string `json:"category"`
	SavePath string `json:"savePath,omitempty"`
}

// ProtocolsResponse lists torrent | usenet | source.
type ProtocolsResponse struct {
	Protocols []string `json:"protocols"`
}

// File is one file of a payload.
type File struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// FilesResponse lists a payload's files.
type FilesResponse struct {
	Files []File `json:"files"`
}

// SpeedLimits are global limits in KiB/s; 0 is unlimited.
type SpeedLimits struct {
	DownKiB int64 `json:"downKiB"`
	UpKiB   int64 `json:"upKiB"`
}

// Toggle is an on/off state.
type Toggle struct {
	On bool `json:"on"`
}

// Stats is the client-wide state.
type Stats struct {
	DownSpeed int64       `json:"downSpeed"`
	UpSpeed   int64       `json:"upSpeed"`
	FreeSpace int64       `json:"freeSpace"`
	Paused    bool        `json:"paused"`
	Limits    SpeedLimits `json:"limits"`
	AltSpeed  bool        `json:"altSpeed"`
}

// Tracker is one tracker of a download.
type Tracker struct {
	URL     string `json:"url"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Seeds   int    `json:"seeds"`
	Peers   int    `json:"peers"`
}

// TrackersResponse lists a download's trackers.
type TrackersResponse struct {
	Trackers []Tracker `json:"trackers"`
}
