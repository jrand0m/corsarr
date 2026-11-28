package profiles

import (
	"context"
	"slices"
	"strings"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

// Default profile keys per kind (what a new item gets when none is chosen).
var defaultKeys = map[domain.MediaKind]string{domain.KindTV: "hd1080p", domain.KindMovie: "hd1080p", domain.KindMusic: "standard"}

// ProfileRow is a profile with where it comes from and how many items use it.
type ProfileRow struct {
	decide.ProfileDoc
	Builtin bool            `json:"builtin"`
	Origin  string          `json:"origin"` // builtin | database | config
	Managed *config.Managed `json:"managed,omitempty"`
	Items   int             `json:"items"`
	// Default names the kinds whose new items get this profile.
	Default []domain.MediaKind `json:"default,omitempty"`
}

// videoKinds are the kinds sharing the video ladder (and the built-in video
// profiles' keys).
var videoKinds = []domain.MediaKind{domain.KindTV, domain.KindMovie}

func builtinRows(c *decide.Catalog) []ProfileRow {
	var out []ProfileRow
	add := func(p decide.QualityProfile, kinds []domain.MediaKind) {
		d := p.Doc()
		d.Kinds = kinds
		row := ProfileRow{ProfileDoc: d, Builtin: true, Origin: "builtin"}
		for _, k := range kinds {
			if defaultKeys[k] == d.Key {
				row.Default = append(row.Default, k)
			}
		}
		out = append(out, row)
	}
	for _, p := range c.Builtins(domain.KindMovie) {
		add(p, videoKinds)
	}
	for _, p := range c.Builtins(domain.KindMusic) {
		add(p, []domain.MediaKind{domain.KindMusic})
	}
	return out
}

func servesAny(kinds []domain.MediaKind, want domain.MediaKind) bool {
	return want == "" || slices.Contains(kinds, want)
}

// Profiles lists built-in then custom profiles; kind filters by served kind.
func (s *Service) Profiles(ctx context.Context, kind domain.MediaKind) ([]ProfileRow, error) {
	c := decide.CurrentCatalog()
	out := []ProfileRow{}
	for _, r := range builtinRows(c) {
		if servesAny(r.Kinds, kind) {
			out = append(out, r)
		}
	}
	for _, p := range c.Customs() {
		if kind != "" && !p.Serves(kind) {
			continue
		}
		row, err := s.customRow(ctx, p)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

func (s *Service) customRow(ctx context.Context, p decide.QualityProfile) (ProfileRow, error) {
	row := ProfileRow{ProfileDoc: p.Doc(), Origin: "database"}
	if m := s.managed("qualityProfiles", p.Key()); m != nil {
		row.Origin, row.Managed = "config", m
	}
	n, err := s.d.Repo.ProfileUse(ctx, p.Key())
	row.Items = n
	return row, err
}

// Profile finds one profile; kind picks between built-ins sharing a key
// ("any" exists for video and for music).
func (s *Service) Profile(ctx context.Context, key string, kind domain.MediaKind) (ProfileRow, error) {
	c := decide.CurrentCatalog()
	if p, ok := c.Custom(key); ok {
		return s.customRow(ctx, p)
	}
	for _, r := range builtinRows(c) {
		if r.Key == key && servesAny(r.Kinds, kind) {
			return r, nil
		}
	}
	return ProfileRow{}, ErrNotFound
}

// CreateProfile adds a custom profile; an empty key is derived from the name.
func (s *Service) CreateProfile(ctx context.Context, d decide.ProfileDoc) (ProfileRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addProfile(ctx, d)
}

func (s *Service) profileTaken(k string) bool {
	_, ok := decide.CurrentCatalog().Custom(k)
	return ok || decide.IsBuiltinKey(k) || s.managed("qualityProfiles", k) != nil
}

func (s *Service) addProfile(ctx context.Context, d decide.ProfileDoc) (ProfileRow, error) {
	if d.Key == "" {
		d.Key = freeKey(d.Name, s.profileTaken)
	} else if s.profileTaken(d.Key) && !decide.IsBuiltinKey(d.Key) {
		return ProfileRow{}, ErrExists
	}
	return s.saveProfile(ctx, d, "created")
}

// UpdateProfile replaces a database profile.
func (s *Service) UpdateProfile(ctx context.Context, key string, d decide.ProfileDoc) (ProfileRow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case decide.IsBuiltinKey(key):
		return ProfileRow{}, ErrBuiltin
	case s.managed("qualityProfiles", key) != nil:
		return ProfileRow{}, ErrManaged
	case indexOf(s.profiles, key, profileKey) < 0:
		return ProfileRow{}, ErrNotFound
	}
	d.Key = key
	return s.saveProfile(ctx, d, "updated")
}

func (s *Service) saveProfile(ctx context.Context, d decide.ProfileDoc, action string) (ProfileRow, error) {
	p, errs := decide.CompileProfile(d, decide.CurrentCatalog().FormatMap())
	if len(errs) > 0 {
		return ProfileRow{}, InvalidError{errs}
	}
	d = p.Doc()
	if err := s.store(ctx, store.TableProfiles, d.Key, d.Name, d); err != nil {
		return ProfileRow{}, err
	}
	s.profiles = upsert(s.profiles, d, profileKey)
	c := s.rebuild()
	s.publish("profile", d.Key, d.Name, action)
	saved, _ := c.Custom(d.Key)
	return s.customRow(ctx, saved)
}

// DeleteProfile removes a database profile no item uses.
func (s *Service) DeleteProfile(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case decide.IsBuiltinKey(key):
		return ErrBuiltin
	case s.managed("qualityProfiles", key) != nil:
		return ErrManaged
	}
	i := indexOf(s.profiles, key, profileKey)
	if i < 0 {
		return ErrNotFound
	}
	n, err := s.d.Repo.ProfileUse(ctx, key)
	if err != nil {
		return err
	}
	if n > 0 {
		return InUseError{Items: n}
	}
	if _, err := s.d.Repo.Delete(ctx, store.TableProfiles, key); err != nil {
		return err
	}
	name := s.profiles[i].Name
	s.profiles = slices.Delete(slices.Clone(s.profiles), i, i+1)
	s.rebuild()
	s.publish("profile", key, name, "deleted")
	return nil
}

// DuplicateRequest names the copy; Kind picks a built-in sharing a key.
type DuplicateRequest struct {
	Name string           `json:"name,omitempty"`
	Kind domain.MediaKind `json:"kind,omitempty"`
}

// DuplicateProfile copies any profile into a new custom one.
func (s *Service) DuplicateProfile(ctx context.Context, key string, req DuplicateRequest) (ProfileRow, error) {
	src, err := s.Profile(ctx, key, req.Kind)
	if err != nil {
		return ProfileRow{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d := src.ProfileDoc
	d.Key, d.Name = "", strings.TrimSpace(req.Name)
	if d.Name == "" {
		d.Name = src.Name + " (copy)"
	}
	return s.addProfile(ctx, d)
}

// QualityChoice is one quality an editor can allow.
type QualityChoice struct {
	ID         decide.QualityID `json:"id"`
	Name       string           `json:"name"`
	BelowFloor bool             `json:"belowFloor,omitempty"`
}

// Choices is what the format and profile editors offer.
type Choices struct {
	ConditionKinds []decide.KindInfo                    `json:"conditionKinds"`
	Qualities      map[domain.MediaKind][]QualityChoice `json:"qualities"`
}

// EditorChoices lists condition kinds and the qualities of each kind.
func EditorChoices() Choices {
	ladder := func(qs []decide.Quality) []QualityChoice {
		out := make([]QualityChoice, 0, len(qs))
		for _, q := range qs {
			out = append(out, QualityChoice{ID: q.ID, Name: q.Name, BelowFloor: decide.BelowFloor(q.ID)})
		}
		return out
	}
	video := ladder(decide.Ladder())
	return Choices{ConditionKinds: decide.ConditionKinds(), Qualities: map[domain.MediaKind][]QualityChoice{
		domain.KindTV: video, domain.KindMovie: video, domain.KindMusic: ladder(decide.AudioLadder())}}
}
