package migrate

import (
	"fmt"
	"sort"

	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

// ── series and movies ───────────────────────────────────────────────────────

func (b *builder) library(s Snapshot) {
	if !b.o.Scope.Has(CatLibrary) {
		return
	}
	for _, ser := range s.Series {
		b.series(s, ser)
	}
	for _, m := range s.Movies {
		b.movie(s, m)
	}
	for _, a := range s.Artists {
		b.artist(s, a)
	}
	b.removed(s)
}

// decide compares a source object with what earlier runs and the owner left.
func (b *builder) decide(key store.ProvenanceKey, src string, inCorsarr bool) (Outcome, string, int64) {
	b.seen[key] = true
	if p, ok := b.st.Prov[key]; ok {
		cur, exists := b.st.Current[key]
		switch {
		case !exists:
			return OutSkip, "deleted in corsarr after an earlier run; not re-created", 0
		case p.SourceHash == src:
			return OutUnchanged, "no change since the last run", p.TargetID
		case cur != p.TargetHash:
			return OutConflict, "changed in corsarr and at the source since the last run; kept yours", p.TargetID
		}
		return OutUpdate, "changed at the source since the last run; corsarr's copy was untouched", p.TargetID
	}
	if inCorsarr {
		return OutConflict, "already in corsarr (added there); kept as is", 0
	}
	return OutCreate, "new; monitoring, profile and files come across", 0
}

func (b *builder) series(s Snapshot, src SrcSeries) {
	e := Entry{ID: entryID(s.Source.App, "series", src.ID), Category: CatLibrary, Source: s.Source.App,
		Name: fmt.Sprintf("%s (%d)", src.Title, src.Year)}
	if src.TMDB == 0 {
		e.Outcome, e.Reason = OutUnsupported, "the source has no TMDB id for it; add it in corsarr by hand"
		b.add(e)
		b.seriesFiles(s, src, e)
		return
	}
	ser, seasons, eps := seriesTarget(src, s, b.o.Rewrite)
	key := b.provKey(s, "series", src.ID)
	srcHash := hashOf(src)
	var targetID int64
	_, exists := b.st.Index.SeriesByTMDB[src.TMDB]
	e.Outcome, e.Reason, targetID = b.decide(key, srcHash, exists)
	files := b.seriesFiles(s, src, e)
	if e.Outcome.writes() {
		b.kinds[domain.KindTV] = s.Source.App
		e.op = seriesOp{series: ser, seasons: seasons, eps: eps, files: files, prov: store.Provenance{Key: key, TargetID: targetID,
			SourceHash: srcHash, TargetHash: SeriesTargetHash(ser, seasons, eps)}}
	}
	b.add(e)
}

// seriesFiles reports every file of a series and returns those to adopt.
func (b *builder) seriesFiles(s Snapshot, src SrcSeries, parent Entry) []store.EpisodeFile {
	byFile := map[int][]store.EpisodeKey{}
	for _, ep := range src.Episodes {
		if ep.FileID > 0 {
			byFile[ep.FileID] = append(byFile[ep.FileID], store.EpisodeKey{Season: ep.Season, Number: ep.Number})
		}
	}
	var out []store.EpisodeFile
	for _, f := range src.Files {
		if lf, ok := b.file(s, f, "episodefile", parent); ok && len(byFile[f.ID]) > 0 {
			out = append(out, store.EpisodeFile{File: lf, Episodes: byFile[f.ID]})
		}
	}
	return out
}

// file reports one source file; ok means it is adopted in place.
func (b *builder) file(s Snapshot, f SrcFile, typ string, parent Entry) (domain.LibraryFile, bool) {
	lf, exact := libraryFile(f, b.o.Rewrite, s.Source.App.Kind())
	e := Entry{ID: entryID(s.Source.App, typ, f.ID), Category: CatFiles, Source: s.Source.App, Name: lf.Path}
	defer func() { b.add(e) }()
	if !parent.Outcome.writes() {
		e.Outcome, e.Reason = parent.Outcome, "follows its item: "+parent.Reason
		return lf, false
	}
	e.Outcome, e.Reason = b.fileCheck(lf)
	if e.Outcome != OutCreate {
		return lf, false
	}
	if !exact {
		e.Outcome, e.Reason = OutApproximate, e.Reason+fmt.Sprintf("; quality %q is recorded as unknown", f.Quality)
	}
	return lf, true
}

func (b *builder) fileCheck(lf domain.LibraryFile) (Outcome, string) {
	if _, ok := b.st.Index.FilePaths[lf.Path]; ok {
		return OutUnchanged, "already linked"
	}
	size, ok := b.st.Stat(lf.Path)
	switch {
	case !ok:
		return OutFileMissing, "not found on disk; the item is kept without it"
	case size != lf.Size:
		return OutFileMissing, fmt.Sprintf("size on disk (%d) differs from the source's (%d); not linked", size, lf.Size)
	}
	return OutCreate, "adopted where it is: no move, copy or rename"
}

func (b *builder) movie(s Snapshot, src SrcMovie) {
	e := Entry{ID: entryID(s.Source.App, "movie", src.ID), Category: CatLibrary, Source: s.Source.App,
		Name: fmt.Sprintf("%s (%d)", src.Title, src.Year)}
	m, notes := movieTarget(src, s, b.o.Rewrite)
	key := b.provKey(s, "movie", src.ID)
	srcHash := hashOf(src)
	var targetID int64
	_, exists := b.st.Index.MovieByTMDB[src.TMDB]
	e.Outcome, e.Reason, targetID = b.decide(key, srcHash, exists)
	if src.TMDB == 0 {
		e.Outcome, e.Reason = OutUnsupported, "the source has no TMDB id for it; add it in corsarr by hand"
	}
	var file *domain.LibraryFile
	if src.File != nil {
		if lf, ok := b.file(s, *src.File, "moviefile", e); ok {
			file = &lf
		}
	}
	if e.Outcome.writes() {
		b.kinds[domain.KindMovie] = s.Source.App
		e.op = movieOp{movie: m, file: file, prov: store.Provenance{Key: key, TargetID: targetID, SourceHash: srcHash,
			TargetHash: MovieTargetHash(m)}}
		e.Reason = joinNotes(e.Reason, notes)
	}
	b.add(e)
}

func joinNotes(reason string, notes []string) string {
	for _, n := range notes {
		reason += "; " + n
	}
	return reason
}

// libraryTypes are the provenance object types of library items.
var libraryTypes = map[string]bool{"series": true, "movie": true, "artist": true}

// removed reports library objects of earlier runs the source no longer has.
func (b *builder) removed(s Snapshot) {
	var keys []store.ProvenanceKey
	for k := range b.st.Prov {
		if k.App == string(s.Source.App) && k.Instance == s.Source.Instance && libraryTypes[k.ObjectType] && !b.seen[k] {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].ObjectID < keys[j].ObjectID })
	for _, k := range keys {
		b.add(Entry{ID: entryID(s.Source.App, k.ObjectType, k.ObjectID), Category: CatLibrary, Source: s.Source.App,
			Name: fmt.Sprintf("%s %s", k.ObjectType, k.ObjectID), Outcome: OutSkip, Reason: "removed at source; kept in corsarr"})
	}
}

// ── item keys of a snapshot ──────────────────────────────────────────────────

// keysOf resolves source item ids to join keys.
type keysOf struct {
	episodes map[int]itemKey
	movies   map[int]itemKey
	albums   map[int]itemKey
	series   map[int][]itemKey
}

func lookups(s Snapshot) keysOf {
	k := keysOf{episodes: map[int]itemKey{}, movies: map[int]itemKey{}, albums: map[int]itemKey{}, series: map[int][]itemKey{}}
	for _, ser := range s.Series {
		for _, ep := range ser.Episodes {
			key := itemKey{Kind: domain.KindTV, TMDB: ser.TMDB, Ep: &store.EpisodeKey{Season: ep.Season, Number: ep.Number}}
			k.episodes[ep.ID] = key
			k.series[ser.ID] = append(k.series[ser.ID], key)
		}
	}
	for _, m := range s.Movies {
		k.movies[m.ID] = itemKey{Kind: domain.KindMovie, TMDB: m.TMDB}
	}
	for _, a := range s.Artists {
		for _, al := range a.Albums {
			k.albums[al.ID] = itemKey{Kind: domain.KindMusic, Artist: a.MBID, Album: al.MBID}
		}
	}
	return k
}

// items lists the known keys among episode, movie and album ids.
func (k keysOf) items(episodes []int, movie int, albums []int) []itemKey {
	out := known(k.episodes, episodes)
	if key, ok := k.movies[movie]; ok {
		out = append(out, key)
	}
	return append(out, known(k.albums, albums)...)
}

func known(m map[int]itemKey, ids []int) []itemKey {
	var out []itemKey
	for _, id := range ids {
		if key, ok := m[id]; ok {
			out = append(out, key)
		}
	}
	return out
}

// ── in-flight downloads ─────────────────────────────────────────────────────

func (b *builder) downloads(s Snapshot) {
	groups := map[string][]SrcQueue{}
	var ids []string
	for _, q := range s.Queue {
		if q.DownloadID == "" {
			continue
		}
		if _, ok := groups[q.DownloadID]; !ok {
			ids = append(ids, q.DownloadID)
		}
		groups[q.DownloadID] = append(groups[q.DownloadID], q)
	}
	sort.Strings(ids)
	keys := lookups(s)
	for _, id := range ids {
		b.download(s, id, groups[id], keys)
	}
}

func (b *builder) download(s Snapshot, id string, rows []SrcQueue, keys keysOf) {
	e := Entry{ID: entryID(s.Source.App, "download", id), Category: CatDownloads, Source: s.Source.App, Name: rows[0].Title}
	defer func() { b.add(e) }()
	hit, inClient := b.st.Client[id]
	switch {
	case rows[0].Protocol != string(domain.ProtoTorrent):
		e.Outcome, e.Reason = OutUnsupported, "only torrent downloads can be adopted yet; the source app finishes it"
	case b.st.Live[id]:
		e.Outcome, e.Reason = OutUnchanged, "already tracked by corsarr"
	case b.st.Client == nil:
		e.Outcome, e.Reason = OutSkip, "no download client in corsarr yet; add the qBittorrent snippet to your config file and run again"
	case !inClient:
		e.Outcome, e.Reason = OutGone, "no longer in the download client"
	default:
		var eps, albums []int
		for _, r := range rows {
			eps, albums = append(eps, r.EpisodeID), append(albums, r.AlbumID)
		}
		e.Outcome = OutCreate
		e.Reason = fmt.Sprintf("adopted as one download linked to %d items; the client is not touched", len(rows))
		e.op = downloadOp{hit: hit, id: id, title: rows[0].Title, protocol: domain.ProtoTorrent, kind: s.Source.App.Kind(),
			size: rows[0].Size, items: keys.items(eps, rows[0].MovieID, albums)}
	}
}

// ── blocklist and history ───────────────────────────────────────────────────

func (b *builder) blocks(s Snapshot) {
	keys := lookups(s)
	for _, bl := range s.Blocklist {
		e := Entry{ID: entryID(s.Source.App, "blocklist", bl.ID), Category: CatBlocklist, Source: s.Source.App, Name: bl.Title}
		key := b.provKey(s, "blocklist", bl.ID)
		items := keys.items(bl.EpisodeIDs, bl.MovieID, bl.AlbumIDs)
		switch _, done := b.st.Prov[key]; {
		case done:
			e.Outcome, e.Reason = OutUnchanged, "imported by an earlier run"
		case len(items) == 0:
			e.Outcome, e.Reason = OutSkip, "its item is not in the source library"
		default:
			e.Outcome, e.Reason = OutCreate, "kept from grabbing again; marked imported from "+string(s.Source.App)
			e.op = blockOp{items: items, prov: store.Provenance{Key: key}, entry: domain.BlocklistEntry{
				Release: domain.ReleaseIdentity{Title: bl.Title}, Source: "imported", AddedAt: bl.Date,
				Reason: joinNotes("imported from "+string(s.Source.App), nonEmpty(bl.Message, "indexer "+bl.Indexer))}}
		}
		b.add(e)
	}
}

func nonEmpty(list ...string) []string {
	var out []string
	for _, s := range list {
		if s != "" && s != "indexer " {
			out = append(out, s)
		}
	}
	return out
}

// historyTypes maps the source event names corsarr keeps.
var historyTypes = map[string]domain.EventType{
	"grabbed": domain.EvGrabSent, "downloadFolderImported": domain.EvImportCompleted,
	"movieFolderImported": domain.EvImportCompleted, "downloadFailed": domain.EvDownloadFailed,
	"episodeFileDeleted": domain.EvFileDeleted, "movieFileDeleted": domain.EvFileDeleted,
	"episodeFileRenamed": domain.EvFileRenamed, "movieFileRenamed": domain.EvFileRenamed,
	"trackFileImported": domain.EvImportCompleted, "trackFileDeleted": domain.EvFileDeleted,
	"trackFileRenamed": domain.EvFileRenamed,
}

func (b *builder) history(s Snapshot) {
	keys := lookups(s)
	for _, h := range s.History {
		e := Entry{ID: entryID(s.Source.App, "history", h.ID), Category: CatHistory, Source: s.Source.App, Name: h.Title}
		key := b.provKey(s, "history", h.ID)
		t, known := historyTypes[h.Event]
		switch _, done := b.st.Prov[key]; {
		case done:
			e.Outcome, e.Reason = OutUnchanged, "imported by an earlier run"
		case !known:
			e.Outcome, e.Reason = OutSkip, h.Event+" rows are not kept"
		default:
			e.Outcome, e.Reason = OutCreate, "kept in History, marked imported from "+string(s.Source.App)
			e.op = historyOp{title: h.Title, prov: store.Provenance{Key: key}, items: keys.items([]int{h.EpisodeID}, h.MovieID, []int{h.AlbumID}),
				ev: domain.Event{SchemaVersion: 1, EventID: "import-" + s.Source.Instance + "-" + fmt.Sprint(h.ID), Type: t,
					OccurredAt: h.Date, Actor: "import:" + string(s.Source.App), Imported: true, Kind: s.Source.App.Kind(),
					Data: map[string]any{"sourceTitle": h.Title, "downloadId": h.DownloadID, "importedFrom": string(s.Source.App)}}}
		}
		b.add(e)
	}
}
