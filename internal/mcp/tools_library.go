package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/automation"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
	"github.com/jrand0m/corsarr/internal/store"
	"github.com/jrand0m/corsarr/internal/tv"
)

// libraryTools read the library and system state and run its commands.
func (s *Service) libraryTools() []Tool {
	return []Tool{
		{Name: "search_library", Title: "Search the library", Description: "Find movies, series and artists corsarr already has by title or name.",
			Input: object(withPaging(map[string]schema{"query": str("Part of a title or name; empty lists everything."),
				"kind": kindEnum("Limit to one kind.")})), Run: bind(s.searchLibrary)},
		{Name: "get_item", Title: "Get an item", Description: "One library item in detail: a movie with its files, a series with seasons " +
			"(and the episodes of one season), an artist with albums, or one album with its tracks.",
			Input: object(map[string]schema{"kind": kindEnum("Media kind."), "id": integer("Library id: movie, series or artist."),
				"albumId": integer("Music: an album's library id instead of id."), "season": integer("TV: list this season's episodes.")}, "kind"),
			Run: bind(s.getItem)},
		{Name: "list_wanted", Title: "List wanted", Description: "What corsarr is still looking for: missing items or ones below their quality cutoff.",
			Input: object(withPaging(map[string]schema{"kind": kindEnum("Media kind."),
				"reason": enum("missing (default) or cutoff_unmet.", "missing", "cutoff_unmet")}), "kind"), Run: bind(s.listWanted)},
		{Name: "get_calendar", Title: "Calendar", Description: "Episodes airing and movies releasing in a date window (default: today and the next 7 days).",
			Input: object(map[string]schema{"from": str("Start date YYYY-MM-DD."), "to": str("End date YYYY-MM-DD (exclusive).")}), Run: bind(s.getCalendar)},
		{Name: "get_history", Title: "History", Description: "Recent activity: grabs, imports, failures, health changes, newest first.",
			Input: object(withPaging(map[string]schema{"type": str("Only events whose type starts with this, e.g. grab or import.")})), Run: bind(s.getHistory)},
		{Name: "get_health", Title: "Health", Description: "Integration health (indexers, download clients, metadata), root folders and media kind modes.",
			Input: object(map[string]schema{}), Run: bind(s.getHealth)},
		{Name: "search_releases", Title: "Search releases", OpenWorld: true,
			Description: "Ask the indexers for releases of one item and show each decision with its reasons, without grabbing anything.",
			Input: object(map[string]schema{"kind": kindEnum("Media kind."), "id": integer("Movie id, TV episode id or music album id."),
				"seriesId": integer("TV: with season, search a whole season."), "season": integer("TV: the season to search."),
				"limit": integer("Releases to return (default 50, at most 200).")}, "kind"), Run: bind(s.searchReleases)},
		{Name: "list_profiles", Title: "Quality profiles", Description: "The quality profiles (keys for request tools) per kind, and the music metadata profiles.",
			Input: object(map[string]schema{"kind": kindEnum("Limit to one kind.")}), Run: bind(s.listProfiles)},
		{Name: "list_root_folders", Title: "Root folders", Description: "Library root folders with their kind, free space and problems.",
			Input: object(map[string]schema{}), Run: bind(s.listRootFolders)},
		{Name: "set_monitored", Title: "Set monitored", Write: true, Idempotent: true,
			Description: "Monitor or unmonitor a movie, series, season, episodes, artist or albums.",
			Input: object(map[string]schema{"kind": kindEnum("Media kind."), "id": integer("Movie, series or artist id."),
				"monitored": boolean("Monitor (true) or stop (false)."), "season": integer("TV: only this season."),
				"episodeIds": array("TV: only these episodes.", integer("Episode id.")), "albumIds": array("Music: only these albums.", integer("Album id."))},
				"kind", "monitored"), Run: bind(s.setMonitored)},
		{Name: "start_search", Title: "Start a search", Write: true,
			Description: "Search the indexers now and grab the best accepted release (recorded only in shadow mode). Runs in the background; follow with request_status.",
			Input: object(map[string]schema{"kind": kindEnum("Media kind."), "id": integer("Movie, series or artist id."),
				"season": integer("TV: only this season."), "episodeId": integer("TV: only this episode."), "albumId": integer("Music: only this album.")}, "kind"),
			Run: bind(s.startSearch)},
		{Name: "rescan_item", Title: "Refresh an item", Write: true, Idempotent: true, OpenWorld: true,
			Description: "Re-read an item's metadata (new episodes or albums). Movies are refreshed all together. Runs in the background.",
			Input:       object(map[string]schema{"kind": kindEnum("Media kind."), "id": integer("Series or artist id (ignored for movies).")}, "kind"),
			Run:         bind(s.rescanItem)},
	}
}

// ── search_library ───────────────────────────────────────────────────────────

// LibraryItem is one search_library row.
type LibraryItem struct {
	Kind      domain.MediaKind `json:"kind"`
	ID        int64            `json:"id"`
	Title     string           `json:"title"`
	Year      int              `json:"year,omitempty"`
	Monitored bool             `json:"monitored"`
}

type libraryArgs struct {
	paging
	Query string `json:"query"`
	Kind  string `json:"kind"`
}

func (s *Service) searchLibrary(ctx context.Context, in libraryArgs) (Result, error) {
	q := strings.ToLower(strings.TrimSpace(in.Query))
	all := []LibraryItem{}
	add := func(it LibraryItem) error {
		if strings.Contains(strings.ToLower(it.Title), q) {
			all = append(all, it)
		}
		return nil
	}
	for _, k := range s.findKinds(in.Kind) {
		if err := s.eachItem(ctx, k, add); err != nil {
			return Result{}, err
		}
	}
	p, err := slicePage(all, in.paging)
	return Result{Text: countText(len(p.Items), "library items", p.NextCursor), Data: p}, err
}

func (s *Service) eachItem(ctx context.Context, k domain.MediaKind, fn func(LibraryItem) error) error {
	if err := s.usable(k); err != nil {
		return err
	}
	switch k {
	case domain.KindMovie:
		return s.d.Movies.Each(ctx, func(m domain.Movie) error {
			return fn(LibraryItem{Kind: k, ID: m.ID, Title: m.Title, Year: m.Year, Monitored: m.Monitored})
		})
	case domain.KindTV:
		return s.d.TV.EachSeries(ctx, func(x domain.Series) error {
			return fn(LibraryItem{Kind: k, ID: x.ID, Title: x.Title, Year: x.Year, Monitored: x.Monitored})
		})
	}
	return s.d.Music.EachArtist(ctx, func(a domain.Artist) error {
		return fn(LibraryItem{Kind: k, ID: a.ID, Title: a.Name, Monitored: a.Monitored})
	})
}

// ── get_item ─────────────────────────────────────────────────────────────────

type itemArgs struct {
	Kind    string `json:"kind"`
	ID      int64  `json:"id"`
	AlbumID int64  `json:"albumId"`
	Season  *int   `json:"season"`
}

func (s *Service) getItem(ctx context.Context, in itemArgs) (Result, error) {
	kind, err := s.kindArg(in.Kind)
	if err != nil {
		return Result{}, err
	}
	get := map[domain.MediaKind]func(context.Context, itemArgs) (string, any, error){
		domain.KindMovie: s.movieItem, domain.KindTV: s.seriesItem, domain.KindMusic: s.musicItem,
	}[kind]
	title, data, err := get(ctx, in)
	return Result{Text: title, Data: data}, err
}

func (s *Service) movieItem(ctx context.Context, in itemArgs) (string, any, error) {
	m, err := s.d.Movies.Get(ctx, in.ID)
	if err != nil {
		return "", nil, err
	}
	files, err := s.d.Library.FilesOf(ctx, m.Ref())
	return fmt.Sprintf("%s (%d)", m.Title, m.Year), MovieItemOut{Movie: m, Files: files}, err
}

func (s *Service) seriesItem(ctx context.Context, in itemArgs) (string, any, error) {
	d, err := s.d.TV.Detail(ctx, in.ID)
	if err != nil {
		return "", nil, err
	}
	out := SeriesItemOut{Series: d.Series, Seasons: d.Seasons}
	if in.Season != nil {
		for _, e := range d.Episodes {
			if e.Season == *in.Season {
				out.Episodes = append(out.Episodes, e)
			}
		}
	}
	return fmt.Sprintf("%s (%d): %d seasons", d.Series.Title, d.Series.Year, len(d.Seasons)), out, nil
}

func (s *Service) musicItem(ctx context.Context, in itemArgs) (string, any, error) {
	if in.AlbumID > 0 {
		p, err := s.d.Music.AlbumPage(ctx, in.AlbumID)
		return p.Artist.Name + " - " + p.Album.Title, p, err
	}
	d, err := s.d.Music.Detail(ctx, in.ID)
	return fmt.Sprintf("%s: %d albums", d.Artist.Name, len(d.Albums)), d, err
}

// ── list_wanted ──────────────────────────────────────────────────────────────

type wantedArgs struct {
	paging
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

func (s *Service) listWanted(_ context.Context, in wantedArgs) (Result, error) {
	kind, err := s.kindArg(in.Kind)
	if err != nil {
		return Result{}, err
	}
	reason := automation.ReasonMissing
	if in.Reason == string(automation.ReasonCutoffUnmet) {
		reason = automation.ReasonCutoffUnmet
	}
	cursor, err := in.offset()
	if err != nil {
		return Result{}, err
	}
	items, next := s.wantedPage(kind, reason, cursor, in.size())
	p := page[any]{Items: items, NextCursor: nextCursor(next)}
	return Result{Text: countText(len(items), "wanted "+string(kind)+" items ("+string(reason)+")", p.NextCursor), Data: p}, nil
}

func (s *Service) wantedPage(kind domain.MediaKind, r automation.Reason, cursor int64, limit int) ([]any, int64) {
	switch kind {
	case domain.KindMovie:
		return anyPage(s.d.MovieSearch.Index().Page(r, cursor, limit))
	case domain.KindTV:
		return anyPage(s.d.TVSearch.Index().Page(r, cursor, limit))
	}
	return anyPage(s.d.MusicSearch.Index().Page(r, cursor, limit))
}

func anyPage[T any](list []T, next int64) ([]any, int64) {
	out := make([]any, len(list))
	for i, v := range list {
		out[i] = v
	}
	return out, next
}

// ── get_calendar ─────────────────────────────────────────────────────────────

type calendarArgs struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// MovieDate is a movie release in the calendar window.
type MovieDate struct {
	Movie   domain.Movie `json:"movie"`
	Release string       `json:"release"` // inCinemas | digital | physical
	Date    time.Time    `json:"date"`
}

func (s *Service) getCalendar(ctx context.Context, in calendarArgs) (Result, error) {
	from, to, err := s.window(in)
	if err != nil {
		return Result{}, err
	}
	out := CalendarOut{From: from.Format(time.DateOnly), To: to.Format(time.DateOnly)}
	episodes, movieDates := []tv.CalendarItem{}, []MovieDate{}
	if s.usable(domain.KindTV) == nil {
		if episodes, err = s.d.TV.Upcoming(ctx, from, to); err != nil {
			return Result{}, err
		}
	}
	if s.usable(domain.KindMovie) == nil {
		if movieDates, err = s.movieDates(ctx, from, to); err != nil {
			return Result{}, err
		}
	}
	out.Episodes, out.Movies = episodes, movieDates
	return Result{Text: fmt.Sprintf("%d episodes and %d movie releases between %s and %s", len(episodes), len(movieDates),
		out.From, out.To), Data: out}, nil
}

func (s *Service) window(in calendarArgs) (time.Time, time.Time, error) {
	from := s.d.Now().UTC().Truncate(24 * time.Hour)
	if err := parseDate(in.From, "from", &from); err != nil {
		return from, from, err
	}
	to := from.Add(7 * 24 * time.Hour)
	err := parseDate(in.To, "to", &to)
	return from, to, err
}

func parseDate(v, field string, into *time.Time) error {
	if v == "" {
		return nil
	}
	t, err := time.Parse(time.DateOnly, v)
	if err != nil {
		return invalidArg(field, "must be a date YYYY-MM-DD")
	}
	*into = t
	return nil
}

func (s *Service) movieDates(ctx context.Context, from, to time.Time) ([]MovieDate, error) {
	out := []MovieDate{}
	err := s.d.Movies.Each(ctx, func(m domain.Movie) error {
		for name, d := range map[string]*time.Time{"inCinemas": m.InCinemas, "digital": m.Digital, "physical": m.Physical} {
			if d != nil && !d.Before(from) && d.Before(to) {
				out = append(out, MovieDate{Movie: m, Release: name, Date: *d})
			}
		}
		return nil
	})
	return out, err
}

// ── get_history and get_health ───────────────────────────────────────────────

type historyArgs struct {
	paging
	Type string `json:"type"`
}

func (s *Service) getHistory(ctx context.Context, in historyArgs) (Result, error) {
	cursor, err := in.offset()
	if err != nil {
		return Result{}, err
	}
	list, next, err := s.d.History.List(ctx, cursor, in.size())
	if err != nil {
		return Result{}, err
	}
	items := []store.HistoryEntry{}
	for _, e := range list {
		if strings.HasPrefix(string(e.Event.Type), in.Type) {
			items = append(items, e)
		}
	}
	p := page[store.HistoryEntry]{Items: items, NextCursor: nextCursor(next)}
	return Result{Text: countText(len(items), "history events", p.NextCursor), Data: p}, nil
}

func (s *Service) getHealth(ctx context.Context, _ struct{}) (Result, error) {
	roots, err := s.d.RootFolders.List(ctx)
	if err != nil {
		return Result{}, err
	}
	list := s.d.Health.List()
	modes := map[domain.MediaKind]domain.KindMode{}
	for _, k := range []domain.MediaKind{domain.KindMovie, domain.KindTV, domain.KindMusic, domain.KindBook} {
		modes[k] = s.d.Modes.Of(k)
	}
	bad := 0
	for _, h := range list {
		if h.Status != domain.HealthOK {
			bad++
		}
	}
	return Result{Text: fmt.Sprintf("%d integrations, %d not ok; %d root folders", len(list), bad, len(roots)),
		Data: HealthOut{Integrations: list, RootFolders: roots, Modes: modes}}, nil
}

// ── search_releases ──────────────────────────────────────────────────────────

type releaseArgs struct {
	Kind     string `json:"kind"`
	ID       int64  `json:"id"`
	SeriesID int64  `json:"seriesId"`
	Season   int    `json:"season"`
	Limit    int    `json:"limit"`
}

// collector keeps the interactive search's records, bounded.
type collector struct {
	mu      sync.Mutex
	max     int
	records []any
	errors  []string
	more    int
}

func (c *collector) Record(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.records) >= c.max {
		c.more++
		return nil
	}
	c.records = append(c.records, v)
	return nil
}

func (c *collector) Error(source string, err error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errors = append(c.errors, source+": "+err.Error())
	return nil
}

func (s *Service) searchReleases(ctx context.Context, in releaseArgs) (Result, error) {
	kind, err := s.kindArg(in.Kind)
	if err != nil {
		return Result{}, err
	}
	c := &collector{max: paging{Limit: in.Limit}.size(), records: []any{}, errors: []string{}}
	sum, err := s.interactive(ctx, kind, in, c)
	if err != nil {
		return Result{}, err
	}
	text := fmt.Sprintf("%d releases, %d accepted (%d indexers, %d failed); nothing was grabbed", sum.Releases, sum.Accepted, sum.Indexers, sum.Failed)
	return Result{Text: text, Data: ReleasesOut{Summary: sum, Releases: c.records, Omitted: c.more, Errors: c.errors}}, nil
}

func (s *Service) interactive(ctx context.Context, kind domain.MediaKind, in releaseArgs, c *collector) (automation.Summary, error) {
	switch {
	case kind == domain.KindMovie:
		return s.d.MovieSearch.Interactive(ctx, in.ID, c)
	case kind == domain.KindMusic:
		return s.d.MusicSearch.Interactive(ctx, in.ID, c)
	case in.SeriesID > 0:
		return s.d.TVSearch.InteractiveSeason(ctx, in.SeriesID, in.Season, c)
	}
	return s.d.TVSearch.InteractiveEpisode(ctx, in.ID, c)
}

// ── profiles and root folders ────────────────────────────────────────────────

// ProfileView is one quality profile.
type ProfileView struct {
	Key      string           `json:"key"`
	Name     string           `json:"name"`
	Kind     domain.MediaKind `json:"kind"`
	Upgrades bool             `json:"upgrades"`
	Cutoff   string           `json:"cutoff,omitempty"`
}

type kindFilter struct {
	Kind string `json:"kind"`
}

func (s *Service) listProfiles(_ context.Context, in kindFilter) (Result, error) {
	out := []ProfileView{}
	for _, k := range s.findKinds(in.Kind) {
		for _, p := range decide.DefaultProfiles(k) {
			spec := p.Spec()
			out = append(out, ProfileView{Key: movies.ProfileKey(spec.Name), Name: spec.Name, Kind: k, Upgrades: spec.Upgrades, Cutoff: string(spec.Cutoff)})
		}
	}
	data := ProfilesOut{Profiles: out}
	if in.Kind == "" || in.Kind == string(domain.KindMusic) {
		data.MetadataProfiles = domain.MetadataProfiles
	}
	return Result{Text: fmt.Sprintf("%d quality profiles", len(out)), Data: data}, nil
}

func (s *Service) listRootFolders(ctx context.Context, _ struct{}) (Result, error) {
	list, err := s.d.RootFolders.List(ctx)
	return Result{Text: fmt.Sprintf("%d root folders", len(list)), Data: RootFoldersOut{RootFolders: list}}, err
}
