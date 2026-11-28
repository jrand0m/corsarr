package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/queue"
	"github.com/jrand0m/corsarr/internal/tv"
)

// requestTools find and request media of every kind and follow requests up.
func (s *Service) requestTools() []Tool {
	return []Tool{
		{Name: "find_media", Title: "Find media", OpenWorld: true,
			Description: "Search movies, TV shows and music (artists, albums or tracks) at the metadata providers. " +
				"Returns candidates with external ids, year, poster and whether corsarr already has them. Show the person the candidate before requesting it.",
			Input: object(map[string]schema{
				"query":  str("What to look for: a title or name."),
				"kind":   enum("Limit to one kind; all kinds when omitted.", "movie", "tv", "music", "book"),
				"level":  enum("Music only: artist (default), album or track.", "artist", "album", "track"),
				"artist": str("Music album/track level: the artist's name or MusicBrainz id."),
			}, "query"),
			Run: bind(s.findMedia)},
		{Name: "request_movie", Title: "Request a movie", Write: true, Idempotent: true, OpenWorld: true,
			Description: "Add a movie by TMDB or IMDb id and search for it. Asking again returns the existing movie and its status.",
			Input: object(map[string]schema{
				"tmdbId":     integer("TMDB id (from find_media)."),
				"imdbId":     str("IMDb id, when the TMDB id is unknown."),
				"profileKey": str("Quality profile key (list_profiles); the default profile when omitted."),
				"rootFolder": str("Root folder path (list_root_folders); the first movie root when omitted."),
				"search":     boolean("Search right away (default true)."),
			}),
			Run: bind(s.requestMovie)},
		{Name: "request_show", Title: "Request a TV show", Write: true, Idempotent: true, OpenWorld: true,
			Description: "Add a series by TMDB or TVDB id with what to monitor (all, future, missing, first or latest season, given seasons or episodes) and search for it. " +
				"For a series already in the library, given seasons or episodes are monitored and searched.",
			Input: object(map[string]schema{
				"tmdbId":     integer("TMDB id (from find_media)."),
				"tvdbId":     integer("TVDB id, when the TMDB id is unknown."),
				"monitor":    enum("What to monitor (default all); seasons or episodes override it.", tvMonitorNames()...),
				"seasons":    array("Monitor exactly these season numbers.", integer("Season number.")),
				"episodes":   array("Monitor exactly these episodes.", object(map[string]schema{"season": integer("Season."), "episode": integer("Episode.")}, "season", "episode")),
				"profileKey": str("Quality profile key (list_profiles)."),
				"rootFolder": str("Root folder path (list_root_folders)."),
				"search":     boolean("Search right away (default true)."),
			}),
			Run: bind(s.requestShow)},
		{Name: "request_music", Title: "Request music", Write: true, Idempotent: true, OpenWorld: true,
			Description: "Request an artist (with monitor all, future or none), one album (release group; the artist is added unmonitored when absent) " +
				"or one track (resolved to the album, EP or single that holds it, which is requested).",
			Input: object(map[string]schema{
				"artistMbid": str("MusicBrainz id of the artist (from find_media); always required."),
				"albumMbid":  str("MusicBrainz release group id: request this album only."),
				"track":      str("A track title: request the album, EP or single that holds it."),
				"monitor":    enum("Artist requests: which albums to monitor (default all).", "all", "future", "none"),
				"profileKey": str("Quality profile key (list_profiles)."),
				"rootFolder": str("Root folder path (list_root_folders)."),
				"search":     boolean("Search right away (default true)."),
			}, "artistMbid"),
			Run: bind(s.requestMusic)},
		{Name: "request_book", Title: "Request a book", Write: true, Idempotent: true,
			Description: "Request an ebook or audiobook by ISBN or OpenLibrary id. Refused with the reason while books are disabled or unsupported.",
			Input: object(map[string]schema{
				"isbn":          str("ISBN-10 or ISBN-13."),
				"openLibraryId": str("OpenLibrary work or edition id."),
				"format":        enum("Which format (default ebook).", "ebook", "audiobook", "both"),
			}),
			Run: bind(s.requestBook)},
		{Name: "request_status", Title: "Request status",
			Description: "What happened to a request: imported (with paths), downloading (release, client, progress), searching, " +
				"or why not (no releases, all rejected, shadow mode, unmonitored).",
			Input: object(targetProps(), "kind"),
			Run:   bind(s.requestStatus)},
		{Name: "cancel_request", Title: "Cancel a request", Write: true, Destructive: true, Idempotent: true,
			Description: "Stop wanting an item: unmonitor it and remove its queued downloads. Downloaded data is kept unless deleteFiles is true.",
			Input: object(withTarget(map[string]schema{
				"deleteFiles": boolean("Also delete the removed downloads' data from the download client."),
			}), "kind"),
			Run: bind(s.cancelRequest)},
	}
}

func withTarget(extra map[string]schema) map[string]schema {
	props := targetProps()
	for k, v := range extra {
		props[k] = v
	}
	return props
}

// ── find_media ───────────────────────────────────────────────────────────────

// Candidate is one find_media match.
type Candidate struct {
	Kind      domain.MediaKind  `json:"kind"`
	Level     string            `json:"level,omitempty"`
	Title     string            `json:"title"`
	Artist    string            `json:"artist,omitempty"`
	Year      int               `json:"year,omitempty"`
	Type      string            `json:"type,omitempty"`
	IDs       map[string]string `json:"ids"`
	PosterURL string            `json:"posterUrl,omitempty"`
	Overview  string            `json:"overview,omitempty"`
	InLibrary bool              `json:"inLibrary"`
	LibraryID int64             `json:"libraryId,omitempty"`
}

// maxCandidates bounds the matches of one kind.
const maxCandidates = 10

type findArgs struct {
	Query  string `json:"query"`
	Kind   string `json:"kind"`
	Level  string `json:"level"`
	Artist string `json:"artist"`
}

func (s *Service) findMedia(ctx context.Context, in findArgs) (Result, error) {
	if strings.TrimSpace(in.Query) == "" {
		return Result{}, invalidArg("query", "is required")
	}
	out, notes := []Candidate{}, []string{}
	for _, k := range s.findKinds(in.Kind) {
		found, err := s.findKind(ctx, k, in)
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s: %s", k, errText(err)))
			continue
		}
		out = append(out, found...)
	}
	data := FindOut{Candidates: out}
	if len(notes) > 0 {
		data.Notes = notes
	}
	return Result{Text: candidatesText(out, notes), Data: data}, nil
}

// findKinds is the asked kind, else every kind that is not disabled.
func (s *Service) findKinds(kind string) []domain.MediaKind {
	if kind != "" {
		return []domain.MediaKind{domain.MediaKind(kind)}
	}
	var out []domain.MediaKind
	for _, k := range []domain.MediaKind{domain.KindMovie, domain.KindTV, domain.KindMusic} {
		if s.usable(k) == nil {
			out = append(out, k)
		}
	}
	return out
}

func (s *Service) findKind(ctx context.Context, k domain.MediaKind, in findArgs) ([]Candidate, error) {
	if _, err := s.kindArg(string(k)); err != nil {
		return nil, err
	}
	find := map[domain.MediaKind]func(context.Context, findArgs) ([]Candidate, error){
		domain.KindMovie: s.findMovies, domain.KindTV: s.findShows, domain.KindMusic: s.findMusicMedia,
	}[k]
	list, err := find(ctx, in)
	return list[:min(len(list), maxCandidates)], err
}

func (s *Service) findMovies(ctx context.Context, in findArgs) ([]Candidate, error) {
	recs, err := s.d.Movies.Lookup(ctx, in.Query)
	if err != nil {
		return nil, err
	}
	have := map[string]int64{}
	err = s.d.Movies.Each(ctx, func(m domain.Movie) error {
		have[strconv.FormatInt(m.TMDBID, 10)] = m.ID
		return nil
	})
	out := make([]Candidate, 0, len(recs))
	for _, r := range recs {
		id := have[r.IDs["tmdb"]]
		out = append(out, Candidate{Kind: domain.KindMovie, Title: r.Title, Year: r.Year, IDs: r.IDs, PosterURL: r.PosterURL,
			Overview: short(r.Overview), InLibrary: id > 0, LibraryID: id})
	}
	return out, err
}

func (s *Service) findShows(ctx context.Context, in findArgs) ([]Candidate, error) {
	items, err := s.d.TV.Lookup(ctx, in.Query)
	out := make([]Candidate, 0, len(items))
	for _, it := range items {
		out = append(out, Candidate{Kind: domain.KindTV, Title: it.Title, Year: it.Year, IDs: it.IDs, PosterURL: it.PosterURL,
			Overview: short(it.Overview), InLibrary: it.LibraryID > 0, LibraryID: it.LibraryID})
	}
	return out, err
}

// short trims an overview to keep results small.
func short(s string) string {
	const max = 280
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

func candidatesText(list []Candidate, notes []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d candidates", len(list))
	for _, c := range list {
		fmt.Fprintf(&b, "\n- %s", candidateLine(c))
	}
	for _, n := range notes {
		fmt.Fprintf(&b, "\nnote: %s", n)
	}
	return b.String()
}

func candidateLine(c Candidate) string {
	line := fmt.Sprintf("[%s] %s", c.Kind, c.Title)
	if c.Artist != "" {
		line += " by " + c.Artist
	}
	if c.Year > 0 {
		line += fmt.Sprintf(" (%d)", c.Year)
	}
	if c.InLibrary {
		line += " — in library"
	}
	return line + " " + idsText(c.IDs)
}

func idsText(ids map[string]string) string {
	parts := []string{}
	for _, k := range []string{"tmdb", "tvdb", "imdb", "mbid", "artistMbid"} {
		if v := ids[k]; v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	return strings.Join(parts, " ")
}

// ── request_movie ────────────────────────────────────────────────────────────

type movieRequest struct {
	TMDBID     int64  `json:"tmdbId"`
	IMDBID     string `json:"imdbId"`
	ProfileKey string `json:"profileKey"`
	RootFolder string `json:"rootFolder"`
	Search     *bool  `json:"search"`
}

func wantSearch(b *bool) bool { return b == nil || *b }

func (s *Service) requestMovie(ctx context.Context, in movieRequest) (Result, error) {
	if err := s.usable(domain.KindMovie); err != nil {
		return Result{}, err
	}
	id, err := s.movieTMDB(ctx, in)
	if err != nil {
		return Result{}, err
	}
	m, created, err := s.d.Movies.Add(ctx, movies.AddRequest{TMDBID: id, ProfileKey: in.ProfileKey, RootFolder: in.RootFolder, SearchNow: wantSearch(in.Search)})
	if err != nil {
		return Result{}, err
	}
	return s.requested(ctx, domain.KindMovie, target{ID: m.ID}, created)
}

// movieTMDB is the TMDB id, looked up from the IMDb id when needed.
func (s *Service) movieTMDB(ctx context.Context, in movieRequest) (int64, error) {
	if in.TMDBID > 0 {
		return in.TMDBID, nil
	}
	if in.IMDBID == "" {
		return 0, invalidArg("tmdbId", "give tmdbId or imdbId")
	}
	recs, err := s.d.Movies.Lookup(ctx, "imdb:"+in.IMDBID)
	if err != nil || len(recs) == 0 {
		return 0, notFoundOr(err, "no movie with IMDb id "+in.IMDBID)
	}
	return strconv.ParseInt(recs[0].IDs["tmdb"], 10, 64)
}

func notFoundOr(err error, msg string) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("%s", msg)
}

// requested answers a request with the item's status.
func (s *Service) requested(ctx context.Context, kind domain.MediaKind, t target, created bool) (Result, error) {
	st, err := s.status(ctx, kind, t)
	if err != nil {
		return Result{}, err
	}
	st.Created = created
	verb := "already in the library"
	if created {
		verb = "requested"
	}
	return Result{Text: verb + ": " + statusText(st), Data: st}, nil
}

// ── request_show ─────────────────────────────────────────────────────────────

type episodeNo struct {
	Season  int `json:"season"`
	Episode int `json:"episode"`
}

type showRequest struct {
	TMDBID     int64       `json:"tmdbId"`
	TVDBID     int64       `json:"tvdbId"`
	Monitor    string      `json:"monitor"`
	Seasons    []int       `json:"seasons"`
	Episodes   []episodeNo `json:"episodes"`
	ProfileKey string      `json:"profileKey"`
	RootFolder string      `json:"rootFolder"`
	Search     *bool       `json:"search"`
}

// tvMonitors maps the tool's monitor words onto the TV module's modes.
var tvMonitors = map[string]domain.MonitorMode{
	"": domain.MonitorAll, "all": domain.MonitorAll, "future": domain.MonitorFuture, "missing": domain.MonitorMissing,
	"first_season": domain.MonitorFirstSeason, "latest_season": domain.MonitorLatestSeason, "pilot": domain.MonitorPilot,
	"none": domain.MonitorNone,
}

func tvMonitorNames() []string {
	return []string{"all", "future", "missing", "first_season", "latest_season", "pilot", "none"}
}

func (s *Service) requestShow(ctx context.Context, in showRequest) (Result, error) {
	if err := s.usable(domain.KindTV); err != nil {
		return Result{}, err
	}
	req, err := s.showAdd(ctx, in)
	if err != nil {
		return Result{}, err
	}
	ser, created, err := s.d.TV.Add(ctx, req)
	if err != nil {
		return Result{}, err
	}
	if err := s.monitorParts(ctx, ser.ID, in, created); err != nil {
		return Result{}, err
	}
	return s.requested(ctx, domain.KindTV, target{ID: ser.ID}, created)
}

// showAdd builds the add request; episode picks start from nothing
// monitored and are applied after the add.
func (s *Service) showAdd(ctx context.Context, in showRequest) (tv.AddRequest, error) {
	mode, ok := tvMonitors[in.Monitor]
	if !ok {
		return tv.AddRequest{}, invalidArg("monitor", "must be one of "+strings.Join(tvMonitorNames(), ", "))
	}
	id, err := s.showTMDB(ctx, in)
	req := tv.AddRequest{TMDBID: id, Monitor: string(mode), Seasons: in.Seasons, ProfileKey: in.ProfileKey, RootFolder: in.RootFolder,
		SearchNow: wantSearch(in.Search) && len(in.Episodes) == 0}
	if len(in.Episodes) > 0 {
		req.Monitor, req.Seasons = string(domain.MonitorNone), nil
	}
	return req, err
}

func (s *Service) showTMDB(ctx context.Context, in showRequest) (int64, error) {
	if in.TMDBID > 0 {
		return in.TMDBID, nil
	}
	if in.TVDBID <= 0 {
		return 0, invalidArg("tmdbId", "give tmdbId or tvdbId")
	}
	items, err := s.d.TV.Lookup(ctx, "tvdb:"+strconv.FormatInt(in.TVDBID, 10))
	if err != nil || len(items) == 0 {
		return 0, notFoundOr(err, fmt.Sprintf("no series with TVDB id %d", in.TVDBID))
	}
	return strconv.ParseInt(items[0].IDs["tmdb"], 10, 64)
}

// monitorParts applies season and episode picks: episodes always (they were
// added unmonitored), seasons on a series that was already there; then it
// searches what was picked.
func (s *Service) monitorParts(ctx context.Context, seriesID int64, in showRequest, created bool) error {
	if len(in.Episodes) == 0 && (created || len(in.Seasons) == 0) {
		return nil
	}
	ids, err := s.pickEpisodes(ctx, seriesID, in)
	if err != nil {
		return err
	}
	if _, err := s.d.TV.SetEpisodesMonitored(ctx, ids, true); err != nil {
		return err
	}
	if wantSearch(in.Search) {
		s.searchEpisodes(ids)
	}
	return nil
}

// pickEpisodes resolves the season and episode picks to episode ids.
func (s *Service) pickEpisodes(ctx context.Context, seriesID int64, in showRequest) ([]int64, error) {
	eps, err := s.d.TV.Episodes(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	want := map[episodeNo]bool{}
	for _, e := range in.Episodes {
		want[e] = true
	}
	seasons := map[int]bool{}
	for _, n := range in.Seasons {
		seasons[n] = true
	}
	var ids []int64
	for _, e := range eps {
		if want[episodeNo{e.Season, e.Number}] || seasons[e.Season] {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 {
		return nil, invalidArg("episodes", "no episode of the series matches the picked seasons or episodes")
	}
	return ids, nil
}

func (s *Service) searchEpisodes(ids []int64) {
	s.d.Go(func(ctx context.Context) {
		for _, id := range ids {
			_, err := s.d.TVSearch.SearchEpisode(ctx, id, domain.OriginAutomatic)
			s.logErr("episode search", err)
		}
	})
}

func (s *Service) logErr(what string, err error) {
	if err != nil {
		s.d.Log.Warn("mcp: "+what, "err", err)
	}
}

// ── request_book ─────────────────────────────────────────────────────────────

type bookRequest struct {
	ISBN          string `json:"isbn"`
	OpenLibraryID string `json:"openLibraryId"`
	Format        string `json:"format"`
}

// requestBook refuses: a disabled kind with its mode, otherwise because this
// build has no book library yet.
func (s *Service) requestBook(context.Context, bookRequest) (Result, error) {
	if err := s.usable(domain.KindBook); err != nil {
		return Result{}, err
	}
	return Result{}, unsupported("books are not supported by this build yet; nothing was requested")
}

// ── request_status and cancel_request ────────────────────────────────────────

func (s *Service) requestStatus(ctx context.Context, in target) (Result, error) {
	kind, t, err := s.targetArg(ctx, in)
	if err != nil {
		return Result{}, err
	}
	st, err := s.status(ctx, kind, t)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: statusText(st), Data: st}, nil
}

// targetArg validates the kind and resolves external ids.
func (s *Service) targetArg(ctx context.Context, in target) (domain.MediaKind, target, error) {
	kind, err := s.kindArg(in.Kind)
	if err != nil {
		return "", in, err
	}
	t, err := s.resolve(ctx, in)
	return kind, t, err
}

type cancelArgs struct {
	target
	DeleteFiles bool `json:"deleteFiles"`
}

func (s *Service) cancelRequest(ctx context.Context, in cancelArgs) (Result, error) {
	kind, t, err := s.targetArg(ctx, in.target)
	if err != nil {
		return Result{}, err
	}
	if err := s.unmonitor(ctx, kind, t); err != nil {
		return Result{}, err
	}
	st, err := s.status(ctx, kind, t)
	if err != nil {
		return Result{}, err
	}
	removed, err := s.removeDownloads(ctx, st.Downloads, in.DeleteFiles)
	st.Downloads = []DownloadView{}
	judge(&st)
	text := fmt.Sprintf("cancelled %s: unmonitored, %d downloads removed", st.Title, removed)
	return Result{Text: text, Data: CancelOut{Status: st, RemovedDownloads: removed}}, err
}

func (s *Service) unmonitor(ctx context.Context, kind domain.MediaKind, t target) error {
	off := false
	var err error
	switch {
	case kind == domain.KindMovie:
		_, err = s.d.Movies.Update(ctx, t.ID, movies.Patch{Monitored: &off})
	case kind == domain.KindTV:
		_, err = s.d.TV.Update(ctx, t.ID, tv.Patch{Monitored: &off})
	case t.AlbumID > 0:
		_, err = s.d.Music.SetAlbumsMonitored(ctx, []int64{t.AlbumID}, false)
	default:
		err = s.setArtistMonitored(ctx, t.ID, false)
	}
	return err
}

func (s *Service) removeDownloads(ctx context.Context, list []DownloadView, deleteFiles bool) (int, error) {
	n := 0
	for _, d := range list {
		if err := s.d.Queue.Remove(ctx, d.ID, queue.RemoveOptions{DeleteFiles: deleteFiles}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
