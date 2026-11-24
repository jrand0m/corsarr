package migrate

import (
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// SourceInfo describes one connected source; it never carries the key.
type SourceInfo struct {
	App      App    `json:"app"`
	URL      string `json:"url"`
	Instance string `json:"instance"`
	Name     string `json:"name,omitempty"`
	Version  string `json:"version,omitempty"`
	Range    string `json:"supported"`
	Error    string `json:"error,omitempty"`
}

// Snapshot is everything read from one source, parsed once at the boundary.
type Snapshot struct {
	Source        SourceInfo
	Series        []SrcSeries
	Movies        []SrcMovie
	Profiles      map[int]SrcProfile
	CustomFormats []string
	RootFolders   []string
	Tags          map[int]string
	Clients       []SrcClient
	Indexers      []SrcIndexer
	Rename        *bool // nil = not read
	Queue         []SrcQueue
	Blocklist     []SrcBlock
	History       []SrcHistory
}

// SrcSeries is a source series with its episodes and files.
type SrcSeries struct {
	ID            int
	TMDB, TVDB    int64
	IMDB, Title   string
	Year          int
	Path, Root    string
	ProfileID     int
	Monitored     bool
	SeasonFolders bool
	Type          domain.SeriesType
	MonitorNew    domain.MonitorNew
	Tags          []int
	Status        string
	Network       string
	Runtime       int
	Seasons       []domain.Season
	Episodes      []SrcEpisode
	Files         []SrcFile
}

// SrcEpisode is a source episode.
type SrcEpisode struct {
	ID                       int
	Season, Number, Absolute int
	Title                    string
	AirAt                    *time.Time
	Monitored                bool
	FileID                   int
}

// SrcFile is a source media file; Quality is the source's quality name.
type SrcFile struct {
	ID      int
	Path    string
	Size    int64
	Quality string
	Group   string
	Edition string
	Added   time.Time
}

// SrcMovie is a source movie with its file.
type SrcMovie struct {
	ID                           int
	TMDB                         int64
	IMDB, Title, Original        string
	Year                         int
	Path, Root                   string
	ProfileID                    int
	Monitored                    bool
	MinAvailability              string
	Tags                         []int
	Runtime                      int
	InCinemas, Digital, Physical *time.Time
	File                         *SrcFile
}

// SrcProfile is a source quality profile reduced to what mapping needs.
type SrcProfile struct {
	ID       int
	Name     string
	Cutoff   string // quality or group name at the cutoff
	Upgrades bool
	Scored   int // custom formats with a non-zero score
}

// SrcClient is a source download client.
type SrcClient struct {
	ID             int
	Name           string
	Impl           string
	Protocol       string
	Enabled        bool
	Host           string
	Port           int
	TLS            bool
	URLBase        string
	Username       string
	PasswordMasked bool
	Category       string
}

// SrcIndexer is a source indexer (an indexer manager's, or an app's own).
type SrcIndexer struct {
	ID         int
	Name       string
	Impl       string
	Protocol   string
	Enabled    bool
	Priority   int
	BaseURL    string
	APIPath    string
	KeyMasked  bool
	Definition string
}

// SrcQueue is one source queue row; a pack is several rows sharing DownloadID.
type SrcQueue struct {
	DownloadID string
	Title      string
	Protocol   string
	Client     string
	Size       int64
	SeriesID   int
	EpisodeID  int
	MovieID    int
}

// SrcBlock is a source blocklist entry.
type SrcBlock struct {
	ID         int
	SeriesID   int
	EpisodeIDs []int
	MovieID    int
	Title      string
	Indexer    string
	Message    string
	Date       time.Time
}

// SrcHistory is a source history row.
type SrcHistory struct {
	ID         int
	Event      string
	Date       time.Time
	Title      string
	SeriesID   int
	EpisodeID  int
	MovieID    int
	DownloadID string
}
