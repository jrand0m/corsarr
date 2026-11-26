package legacy

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/plugin"
)

// RadarrVersion is the API version the Radarr shim reports.
const RadarrVersion = "5.0.0"

func (s *Service) radarrRoutes(m *http.ServeMux) {
	const p = "/legacy/radarr/api/v3/"
	m.HandleFunc("GET "+p+"system/status", s.statusOf(config.LegacyRadarr, RadarrVersion))
	m.HandleFunc("GET "+p+"qualityprofile", s.profiles(domain.KindMovie))
	m.HandleFunc("GET "+p+"rootfolder", s.rootFolders(domain.KindMovie))
	m.HandleFunc("GET "+p+"tag", s.listTags)
	m.HandleFunc("POST "+p+"tag", s.createTag)
	m.HandleFunc("GET "+p+"movie", s.listMovies)
	m.HandleFunc("GET "+p+"movie/lookup", s.lookupMovies)
	m.HandleFunc("GET "+p+"movie/lookup/tmdb", s.lookupTMDB)
	m.HandleFunc("GET "+p+"movie/{id}", s.getMovie)
	m.HandleFunc("POST "+p+"movie", s.addMovie)
	m.HandleFunc("PUT "+p+"movie", s.updateMovie)
	m.HandleFunc("PUT "+p+"movie/{id}", s.updateMovie)
	m.HandleFunc("POST "+p+"command", s.movieCommand)
	m.HandleFunc("GET "+p+"command/{id}", s.getCommand)
	m.HandleFunc("GET "+p+"queue", s.queuePage(domain.KindMovie, s.fillMovie))
	m.HandleFunc("GET "+p+"queue/details", s.queueDetails(domain.KindMovie, s.fillMovie))
	m.HandleFunc("GET "+p+"queue/status", s.queueStatus(domain.KindMovie))
	m.HandleFunc("GET "+p+"calendar", s.movieCalendar)
}

type movie struct {
	ID                  int64    `json:"id"`
	Title               string   `json:"title"`
	OriginalTitle       string   `json:"originalTitle"`
	SortTitle           string   `json:"sortTitle"`
	Year                int      `json:"year"`
	TmdbID              int64    `json:"tmdbId"`
	ImdbID              string   `json:"imdbId"`
	TitleSlug           string   `json:"titleSlug"`
	Overview            string   `json:"overview"`
	Status              string   `json:"status"`
	Monitored           bool     `json:"monitored"`
	HasFile             bool     `json:"hasFile"`
	IsAvailable         bool     `json:"isAvailable"`
	MinimumAvailability string   `json:"minimumAvailability"`
	QualityProfileID    int64    `json:"qualityProfileId"`
	RootFolderPath      string   `json:"rootFolderPath,omitempty"`
	Path                string   `json:"path,omitempty"`
	Runtime             int      `json:"runtime"`
	InCinemas           string   `json:"inCinemas,omitempty"`
	DigitalRelease      string   `json:"digitalRelease,omitempty"`
	PhysicalRelease     string   `json:"physicalRelease,omitempty"`
	Added               string   `json:"added,omitempty"`
	Genres              []string `json:"genres"`
	Tags                []int64  `json:"tags"`
	Images              []image  `json:"images"`
	RemotePoster        string   `json:"remotePoster,omitempty"`
	SizeOnDisk          int64    `json:"sizeOnDisk"`
}

// releaseStatus words a movie's release state from its dates.
func releaseStatus(now time.Time, cinemas, digital, physical *time.Time) string {
	past := func(t *time.Time) bool { return t != nil && !t.After(now) }
	switch {
	case past(digital) || past(physical):
		return "released"
	case past(cinemas):
		return "inCinemas"
	case cinemas != nil || digital != nil || physical != nil:
		return "announced"
	}
	return "tba"
}

func (s *Service) movieOf(ctx context.Context, m domain.Movie) movie {
	now := s.d.Now()
	return movie{ID: m.ID, Title: m.Title, OriginalTitle: m.OriginalTitle, SortTitle: slug(m.Title), Year: m.Year,
		TmdbID: m.TMDBID, ImdbID: m.IMDBID, TitleSlug: slug(m.Title) + "-" + strconv.FormatInt(m.TMDBID, 10),
		Status: releaseStatus(now, m.InCinemas, m.Digital, m.Physical), Monitored: m.Monitored, HasFile: m.FileID != 0,
		IsAvailable: movies.IsAvailable(m, now), MinimumAvailability: string(m.MinAvailability),
		QualityProfileID: s.idOf(ctx, spaceProfile, m.ProfileKey), RootFolderPath: m.RootFolder, Path: m.Path,
		Runtime: m.RuntimeMin, InCinemas: isoTime(m.InCinemas), DigitalRelease: isoTime(m.Digital),
		PhysicalRelease: isoTime(m.Physical), Added: m.AddedAt.UTC().Format(time.RFC3339), Genres: []string{}, Tags: []int64{},
		Images: []image{}}
}

func (s *Service) recordOf(rec plugin.MovieRecord) movie {
	tmdb := idNum(rec.IDs["tmdb"])
	return movie{Title: rec.Title, OriginalTitle: rec.OriginalTitle, SortTitle: slug(rec.Title), Year: rec.Year, TmdbID: tmdb,
		ImdbID: rec.IDs["imdb"], TitleSlug: slug(rec.Title) + "-" + rec.IDs["tmdb"], Overview: rec.Overview,
		Status: releaseStatus(s.d.Now(), rec.InCinemas, rec.Digital, rec.Physical), MinimumAvailability: string(domain.AvailReleased),
		Runtime: rec.RuntimeMin, InCinemas: isoTime(rec.InCinemas), DigitalRelease: isoTime(rec.Digital),
		PhysicalRelease: isoTime(rec.Physical), Genres: nonNilStrings(rec.Genres), Tags: []int64{}, Images: images(rec.PosterURL),
		RemotePoster: rec.PosterURL}
}

// library lists the movies keep accepts.
func (s *Service) library(ctx context.Context, keep func(domain.Movie) bool) ([]domain.Movie, error) {
	var out []domain.Movie
	err := s.d.Movies.Each(ctx, func(m domain.Movie) error {
		if keep(m) {
			out = append(out, m)
		}
		return nil
	})
	return out, err
}

func (s *Service) listMovies(w http.ResponseWriter, r *http.Request) {
	tmdb := queryInt64(r, "tmdbId")
	list, err := s.library(r.Context(), func(m domain.Movie) bool { return tmdb == 0 || m.TMDBID == tmdb })
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]movie, 0, len(list))
	for _, m := range list {
		out = append(out, s.movieOf(r.Context(), m))
	}
	ok(w, out)
}

func (s *Service) getMovie(w http.ResponseWriter, r *http.Request) {
	if id, valid := pathID(w, r); valid {
		s.writeMovie(w, r, id, http.StatusOK)
	}
}

func (s *Service) writeMovie(w http.ResponseWriter, r *http.Request, id int64, status int) {
	m, err := s.d.Movies.Get(r.Context(), id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, status, s.movieOf(r.Context(), m))
}

// lookup searches the provider and marks matches already in the library.
func (s *Service) lookup(ctx context.Context, term string) ([]movie, error) {
	recs, err := s.d.Movies.Lookup(ctx, term)
	if err != nil {
		return nil, err
	}
	have, err := s.library(ctx, func(domain.Movie) bool { return true })
	if err != nil {
		return nil, err
	}
	byTMDB := map[int64]domain.Movie{}
	for _, m := range have {
		byTMDB[m.TMDBID] = m
	}
	out := make([]movie, 0, len(recs))
	for _, rec := range recs {
		if m, in := byTMDB[idNum(rec.IDs["tmdb"])]; in {
			out = append(out, s.movieOf(ctx, m))
			continue
		}
		out = append(out, s.recordOf(rec))
	}
	return out, nil
}

func (s *Service) lookupMovies(w http.ResponseWriter, r *http.Request) {
	out, err := s.lookup(r.Context(), r.URL.Query().Get("term"))
	if err != nil {
		s.writeErr(w, err)
		return
	}
	ok(w, out)
}

func (s *Service) lookupTMDB(w http.ResponseWriter, r *http.Request) {
	out, err := s.lookup(r.Context(), "tmdb:"+strconv.FormatInt(queryInt64(r, "tmdbId"), 10))
	switch {
	case err != nil:
		s.writeErr(w, err)
	case len(out) == 0:
		notFound(w, r)
	default:
		ok(w, out[0])
	}
}

// movieBody is what the clients send to add or edit a movie.
type movieBody struct {
	ID                  int64   `json:"id"`
	TmdbID              int64   `json:"tmdbId"`
	QualityProfileID    int64   `json:"qualityProfileId"`
	RootFolderPath      string  `json:"rootFolderPath"`
	Monitored           *bool   `json:"monitored"`
	MinimumAvailability *string `json:"minimumAvailability"`
	AddOptions          struct {
		SearchForMovie bool `json:"searchForMovie"`
	} `json:"addOptions"`
}

// availability maps the client's minimum availability onto corsarr's.
func availability(v *string) string {
	if v == nil {
		return ""
	}
	if *v == "tba" {
		return string(domain.AvailAnnounced)
	}
	return *v
}

func (s *Service) addMovie(w http.ResponseWriter, r *http.Request) {
	var b movieBody
	if !s.writable(w, domain.KindMovie) || !decode(w, r, &b) {
		return
	}
	key, known := s.profileKey(r.Context(), b.QualityProfileID)
	if !known {
		invalid(w, "qualityProfileId", "unknown quality profile")
		return
	}
	m, created, err := s.d.Movies.Add(r.Context(), movies.AddRequest{TMDBID: b.TmdbID, RootFolder: b.RootFolderPath, ProfileKey: key,
		Monitored: b.Monitored, MinAvailability: availability(b.MinimumAvailability), SearchNow: b.AddOptions.SearchForMovie})
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, createdStatus(created), s.movieOf(r.Context(), m))
}

func (s *Service) updateMovie(w http.ResponseWriter, r *http.Request) {
	var b movieBody
	if !s.writable(w, domain.KindMovie) || !decode(w, r, &b) || !bodyID(w, r, &b.ID) {
		return
	}
	p, err := s.moviePatch(r.Context(), b)
	var m domain.Movie
	if err == nil {
		m, err = s.d.Movies.Update(r.Context(), b.ID, p)
	}
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, s.movieOf(r.Context(), m))
}

// moviePatch turns a whole movie resource into the native edit.
func (s *Service) moviePatch(ctx context.Context, b movieBody) (movies.Patch, error) {
	p := movies.Patch{Monitored: b.Monitored}
	if b.QualityProfileID != 0 {
		key, known := s.profileKey(ctx, b.QualityProfileID)
		if !known {
			return p, movies.FieldError{Field: "qualityProfileId", Msg: "unknown quality profile"}
		}
		p.ProfileKey = &key
	}
	if b.MinimumAvailability != nil {
		a := availability(b.MinimumAvailability)
		p.MinAvailability = &a
	}
	if b.RootFolderPath != "" {
		p.RootFolder = &b.RootFolderPath
	}
	return p, nil
}

// movieCommand queues MoviesSearch through movie automation.
func (s *Service) movieCommand(w http.ResponseWriter, r *http.Request) {
	var b commandBody
	if !s.writable(w, domain.KindMovie) || !decode(w, r, &b) {
		return
	}
	if b.Name != "MoviesSearch" || len(b.MovieIDs) == 0 {
		invalid(w, "name", errUnknownCommand(b.Name).Error())
		return
	}
	writeJSON(w, http.StatusCreated, s.start(b.Name, b.echo(), func(ctx context.Context) error {
		for _, id := range b.MovieIDs {
			if _, err := s.d.Movies.Get(ctx, id); err != nil {
				return err
			}
			s.d.MovieSearch.Request(ctx, id)
		}
		return nil
	}))
}

// fillMovie names a movie queue row's movie.
func (s *Service) fillMovie(ctx context.Context, rec *queueRecord, ref domain.ItemRef) {
	rec.MovieID = ref.ID
	if m, err := s.d.Movies.Get(ctx, ref.ID); err == nil {
		rec.Movie = &titleRef{ID: m.ID, Title: m.Title}
	}
}

// movieCalendar lists the movies with a release date in the window
// (monitored only unless unmonitored=true).
func (s *Service) movieCalendar(w http.ResponseWriter, r *http.Request) {
	from, to := s.window(r)
	all := r.URL.Query().Get("unmonitored") == "true"
	in := func(t *time.Time) bool { return t != nil && !t.Before(from) && t.Before(to) }
	list, err := s.library(r.Context(), func(m domain.Movie) bool {
		return (all || m.Monitored) && (in(m.InCinemas) || in(m.Digital) || in(m.Physical))
	})
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]movie, 0, len(list))
	for _, m := range list {
		out = append(out, s.movieOf(r.Context(), m))
	}
	ok(w, out)
}
