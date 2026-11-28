package migrate

import (
	"encoding/json"
	"time"
)

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

type wProfile struct {
	ID                    int
	Name                  string
	UpgradeAllowed        bool
	Cutoff                int
	Items                 []wProfileItem
	FormatItems           []wFormatScore
	MinFormatScore        int
	CutoffFormatScore     int
	MinUpgradeFormatScore int
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
	RenameTracks   *bool
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
	AlbumID        int `json:"albumId"`
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
	AlbumIDs    []int `json:"albumIds"`
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
	AlbumID     int    `json:"albumId"`
	DownloadID  string `json:"downloadId"`
}

// ── music ────────────────────────────────────────────────────────────────────

type wArtist struct {
	ID                int
	ArtistName        string
	ForeignArtistID   string `json:"foreignArtistId"`
	SortName          string
	Disambiguation    string
	ArtistType        string
	Path              string
	RootFolderPath    string
	QualityProfileID  int `json:"qualityProfileId"`
	MetadataProfileID int `json:"metadataProfileId"`
	Monitored         bool
	MonitorNewItems   string
	Tags              []int
}

// wRelease is one edition of an album; the monitored one is the selected edition.
type wRelease struct {
	ForeignReleaseID string `json:"foreignReleaseId"`
	TrackCount       int
	MediumCount      int
	Monitored        bool
}

type wAlbum struct {
	ID             int
	Title          string
	ForeignAlbumID string `json:"foreignAlbumId"`
	AlbumType      string
	SecondaryTypes []wTypeName
	ReleaseDate    *time.Time
	Monitored      bool
	Releases       []wRelease
}

// wTypeName reads a type given either as a bare name or as an object with one.
type wTypeName string

func (t *wTypeName) UnmarshalJSON(b []byte) error {
	var name string
	if json.Unmarshal(b, &name) == nil {
		*t = wTypeName(name)
		return nil
	}
	var obj struct{ Name string }
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	*t = wTypeName(obj.Name)
	return nil
}

type wTrack struct {
	ID                  int
	AlbumID             int    `json:"albumId"`
	ForeignRecordingID  string `json:"foreignRecordingId"`
	TrackFileID         int    `json:"trackFileId"`
	TrackNumber         string
	AbsoluteTrackNumber int
	MediumNumber        int
	Title               string
	Duration            int // milliseconds
}

type wTrackFile struct {
	wFile
	AlbumID int `json:"albumId"`
}

// wAllowedType is one entry of a metadata profile's type lists.
type wAllowedType struct {
	AlbumType     *struct{ Name string } `json:"albumType"`
	ReleaseStatus *struct{ Name string } `json:"releaseStatus"`
	Allowed       bool
}

type wMetadataProfile struct {
	ID                  int
	Name                string
	PrimaryAlbumTypes   []wAllowedType
	SecondaryAlbumTypes []wAllowedType
	ReleaseStatuses     []wAllowedType
}
