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
	ReadMs   int64  `json:"readMs"` // how long reading this source took
}

// Snapshot is everything read from one source, parsed once at the boundary.
type Snapshot struct {
	Source        SourceInfo
	Series        []SrcSeries
	Movies        []SrcMovie
	Artists       []SrcArtist
	Profiles      map[int]SrcProfile
	MetaProfiles  map[int]SrcMetaProfile
	CustomFormats []SrcFormat
	Sizes         map[string]SrcSize // quality definitions by source quality name
	// ProfileKeys maps a source profile id onto the corsarr profile key the
	// plan gives it (set while planning).
	ProfileKeys map[int]string
	RootFolders []string
	Tags        map[int]string
	Clients     []SrcClient
	Indexers    []SrcIndexer
	Naming      *SrcNaming // nil = not read
	Queue       []SrcQueue
	Blocklist   []SrcBlock
	History     []SrcHistory
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
	Scored   int      // custom formats with a non-zero score
	Allowed  []string // names of the allowed qualities
	// The full profile, for custom profiles.
	CutoffID    int
	Items       []SrcQualityItem
	Scores      []SrcScore
	MinScore    int
	CutoffScore int
	StepScore   int
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
	AlbumID    int
}

// SrcBlock is a source blocklist entry.
type SrcBlock struct {
	ID         int
	SeriesID   int
	EpisodeIDs []int
	MovieID    int
	AlbumIDs   []int
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
	AlbumID    int
	DownloadID string
}

// SrcArtist is a source artist with its albums, the tracks of each album's
// selected edition, and its track files.
type SrcArtist struct {
	ID             int
	MBID           string
	Name, SortName string
	Disambiguation string
	Type           string
	Path, Root     string
	ProfileID      int
	MetaProfileID  int
	Monitored      bool
	MonitorNew     string
	Tags           []int
	Albums         []SrcAlbum
	Files          []SrcTrackFile
}

// SrcAlbum is a source album (release group) with its selected edition.
type SrcAlbum struct {
	ID             int
	MBID           string
	Title          string
	Type           string
	SecondaryTypes []string
	ReleaseDate    *time.Time
	Monitored      bool
	EditionMBID    string
	MediumCount    int
	Tracks         []SrcTrack
}

// SrcTrack is one track of an album's selected edition.
type SrcTrack struct {
	ID          int
	RecordingID string
	Medium      int
	Number      int
	Title       string
	DurationSec int
	FileID      int
}

// SrcTrackFile is a source track file and the album it belongs to.
type SrcTrackFile struct {
	SrcFile
	AlbumID int
}

// SrcMetaProfile is a source metadata profile: the allowed album types
// (lower case) and release statuses.
type SrcMetaProfile struct {
	ID        int
	Name      string
	Primary   []string
	Secondary []string
	Statuses  []string
}
