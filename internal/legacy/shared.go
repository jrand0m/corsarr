package legacy

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/movies"
)

// Id map spaces.
const (
	spaceProfile = "profile"
	spaceRoot    = "rootfolder"
	spaceTag     = "tag"
	spaceIndexer = "indexer"
)

// ── system/status ────────────────────────────────────────────────────────────

type status struct {
	AppName      string `json:"appName"`
	InstanceName string `json:"instanceName"`
	Version      string `json:"version"`
	URLBase      string `json:"urlBase"`
	StartTime    string `json:"startTime"`
}

// statusOf answers system/status with the incumbent major version clients
// branch on (e.g. no language profiles from Sonarr 4 on).
func (s *Service) statusOf(app, version string) http.HandlerFunc {
	started := s.d.Now().UTC().Format(time.RFC3339)
	return func(w http.ResponseWriter, _ *http.Request) {
		base := strings.TrimSuffix(s.d.Config().Server.BasePath, "/")
		ok(w, status{AppName: "corsarr", InstanceName: "corsarr", Version: version, URLBase: base + "/legacy/" + app, StartTime: started})
	}
}

// ── ids ──────────────────────────────────────────────────────────────────────

// idOf is the stable integer id of a key (0 when the map fails, logged).
func (s *Service) idOf(ctx context.Context, space, key string) int64 {
	id, err := s.d.IDs.ID(ctx, space, key)
	if err != nil {
		s.d.Log.Warn("legacy id", "space", space, "key", key, "err", err)
	}
	return id
}

// keyOf resolves an integer id back to its key.
func (s *Service) keyOf(ctx context.Context, space string, id int64) (string, bool) {
	key, found, err := s.d.IDs.Key(ctx, space, id)
	return key, found && err == nil
}

func (s *Service) tagIDs(ctx context.Context, labels []string) []int64 {
	out := make([]int64, 0, len(labels))
	for _, l := range labels {
		out = append(out, s.idOf(ctx, spaceTag, l))
	}
	return out
}

// tagLabels resolves tag ids; ids that name no tag are dropped.
func (s *Service) tagLabels(ctx context.Context, ids []int64) []string {
	out := []string{}
	for _, id := range ids {
		if l, found := s.keyOf(ctx, spaceTag, id); found {
			out = append(out, l)
		}
	}
	return out
}

// profileKey resolves a qualityProfileId; 0 means the default profile.
func (s *Service) profileKey(ctx context.Context, id int64) (string, bool) {
	if id == 0 {
		return "", true
	}
	return s.keyOf(ctx, spaceProfile, id)
}

// ── quality profiles, root folders, tags ─────────────────────────────────────

type profile struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	UpgradeAllowed bool   `json:"upgradeAllowed"`
}

func (s *Service) profiles(kind domain.MediaKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out := []profile{}
		for _, p := range decide.DefaultProfiles(kind) {
			spec := p.Spec()
			out = append(out, profile{ID: s.idOf(r.Context(), spaceProfile, movies.ProfileKey(spec.Name)), Name: spec.Name, UpgradeAllowed: spec.Upgrades})
		}
		ok(w, out)
	}
}

type rootFolder struct {
	ID              int64    `json:"id"`
	Path            string   `json:"path"`
	Accessible      bool     `json:"accessible"`
	FreeSpace       int64    `json:"freeSpace"`
	UnmappedFolders []string `json:"unmappedFolders"`
}

func (s *Service) rootFolders(kind domain.MediaKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := s.d.RootFolders.List(r.Context())
		if err != nil {
			s.writeErr(w, err)
			return
		}
		out := []rootFolder{}
		for _, f := range list {
			if f.Kind == kind {
				out = append(out, rootFolder{ID: s.idOf(r.Context(), spaceRoot, f.Path), Path: f.Path, Accessible: f.Accessible,
					FreeSpace: f.FreeSpace, UnmappedFolders: []string{}})
			}
		}
		ok(w, out)
	}
}

type tag struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
}

// listTags lists the tags clients created plus those series carry.
func (s *Service) listTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.d.TV != nil {
		err := s.d.TV.EachSeries(ctx, func(ser domain.Series) error {
			s.tagIDs(ctx, ser.Tags) // assigns ids to tags set in the UI
			return nil
		})
		if err != nil {
			s.writeErr(w, err)
			return
		}
	}
	labels, err := s.d.IDs.Keys(ctx, spaceTag)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := []tag{}
	for _, l := range labels {
		out = append(out, tag{ID: s.idOf(ctx, spaceTag, l), Label: l})
	}
	ok(w, out)
}

func (s *Service) createTag(w http.ResponseWriter, r *http.Request) {
	var body tag
	if !decode(w, r, &body) {
		return
	}
	label := strings.TrimSpace(body.Label)
	if label == "" {
		invalid(w, "label", "a label is required")
		return
	}
	id, err := s.d.IDs.ID(r.Context(), spaceTag, label)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tag{ID: id, Label: label})
}

func emptyList(w http.ResponseWriter, _ *http.Request) { ok(w, []any{}) }

// ── paging, ids and times ────────────────────────────────────────────────────

type page struct {
	Page          int    `json:"page"`
	PageSize      int    `json:"pageSize"`
	SortKey       string `json:"sortKey,omitempty"`
	SortDirection string `json:"sortDirection"`
	TotalRecords  int    `json:"totalRecords"`
	Records       any    `json:"records"`
}

// pageOf slices a full list by the page and pageSize query parameters.
func pageOf[T any](r *http.Request, all []T) page {
	q := r.URL.Query()
	n, size := queryInt(q.Get("page"), 1), queryInt(q.Get("pageSize"), 10)
	if n < 1 {
		n = 1
	}
	if size < 1 {
		size = 10
	}
	from := min((n-1)*size, len(all))
	to := min(from+size, len(all))
	return page{Page: n, PageSize: size, SortDirection: "default", TotalRecords: len(all), Records: append([]T{}, all[from:to]...)}
}

func queryInt(v string, def int) int {
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func queryInt64(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(r.URL.Query().Get(name), 10, 64)
	return n
}

// bodyID takes the resource id from the path when the route has one, else
// keeps the body's; false answered 404.
func bodyID(w http.ResponseWriter, r *http.Request, id *int64) bool {
	if r.PathValue("id") == "" {
		return true
	}
	v, valid := pathID(w, r)
	*id = v
	return valid
}

// createdStatus is 201 for a new item and 200 for one already there.
func createdStatus(created bool) int {
	if created {
		return http.StatusCreated
	}
	return http.StatusOK
}

// pathID parses the {id} path value; false answered 404.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		notFound(w, r)
		return 0, false
	}
	return id, true
}

func isoTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func isoDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// window reads the calendar's start and end (dates or timestamps); the
// defaults are today and two days later.
func (s *Service) window(r *http.Request) (time.Time, time.Time) {
	today := s.d.Now().UTC().Truncate(24 * time.Hour)
	from := parseTime(r.URL.Query().Get("start"), today)
	return from, parseTime(r.URL.Query().Get("end"), from.Add(48*time.Hour))
}

func parseTime(v string, def time.Time) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC()
		}
	}
	return def
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slug is a URL-safe title.
func slug(title string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(title), "-"), "-")
}

type image struct {
	CoverType string `json:"coverType"`
	URL       string `json:"url"`
	RemoteURL string `json:"remoteUrl"`
}

func images(poster string) []image {
	if poster == "" {
		return []image{}
	}
	return []image{{CoverType: "poster", URL: poster, RemoteURL: poster}}
}

// ── commands ─────────────────────────────────────────────────────────────────

type command struct {
	ID      int64          `json:"id"`
	Name    string         `json:"name"`
	Status  string         `json:"status"` // queued | started | completed | failed
	Queued  string         `json:"queued"`
	Message string         `json:"message,omitempty"`
	Body    map[string]any `json:"body"`
}

// commands remembers the latest commands for GET command/{id}.
type commands struct {
	mu   sync.Mutex
	next int64
	byID map[int64]*command
}

const keepCommands = 100

// start records a command and runs it in the background.
func (s *Service) start(name string, body map[string]any, run func(context.Context) error) command {
	c := &s.cmd
	c.mu.Lock()
	if c.byID == nil {
		c.byID = map[int64]*command{}
	}
	c.next++
	cmd := &command{ID: c.next, Name: name, Status: "queued", Queued: s.d.Now().UTC().Format(time.RFC3339), Body: body}
	c.byID[cmd.ID] = cmd
	delete(c.byID, cmd.ID-keepCommands)
	out := *cmd
	c.mu.Unlock()
	s.d.Go(func(ctx context.Context) {
		s.finish(cmd.ID, run(ctx))
	})
	return out
}

func (s *Service) finish(id int64, err error) {
	c := &s.cmd
	c.mu.Lock()
	defer c.mu.Unlock()
	cmd, found := c.byID[id]
	if !found {
		return
	}
	cmd.Status = "completed"
	if err != nil {
		cmd.Status, cmd.Message = "failed", err.Error()
		s.d.Log.Warn("legacy command", "name", cmd.Name, "err", err)
	}
}

func (s *Service) getCommand(w http.ResponseWriter, r *http.Request) {
	id, valid := pathID(w, r)
	if !valid {
		return
	}
	c := &s.cmd
	c.mu.Lock()
	cmd, found := c.byID[id]
	var out command
	if found {
		out = *cmd
	}
	c.mu.Unlock()
	if !found {
		notFound(w, r)
		return
	}
	ok(w, out)
}

// commandBody is what the clients send to POST command.
type commandBody struct {
	Name         string  `json:"name"`
	SeriesID     int64   `json:"seriesId"`
	SeasonNumber *int    `json:"seasonNumber"`
	EpisodeIDs   []int64 `json:"episodeIds"`
	MovieIDs     []int64 `json:"movieIds"`
}

func (b commandBody) echo() map[string]any {
	out := map[string]any{"name": b.Name}
	if b.SeriesID != 0 {
		out["seriesId"] = b.SeriesID
	}
	if b.SeasonNumber != nil {
		out["seasonNumber"] = *b.SeasonNumber
	}
	if len(b.EpisodeIDs) > 0 {
		out["episodeIds"] = b.EpisodeIDs
	}
	if len(b.MovieIDs) > 0 {
		out["movieIds"] = b.MovieIDs
	}
	return out
}

func errUnknownCommand(name string) error { return fmt.Errorf("unknown command %q", name) }
