// Package profiles is the single writer of custom formats and custom quality
// profiles: it keeps the ones made in the web UI or by a migration in the
// database, merges those from the config file (which win and are read-only),
// compiles the whole set into the decision catalog and swaps it in, and
// serves the settings API.
package profiles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/parse"
	"github.com/jrand0m/corsarr/internal/store"
)

// Repo persists documents (store.Quality implements it).
type Repo interface {
	List(ctx context.Context, t store.QualityTable) ([]store.QualityObject, error)
	Save(ctx context.Context, t store.QualityTable, o store.QualityObject, now time.Time) error
	Delete(ctx context.Context, t store.QualityTable, key string) (bool, error)
	ProfileUse(ctx context.Context, key string) (int, error)
}

// Publisher records History events (events.Bus implements it).
type Publisher interface {
	Publish(domain.Event) (domain.Event, error)
}

// Deps wire the service.
type Deps struct {
	Repo   Repo
	Config func() *config.Config
	Pub    Publisher
	Now    func() time.Time
	Log    *slog.Logger
}

// Errors the API maps to statuses.
var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("an object with this key already exists")
	ErrManaged  = errors.New("set in the config file; change it there")
	ErrBuiltin  = errors.New("built-in profiles cannot be changed; duplicate it instead")
)

// InvalidError lists a document's validation failures.
type InvalidError struct{ Errs []decide.DocError }

func (e InvalidError) Error() string {
	msgs := make([]string, len(e.Errs))
	for i, x := range e.Errs {
		msgs[i] = x.Error()
	}
	return strings.Join(msgs, "; ")
}

// InUseError refuses a delete: the format is scored by profiles, or the
// profile is used by items.
type InUseError struct {
	Profiles []string
	Items    int
}

func (e InUseError) Error() string {
	if len(e.Profiles) > 0 {
		return "used by the profiles " + strings.Join(e.Profiles, ", ")
	}
	return fmt.Sprintf("used by %d series, movies or artists; give them another profile first", e.Items)
}

// Service is the one writer of custom formats, custom profiles and the
// decision catalog.
type Service struct {
	d        Deps
	mu       sync.Mutex // serialises writes and rebuilds
	formats  []decide.FormatDoc
	profiles []decide.ProfileDoc
}

// New builds the service; call Load before deciding anything.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Service{d: d}
}

// Load reads the database documents and puts the merged catalog in force.
func (s *Service) Load(ctx context.Context) error {
	formats, err := readDocs[decide.FormatDoc](ctx, s.d.Repo, store.TableFormats)
	if err != nil {
		return err
	}
	profiles, err := readDocs[decide.ProfileDoc](ctx, s.d.Repo, store.TableProfiles)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.formats, s.profiles = formats, profiles
	s.rebuild()
	return nil
}

func readDocs[T any](ctx context.Context, r Repo, t store.QualityTable) ([]T, error) {
	rows, err := r.List(ctx, t)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(rows))
	for _, row := range rows {
		var d T
		if err := json.Unmarshal(row.Doc, &d); err != nil {
			return nil, fmt.Errorf("%s %s: %w", t, row.Key, err)
		}
		out = append(out, d)
	}
	return out, nil
}

// OnConfig recompiles after a config change.
func (s *Service) OnConfig() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rebuild()
}

func (s *Service) cfg() *config.Config {
	if s.d.Config == nil {
		return &config.Config{}
	}
	if c := s.d.Config(); c != nil {
		return c
	}
	return &config.Config{}
}

// merged is the database documents with the config file's on top.
func (s *Service) merged() ([]decide.FormatDoc, []decide.ProfileDoc) {
	c := s.cfg()
	return mergeDocs(s.formats, c.CustomFormats, func(d decide.FormatDoc) string { return d.Key }),
		mergeDocs(s.profiles, c.QualityProfiles, func(d decide.ProfileDoc) string { return d.Key })
}

// mergeDocs keeps the database documents whose key the config file does not
// use, then adds the config file's (pure).
func mergeDocs[T any](db, file []T, key func(T) string) []T {
	inFile := map[string]bool{}
	for _, d := range file {
		inFile[key(d)] = true
	}
	out := make([]T, 0, len(db)+len(file))
	for _, d := range db {
		if !inFile[key(d)] {
			out = append(out, d)
		}
	}
	return append(out, file...)
}

// rebuild compiles the merged documents and swaps the catalog; documents
// that no longer compile (a format removed from the config file) are left
// out and logged. Callers hold mu.
func (s *Service) rebuild() *decide.Catalog {
	formats, profiles := s.merged()
	c, problems := decide.BuildCatalog(formats, profiles)
	for _, p := range problems {
		s.d.Log.Warn("custom format or profile left out", "object", p.Object, "key", p.Key, "problem", p.DocError.Error())
	}
	decide.SetCatalog(c)
	return c
}

func (s *Service) managed(section, key string) *config.Managed {
	return s.cfg().ManagedBy(section + "." + key)
}

func (s *Service) publish(object, key, name, action string) {
	if s.d.Pub == nil {
		return
	}
	_, _ = s.d.Pub.Publish(domain.Event{Type: domain.EvQualityChanged, Actor: "user",
		Data: domain.QualityChange{Object: object, Key: key, Name: name, Action: action}})
}

// ── formats ─────────────────────────────────────────────────────────────────

// FormatRow is a format with where it comes from and who uses it.
type FormatRow struct {
	decide.FormatDoc
	Origin  string          `json:"origin"` // database | config
	Managed *config.Managed `json:"managed,omitempty"`
	UsedBy  []string        `json:"usedBy"`
}

// Formats lists every custom format in force, by name.
func (s *Service) Formats() []FormatRow {
	c := decide.CurrentCatalog()
	out := []FormatRow{}
	for _, f := range c.Formats() {
		out = append(out, s.formatRow(c, f))
	}
	return out
}

func (s *Service) formatRow(c *decide.Catalog, f decide.CustomFormat) FormatRow {
	row := FormatRow{FormatDoc: f.Doc(), Origin: "database", UsedBy: []string{}}
	if m := s.managed("customFormats", f.Key()); m != nil {
		row.Origin, row.Managed = "config", m
	}
	for _, p := range c.UsedBy(f.Key()) {
		row.UsedBy = append(row.UsedBy, p.Key())
	}
	return row
}

// Format finds one custom format.
func (s *Service) Format(key string) (FormatRow, error) {
	c := decide.CurrentCatalog()
	f, ok := c.Format(key)
	if !ok {
		return FormatRow{}, ErrNotFound
	}
	return s.formatRow(c, f), nil
}

// CreateFormat adds a format; an empty key is derived from the name.
func (s *Service) CreateFormat(ctx context.Context, d decide.FormatDoc) (FormatRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	taken := func(k string) bool {
		_, ok := decide.CurrentCatalog().Format(k)
		return ok || s.managed("customFormats", k) != nil
	}
	if d.Key == "" {
		d.Key = freeKey(d.Name, taken)
	} else if taken(d.Key) {
		return FormatRow{}, ErrExists
	}
	return s.saveFormat(ctx, d, "created")
}

// UpdateFormat replaces a database format.
func (s *Service) UpdateFormat(ctx context.Context, key string, d decide.FormatDoc) (FormatRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.managed("customFormats", key) != nil {
		return FormatRow{}, ErrManaged
	}
	if indexOf(s.formats, key, formatKey) < 0 {
		return FormatRow{}, ErrNotFound
	}
	d.Key = key
	return s.saveFormat(ctx, d, "updated")
}

func formatKey(d decide.FormatDoc) string   { return d.Key }
func profileKey(d decide.ProfileDoc) string { return d.Key }

func indexOf[T any](docs []T, key string, k func(T) string) int {
	return slices.IndexFunc(docs, func(d T) bool { return k(d) == key })
}

func upsert[T any](docs []T, d T, k func(T) string) []T {
	if i := indexOf(docs, k(d), k); i >= 0 {
		out := slices.Clone(docs)
		out[i] = d
		return out
	}
	return append(slices.Clone(docs), d)
}

func (s *Service) saveFormat(ctx context.Context, d decide.FormatDoc, action string) (FormatRow, error) {
	f, errs := decide.CompileFormat(d)
	if len(errs) > 0 {
		return FormatRow{}, InvalidError{errs}
	}
	d = f.Doc() // normalised values
	if err := s.store(ctx, store.TableFormats, d.Key, d.Name, d); err != nil {
		return FormatRow{}, err
	}
	s.formats = upsert(s.formats, d, formatKey)
	c := s.rebuild()
	s.publish("format", d.Key, d.Name, action)
	saved, _ := c.Format(d.Key)
	return s.formatRow(c, saved), nil
}

func (s *Service) store(ctx context.Context, t store.QualityTable, key, name string, doc any) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return s.d.Repo.Save(ctx, t, store.QualityObject{Key: key, Name: name, Doc: raw}, s.d.Now())
}

// DeleteFormat removes a database format no profile scores.
func (s *Service) DeleteFormat(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.managed("customFormats", key) != nil {
		return ErrManaged
	}
	i := indexOf(s.formats, key, formatKey)
	if i < 0 {
		return ErrNotFound
	}
	if users := decide.CurrentCatalog().UsedBy(key); len(users) > 0 {
		names := make([]string, len(users))
		for j, p := range users {
			names[j] = p.Spec().Name
		}
		return InUseError{Profiles: names}
	}
	if _, err := s.d.Repo.Delete(ctx, store.TableFormats, key); err != nil {
		return err
	}
	name := s.formats[i].Name
	s.formats = slices.Delete(slices.Clone(s.formats), i, i+1)
	s.rebuild()
	s.publish("format", key, name, "deleted")
	return nil
}

// freeKey derives an unused key from a name: "hd1080p", then "hd1080p2", ….
func freeKey(name string, taken func(string) bool) string {
	base := decide.KeyOf(name)
	free := func(k string) bool { return !taken(k) && !decide.IsBuiltinKey(k) }
	k := base
	for n := 2; !free(k); n++ {
		suffix := fmt.Sprint(n)
		k = base[:min(len(base), 48-len(suffix))] + suffix
	}
	return k
}

// ── test box ────────────────────────────────────────────────────────────────

// TestRequest evaluates titles against every format, or one unsaved format.
type TestRequest struct {
	Titles []string          `json:"titles"`
	Size   int64             `json:"size,omitempty"`
	Flags  []string          `json:"flags,omitempty"`
	Format *decide.FormatDoc `json:"format,omitempty"`
}

// FormatResult is one format's result for one title.
type FormatResult struct {
	Key        string `json:"key"`
	Name       string `json:"name"`
	Matched    bool   `json:"matched"`
	Conditions []bool `json:"conditions"`
}

// TitleResult is everything the test box shows for one title.
type TitleResult struct {
	Title   string              `json:"title"`
	Parsed  parse.ParsedRelease `json:"parsed"`
	Formats []FormatResult      `json:"formats"`
}

// maxTestTitles bounds one test request.
const maxTestTitles = 50

// Test evaluates the request's titles.
func (s *Service) Test(req TestRequest) ([]TitleResult, error) {
	formats := decide.CurrentCatalog().Formats()
	if req.Format != nil {
		d := *req.Format
		if d.Key == "" {
			d.Key = "draft"
		}
		if strings.TrimSpace(d.Name) == "" {
			d.Name = "Draft"
		}
		f, errs := decide.CompileFormat(d)
		if len(errs) > 0 {
			return nil, InvalidError{errs}
		}
		formats = []decide.CustomFormat{f}
	}
	if len(req.Titles) > maxTestTitles {
		req.Titles = req.Titles[:maxTestTitles]
	}
	out := []TitleResult{}
	for _, t := range req.Titles {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, testTitle(t, req, formats))
		}
	}
	return out, nil
}

func testTitle(title string, req TestRequest, formats []decide.CustomFormat) TitleResult {
	p := parse.ParseRelease(title)
	subj := decide.SubjectOf(domain.Release{Title: title, Size: req.Size, Flags: req.Flags}, p)
	res := TitleResult{Title: title, Parsed: p, Formats: []FormatResult{}}
	for _, f := range formats {
		res.Formats = append(res.Formats, FormatResult{Key: f.Key(), Name: f.Name(), Matched: f.Matches(subj), Conditions: f.Explain(subj)})
	}
	return res
}
