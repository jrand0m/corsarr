package legacy

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/tv"
	"github.com/jrand0m/corsarr/internal/tvauto"
)

// SonarrVersion is the API version the Sonarr shim reports.
const SonarrVersion = "4.0.0"

func (s *Service) sonarrRoutes(m *http.ServeMux) {
	const p = "/legacy/sonarr/api/v3/"
	m.HandleFunc("GET "+p+"system/status", s.statusOf(config.LegacySonarr, SonarrVersion))
	m.HandleFunc("GET "+p+"qualityprofile", s.profiles(domain.KindTV))
	m.HandleFunc("GET "+p+"languageprofile", emptyList)
	m.HandleFunc("GET "+p+"rootfolder", s.rootFolders(domain.KindTV))
	m.HandleFunc("GET "+p+"tag", s.listTags)
	m.HandleFunc("POST "+p+"tag", s.createTag)
	m.HandleFunc("GET "+p+"series", s.listSeries)
	m.HandleFunc("GET "+p+"series/lookup", s.lookupSeries)
	m.HandleFunc("GET "+p+"series/{id}", s.getSeries)
	m.HandleFunc("POST "+p+"series", s.addSeries)
	m.HandleFunc("PUT "+p+"series", s.updateSeries)
	m.HandleFunc("PUT "+p+"series/{id}", s.updateSeries)
	m.HandleFunc("GET "+p+"episode", s.listEpisodes)
	m.HandleFunc("POST "+p+"command", s.tvCommand)
	m.HandleFunc("GET "+p+"command/{id}", s.getCommand)
	m.HandleFunc("GET "+p+"queue", s.queuePage(domain.KindTV, s.fillEpisode))
	m.HandleFunc("GET "+p+"queue/details", s.queueDetails(domain.KindTV, s.fillEpisode))
	m.HandleFunc("GET "+p+"queue/status", s.queueStatus(domain.KindTV))
	m.HandleFunc("GET "+p+"wanted/missing", s.missingEpisodes)
	m.HandleFunc("GET "+p+"calendar", s.tvCalendar)
}

// ── shapes ───────────────────────────────────────────────────────────────────

type seasonStats struct {
	EpisodeFileCount  int     `json:"episodeFileCount"`
	EpisodeCount      int     `json:"episodeCount"`
	TotalEpisodeCount int     `json:"totalEpisodeCount"`
	SizeOnDisk        int64   `json:"sizeOnDisk"`
	PercentOfEpisodes float64 `json:"percentOfEpisodes"`
}

type seriesStats struct {
	SeasonCount int `json:"seasonCount"`
	seasonStats
}

type season struct {
	SeasonNumber int          `json:"seasonNumber"`
	Monitored    bool         `json:"monitored"`
	Statistics   *seasonStats `json:"statistics,omitempty"`
}

type series struct {
	ID                int64        `json:"id"`
	Title             string       `json:"title"`
	SortTitle         string       `json:"sortTitle"`
	Status            string       `json:"status"`
	Overview          string       `json:"overview"`
	Network           string       `json:"network"`
	Year              int          `json:"year"`
	Path              string       `json:"path,omitempty"`
	RootFolderPath    string       `json:"rootFolderPath,omitempty"`
	QualityProfileID  int64        `json:"qualityProfileId"`
	LanguageProfileID int64        `json:"languageProfileId"`
	SeasonFolder      bool         `json:"seasonFolder"`
	Monitored         bool         `json:"monitored"`
	Runtime           int          `json:"runtime"`
	TvdbID            int64        `json:"tvdbId"`
	TmdbID            int64        `json:"tmdbId"`
	ImdbID            string       `json:"imdbId"`
	TitleSlug         string       `json:"titleSlug"`
	SeriesType        string       `json:"seriesType"`
	Genres            []string     `json:"genres"`
	Tags              []int64      `json:"tags"`
	Added             string       `json:"added,omitempty"`
	Images            []image      `json:"images"`
	RemotePoster      string       `json:"remotePoster,omitempty"`
	Seasons           []season     `json:"seasons"`
	Statistics        *seriesStats `json:"statistics,omitempty"`
}

type episode struct {
	ID                    int64     `json:"id"`
	SeriesID              int64     `json:"seriesId"`
	EpisodeFileID         int64     `json:"episodeFileId"`
	SeasonNumber          int       `json:"seasonNumber"`
	EpisodeNumber         int       `json:"episodeNumber"`
	AbsoluteEpisodeNumber int       `json:"absoluteEpisodeNumber,omitempty"`
	Title                 string    `json:"title"`
	AirDate               string    `json:"airDate,omitempty"`
	AirDateUtc            string    `json:"airDateUtc,omitempty"`
	Runtime               int       `json:"runtime"`
	HasFile               bool      `json:"hasFile"`
	Monitored             bool      `json:"monitored"`
	Series                *titleRef `json:"series,omitempty"`
}

func episodeOf(e domain.Episode, ser *titleRef) episode {
	return episode{ID: e.ID, SeriesID: e.SeriesID, EpisodeFileID: e.FileID, SeasonNumber: e.Season, EpisodeNumber: e.Number,
		AbsoluteEpisodeNumber: e.Absolute, Title: e.Title, AirDate: isoDate(e.AirAt), AirDateUtc: isoTime(e.AirAt),
		Runtime: e.RuntimeMin, HasFile: e.FileID != 0, Monitored: e.Monitored, Series: ser}
}

func percent(have, of int) float64 {
	if of == 0 {
		return 0
	}
	return float64(have) * 100 / float64(of)
}

// seriesOf shapes a library series with its season statistics.
func (s *Service) seriesOf(ctx context.Context, d tv.Detail) series {
	ser, st := d.Series.Series, d.Series.Stats
	out := series{ID: ser.ID, Title: ser.Title, SortTitle: ser.SortTitle, Status: ser.Status, Overview: ser.Overview,
		Network: ser.Network, Year: ser.Year, Path: ser.Path, RootFolderPath: ser.RootFolder,
		QualityProfileID: s.idOf(ctx, spaceProfile, ser.ProfileKey), SeasonFolder: ser.SeasonFolders, Monitored: ser.Monitored,
		Runtime: ser.RuntimeMin, TvdbID: ser.TVDBID, TmdbID: ser.TMDBID, ImdbID: ser.IMDBID, TitleSlug: slug(ser.Title),
		SeriesType: string(ser.Type), Genres: nonNilStrings(ser.Genres), Tags: s.tagIDs(ctx, ser.Tags),
		Added: ser.AddedAt.UTC().Format(time.RFC3339), Images: images(ser.PosterURL), RemotePoster: ser.PosterURL, Seasons: []season{},
		Statistics: &seriesStats{SeasonCount: st.SeasonCount, seasonStats: seasonStats{EpisodeFileCount: st.EpisodeFileCount,
			EpisodeCount: st.EpisodeCount, TotalEpisodeCount: st.TotalEpisodeCount, SizeOnDisk: st.SizeOnDisk,
			PercentOfEpisodes: percent(st.EpisodeFileCount, st.EpisodeCount)}}}
	for _, sv := range d.Seasons {
		out.Seasons = append(out.Seasons, season{SeasonNumber: sv.Number, Monitored: sv.Monitored, Statistics: &seasonStats{
			EpisodeFileCount: sv.FileCount, EpisodeCount: sv.AiredCount, TotalEpisodeCount: sv.EpisodeCount,
			SizeOnDisk: sv.SizeOnDisk, PercentOfEpisodes: percent(sv.FileCount, sv.AiredCount)}})
	}
	return out
}

// lookupOf shapes a provider match that is not in the library.
func lookupOf(it tv.LookupItem) series {
	rec := it.SeriesRecord
	out := series{Title: rec.Title, SortTitle: tv.SortTitle(rec.Title), Status: rec.Status, Overview: rec.Overview,
		Network: rec.Network, Year: rec.Year, Runtime: rec.RuntimeMin, TvdbID: idNum(rec.IDs["tvdb"]),
		TmdbID: idNum(rec.IDs["tmdb"]), ImdbID: rec.IDs["imdb"], TitleSlug: slug(rec.Title), SeriesType: string(domain.SeriesStandard),
		Genres: nonNilStrings(rec.Genres), Tags: []int64{}, Images: images(rec.PosterURL), RemotePoster: rec.PosterURL,
		SeasonFolder: true, Seasons: []season{}}
	for n := 1; n <= rec.SeasonCount; n++ {
		out.Seasons = append(out.Seasons, season{SeasonNumber: n})
	}
	return out
}

func idNum(v string) int64 {
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// ── series ───────────────────────────────────────────────────────────────────

// allSeries shapes every library series; keep filters by series.
func (s *Service) allSeries(ctx context.Context, keep func(domain.Series) bool) ([]series, error) {
	var ids []int64
	err := s.d.TV.EachSeries(ctx, func(ser domain.Series) error {
		if keep(ser) {
			ids = append(ids, ser.ID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := []series{}
	for _, id := range ids {
		d, err := s.d.TV.Detail(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, s.seriesOf(ctx, d))
	}
	return out, nil
}

func (s *Service) listSeries(w http.ResponseWriter, r *http.Request) {
	tvdb := queryInt64(r, "tvdbId")
	list, err := s.allSeries(r.Context(), func(ser domain.Series) bool { return tvdb == 0 || ser.TVDBID == tvdb })
	if err != nil {
		s.writeErr(w, err)
		return
	}
	ok(w, list)
}

func (s *Service) getSeries(w http.ResponseWriter, r *http.Request) {
	id, valid := pathID(w, r)
	if !valid {
		return
	}
	s.writeSeries(w, r, id, http.StatusOK)
}

func (s *Service) writeSeries(w http.ResponseWriter, r *http.Request, id int64, status int) {
	d, err := s.d.TV.Detail(r.Context(), id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, status, s.seriesOf(r.Context(), d))
}

// lookupSeries answers term=tvdb:N, tmdb:N, imdb:tt… or free text; matches
// in the library come back with their library id and state.
func (s *Service) lookupSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := s.d.TV.Lookup(ctx, r.URL.Query().Get("term"))
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := []series{}
	for _, it := range items {
		if it.LibraryID == 0 {
			out = append(out, lookupOf(it))
			continue
		}
		d, err := s.d.TV.Detail(ctx, it.LibraryID)
		if err != nil {
			s.writeErr(w, err)
			return
		}
		out = append(out, s.seriesOf(ctx, d))
	}
	ok(w, out)
}

// seriesBody is what the clients send to add or edit a series.
type seriesBody struct {
	ID               int64    `json:"id"`
	TvdbID           int64    `json:"tvdbId"`
	TmdbID           int64    `json:"tmdbId"`
	QualityProfileID int64    `json:"qualityProfileId"`
	RootFolderPath   string   `json:"rootFolderPath"`
	Monitored        *bool    `json:"monitored"`
	SeasonFolder     *bool    `json:"seasonFolder"`
	SeriesType       string   `json:"seriesType"`
	Tags             *[]int64 `json:"tags"`
	Seasons          []season `json:"seasons"`
	AddOptions       struct {
		Monitor                  string `json:"monitor"`
		SearchForMissingEpisodes bool   `json:"searchForMissingEpisodes"`
	} `json:"addOptions"`
}

// monitorModes maps the add option onto corsarr's monitoring modes.
var monitorModes = map[string]domain.MonitorMode{
	"all": domain.MonitorAll, "future": domain.MonitorFuture, "missing": domain.MonitorMissing,
	"existing": domain.MonitorExisting, "firstSeason": domain.MonitorFirstSeason, "latestSeason": domain.MonitorLatestSeason,
	"lastSeason": domain.MonitorLatestSeason, "pilot": domain.MonitorPilot, "none": domain.MonitorNone,
}

var errNoSeries = errors.New("no series matches this tvdbId")

// tmdbOf resolves the series to its TMDB id (corsarr's series key).
func (s *Service) tmdbOf(ctx context.Context, b seriesBody) (int64, error) {
	if b.TmdbID > 0 {
		return b.TmdbID, nil
	}
	if b.TvdbID <= 0 {
		return 0, tv.FieldError{Field: "tvdbId", Msg: "a tvdbId is required"}
	}
	items, err := s.d.TV.Lookup(ctx, "tvdb:"+strconv.FormatInt(b.TvdbID, 10))
	if err != nil {
		return 0, err
	}
	if len(items) == 0 || idNum(items[0].IDs["tmdb"]) == 0 {
		return 0, tv.FieldError{Field: "tvdbId", Msg: errNoSeries.Error()}
	}
	return idNum(items[0].IDs["tmdb"]), nil
}

// addRequest builds the native add. Without a monitor option the requested
// seasons are monitored, so the search on add covers exactly them.
func (s *Service) addRequest(ctx context.Context, b seriesBody) (tv.AddRequest, error) {
	tmdb, err := s.tmdbOf(ctx, b)
	if err != nil {
		return tv.AddRequest{}, err
	}
	key, known := s.profileKey(ctx, b.QualityProfileID)
	if !known {
		return tv.AddRequest{}, tv.FieldError{Field: "qualityProfileId", Msg: "unknown quality profile"}
	}
	req := tv.AddRequest{TMDBID: tmdb, RootFolder: b.RootFolderPath, ProfileKey: key, SeasonFolders: b.SeasonFolder,
		SeriesType: b.SeriesType, Tags: s.tagLabels(ctx, deref(b.Tags)), Monitor: string(domain.MonitorAll),
		SearchNow: b.AddOptions.SearchForMissingEpisodes}
	if mode, given := monitorModes[b.AddOptions.Monitor]; given {
		req.Monitor = string(mode)
		return req, nil
	}
	if len(b.Seasons) > 0 {
		req.Seasons = []int{}
		for _, sn := range b.Seasons {
			if sn.Monitored {
				req.Seasons = append(req.Seasons, sn.SeasonNumber)
			}
		}
	}
	return req, nil
}

func deref[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}

func (s *Service) addSeries(w http.ResponseWriter, r *http.Request) {
	var b seriesBody
	if !s.writable(w, domain.KindTV) || !decode(w, r, &b) {
		return
	}
	ctx := r.Context()
	req, err := s.addRequest(ctx, b)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	ser, created, err := s.d.TV.Add(ctx, req)
	if err == nil && created {
		err = s.unmonitorIfAsked(ctx, ser.ID, b.Monitored)
	}
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeSeries(w, r, ser.ID, createdStatus(created))
}

// unmonitorIfAsked unmonitors a series added with monitored false.
func (s *Service) unmonitorIfAsked(ctx context.Context, id int64, monitored *bool) error {
	if monitored == nil || *monitored {
		return nil
	}
	_, err := s.d.TV.Update(ctx, id, tv.Patch{Monitored: monitored})
	return err
}

// monitorSeasons applies the seasons' monitored flags that differ from
// current; seasons the series does not have are skipped.
func (s *Service) monitorSeasons(ctx context.Context, id int64, want []season, current map[int]bool) error {
	for _, sn := range want {
		if was, known := current[sn.SeasonNumber]; !known || was == sn.Monitored {
			continue
		}
		if err := s.d.TV.SetSeasonMonitored(ctx, id, sn.SeasonNumber, sn.Monitored); err != nil {
			return err
		}
	}
	return nil
}

// updateSeries applies monitoring, profile, folder, type, tag and season
// changes from a whole series resource.
func (s *Service) updateSeries(w http.ResponseWriter, r *http.Request) {
	var b seriesBody
	if !s.writable(w, domain.KindTV) || !decode(w, r, &b) || !bodyID(w, r, &b.ID) {
		return
	}
	ctx := r.Context()
	d, err := s.d.TV.Detail(ctx, b.ID)
	if err == nil {
		err = s.applySeries(ctx, d, b)
	}
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeSeries(w, r, b.ID, http.StatusAccepted)
}

func (s *Service) applySeries(ctx context.Context, d tv.Detail, b seriesBody) error {
	p := tv.Patch{Monitored: b.Monitored, SeasonFolders: b.SeasonFolder}
	if b.QualityProfileID != 0 {
		key, known := s.profileKey(ctx, b.QualityProfileID)
		if !known {
			return tv.FieldError{Field: "qualityProfileId", Msg: "unknown quality profile"}
		}
		p.ProfileKey = &key
	}
	if b.SeriesType != "" {
		p.SeriesType = &b.SeriesType
	}
	if b.Tags != nil {
		labels := s.tagLabels(ctx, *b.Tags)
		p.Tags = &labels
	}
	if _, err := s.d.TV.Update(ctx, b.ID, p); err != nil {
		return err
	}
	current := map[int]bool{}
	for _, sv := range d.Seasons {
		current[sv.Number] = sv.Monitored
	}
	return s.monitorSeasons(ctx, b.ID, b.Seasons, current)
}

// ── episodes, wanted, calendar ───────────────────────────────────────────────

func (s *Service) listEpisodes(w http.ResponseWriter, r *http.Request) {
	id := queryInt64(r, "seriesId")
	if id <= 0 {
		invalid(w, "seriesId", "seriesId is required")
		return
	}
	d, err := s.d.TV.Detail(r.Context(), id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]episode, 0, len(d.Episodes))
	for _, e := range d.Episodes {
		out = append(out, episodeOf(e.Episode, nil))
	}
	ok(w, out)
}

func (s *Service) missingEpisodes(w http.ResponseWriter, r *http.Request) {
	entries, _ := s.d.TVSearch.Index().Page(automation.ReasonMissing, 0, 1<<30)
	out := make([]episode, 0, len(entries))
	for _, e := range entries {
		out = append(out, episodeOf(e.Episode.Episode, &titleRef{ID: e.Series.ID, Title: e.Series.Title}))
	}
	p := pageOf(r, out)
	p.SortKey = "airDateUtc"
	ok(w, p)
}

func (s *Service) tvCalendar(w http.ResponseWriter, r *http.Request) {
	from, to := s.window(r)
	items, err := s.d.TV.Upcoming(r.Context(), from, to)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]episode, 0, len(items))
	for _, it := range items {
		out = append(out, episodeOf(it.Episode.Episode, &titleRef{ID: it.Series.ID, Title: it.Series.Title}))
	}
	ok(w, out)
}

// fillEpisode names a TV queue row's episode and series.
func (s *Service) fillEpisode(ctx context.Context, rec *queueRecord, ref domain.ItemRef) {
	e, err := s.d.TV.Episode(ctx, ref.ID)
	if err != nil {
		return
	}
	n := e.Season
	rec.EpisodeID, rec.SeriesID, rec.SeasonNumber = e.ID, e.SeriesID, &n
	rec.Episode = &queueEpisode{ID: e.ID, SeriesID: e.SeriesID, SeasonNumber: e.Season, EpisodeNumber: e.Number,
		Title: e.Title, AirDateUtc: isoTime(e.AirAt)}
	if ser, err := s.d.TV.Series(ctx, e.SeriesID); err == nil {
		rec.Series = &titleRef{ID: ser.ID, Title: ser.Title}
	}
}

// ── commands ─────────────────────────────────────────────────────────────────

// tvCommand runs SeriesSearch, SeasonSearch and EpisodeSearch through TV
// automation in the background.
func (s *Service) tvCommand(w http.ResponseWriter, r *http.Request) {
	var b commandBody
	if !s.writable(w, domain.KindTV) || !decode(w, r, &b) {
		return
	}
	run, err := s.tvRun(b)
	if err != nil {
		invalid(w, "name", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, s.start(b.Name, b.echo(), run))
}

func (s *Service) tvRun(b commandBody) (func(context.Context) error, error) {
	search := s.d.TVSearch
	switch {
	case b.Name == "SeriesSearch" && b.SeriesID > 0:
		return func(ctx context.Context) error {
			return summaryErr(search.SearchSeries(ctx, b.SeriesID, domain.OriginAutomatic))
		}, nil
	case b.Name == "SeasonSearch" && b.SeriesID > 0 && b.SeasonNumber != nil:
		return func(ctx context.Context) error {
			return summaryErr(search.SearchSeason(ctx, b.SeriesID, *b.SeasonNumber, domain.OriginAutomatic))
		}, nil
	case b.Name == "EpisodeSearch" && len(b.EpisodeIDs) > 0:
		return func(ctx context.Context) error {
			var errs []error
			for _, id := range b.EpisodeIDs {
				errs = append(errs, summaryErr(search.SearchEpisode(ctx, id, domain.OriginAutomatic)))
			}
			return errors.Join(errs...)
		}, nil
	}
	return nil, errUnknownCommand(b.Name)
}

func summaryErr(_ tvauto.Summary, err error) error { return err }
