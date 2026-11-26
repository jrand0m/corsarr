package legacy

import (
	"net/http"
	"time"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
)

// The read-only shims dashboards use: Lidarr (music) and Prowlarr (indexers).

// LidarrVersion and ProwlarrVersion are the API versions those shims report.
const (
	LidarrVersion   = "2.0.0"
	ProwlarrVersion = "1.0.0"
)

func (s *Service) lidarrRoutes(m *http.ServeMux) {
	const p = "/legacy/lidarr/api/v1/"
	m.HandleFunc("GET "+p+"system/status", s.statusOf(config.LegacyLidarr, LidarrVersion))
	m.HandleFunc("GET "+p+"artist", s.listArtists)
	m.HandleFunc("GET "+p+"wanted/missing", s.missingAlbums)
	m.HandleFunc("GET "+p+"queue/status", s.queueStatus(domain.KindMusic))
}

func (s *Service) prowlarrRoutes(m *http.ServeMux) {
	const p = "/legacy/prowlarr/api/v1/"
	m.HandleFunc("GET "+p+"system/status", s.statusOf(config.LegacyProwlarr, ProwlarrVersion))
	m.HandleFunc("GET "+p+"indexer", s.listIndexers)
	m.HandleFunc("GET "+p+"indexerstats", s.indexerStats)
}

type artist struct {
	ID               int64   `json:"id"`
	ArtistName       string  `json:"artistName"`
	SortName         string  `json:"sortName"`
	ForeignArtistID  string  `json:"foreignArtistId"`
	Disambiguation   string  `json:"disambiguation"`
	ArtistType       string  `json:"artistType"`
	Monitored        bool    `json:"monitored"`
	Path             string  `json:"path"`
	RootFolderPath   string  `json:"rootFolderPath"`
	QualityProfileID int64   `json:"qualityProfileId"`
	Tags             []int64 `json:"tags"`
	Added            string  `json:"added"`
}

func (s *Service) listArtists(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := []artist{}
	err := s.d.Music.EachArtist(ctx, func(a domain.Artist) error {
		out = append(out, artist{ID: a.ID, ArtistName: a.Name, SortName: a.SortName, ForeignArtistID: a.MBID,
			Disambiguation: a.Disambiguation, ArtistType: a.Type, Monitored: a.Monitored, Path: a.Path, RootFolderPath: a.RootFolder,
			QualityProfileID: s.idOf(ctx, spaceProfile, a.ProfileKey), Tags: s.tagIDs(ctx, a.Tags),
			Added: a.AddedAt.UTC().Format(time.RFC3339)})
		return nil
	})
	if err != nil {
		s.writeErr(w, err)
		return
	}
	ok(w, out)
}

type album struct {
	ID             int64     `json:"id"`
	ArtistID       int64     `json:"artistId"`
	ForeignAlbumID string    `json:"foreignAlbumId"`
	Title          string    `json:"title"`
	AlbumType      string    `json:"albumType"`
	ReleaseDate    string    `json:"releaseDate,omitempty"`
	Monitored      bool      `json:"monitored"`
	Artist         artistRef `json:"artist"`
}

type artistRef struct {
	ID         int64  `json:"id"`
	ArtistName string `json:"artistName"`
}

func (s *Service) missingAlbums(w http.ResponseWriter, r *http.Request) {
	entries, _ := s.d.MusicWanted.Index().Page(automation.ReasonMissing, 0, 1<<30)
	out := make([]album, 0, len(entries))
	for _, e := range entries {
		out = append(out, album{ID: e.Album.ID, ArtistID: e.Album.ArtistID, ForeignAlbumID: e.Album.MBID, Title: e.Album.Title,
			AlbumType: string(e.Album.PrimaryType), ReleaseDate: isoTime(e.Album.ReleaseDate), Monitored: e.Album.Monitored,
			Artist: artistRef{ID: e.Artist.ID, ArtistName: e.Artist.Name}})
	}
	p := pageOf(r, out)
	p.SortKey = "releaseDate"
	ok(w, p)
}

type indexer struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Enable         bool   `json:"enable"`
	Protocol       string `json:"protocol"`
	Priority       int    `json:"priority"`
	Implementation string `json:"implementation"`
	DefinitionName string `json:"definitionName"`
}

func (s *Service) listIndexers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.d.Indexers.List(r.Context())
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]indexer, 0, len(rows))
	for _, row := range rows {
		out = append(out, indexer{ID: s.idOf(r.Context(), spaceIndexer, string(row.Key)), Name: nameOf(row.Name, string(row.Key)),
			Enable: row.Enabled, Protocol: string(domain.ProtoTorrent), Priority: row.Priority, Implementation: row.PluginName,
			DefinitionName: row.Plugin})
	}
	ok(w, out)
}

func nameOf(name, key string) string {
	if name == "" {
		return key
	}
	return name
}

type indexerStat struct {
	IndexerID                 int64  `json:"indexerId"`
	IndexerName               string `json:"indexerName"`
	AverageResponseTime       int    `json:"averageResponseTime"`
	NumberOfQueries           int    `json:"numberOfQueries"`
	NumberOfGrabs             int    `json:"numberOfGrabs"`
	NumberOfRssQueries        int    `json:"numberOfRssQueries"`
	NumberOfAuthQueries       int    `json:"numberOfAuthQueries"`
	NumberOfFailedQueries     int    `json:"numberOfFailedQueries"`
	NumberOfFailedGrabs       int    `json:"numberOfFailedGrabs"`
	NumberOfFailedRssQueries  int    `json:"numberOfFailedRssQueries"`
	NumberOfFailedAuthQueries int    `json:"numberOfFailedAuthQueries"`
}

type indexerStats struct {
	Indexers   []indexerStat `json:"indexers"`
	UserAgents []any         `json:"userAgents"`
	Hosts      []any         `json:"hosts"`
}

// indexerStats reports the queries corsarr logged (its query-limit window)
// and the grabs of each indexer's releases; what corsarr does not record
// (response times, RSS, auth, failed queries) is zero.
func (s *Service) indexerStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := s.d.Indexers.List(ctx)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	grabs, err := s.d.Grabs.GrabsByIndexer(ctx)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := indexerStats{Indexers: []indexerStat{}, UserAgents: []any{}, Hosts: []any{}}
	for _, row := range rows {
		n, _, err := s.d.Queries.Queries(ctx, row.Key, time.Time{})
		if err != nil {
			s.writeErr(w, err)
			return
		}
		g := grabs[row.Key]
		out.Indexers = append(out.Indexers, indexerStat{IndexerID: s.idOf(ctx, spaceIndexer, string(row.Key)),
			IndexerName: nameOf(row.Name, string(row.Key)), NumberOfQueries: n, NumberOfGrabs: g.Grabs, NumberOfFailedGrabs: g.Failed})
	}
	ok(w, out)
}
