package plugin

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// HTTPDoer is the shared outbound client every plugin must use.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// ── indexer ──────────────────────────────────────────────────────────────────

// Caps is what an indexer supports.
type Caps struct {
	Modes      []domain.SearchMode `json:"modes"`
	IDParams   []string            `json:"idParams"` // imdbid, tmdbid, tvdbid, …
	Categories map[int]string      `json:"categories"`
	Limit      int                 `json:"limit"`
}

// Payload is what a downloader receives after an indexer resolves a grab.
type Payload struct {
	Kind    string // torrent|magnet|nzb|source
	URL     string // magnet or fetchable URL (no credentials leak beyond the indexer)
	Content []byte // .torrent / .nzb bytes when fetched by the indexer
	Name    string
}

// Indexer finds releases (Torznab/Newznab/definition/native trackers).
type Indexer interface {
	Tester
	Caps(ctx context.Context) (Caps, error)
	Search(ctx context.Context, req domain.SearchRequest) ([]domain.Release, error)
	// Feed returns releases newer than the cursor (RSS) and the new cursor.
	Feed(ctx context.Context, kind domain.MediaKind, cursor string) ([]domain.Release, string, error)
	// Resolve fetches the grab payload with the indexer's own credentials.
	Resolve(ctx context.Context, rel domain.Release) (Payload, error)
}

// ── downloader ───────────────────────────────────────────────────────────────

// Snapshot is one download as the client reports it.
type Snapshot struct {
	DownloadID     string
	Name           string
	Category       string
	State          domain.DownloadState // client view mapped onto queue states
	Progress       float64
	Size           int64
	Downloaded     int64
	Speed          int64
	ETASeconds     int64
	UpSpeed        int64   // bytes/s sent to peers; 0 for clients that do not upload
	Ratio          float64 // uploaded / downloaded as the client reports it; 0 when it has none
	Seeders        int
	Peers          int
	SavePath       string
	ContentPath    string // the payload's file or root folder (qBittorrent content_path)
	Files          []string
	LastActivityAt *time.Time
	Error          string
}

// AddRequest asks a downloader to start a payload.
type AddRequest struct {
	Payload  Payload
	Category string
	SavePath string
	Paused   bool
}

// Priority moves a download in the client queue.
type Priority string

const (
	PrioTop    Priority = "top"
	PrioUp     Priority = "up"
	PrioDown   Priority = "down"
	PrioBottom Priority = "bottom"
)

// Priority levels set a download's standing in its client instead of moving
// it one step; a client that has levels implements PriorityLeveler.
const (
	PrioLow    Priority = "low"
	PrioNormal Priority = "normal"
	PrioHigh   Priority = "high"
	PrioForce  Priority = "force" // start now, ahead of the client's queue limits
)

var (
	priorityLevels = map[Priority]bool{PrioLow: true, PrioNormal: true, PrioHigh: true, PrioForce: true}
	priorityMoves  = map[Priority]bool{PrioTop: true, PrioUp: true, PrioDown: true, PrioBottom: true}
)

// IsLevel reports whether p is a priority level rather than a queue move.
func (p Priority) IsLevel() bool { return priorityLevels[p] }

// IsMove reports whether p moves a download in its client's queue.
func (p Priority) IsMove() bool { return priorityMoves[p] }

// Downloader drives one download client (qBittorrent, SABnzbd-compatible, …).
type Downloader interface {
	Tester
	Protocols() []domain.Protocol
	Add(ctx context.Context, req AddRequest) (downloadID string, err error)
	// List returns one batched snapshot of every download in the categories.
	List(ctx context.Context, categories []string) ([]Snapshot, error)
	Remove(ctx context.Context, downloadID string, deleteFiles bool) error
	Pause(ctx context.Context, downloadID string) error
	Resume(ctx context.Context, downloadID string) error
	SetPriority(ctx context.Context, downloadID string, p Priority) error
	EnsureCategory(ctx context.Context, category, savePath string) error
}

// FileEntry is one file of a download's payload.
type FileEntry struct {
	Path string // relative to the payload root
	Size int64
}

// FileLister is an optional downloader capability: the payload's file list,
// empty while torrent metadata is still unknown.
type FileLister interface {
	Files(ctx context.Context, downloadID string) ([]FileEntry, error)
}

// PriorityLeveler is an optional downloader capability: set a download's
// priority level (low, normal, high, force).
type PriorityLeveler interface {
	SetPriorityLevel(ctx context.Context, downloadID string, level Priority) error
}

// GlobalPauser is an optional downloader capability: stop and restart every
// download of the client at once.
type GlobalPauser interface {
	PauseAll(ctx context.Context) error
	ResumeAll(ctx context.Context) error
}

// SpeedLimits are a client's global transfer limits in KiB/s; 0 is unlimited.
type SpeedLimits struct {
	DownKiB int64 `json:"downKiB"`
	UpKiB   int64 `json:"upKiB"`
}

// SpeedLimiter is an optional downloader capability: global speed limits and
// the client's alternative-speed mode. A client lacking one part (an upload
// limit, an alternative mode) answers ErrUnsupported for that part.
type SpeedLimiter interface {
	SpeedLimits(ctx context.Context) (SpeedLimits, error)
	SetSpeedLimits(ctx context.Context, l SpeedLimits) error
	AltSpeed(ctx context.Context) (bool, error)
	SetAltSpeed(ctx context.Context, on bool) error
}

// Rechecker is an optional downloader capability: verify a download's data again.
type Rechecker interface {
	Recheck(ctx context.Context, downloadID string) error
}

// Reannouncer is an optional downloader capability: contact the trackers now.
type Reannouncer interface {
	Reannounce(ctx context.Context, downloadID string) error
}

// Categoriser is an optional downloader capability: move a download to
// another client category.
type Categoriser interface {
	SetCategory(ctx context.Context, downloadID, category string) error
}

// ClientStatus is the client-wide state at one moment.
type ClientStatus struct {
	DownSpeed int64       `json:"downSpeed"` // bytes/s
	UpSpeed   int64       `json:"upSpeed"`   // bytes/s
	FreeSpace int64       `json:"freeSpace"` // bytes free at the default save path; -1 unknown
	Paused    bool        `json:"paused"`    // every download is stopped
	Limits    SpeedLimits `json:"limits"`
	AltSpeed  bool        `json:"altSpeed"`
}

// ClientStats is an optional downloader capability: current speeds, free
// space, paused state and limits in one call.
type ClientStats interface {
	Stats(ctx context.Context) (ClientStatus, error)
}

// Tracker is one tracker of a download as the client reports it.
type Tracker struct {
	URL     string `json:"url"`
	Status  string `json:"status"` // working | updating | not_contacted | not_working | disabled
	Message string `json:"message,omitempty"`
	Seeds   int    `json:"seeds"`
	Peers   int    `json:"peers"`
}

// TrackerLister is an optional downloader capability: a download's trackers.
type TrackerLister interface {
	Trackers(ctx context.Context, downloadID string) ([]Tracker, error)
}

// ── metadata ─────────────────────────────────────────────────────────────────

// MovieRecord is the movie metadata shape.
type MovieRecord struct {
	IDs           map[string]string `json:"ids"` // tmdb, imdb
	Title         string            `json:"title"`
	OriginalTitle string            `json:"originalTitle,omitempty"`
	AltTitles     []string          `json:"altTitles,omitempty"`
	Year          int               `json:"year"`
	Overview      string            `json:"overview,omitempty"`
	RuntimeMin    int               `json:"runtimeMin,omitempty"`
	Status        string            `json:"status,omitempty"`
	InCinemas     *time.Time        `json:"inCinemas,omitempty"`
	Digital       *time.Time        `json:"digital,omitempty"`
	Physical      *time.Time        `json:"physical,omitempty"`
	PosterURL     string            `json:"posterUrl,omitempty"`
	FanartURL     string            `json:"fanartUrl,omitempty"` // backdrop
	Genres        []string          `json:"genres,omitempty"`
}

// MovieMetadata serves movie lookups (TMDB first).
type MovieMetadata interface {
	Tester
	SearchMovies(ctx context.Context, query string, year int) ([]MovieRecord, error)
	Movie(ctx context.Context, ids map[string]string) (MovieRecord, error)
}

// EpisodeRecord is one episode as a TV metadata provider reports it.
type EpisodeRecord struct {
	Season     int        `json:"season"`
	Number     int        `json:"number"`
	Absolute   int        `json:"absolute,omitempty"`
	Title      string     `json:"title"`
	AirAt      *time.Time `json:"airAt,omitempty"` // UTC; nil = to be announced
	RuntimeMin int        `json:"runtimeMin,omitempty"`
}

// SeriesRecord is the TV series metadata shape; Episodes is filled by
// Series and left empty by SearchSeries.
type SeriesRecord struct {
	IDs         map[string]string `json:"ids"` // tmdb, tvdb, imdb
	Title       string            `json:"title"`
	AltTitles   []string          `json:"altTitles,omitempty"`
	Year        int               `json:"year"`
	Status      string            `json:"status,omitempty"` // continuing | ended | upcoming
	Network     string            `json:"network,omitempty"`
	Overview    string            `json:"overview,omitempty"`
	RuntimeMin  int               `json:"runtimeMin,omitempty"`
	Genres      []string          `json:"genres,omitempty"`
	PosterURL   string            `json:"posterUrl,omitempty"`
	FanartURL   string            `json:"fanartUrl,omitempty"` // backdrop
	SeasonCount int               `json:"seasonCount"`
	Episodes    []EpisodeRecord   `json:"episodes,omitempty"`
}

// SeriesMetadata serves TV lookups.
type SeriesMetadata interface {
	Tester
	SearchSeries(ctx context.Context, query string) ([]SeriesRecord, error)
	// Series returns the full record with every episode for ids["tmdb"],
	// resolving ids["tvdb"] or ids["imdb"] first.
	Series(ctx context.Context, ids map[string]string) (SeriesRecord, error)
}

// ── media server ─────────────────────────────────────────────────────────────

// MediaServer refreshes a library after imports (Plex first).
type MediaServer interface {
	Tester
	RefreshPaths(ctx context.Context, paths []string) error
}

// ── notifier ─────────────────────────────────────────────────────────────────

// Message is a rendered notification.
type Message struct {
	Title    string
	Body     string
	Priority int
}

// Notifier delivers an event to one connection's service URLs.
type Notifier interface {
	Tester
	Schemes() []string
	Send(ctx context.Context, ev domain.Event, msg Message) error
}

// ── helpers ──────────────────────────────────────────────────────────────────

// ReadLimited reads at most n bytes of a body: plugins never load unbounded
// responses into memory.
func ReadLimited(r io.Reader, n int64) ([]byte, error) { return io.ReadAll(io.LimitReader(r, n)) }

// DataKeeper is an optional downloader capability flag: a client that cannot
// delete a download's files with it reports true, Remove with deleteFiles
// answers ErrUnsupported, and callers remove without data.
type DataKeeper interface {
	KeepsDataOnRemove() bool
}

// ── optional capabilities ────────────────────────────────────────────────────

// CapabilityGate is implemented by plugins whose optional capabilities are
// known only at run time (plugin modules): such a plugin implements every
// optional interface of its kind and reports which ones really work.
type CapabilityGate interface {
	Supports(capability reflect.Type) bool
}

// As is the type assertion callers use for optional capabilities: impl must
// implement T and, when it is a CapabilityGate, support it.
func As[T any](impl any) (T, bool) {
	t, ok := impl.(T)
	if !ok {
		return t, false
	}
	if g, gated := impl.(CapabilityGate); gated && !g.Supports(reflect.TypeFor[T]()) {
		var zero T
		return zero, false
	}
	return t, true
}
