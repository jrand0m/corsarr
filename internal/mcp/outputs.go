package mcp

import (
	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/music"
	"github.com/jrand0m/corsarr/internal/musicauto"
	"github.com/jrand0m/corsarr/internal/queue"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/tv"
	"github.com/jrand0m/corsarr/internal/tvauto"
)

// The structured results of the tools. Every tool declares the schema of
// its result type as outputSchema.

// ActionOut answers a download or client control action.
type ActionOut struct {
	Action   string `json:"action"`
	Sent     bool   `json:"sent"`
	Recorded bool   `json:"recorded,omitempty"` // shadow mode: recorded instead of sent
	Reason   string `json:"reason,omitempty"`
	On       *bool  `json:"on,omitempty"` // toggle_alt_speed: whether the alternative limits are on
}

// DownloadersOut lists the download clients.
type DownloadersOut struct {
	Downloaders []queue.ClientView `json:"downloaders"`
}

// TestOut is a passed client test.
type TestOut struct {
	Downloader string `json:"downloader"`
	OK         bool   `json:"ok"`
}

// GrabOut answers a manual grab.
type GrabOut struct {
	Sent     bool            `json:"sent"`
	Recorded bool            `json:"recorded,omitempty"`
	Grab     *domain.Grab    `json:"grab,omitempty"`
	Release  *domain.Release `json:"release,omitempty"` // the release recorded in shadow mode
	Decision domain.Decision `json:"decision"`
}

// AddURLOut answers add_download_url.
type AddURLOut struct {
	Sent     bool         `json:"sent"`
	Recorded bool         `json:"recorded,omitempty"`
	Grab     *domain.Grab `json:"grab,omitempty"`
	Items    int          `json:"items"`
}

// MonitoredOut answers set_monitored.
type MonitoredOut struct {
	Kind      domain.MediaKind `json:"kind"`
	Monitored bool             `json:"monitored"`
	Changed   string           `json:"changed"`
}

// StartedOut answers a command started in the background.
type StartedOut struct {
	Started bool             `json:"started"`
	Kind    domain.MediaKind `json:"kind"`
	Mode    domain.KindMode  `json:"mode,omitempty"`
	ID      int64            `json:"id,omitempty"`
}

// MovieItemOut is get_item for a movie.
type MovieItemOut struct {
	Movie domain.Movie         `json:"movie"`
	Files []domain.LibraryFile `json:"files"`
}

// SeriesItemOut is get_item for a series, with one season's episodes when
// asked.
type SeriesItemOut struct {
	Series   tv.Summary       `json:"series"`
	Seasons  []tv.SeasonView  `json:"seasons"`
	Episodes []tv.EpisodeView `json:"episodes,omitempty"`
}

// CalendarOut is get_calendar.
type CalendarOut struct {
	From     string            `json:"from"`
	To       string            `json:"to"`
	Episodes []tv.CalendarItem `json:"episodes"`
	Movies   []MovieDate       `json:"movies"`
}

// HealthOut is get_health.
type HealthOut struct {
	Integrations []domain.IntegrationHealth           `json:"integrations"`
	RootFolders  []domain.RootFolder                  `json:"rootFolders"`
	Modes        map[domain.MediaKind]domain.KindMode `json:"modes"`
}

// ReleasesOut is search_releases: each release with its decision.
type ReleasesOut struct {
	Summary  automation.Summary `json:"summary"`
	Releases []any              `json:"releases"` // automation.Decided
	Omitted  int                `json:"omitted"`
	Errors   []string           `json:"errors"`
}

// ProfilesOut is list_profiles.
type ProfilesOut struct {
	Profiles         []ProfileView            `json:"profiles"`
	MetadataProfiles []domain.MetadataProfile `json:"metadataProfiles,omitempty"`
}

// RootFoldersOut is list_root_folders.
type RootFoldersOut struct {
	RootFolders []domain.RootFolder `json:"rootFolders"`
}

// FindOut is find_media.
type FindOut struct {
	Candidates []Candidate `json:"candidates"`
	Notes      []string    `json:"notes,omitempty"`
}

// CancelOut is cancel_request.
type CancelOut struct {
	Status           Status `json:"status"`
	RemovedDownloads int    `json:"removedDownloads"`
}

// toolOutputs are the output schemas by tool.
func toolOutputs() map[string]schema {
	action, status := outputOf[ActionOut](), outputOf[Status]()
	out := map[string]schema{
		"find_media": outputOf[FindOut](), "request_movie": status, "request_show": status, "request_music": status,
		"request_book": status, "request_status": status, "cancel_request": outputOf[CancelOut](),
		"search_library": outputOf[page[LibraryItem]](), "get_item": itemOutput(), "list_wanted": wantedOutput(),
		"get_calendar": outputOf[CalendarOut](), "get_history": outputOf[page[store.HistoryEntry]](),
		"get_health": outputOf[HealthOut](), "search_releases": releasesOutput(), "list_profiles": outputOf[ProfilesOut](),
		"list_root_folders": outputOf[RootFoldersOut](), "set_monitored": outputOf[MonitoredOut](),
		"start_search": outputOf[StartedOut](), "rescan_item": outputOf[StartedOut](),
		"list_downloaders": outputOf[DownloadersOut](), "get_queue": outputOf[page[store.StoredDownload]](),
		"get_download": outputOf[queue.Detail](), "test_downloader": outputOf[TestOut](),
		"grab_release": outputOf[GrabOut](), "add_download_url": outputOf[AddURLOut](),
	}
	for _, name := range []string{"set_download_priority", "set_download_category", "remove_download", "set_speed_limits",
		"toggle_alt_speed", "pause_download", "resume_download", "recheck_download", "reannounce_download", "retry_import",
		"pause_all", "resume_all"} {
		out[name] = action
	}
	return out
}

func itemOutput() schema {
	return oneOfOutputs(outputOf[MovieItemOut](), outputOf[SeriesItemOut](), outputOf[music.Detail](), outputOf[music.AlbumDetail]())
}

// wantedOutput is a page of the kind's wanted entries.
func wantedOutput() schema {
	s := outputOf[page[any]]()
	s["properties"].(map[string]schema)["items"] = schema{"type": "array", "items": schema{"anyOf": []any{
		outputOf[automation.Entry](), outputOf[tvauto.Entry](), outputOf[musicauto.Entry]()}}}
	return s
}

func releasesOutput() schema {
	s := outputOf[ReleasesOut]()
	s["properties"].(map[string]schema)["releases"] = schema{"type": "array", "items": outputOf[automation.Decided]()}
	return s
}
