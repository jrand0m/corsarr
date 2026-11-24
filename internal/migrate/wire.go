package migrate

import "time"

// Wire shapes of the sources' public JSON resources: only the fields a
// migration reads. JSON field matching is case-insensitive, so most names
// need no tag.

type wStatus struct {
	AppName      string `json:"appName"`
	InstanceName string `json:"instanceName"`
	Version      string `json:"version"`
}

type wSeason struct {
	SeasonNumber int
	Monitored    bool
}

type wSeries struct {
	ID               int
	Title            string
	Year             int
	TvdbID           int64  `json:"tvdbId"`
	TmdbID           int64  `json:"tmdbId"`
	ImdbID           string `json:"imdbId"`
	Path             string
	RootFolderPath   string
	QualityProfileID int `json:"qualityProfileId"`
	Monitored        bool
	SeasonFolder     bool
	SeriesType       string
	MonitorNewItems  string
	Tags             []int
	Status           string
	Network          string
	Runtime          int
	Seasons          []wSeason
}

type wEpisode struct {
	ID                    int
	SeasonNumber          int
	EpisodeNumber         int
	AbsoluteEpisodeNumber int
	Title                 string
	AirDateUtc            *time.Time `json:"airDateUtc"`
	Monitored             bool
	EpisodeFileID         int `json:"episodeFileId"`
}

type wQuality struct {
	Quality struct {
		Name string
	}
}

type wFile struct {
	ID           int
	Path         string
	Size         int64
	Quality      wQuality
	ReleaseGroup string
	Edition      string
	DateAdded    time.Time
}

type wMovie struct {
	ID                  int
	Title               string
	OriginalTitle       string
	Year                int
	TmdbID              int64  `json:"tmdbId"`
	ImdbID              string `json:"imdbId"`
	Path                string
	RootFolderPath      string
	QualityProfileID    int `json:"qualityProfileId"`
	Monitored           bool
	MinimumAvailability string
	Tags                []int
	Runtime             int
	InCinemas           *time.Time
	DigitalRelease      *time.Time
	PhysicalRelease     *time.Time
	MovieFile           *wFile
}

type wProfileItem struct {
	ID      int
	Name    string
	Quality *struct {
		ID   int
		Name string
	}
	Items   []wProfileItem
	Allowed bool
}

type wFormatItem struct {
	Name  string
	Score int
}

type wProfile struct {
	ID             int
	Name           string
	UpgradeAllowed bool
	Cutoff         int
	Items          []wProfileItem
	FormatItems    []wFormatItem
}

type wNamed struct {
	ID   int
	Name string
}

type wRootFolder struct {
	ID   int
	Path string
}

type wTag struct {
	ID    int
	Label string
}

type wField struct {
	Name  string
	Value any
}

// wProvider is a download client or an indexer.
type wProvider struct {
	ID             int
	Name           string
	Implementation string
	Protocol       string
	Enable         bool
	Priority       int
	DefinitionName string
	Fields         []wField
}

type wNaming struct {
	RenameEpisodes *bool
	RenameMovies   *bool
}

type wPage[T any] struct {
	Page         int
	PageSize     int
	TotalRecords int
	Records      []T
}

type wQueue struct {
	SeriesID       int `json:"seriesId"`
	EpisodeID      int `json:"episodeId"`
	MovieID        int `json:"movieId"`
	Title          string
	DownloadID     string `json:"downloadId"`
	Protocol       string
	DownloadClient string
	Size           float64
}

type wBlock struct {
	ID          int
	SeriesID    int   `json:"seriesId"`
	EpisodeIDs  []int `json:"episodeIds"`
	MovieID     int   `json:"movieId"`
	SourceTitle string
	Date        time.Time
	Indexer     string
	Message     string
}

type wHistory struct {
	ID          int
	EventType   string
	Date        time.Time
	SourceTitle string
	SeriesID    int    `json:"seriesId"`
	EpisodeID   int    `json:"episodeId"`
	MovieID     int    `json:"movieId"`
	DownloadID  string `json:"downloadId"`
}
