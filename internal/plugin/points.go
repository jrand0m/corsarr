package plugin

import (
	"context"
	"io"
	"net/http"
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
