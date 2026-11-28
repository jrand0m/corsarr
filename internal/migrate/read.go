package migrate

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Limits of the bounded history read.
const (
	DefaultHistoryDays = 90
	MaxHistoryDays     = 365
	MaxHistoryRows     = 20000
	MaxBlocklistRows   = 5000
	MaxQueueRows       = 2000
)

// ReadOptions choose what a read fetches.
type ReadOptions struct {
	Scope        Scope
	HistorySince time.Time
}

// Progress reports one step of a read or an apply.
type Progress struct {
	Source App    `json:"source"`
	Step   string `json:"step"`
	Done   int    `json:"done"`
	Total  int    `json:"total"`
}

// Read connects, checks the version, and reads everything in scope. A
// refused key or unsupported version stops this source before anything else.
func Read(ctx context.Context, c *Client, o ReadOptions, progress func(Progress)) (Snapshot, error) {
	snap := Snapshot{Source: SourceInfo{App: c.ep.App, URL: c.ep.URL.String(), Instance: c.ep.Instance(), Range: SupportedRange(c.ep.App)},
		Profiles: map[int]SrcProfile{}, MetaProfiles: map[int]SrcMetaProfile{}, Tags: map[int]string{}}
	st, err := c.Status(ctx)
	snap.Source.Name, snap.Source.Version = st.InstanceName, st.Version
	if err != nil {
		snap.Source.Error = err.Error()
		return snap, err
	}
	for _, step := range readSteps(c.ep.App, o) {
		progress(Progress{Source: c.ep.App, Step: step.name})
		if err := step.run(ctx, c, &snap, o); err != nil {
			snap.Source.Error = err.Error()
			return snap, err
		}
	}
	return snap, nil
}

type readStep struct {
	name string
	run  func(context.Context, *Client, *Snapshot, ReadOptions) error
}

// readSteps lists the reads of an app that the scope asks for.
func readSteps(a App, o ReadOptions) []readStep {
	all := []struct {
		apps []App
		cat  Category
		step readStep
	}{
		{[]App{Sonarr, Radarr, Lidarr, Prowlarr}, CatLibrary, readStep{"tags", readTags}},
		{[]App{Sonarr, Radarr, Lidarr}, CatProfiles, readStep{"profiles", readProfiles}},
		{[]App{Lidarr}, CatProfiles, readStep{"metadata profiles", readMetaProfiles}},
		{[]App{Sonarr, Radarr, Lidarr}, CatProfiles, readStep{"custom formats", readFormats}},
		{[]App{Sonarr, Radarr, Lidarr}, CatRootFolders, readStep{"root folders", readRoots}},
		{[]App{Sonarr, Radarr, Lidarr}, CatNaming, readStep{"naming", readNaming}},
		{[]App{Sonarr, Radarr, Lidarr, Prowlarr}, CatClients, readStep{"download clients", readClients}},
		{[]App{Sonarr, Radarr, Lidarr, Prowlarr}, CatIndexers, readStep{"indexers", readIndexers}},
		{[]App{Sonarr}, CatLibrary, readStep{"series", readSeries}},
		{[]App{Radarr}, CatLibrary, readStep{"movies", readMovies}},
		{[]App{Lidarr}, CatLibrary, readStep{"artists", readArtists}},
		{[]App{Sonarr, Radarr, Lidarr}, CatDownloads, readStep{"queue", readQueue}},
		{[]App{Sonarr, Radarr, Lidarr}, CatBlocklist, readStep{"blocklist", readBlocklist}},
		{[]App{Sonarr, Radarr, Lidarr}, CatHistory, readStep{"history", readHistory}},
	}
	var out []readStep
	for _, s := range all {
		if o.Scope.Has(s.cat) && hasApp(s.apps, a) {
			out = append(out, s.step)
		}
	}
	return out
}

func hasApp(list []App, a App) bool {
	for _, x := range list {
		if x == a {
			return true
		}
	}
	return false
}

func readTags(ctx context.Context, c *Client, s *Snapshot, _ ReadOptions) error {
	var tags []wTag
	if err := c.get(ctx, "/tag", nil, &tags); err != nil {
		return err
	}
	for _, t := range tags {
		s.Tags[t.ID] = t.Label
	}
	return nil
}

func readProfiles(ctx context.Context, c *Client, s *Snapshot, _ ReadOptions) error {
	var list []wProfile
	if err := c.get(ctx, "/qualityprofile", nil, &list); err != nil {
		return err
	}
	for _, p := range list {
		s.Profiles[p.ID] = SrcProfile{ID: p.ID, Name: p.Name, Cutoff: cutoffName(p.Items, p.Cutoff), Upgrades: p.UpgradeAllowed,
			Scored: scored(p.FormatItems), Allowed: allowedNames(p.Items, false)}
	}
	return nil
}

// cutoffName finds the quality or group the profile's cutoff id names.
func cutoffName(items []wProfileItem, cutoff int) string {
	for _, it := range items {
		if it.Quality != nil && it.Quality.ID == cutoff {
			return it.Quality.Name
		}
		if it.Quality == nil && it.ID == cutoff {
			return it.Name
		}
		if n := cutoffName(it.Items, cutoff); n != "" {
			return n
		}
	}
	return ""
}

// allowedNames lists the qualities a profile allows; a group's allowed flag
// covers its members.
func allowedNames(items []wProfileItem, inherited bool) []string {
	var out []string
	for _, it := range items {
		allowed := inherited || it.Allowed
		if it.Quality != nil && allowed {
			out = append(out, it.Quality.Name)
		}
		out = append(out, allowedNames(it.Items, allowed)...)
	}
	return out
}

func scored(items []wFormatItem) int {
	n := 0
	for _, f := range items {
		if f.Score != 0 {
			n++
		}
	}
	return n
}

func readFormats(ctx context.Context, c *Client, s *Snapshot, _ ReadOptions) error {
	var list []wNamed
	if err := c.get(ctx, "/customformat", nil, &list); err != nil {
		return err
	}
	for _, f := range list {
		s.CustomFormats = append(s.CustomFormats, f.Name)
	}
	return nil
}

func readRoots(ctx context.Context, c *Client, s *Snapshot, _ ReadOptions) error {
	roots, err := readRootFolders(ctx, c)
	s.RootFolders = append(s.RootFolders, roots...)
	return err
}

func readNaming(ctx context.Context, c *Client, s *Snapshot, _ ReadOptions) error {
	var n wNaming
	if err := c.get(ctx, "/config/naming", nil, &n); err != nil {
		return err
	}
	s.Rename = map[App]*bool{Sonarr: n.RenameEpisodes, Radarr: n.RenameMovies, Lidarr: n.RenameTracks}[c.ep.App]
	return nil
}

func readClients(ctx context.Context, c *Client, s *Snapshot, _ ReadOptions) error {
	var list []wProvider
	if err := c.get(ctx, "/downloadclient", nil, &list); err != nil {
		return err
	}
	for _, p := range list {
		f := fields(p.Fields)
		s.Clients = append(s.Clients, SrcClient{ID: p.ID, Name: p.Name, Impl: p.Implementation, Protocol: p.Protocol, Enabled: p.Enable,
			Host: f.str("host"), Port: f.int("port"), TLS: f.bool("useSsl"), URLBase: f.str("urlBase"), Username: f.str("username"),
			PasswordMasked: f.masked("password"), Category: f.first("tvCategory", "movieCategory", "musicCategory", "category")})
	}
	return nil
}

func readIndexers(ctx context.Context, c *Client, s *Snapshot, _ ReadOptions) error {
	var list []wProvider
	if err := c.get(ctx, "/indexer", nil, &list); err != nil {
		return err
	}
	for _, p := range list {
		f := fields(p.Fields)
		s.Indexers = append(s.Indexers, SrcIndexer{ID: p.ID, Name: p.Name, Impl: p.Implementation, Protocol: p.Protocol,
			Enabled: p.Enable, Priority: p.Priority, BaseURL: f.str("baseUrl"), APIPath: f.str("apiPath"),
			KeyMasked: f.masked("apiKey"), Definition: p.DefinitionName})
	}
	return nil
}

func readSeries(ctx context.Context, c *Client, s *Snapshot, _ ReadOptions) error {
	var list []wSeries
	if err := c.get(ctx, "/series", nil, &list); err != nil {
		return err
	}
	for _, w := range list {
		ser, err := seriesOf(ctx, c, w)
		if err != nil {
			return err
		}
		s.Series = append(s.Series, ser)
	}
	return nil
}

// seriesOf reads one series' episodes and files and parses it.
func seriesOf(ctx context.Context, c *Client, w wSeries) (SrcSeries, error) {
	q := url.Values{"seriesId": {strconv.Itoa(w.ID)}}
	var eps []wEpisode
	var files []wFile
	if err := c.get(ctx, "/episode", q, &eps); err != nil {
		return SrcSeries{}, err
	}
	if err := c.get(ctx, "/episodefile", q, &files); err != nil {
		return SrcSeries{}, err
	}
	typ, err := domain.ParseSeriesType(w.SeriesType)
	if err != nil {
		typ = domain.SeriesStandard
	}
	mn, err := domain.ParseMonitorNew(w.MonitorNewItems)
	if err != nil {
		mn = domain.MonitorNewAll
	}
	ser := SrcSeries{ID: w.ID, TMDB: w.TmdbID, TVDB: w.TvdbID, IMDB: w.ImdbID, Title: w.Title, Year: w.Year, Path: cleanPath(w.Path),
		Root: cleanPath(w.RootFolderPath), ProfileID: w.QualityProfileID, Monitored: w.Monitored, SeasonFolders: w.SeasonFolder,
		Type: typ, MonitorNew: mn, Tags: w.Tags, Status: w.Status, Network: w.Network, Runtime: w.Runtime}
	for _, se := range w.Seasons {
		ser.Seasons = append(ser.Seasons, domain.Season{Number: se.SeasonNumber, Monitored: se.Monitored})
	}
	for _, e := range eps {
		ser.Episodes = append(ser.Episodes, SrcEpisode{ID: e.ID, Season: e.SeasonNumber, Number: e.EpisodeNumber,
			Absolute: e.AbsoluteEpisodeNumber, Title: e.Title, AirAt: e.AirDateUtc, Monitored: e.Monitored, FileID: e.EpisodeFileID})
	}
	for _, f := range files {
		ser.Files = append(ser.Files, fileOf(f))
	}
	return ser, nil
}

func fileOf(f wFile) SrcFile {
	return SrcFile{ID: f.ID, Path: cleanPath(f.Path), Size: f.Size, Quality: f.Quality.Quality.Name, Group: f.ReleaseGroup,
		Edition: f.Edition, Added: f.DateAdded}
}

func readMovies(ctx context.Context, c *Client, s *Snapshot, _ ReadOptions) error {
	var list []wMovie
	if err := c.get(ctx, "/movie", nil, &list); err != nil {
		return err
	}
	for _, w := range list {
		m := SrcMovie{ID: w.ID, TMDB: w.TmdbID, IMDB: w.ImdbID, Title: w.Title, Original: w.OriginalTitle, Year: w.Year,
			Path: cleanPath(w.Path), Root: cleanPath(w.RootFolderPath), ProfileID: w.QualityProfileID, Monitored: w.Monitored,
			MinAvailability: w.MinimumAvailability, Tags: w.Tags, Runtime: w.Runtime, InCinemas: w.InCinemas,
			Digital: w.DigitalRelease, Physical: w.PhysicalRelease}
		if w.MovieFile != nil {
			f := fileOf(*w.MovieFile)
			m.File = &f
		}
		s.Movies = append(s.Movies, m)
	}
	return nil
}

func readQueue(ctx context.Context, c *Client, s *Snapshot, _ ReadOptions) error {
	rows, err := paged(ctx, c, "/queue", nil, MaxQueueRows, func(wQueue) bool { return true })
	for _, r := range rows {
		s.Queue = append(s.Queue, SrcQueue{DownloadID: strings.ToLower(r.DownloadID), Title: r.Title, Protocol: r.Protocol,
			Client: r.DownloadClient, Size: int64(r.Size), SeriesID: r.SeriesID, EpisodeID: r.EpisodeID, MovieID: r.MovieID,
			AlbumID: r.AlbumID})
	}
	return err
}

func readBlocklist(ctx context.Context, c *Client, s *Snapshot, _ ReadOptions) error {
	rows, err := paged(ctx, c, "/blocklist", nil, MaxBlocklistRows, func(wBlock) bool { return true })
	for _, r := range rows {
		s.Blocklist = append(s.Blocklist, SrcBlock{ID: r.ID, SeriesID: r.SeriesID, EpisodeIDs: r.EpisodeIDs, MovieID: r.MovieID,
			AlbumIDs: r.AlbumIDs, Title: r.SourceTitle, Indexer: r.Indexer, Message: r.Message, Date: r.Date})
	}
	return err
}

func readHistory(ctx context.Context, c *Client, s *Snapshot, o ReadOptions) error {
	sort := url.Values{"sortKey": {"date"}, "sortDirection": {"descending"}}
	rows, err := paged(ctx, c, "/history", sort, MaxHistoryRows, func(h wHistory) bool { return !h.Date.Before(o.HistorySince) })
	for _, r := range rows {
		s.History = append(s.History, SrcHistory{ID: r.ID, Event: r.EventType, Date: r.Date, Title: r.SourceTitle,
			SeriesID: r.SeriesID, EpisodeID: r.EpisodeID, MovieID: r.MovieID, AlbumID: r.AlbumID, DownloadID: strings.ToLower(r.DownloadID)})
	}
	return err
}

// cleanPath drops a trailing separator so paths compare as written.
func cleanPath(p string) string {
	if len(p) > 1 {
		return strings.TrimRight(p, "/")
	}
	return p
}

// fieldSet is a provider's settings as name → value.
type fieldSet map[string]any

func fields(list []wField) fieldSet {
	out := fieldSet{}
	for _, f := range list {
		out[f.Name] = f.Value
	}
	return out
}

func (f fieldSet) str(name string) string {
	if v, ok := f[name].(string); ok {
		return v
	}
	if v, ok := f[name]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

func (f fieldSet) int(name string) int {
	n, _ := strconv.Atoi(f.str(name))
	return n
}

func (f fieldSet) bool(name string) bool { b, _ := f[name].(bool); return b }

// masked reports a secret the source hides (it answers asterisks).
func (f fieldSet) masked(name string) bool {
	v := f.str(name)
	return v != "" && strings.Trim(v, "*") == ""
}

func (f fieldSet) first(names ...string) string {
	for _, n := range names {
		if v := f.str(n); v != "" {
			return v
		}
	}
	return ""
}
