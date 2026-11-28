package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jrand0m/corsarr/internal/config"
	"github.com/jrand0m/corsarr/internal/decide"
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

// QualityState is what corsarr already holds of custom formats and
// profiles: content fingerprints of the stored documents by key, and the
// keys the config file sets.
type QualityState struct {
	Formats        map[string]string
	Profiles       map[string]string
	ConfigFormats  map[string]bool
	ConfigProfiles map[string]bool
}

// FormatFingerprint and ProfileFingerprint compare documents by content.
func FormatFingerprint(d decide.FormatDoc) string { return hashOf(d) }

func ProfileFingerprint(d decide.ProfileDoc) string { return hashOf(d) }

// source of one planned object: which app and source id it came from, and
// what could not be carried over.
type qualitySource struct {
	app   App
	id    int
	name  string
	notes []string
}

type plannedFormat struct {
	doc     decide.FormatDoc
	content string // fingerprint without key and name
	from    []qualitySource
}

type plannedProfile struct {
	doc     decide.ProfileDoc
	content string
	from    []qualitySource
	builtin bool
}

// qualityPlan deduplicates formats and profiles across sources.
type qualityPlan struct {
	formats     []*plannedFormat
	profiles    []*plannedProfile
	formatKeys  map[App]map[int]string
	unsupported []qualitySource // formats with no condition left
	taken       map[string]bool
}

func newQualityPlan() *qualityPlan {
	return &qualityPlan{formatKeys: map[App]map[int]string{}, taken: map[string]bool{}}
}

// mapsCustom reports whether an app's profiles and formats map in full.
func mapsCustom(a App) bool { return a == Sonarr || a == Radarr }

// planQuality maps the custom formats and profiles of every Sonarr and
// Radarr snapshot and records each source profile's target key.
func (b *builder) planQuality(snaps []Snapshot) {
	q := newQualityPlan()
	for i := range snaps {
		s := &snaps[i]
		if s.Source.Error != "" || !mapsCustom(s.Source.App) {
			continue
		}
		q.formatKeys[s.Source.App] = map[int]string{}
		for _, f := range s.CustomFormats {
			q.addFormat(s.Source.App, f)
		}
		s.ProfileKeys = map[int]string{}
		for _, id := range sortedIDs(s.Profiles) {
			if key, ok := q.addProfile(s.Source.App, s.Profiles[id], s.Sizes); ok {
				s.ProfileKeys[id] = key
			}
		}
	}
	b.quality = q
}

func sortedIDs[T any](m map[int]T) []int {
	ids := make([]int, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (q *qualityPlan) key(name string) string {
	base := decide.KeyOf(name)
	k := base
	for n := 2; q.taken[k] || decide.IsBuiltinKey(k); n++ {
		k = fmt.Sprintf("%s%d", base, n)
	}
	q.taken[k] = true
	return k
}

// formatContent fingerprints a format's conditions after normalising them
// through the compiler.
func formatContent(d decide.FormatDoc) (decide.FormatDoc, string, []decide.DocError) {
	d.Key = "x"
	if d.Name == "" {
		d.Name = "x"
	}
	f, errs := decide.CompileFormat(d)
	if len(errs) > 0 {
		return d, "", errs
	}
	n := f.Doc()
	n.Name = d.Name
	return n, hashOf(n.Conditions), nil
}

func (q *qualityPlan) addFormat(app App, f SrcFormat) {
	doc, notes, ok := mapFormat(app, f)
	doc, notes = keepCompilable(doc, notes)
	src := qualitySource{app: app, id: f.ID, name: f.Name, notes: notes}
	if !ok || len(doc.Conditions) == 0 {
		q.unsupported = append(q.unsupported, src)
		return
	}
	doc, content, _ := formatContent(doc)
	var sameName *plannedFormat
	for _, p := range q.formats {
		if norm(p.doc.Name) != norm(doc.Name) {
			continue
		}
		if p.content == content {
			p.from = append(p.from, src)
			q.formatKeys[app][f.ID] = p.doc.Key
			return
		}
		sameName = p
	}
	if sameName != nil {
		doc.Name = fmt.Sprintf("%s (%s)", doc.Name, appLabel(app))
		src.notes = append(src.notes, fmt.Sprintf("another source has a different format with this name; this one is named %q", doc.Name))
	}
	doc.Key = q.key(doc.Name)
	q.formats = append(q.formats, &plannedFormat{doc: doc, content: content, from: []qualitySource{src}})
	q.formatKeys[app][f.ID] = doc.Key
}

// keepCompilable drops conditions corsarr cannot compile (regular
// expression features it does not support), with a note each.
func keepCompilable(d decide.FormatDoc, notes []string) (decide.FormatDoc, []string) {
	var kept []decide.ConditionDoc
	for _, c := range d.Conditions {
		if _, errs := decide.CompileFormat(decide.FormatDoc{Key: "x", Name: "x", Conditions: []decide.ConditionDoc{c}}); len(errs) > 0 {
			notes = append(notes, fmt.Sprintf("a %s condition was left out: %s", c.Kind, errs[0].Msg))
			continue
		}
		kept = append(kept, c)
	}
	d.Conditions = kept
	return d, notes
}

func profileContent(d decide.ProfileDoc) string {
	d.Key, d.Name, d.Kinds = "", "", nil
	return hashOf(d)
}

func (q *qualityPlan) addProfile(app App, p SrcProfile, sizes map[string]SrcSize) (string, bool) {
	kind := app.Kind()
	doc, notes := mapCustomProfile(p, kind, q.formatKeys[app], sizes)
	if len(doc.Groups) == 0 {
		return "", false // nothing allowed survived: the built-in mapping reports it
	}
	src := qualitySource{app: app, id: p.ID, name: p.Name, notes: notes}
	if bk, ok := sameAsBuiltin(doc, kind); ok {
		q.profiles = append(q.profiles, &plannedProfile{doc: decide.ProfileDoc{Key: bk, Name: doc.Name}, builtin: true, from: []qualitySource{src}})
		return bk, true
	}
	content := profileContent(doc)
	same, clash := q.findProfile(doc.Name, content)
	if same != nil {
		if !slices.Contains(same.doc.Kinds, kind) {
			same.doc.Kinds = append(same.doc.Kinds, kind)
		}
		same.from = append(same.from, src)
		return same.doc.Key, true
	}
	if clash || decide.IsBuiltinKey(decide.KeyOf(doc.Name)) {
		doc.Name = fmt.Sprintf("%s (%s)", doc.Name, appLabel(app))
	}
	if clash {
		src.notes = append(src.notes, fmt.Sprintf("another source has a different profile with this name; this one is named %q", doc.Name))
	}
	doc.Key = q.key(doc.Name)
	q.profiles = append(q.profiles, &plannedProfile{doc: doc, content: content, from: []qualitySource{src}})
	return doc.Key, true
}

// findProfile finds a planned custom profile with the same name and
// content, or reports that one with the same name differs.
func (q *qualityPlan) findProfile(name, content string) (same *plannedProfile, clash bool) {
	for _, pp := range q.profiles {
		if pp.builtin || norm(pp.doc.Name) != norm(name) {
			continue
		}
		if pp.content == content {
			return pp, false
		}
		clash = true
	}
	return nil, clash
}

// sameAsBuiltin finds the built-in profile with the same name and content.
func sameAsBuiltin(d decide.ProfileDoc, kind domain.MediaKind) (string, bool) {
	key := decide.KeyOf(d.Name)
	p, ok := decide.CurrentCatalog().Lookup(kind, key)
	if !ok || !decide.IsBuiltinKey(key) {
		return "", false
	}
	b := p.Doc()
	b.Kinds = d.Kinds
	return key, profileContent(b) == profileContent(d)
}

// qualityEntries turns the plan into report entries: each target once per
// source object; the first carries the write, later identical ones point at
// it. A profile's write also writes the formats it scores, so the order of
// entries in the report never matters.
func (b *builder) qualityEntries() {
	q := b.quality
	if q == nil {
		return
	}
	byKey := map[string]decide.FormatDoc{}
	for _, f := range q.formats {
		byKey[f.doc.Key] = f.doc
		b.formatEntries(f)
	}
	for _, src := range q.unsupported {
		b.add(Entry{ID: entryID(src.app, "customformat", src.id), Category: CatProfiles, Source: src.app, Name: src.name,
			Outcome: OutUnsupported, Reason: "no condition could be carried over: " + strings.Join(src.notes, "; ")})
	}
	for _, p := range q.profiles {
		b.profileEntries(p, byKey)
	}
}

func (b *builder) formatEntries(f *plannedFormat) {
	st := b.st.Quality
	out, reason := qualityOutcome(st.Formats, st.ConfigFormats, f.doc.Key, FormatFingerprint(f.doc), "custom format")
	for i, src := range f.from {
		e := Entry{ID: entryID(src.app, "customformat", src.id), Category: CatProfiles, Source: src.app, Name: src.name,
			Target: "customformat:" + f.doc.Key}
		e.Outcome, e.Reason = withNotes(out, reason, src.notes)
		if i > 0 {
			e.Outcome, e.Reason = OutUnchanged, fmt.Sprintf("identical to the %s format; both become %q", appLabel(f.from[0].app), f.doc.Name)
		} else if e.Outcome.writes() {
			e.op = qualityOp{formats: []decide.FormatDoc{f.doc}}
		}
		b.add(e)
	}
}

func (b *builder) profileEntries(p *plannedProfile, formats map[string]decide.FormatDoc) {
	st := b.st.Quality
	out, reason := OutUnchanged, "same as the built-in profile "+p.doc.Key
	var needs []decide.FormatDoc
	compiled := map[string]decide.CustomFormat{}
	for _, s := range p.doc.Formats {
		needs = append(needs, formats[s.Format])
		if f, errs := decide.CompileFormat(formats[s.Format]); len(errs) == 0 {
			compiled[s.Format] = f
		}
	}
	if !p.builtin {
		cp, errs := decide.CompileProfile(p.doc, compiled)
		if len(errs) > 0 {
			for _, src := range p.from {
				b.add(Entry{ID: entryID(src.app, "profile", src.id), Category: CatProfiles, Source: src.app, Name: src.name,
					Outcome: OutUnsupported, Reason: "the profile could not be carried over: " + errs[0].Error()})
			}
			return
		}
		p.doc = cp.Doc()
		out, reason = qualityOutcome(st.Profiles, st.ConfigProfiles, p.doc.Key, ProfileFingerprint(p.doc), "profile")
	}
	for i, src := range p.from {
		e := Entry{ID: entryID(src.app, "profile", src.id), Category: CatProfiles, Source: src.app, Name: src.name,
			Target: "profile:" + p.doc.Key}
		e.Outcome, e.Reason = withNotes(out, reason, src.notes)
		if i > 0 {
			e.Outcome, e.Reason = OutUnchanged, fmt.Sprintf("identical to the %s profile; one profile %q serves %s", appLabel(p.from[0].app),
				p.doc.Name, kindList(p.doc.Kinds))
		} else if e.Outcome.writes() {
			e.op = qualityOp{formats: needs, profile: &p.doc}
		}
		b.add(e)
	}
}

func kindList(ks []domain.MediaKind) string {
	parts := make([]string, len(ks))
	for i, k := range ks {
		parts[i] = string(k)
	}
	return strings.Join(parts, " and ")
}

// qualityOutcome compares a planned document with what corsarr holds.
func qualityOutcome(stored map[string]string, managed map[string]bool, key, fingerprint, what string) (Outcome, string) {
	switch have, ok := stored[key]; {
	case managed[key]:
		return OutManaged, fmt.Sprintf("a %s with the key %s is set in your config file; it stays as configured", what, key)
	case !ok:
		return OutCreate, fmt.Sprintf("becomes the %s %s", what, key)
	case have == fingerprint:
		return OutUnchanged, fmt.Sprintf("corsarr already has this %s (%s)", what, key)
	default:
		return OutUpdate, fmt.Sprintf("replaces corsarr's %s %s, which differs", what, key)
	}
}

// withNotes turns a writing outcome into approximate when something could
// not be carried over, listing what.
func withNotes(out Outcome, reason string, notes []string) (Outcome, string) {
	if len(notes) == 0 {
		return out, reason
	}
	if out.writes() {
		out = OutApproximate
	}
	return out, reason + "; " + strings.Join(notes, "; ")
}

// qualityOp writes formats, then a profile, as database documents.
type qualityOp struct {
	formats []decide.FormatDoc
	profile *decide.ProfileDoc
}

func (o qualityOp) apply(ctx context.Context, a *applier) (string, error) {
	target := ""
	for _, f := range o.formats {
		if err := saveDoc(ctx, a, store.TableFormats, f.Key, f.Name, f); err != nil {
			return "", err
		}
		target = "customformat:" + f.Key
	}
	if o.profile != nil {
		if err := saveDoc(ctx, a, store.TableProfiles, o.profile.Key, o.profile.Name, o.profile); err != nil {
			return "", err
		}
		target = "profile:" + o.profile.Key
	}
	return target, nil
}

func saveDoc(ctx context.Context, a *applier, t store.QualityTable, key, name string, doc any) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return a.t.SaveQuality(ctx, t, store.QualityObject{Key: key, Name: name, Doc: raw}, a.now)
}

// quality reads the custom formats and profiles corsarr holds and the keys
// the config file sets.
func (s *Service) quality(ctx context.Context, st *State, _ []Snapshot) error {
	q := configQuality(s.d.Config())
	if err := fingerprints(ctx, s.d.Store.Quality(), store.TableFormats, q.Formats, FormatFingerprint); err != nil {
		return err
	}
	if err := fingerprints(ctx, s.d.Store.Quality(), store.TableProfiles, q.Profiles, ProfileFingerprint); err != nil {
		return err
	}
	st.Quality = q
	return nil
}

// configQuality is an empty quality state with the config file's keys.
func configQuality(cfg *config.Config) QualityState {
	q := QualityState{Formats: map[string]string{}, Profiles: map[string]string{}, ConfigFormats: map[string]bool{}, ConfigProfiles: map[string]bool{}}
	if cfg == nil {
		return q
	}
	for _, f := range cfg.CustomFormats {
		q.ConfigFormats[f.Key] = true
	}
	for _, p := range cfg.QualityProfiles {
		q.ConfigProfiles[p.Key] = true
	}
	return q
}

func fingerprints[T any](ctx context.Context, r store.Quality, t store.QualityTable, out map[string]string, fp func(T) string) error {
	rows, err := r.List(ctx, t)
	if err != nil {
		return err
	}
	for _, row := range rows {
		var d T
		if err := json.Unmarshal(row.Doc, &d); err != nil {
			return fmt.Errorf("%s %s: %w", t, row.Key, err)
		}
		out[row.Key] = fp(d)
	}
	return nil
}
