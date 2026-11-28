package migrate

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

// BuildPlan decides one outcome per source object without writing anything.
// It is pure: the same snapshots, state and options give the same plan.
func BuildPlan(snaps []Snapshot, st State, o Options) Report {
	snaps = slices.Clone(snaps)
	b := newBuilder(snaps, st, o)
	b.planQuality(snaps)
	r := Report{Mode: "plan", Started: st.Now}
	for _, s := range snaps {
		r.Sources = append(r.Sources, s.Source)
		if s.Source.Error == "" {
			b.source(s)
		}
	}
	b.modes()
	b.qualityEntries()
	r.Entries = b.entries
	sortEntries(r.Entries)
	r.PlanHash = r.hash() // before exclusions: leaving objects out does not change the reviewed plan
	b.exclude(r.Entries)
	r.count()
	r.Config, r.Env = b.cfg.render()
	r.Finished = st.Now
	return r
}

type builder struct {
	st        State
	o         Options
	entries   []Entry
	kinds     map[domain.MediaKind]App
	seen      map[store.ProvenanceKey]bool
	cfg       *configDoc
	managers  map[string]bool // hosts of indexer managers in this run
	sabHosts  map[string]bool // hosts of SABnzbd-type clients (bridge detection)
	clientIDs map[string]string
	indexers  map[string]string
	quality   *qualityPlan
}

func newBuilder(snaps []Snapshot, st State, o Options) *builder {
	if o.Scope == nil {
		o.Scope = DefaultScope()
	}
	b := &builder{st: st, o: o, kinds: map[domain.MediaKind]App{}, seen: map[store.ProvenanceKey]bool{}, cfg: newConfigDoc(),
		managers: map[string]bool{}, sabHosts: map[string]bool{}, clientIDs: map[string]string{}, indexers: map[string]string{}}
	for _, s := range snaps {
		if s.Source.App == Prowlarr {
			b.managers[hostOf(s.Source.URL)] = true
		}
		for _, c := range s.Clients {
			if strings.EqualFold(c.Impl, "sabnzbd") {
				b.sabHosts[strings.ToLower(c.Host)] = true
			}
		}
	}
	return b
}

func (b *builder) add(e Entry) { b.entries = append(b.entries, e) }

func (b *builder) source(s Snapshot) {
	for _, step := range []func(Snapshot){b.profiles, b.metaProfiles, b.roots, b.naming, b.clients, b.indexersOf, b.library, b.downloads,
		b.blocks, b.history} {
		step(s)
	}
}

func (b *builder) exclude(list []Entry) {
	for i := range list {
		if b.o.Exclude[list[i].ID] && list[i].Outcome.writes() {
			list[i].Outcome, list[i].Reason, list[i].op = OutSkip, "left out by you", nil
		}
	}
}

func (b *builder) provKey(s Snapshot, typ string, id int) store.ProvenanceKey {
	return store.ProvenanceKey{App: string(s.Source.App), Instance: s.Source.Instance, ObjectType: typ, ObjectID: fmt.Sprint(id)}
}

// ── profiles, root folders, naming ──────────────────────────────────────────

func (b *builder) profiles(s Snapshot) {
	ids := make([]int, 0, len(s.Profiles))
	for id := range s.Profiles {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		if _, planned := s.ProfileKeys[id]; planned {
			continue // a custom profile: planned across sources
		}
		p := s.Profiles[id]
		m := mapProfile(p, s.Source.App.Kind())
		out := OutApproximate
		if m.Exact {
			out = OutUnchanged
		}
		b.add(Entry{ID: entryID(s.Source.App, "profile", id), Category: CatProfiles, Source: s.Source.App, Name: p.Name,
			Outcome: out, Reason: m.Reason, Target: "profile:" + m.Key})
	}
	if mapsCustom(s.Source.App) {
		return // formats are planned across sources
	}
	for _, f := range s.CustomFormats {
		b.add(Entry{ID: entryID(s.Source.App, "customformat", f.ID), Category: CatProfiles, Source: s.Source.App, Name: f.Name,
			Outcome: OutUnsupported, Reason: "custom formats from this app are not carried over; listed so you can recreate it"})
	}
}

func (b *builder) metaProfiles(s Snapshot) {
	ids := make([]int, 0, len(s.MetaProfiles))
	for id := range s.MetaProfiles {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		p := s.MetaProfiles[id]
		m := mapMetaProfile(p)
		out := OutApproximate
		if m.Exact {
			out = OutUnchanged
		}
		b.add(Entry{ID: entryID(s.Source.App, "metadataprofile", id), Category: CatProfiles, Source: s.Source.App, Name: p.Name,
			Outcome: out, Reason: m.Reason, Target: "metadataprofile:" + m.Key})
	}
}

func (b *builder) roots(s Snapshot) {
	for _, p := range s.RootFolders {
		path := b.o.Rewrite.Apply(p)
		e := Entry{ID: entryID(s.Source.App, "rootfolder", path), Category: CatRootFolders, Source: s.Source.App, Name: path,
			Outcome: OutManaged, Reason: "already in your config file"}
		if !b.st.Config.RootFolders[path] {
			b.cfg.root(path, s.Source.App.Kind())
			e.Reason = "add it to your config file (see the snippet)"
		}
		b.add(e)
	}
}

func (b *builder) naming(s Snapshot) {
	if s.Rename == nil {
		return
	}
	state := map[bool]string{true: "on", false: "off"}[*s.Rename]
	b.add(Entry{ID: entryID(s.Source.App, "naming", "rename"), Category: CatNaming, Source: s.Source.App, Name: "Rename files",
		Outcome: OutUnsupported, Reason: "renaming was " + state + " in the source; corsarr names files on import with its own format and has no switch for it yet"})
}

// ── download clients and indexers ───────────────────────────────────────────

func (b *builder) clients(s Snapshot) {
	for _, c := range s.Clients {
		e := Entry{ID: entryID(s.Source.App, "downloadclient", c.ID), Category: CatClients, Source: s.Source.App, Name: c.Name}
		e.Outcome, e.Reason = b.client(s, c)
		b.add(e)
	}
}

func (b *builder) client(s Snapshot, c SrcClient) (Outcome, string) {
	switch strings.ToLower(c.Impl) {
	case "qbittorrent":
	case "sabnzbd":
		return OutUnsupported, "a SABnzbd-type client (often a bridge to another source); keep it running beside corsarr for now"
	default:
		return OutUnsupported, "no " + c.Impl + " plugin yet"
	}
	u := clientURL(c)
	if b.st.Config.Clients[u] {
		return OutManaged, "already in your config file"
	}
	if first, ok := b.clientIDs[u]; ok {
		b.cfg.category(u, s.Source.App.Kind(), c.Category)
		return OutUnchanged, "same client as " + first + "; its category is merged in"
	}
	b.clientIDs[u] = entryID(s.Source.App, "downloadclient", c.ID)
	env := b.cfg.client(u, c, s.Source.App.Kind())
	if c.PasswordMasked {
		return OutSecretNeeded, "the source hides the password; set " + env + " and enable the instance (it starts disabled)"
	}
	return OutManaged, "add it to your config file (see the snippet); set " + env + " if the Web UI needs a password"
}

func clientURL(c SrcClient) string {
	scheme := "http"
	if c.TLS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d%s", scheme, c.Host, c.Port, strings.TrimRight(c.URLBase, "/"))
}

var managedCopy = regexp.MustCompile(`/\d+/?$`)

func (b *builder) indexersOf(s Snapshot) {
	for _, ix := range s.Indexers {
		e := Entry{ID: entryID(s.Source.App, "indexer", ix.ID), Category: CatIndexers, Source: s.Source.App, Name: ix.Name}
		e.Outcome, e.Reason = b.indexer(s, ix)
		b.add(e)
	}
}

func (b *builder) indexer(s Snapshot, ix SrcIndexer) (Outcome, string) {
	switch {
	case b.isCopy(s, ix):
		return OutSkip, "a copy pushed by an indexer manager; the manager's own indexers are imported once"
	case ix.Protocol == "usenet":
		return b.usenet(ix)
	case strings.EqualFold(ix.Impl, "torznab") && ix.BaseURL != "":
		return b.torznab(ix, ix.BaseURL, ix.APIPath, ix.KeyMasked, "")
	case s.Source.App == Prowlarr:
		proxy := strings.TrimRight(s.Source.URL, "/") + "/" + fmt.Sprint(ix.ID)
		return b.torznab(ix, proxy, "/api", true, "searches go through the indexer manager's Torznab feed for this indexer until a built-in definition replaces it; ")
	}
	return OutUnsupported, "no " + ix.Impl + " indexer plugin yet"
}

// isCopy tells an app's indexer that an indexer manager pushed into it: it
// points at a manager in this run, or at a manager-style per-indexer path.
func (b *builder) isCopy(s Snapshot, ix SrcIndexer) bool {
	return s.Source.App != Prowlarr && (b.managers[hostOf(ix.BaseURL)] || managedCopy.MatchString(ix.BaseURL))
}

func (b *builder) usenet(ix SrcIndexer) (Outcome, string) {
	if b.sabHosts[strings.ToLower(hostOf(ix.BaseURL))] {
		return OutUnsupported, "a bridge pair (Newznab indexer and SABnzbd client on one host); keep the bridge running beside corsarr for now"
	}
	return OutUnsupported, "no usenet indexer plugin yet"
}

func (b *builder) torznab(ix SrcIndexer, u, apiPath string, needKey bool, note string) (Outcome, string) {
	if b.st.Config.Indexers[u] {
		return OutManaged, "already in your config file"
	}
	if first, ok := b.indexers[u]; ok {
		return OutUnchanged, "same indexer as " + first
	}
	b.indexers[u] = ix.Name
	env := b.cfg.indexer(u, apiPath, ix, needKey)
	switch {
	case note != "":
		return OutApproximate, note + "set " + env + " to the manager's API key"
	case needKey:
		return OutSecretNeeded, "the source hides the API key; set " + env + " and enable the instance (it starts disabled)"
	}
	return OutManaged, "add it to your config file (see the snippet)"
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// ── media kinds and rules ────────────────────────────────────────────────────

// modes puts each kind that receives items into shadow (or active when the
// owner chose so) and every automation rule into dry-run; a choice already
// made, in the config file or by an override, is kept.
func (b *builder) modes() {
	kinds := make([]string, 0, len(b.kinds))
	for k := range b.kinds {
		kinds = append(kinds, string(k))
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		b.add(b.kindMode(domain.MediaKind(k)))
	}
	if len(kinds) > 0 {
		b.add(b.ruleMode())
	}
}

// kindNames are display names of the media kinds.
var kindNames = map[domain.MediaKind]string{domain.KindTV: "TV", domain.KindMovie: "Movies", domain.KindMusic: "Music", domain.KindBook: "Books"}

func (b *builder) kindMode(k domain.MediaKind) Entry {
	e := Entry{ID: "corsarr:mode:" + string(k), Category: CatModes, Source: b.kinds[k], Name: kindNames[k] + " mode"}
	mode := domain.ModeShadow
	if b.o.Activate {
		mode = domain.ModeActive
	}
	switch cur, ok := b.st.Overrides["kind:"+string(k)]; {
	case b.st.Config.KindExplicit[string(k)]:
		e.Outcome, e.Reason = OutManaged, "your config file sets this kind's mode; set mediaKinds."+string(k)+".mode: shadow there to compare first"
	case ok:
		e.Outcome, e.Reason = OutUnchanged, "kept your choice: "+cur
	default:
		e.Outcome, e.op = OutCreate, modeOp{store.ModeOverride{Scope: store.ScopeKind, Name: string(k), Mode: string(mode)}}
		e.Reason = kindNames[k] + " runs in " + string(mode) + " mode"
		if mode == domain.ModeShadow {
			e.Reason += ": corsarr keeps the library and queue in view but grabs, imports, notifies and deletes nothing until you switch it to active"
		}
	}
	return e
}

func (b *builder) ruleMode() Entry {
	e := Entry{ID: "corsarr:rules:all", Category: CatModes, Name: "Automation rules"}
	if cur, ok := b.st.Overrides["rule:*"]; ok {
		e.Outcome, e.Reason = OutUnchanged, "kept your choice: "+cur
		return e
	}
	e.Outcome, e.op = OutCreate, modeOp{store.ModeOverride{Scope: store.ScopeRule, Name: "*", Mode: "dry-run"}}
	e.Reason = "every automation rule starts in dry-run and records what it would do; a mode your config file sets still wins"
	return e
}
