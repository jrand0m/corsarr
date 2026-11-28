package migrate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

// Target is where an apply writes (the store in production).
type Target interface {
	AdoptSeries(ctx context.Context, s domain.Series, seasons []domain.Season, eps []domain.Episode, files []store.EpisodeFile,
		p store.Provenance) (int64, error)
	AdoptMovie(ctx context.Context, m domain.Movie, f *domain.LibraryFile, p store.Provenance) (int64, error)
	AdoptArtist(ctx context.Context, a domain.Artist, albums []store.AdoptAlbum, p store.Provenance) (int64, error)
	EpisodeIDs(ctx context.Context, tmdbID int64) (map[store.EpisodeKey]int64, error)
	AlbumIDs(ctx context.Context, artistMBID string) (map[string]int64, error)
	Index(ctx context.Context) (store.LibraryIndex, error)
	AdoptDownload(ctx context.Context, g domain.Grab, d domain.Download) error
	Block(ctx context.Context, e domain.BlocklistEntry) error
	AppendHistory(ctx context.Context, ev domain.Event) error
	Record(ctx context.Context, p store.Provenance) error
	SetOverride(ctx context.Context, o store.ModeOverride) error
	SaveQuality(ctx context.Context, t store.QualityTable, o store.QualityObject, now time.Time) error
}

// op is what applying one entry does; it returns the target it wrote.
type op interface {
	apply(ctx context.Context, a *applier) (string, error)
}

// errNoItem says an entry's item is not in corsarr (its parent was skipped).
var errNoItem = errors.New("the item is not in corsarr")

// itemKey names an item by its external join key: a TMDB id (and an
// episode's place), or an artist's and an album's MusicBrainz ids.
type itemKey struct {
	Kind   domain.MediaKind
	TMDB   int64
	Ep     *store.EpisodeKey
	Artist string
	Album  string
}

type applier struct {
	t      Target
	now    time.Time
	eps    map[int64]map[store.EpisodeKey]int64
	movies map[int64]int64
	albums map[string]map[string]int64
}

// resolve finds the corsarr item of a key after the library was adopted.
func (a *applier) resolve(ctx context.Context, k itemKey) (domain.ItemRef, error) {
	switch k.Kind {
	case domain.KindMovie:
		return a.movie(ctx, k.TMDB)
	case domain.KindMusic:
		return a.album(ctx, k.Artist, k.Album)
	}
	ids, ok := a.eps[k.TMDB]
	if !ok {
		var err error
		if ids, err = a.t.EpisodeIDs(ctx, k.TMDB); err != nil {
			return domain.ItemRef{}, err
		}
		a.eps[k.TMDB] = ids
	}
	if id := ids[*k.Ep]; id > 0 {
		return domain.ItemRef{Kind: domain.KindTV, ID: id}, nil
	}
	return domain.ItemRef{}, errNoItem
}

func (a *applier) movie(ctx context.Context, tmdb int64) (domain.ItemRef, error) {
	if a.movies == nil {
		ix, err := a.t.Index(ctx)
		if err != nil {
			return domain.ItemRef{}, err
		}
		a.movies = ix.MovieByTMDB
	}
	if id := a.movies[tmdb]; id > 0 {
		return domain.ItemRef{Kind: domain.KindMovie, ID: id}, nil
	}
	return domain.ItemRef{}, errNoItem
}

func (a *applier) album(ctx context.Context, artist, album string) (domain.ItemRef, error) {
	ids, ok := a.albums[artist]
	if !ok {
		var err error
		if ids, err = a.t.AlbumIDs(ctx, artist); err != nil {
			return domain.ItemRef{}, err
		}
		a.albums[artist] = ids
	}
	if id := ids[album]; id > 0 {
		return domain.ItemRef{Kind: domain.KindMusic, ID: id}, nil
	}
	return domain.ItemRef{}, errNoItem
}

// resolveAll resolves the keys that exist; none at all is errNoItem.
func (a *applier) resolveAll(ctx context.Context, keys []itemKey) ([]domain.ItemRef, error) {
	var out []domain.ItemRef
	for _, k := range keys {
		ref, err := a.resolve(ctx, k)
		if errors.Is(err, errNoItem) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	if len(out) == 0 {
		return nil, errNoItem
	}
	return out, nil
}

// Apply executes the planned entries in report order (modes first, then the
// library, then what refers to it). A failed entry is reported and the run
// goes on; a cancelled context stops at the current entry.
func Apply(ctx context.Context, r *Report, t Target, now time.Time, progress func(Progress)) error {
	a := &applier{t: t, now: now, eps: map[int64]map[store.EpisodeKey]int64{}, albums: map[string]map[string]int64{}}
	for i := range r.Entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		e := &r.Entries[i]
		if e.op == nil || !e.Outcome.writes() {
			continue
		}
		progress(Progress{Source: e.Source, Step: string(e.Category), Done: i + 1, Total: len(r.Entries)})
		applyOne(ctx, a, e)
	}
	r.Mode = "apply"
	r.count()
	return nil
}

func applyOne(ctx context.Context, a *applier, e *Entry) {
	target, err := e.op.apply(ctx, a)
	switch {
	case errors.Is(err, errNoItem):
		e.Outcome, e.Reason = OutSkip, err.Error()+"; it was not migrated"
	case err != nil:
		e.Outcome, e.Reason = OutFailed, err.Error()
	default:
		e.Target = target
	}
}

type seriesOp struct {
	series  domain.Series
	seasons []domain.Season
	eps     []domain.Episode
	files   []store.EpisodeFile
	prov    store.Provenance
}

func (o seriesOp) apply(ctx context.Context, a *applier) (string, error) {
	o.series.AddedAt, o.prov.ImportedAt = a.now, a.now
	id, err := a.t.AdoptSeries(ctx, o.series, o.seasons, o.eps, o.files, o.prov)
	delete(a.eps, o.series.TMDBID)
	return fmt.Sprintf("tv:%d", id), err
}

type movieOp struct {
	movie domain.Movie
	file  *domain.LibraryFile
	prov  store.Provenance
}

func (o movieOp) apply(ctx context.Context, a *applier) (string, error) {
	o.movie.AddedAt, o.prov.ImportedAt = a.now, a.now
	id, err := a.t.AdoptMovie(ctx, o.movie, o.file, o.prov)
	a.movies = nil
	return fmt.Sprintf("movie:%d", id), err
}

type artistOp struct {
	artist domain.Artist
	albums []store.AdoptAlbum
	prov   store.Provenance
}

func (o artistOp) apply(ctx context.Context, a *applier) (string, error) {
	o.artist.AddedAt, o.prov.ImportedAt = a.now, a.now
	id, err := a.t.AdoptArtist(ctx, o.artist, o.albums, o.prov)
	delete(a.albums, o.artist.MBID)
	return fmt.Sprintf("music:%d", id), err
}

// originMigrated marks a grab corsarr adopted instead of sending.
const originMigrated = domain.SearchOrigin("migrated")

type downloadOp struct {
	hit      ClientHit
	id       string
	title    string
	protocol domain.Protocol
	kind     domain.MediaKind
	size     int64
	items    []itemKey
}

// apply records the download as already sent; the client is never called,
// and the queue picks its progress up on the next poll.
func (o downloadOp) apply(ctx context.Context, a *applier) (string, error) {
	refs, err := a.resolveAll(ctx, o.items)
	if err != nil {
		return "", err
	}
	links := make([]domain.DownloadLink, len(refs))
	for i, r := range refs {
		links[i] = domain.DownloadLink{Item: r, Status: domain.LinkPending}
	}
	rel := domain.Release{ID: o.id, Title: o.title, Protocol: o.protocol, Size: o.size, InfoHash: o.id, PublishedAt: a.now}
	g := domain.Grab{Kind: o.kind, ItemRefs: refs, Release: rel, Origin: originMigrated, Downloader: domain.ObjectKey(o.hit.Downloader),
		DownloadID: o.id, SentAt: a.now, Decision: domain.Decision{ReleaseID: o.id, ItemRefs: refs, Accepted: true, Override: domain.OverrideAllowed}}
	d := domain.Download{Downloader: g.Downloader, DownloadID: o.id, Protocol: o.protocol, Kind: o.kind, Title: o.title,
		State: domain.StateQueued, Links: links, Size: o.size, Category: o.hit.Category, SavePath: o.hit.SavePath, AddedAt: a.now}
	return "download:" + o.hit.Downloader + "/" + o.id, a.t.AdoptDownload(ctx, g, d)
}

type blockOp struct {
	items []itemKey
	entry domain.BlocklistEntry
	prov  store.Provenance
}

func (o blockOp) apply(ctx context.Context, a *applier) (string, error) {
	refs, err := a.resolveAll(ctx, o.items)
	if err != nil {
		return "", err
	}
	for _, r := range refs {
		e := o.entry
		e.Item = r
		if err := a.t.Block(ctx, e); err != nil {
			return "", err
		}
	}
	o.prov.TargetKind, o.prov.ImportedAt = string(refs[0].Kind), a.now
	return fmt.Sprintf("blocklist:%d items", len(refs)), a.t.Record(ctx, o.prov)
}

type historyOp struct {
	ev    domain.Event
	title string
	items []itemKey
	prov  store.Provenance
}

// apply writes the row straight into History: it never reaches the event
// bus, so it triggers no notification and no automation.
func (o historyOp) apply(ctx context.Context, a *applier) (string, error) {
	refs, err := a.resolveAll(ctx, o.items)
	if err != nil && !errors.Is(err, errNoItem) {
		return "", err
	}
	for _, r := range refs {
		o.ev.Items = append(o.ev.Items, domain.EventItem{Ref: r, Title: o.title})
	}
	if err := a.t.AppendHistory(ctx, o.ev); err != nil {
		return "", err
	}
	o.prov.TargetKind, o.prov.ImportedAt = string(o.ev.Kind), a.now
	return "history:" + o.ev.EventID, a.t.Record(ctx, o.prov)
}

type modeOp struct{ o store.ModeOverride }

func (m modeOp) apply(ctx context.Context, a *applier) (string, error) {
	m.o.SetAt = a.now
	return m.o.Scope + ":" + m.o.Name + "=" + m.o.Mode, a.t.SetOverride(ctx, m.o)
}
