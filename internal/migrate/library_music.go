package migrate

import (
	"github.com/jrand0m/corsarr/internal/domain"
	"github.com/jrand0m/corsarr/internal/store"
)

// ── artists ──────────────────────────────────────────────────────────────────

func (b *builder) artist(s Snapshot, src SrcArtist) {
	e := Entry{ID: entryID(s.Source.App, "artist", src.ID), Category: CatLibrary, Source: s.Source.App, Name: src.Name}
	if src.MBID == "" {
		e.Outcome, e.Reason = OutUnsupported, "the source has no MusicBrainz id for it; add it in corsarr by hand"
		b.add(e)
		b.artistFiles(s, src, e)
		return
	}
	ar, albums, notes := artistTarget(src, s, b.o.Rewrite)
	key := b.provKey(s, "artist", src.ID)
	srcHash := hashOf(src)
	var targetID int64
	_, exists := b.st.Index.ArtistByMBID[src.MBID]
	e.Outcome, e.Reason, targetID = b.decide(key, srcHash, exists)
	files := b.artistFiles(s, src, e)
	if e.Outcome.writes() {
		b.kinds[domain.KindMusic] = s.Source.App
		for i := range albums {
			albums[i].Files = files[albums[i].Album.MBID]
		}
		e.op = artistOp{artist: ar, albums: albums, prov: store.Provenance{Key: key, TargetID: targetID, SourceHash: srcHash,
			TargetHash: ArtistTargetHash(ar, albumsOf(albums))}}
		e.Reason = joinNotes(e.Reason, notes)
	}
	b.add(e)
}

// trackPlace is where a source track file sits: its album and tracks.
type trackPlace struct {
	album  string
	tracks []store.TrackKey
}

// artistFiles reports every track file of an artist and returns those to
// adopt by album MusicBrainz id.
func (b *builder) artistFiles(s Snapshot, src SrcArtist, parent Entry) map[string][]store.TrackFile {
	places := map[int]trackPlace{}
	for _, al := range src.Albums {
		for _, t := range al.Tracks {
			if t.FileID > 0 && al.MBID != "" {
				p := places[t.FileID]
				p.album, p.tracks = al.MBID, append(p.tracks, store.TrackKey{Medium: t.Medium, Number: t.Number})
				places[t.FileID] = p
			}
		}
	}
	out := map[string][]store.TrackFile{}
	for _, f := range src.Files {
		p, placed := places[f.ID]
		if !placed && parent.Outcome.writes() {
			b.add(Entry{ID: entryID(s.Source.App, "trackfile", f.ID), Category: CatFiles, Source: s.Source.App,
				Name: b.o.Rewrite.Apply(f.Path), Outcome: OutSkip,
				Reason: "not on a track of the album's selected edition at the source; corsarr matches it when the album is imported again"})
			continue
		}
		if lf, ok := b.file(s, f.SrcFile, "trackfile", parent); ok {
			out[p.album] = append(out[p.album], store.TrackFile{File: lf, Tracks: p.tracks})
		}
	}
	return out
}
