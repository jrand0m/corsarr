package store

import (
	"context"
	"database/sql"

	"github.com/jrand0m/corsarr/internal/domain"
)

// TrackKey is a track's place in its album's edition.
type TrackKey struct{ Medium, Number int }

// TrackFile is one adopted file and the tracks it holds.
type TrackFile struct {
	File   domain.LibraryFile
	Tracks []TrackKey
}

// AdoptAlbum is an album to adopt with the tracks of its selected edition
// (none = left for the metadata plugin) and the files to link to them.
type AdoptAlbum struct {
	Album  domain.Album
	Tracks []domain.Track
	Files  []TrackFile
}

// AdoptArtist creates the artist (or updates the one prov.TargetID names),
// upserts its albums, seeds the selected edition of albums that have none
// yet, links the adopted files where they are and records provenance, all
// in one transaction. The artist is left due for a metadata refresh. It
// returns the artist id.
func (r Migration) AdoptArtist(ctx context.Context, a domain.Artist, albums []AdoptAlbum, prov Provenance) (int64, error) {
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		id, err := writeArtist(ctx, tx, a, prov.TargetID)
		if err != nil {
			return err
		}
		plain := make([]domain.Album, len(albums))
		for i, al := range albums {
			plain[i] = al.Album
		}
		if err := upsertAlbums(ctx, tx, id, plain); err != nil {
			return err
		}
		for _, al := range albums {
			if err := adoptAlbum(ctx, tx, id, al); err != nil {
				return err
			}
		}
		prov.TargetKind, prov.TargetID = string(domain.KindMusic), id
		return putProvenance(ctx, tx, prov)
	})
	return prov.TargetID, err
}

func writeArtist(ctx context.Context, tx *sql.Tx, a domain.Artist, id int64) (int64, error) {
	if id > 0 {
		_, err := tx.ExecContext(ctx, `UPDATE artists SET root_folder = ?, path = ?, profile_key = ?, metadata_profile = ?,
			monitor_new = ?, monitored = ?, tags = ? WHERE id = ?`, a.RootFolder, a.Path, a.ProfileKey, a.MetadataProfile,
			string(a.MonitorNew), a.Monitored, jsonText(a.Tags), id)
		return id, err
	}
	err := tx.QueryRowContext(ctx, `INSERT INTO artists (mbid, name, sort_name, disambiguation, artist_type, country, root_folder,
		path, profile_key, metadata_profile, monitor_new, monitored, tags, added_at, refreshed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0) RETURNING id`,
		a.MBID, a.Name, a.SortName, a.Disambiguation, a.Type, a.Country, a.RootFolder, a.Path, a.ProfileKey, a.MetadataProfile,
		string(a.MonitorNew), a.Monitored, jsonText(a.Tags), ms(a.AddedAt)).Scan(&id)
	return id, err
}

// adoptAlbum seeds the album's edition when it has no tracks yet and links
// its files to the tracks they hold.
func adoptAlbum(ctx context.Context, tx *sql.Tx, artistID int64, al AdoptAlbum) error {
	var albumID, tracks int64
	if err := tx.QueryRowContext(ctx, `SELECT id, (SELECT COUNT(*) FROM tracks WHERE album_id = albums.id) FROM albums
		WHERE artist_id = ? AND mbid = ?`, artistID, al.Album.MBID).Scan(&albumID, &tracks); err != nil {
		return err
	}
	if tracks == 0 && len(al.Tracks) > 0 {
		if err := seedEdition(ctx, tx, albumID, al); err != nil {
			return err
		}
	}
	for _, f := range al.Files {
		if err := linkTrackFile(ctx, tx, albumID, f); err != nil {
			return err
		}
	}
	return nil
}

func seedEdition(ctx context.Context, tx *sql.Tx, albumID int64, al AdoptAlbum) error {
	a := al.Album
	if _, err := tx.ExecContext(ctx, `UPDATE albums SET edition_mbid = ?, medium_count = ?, track_count = ?, duration_sec = ?
		WHERE id = ?`, a.EditionMBID, a.MediumCount, a.TrackCount, a.DurationSec, albumID); err != nil {
		return err
	}
	for _, t := range al.Tracks {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tracks (album_id, recording_mbid, medium, medium_format, number, title,
			duration_sec) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (album_id, medium, number) DO NOTHING`,
			albumID, t.RecordingID, t.Medium, t.MediumFormat, t.Number, t.Title, t.DurationSec); err != nil {
			return err
		}
	}
	return nil
}

// linkTrackFile links one file to the tracks it holds; a file whose tracks
// the album's edition lacks stays unlinked.
func linkTrackFile(ctx context.Context, tx *sql.Tx, albumID int64, f TrackFile) error {
	ids, err := trackIDs(ctx, tx, albumID, f.Tracks)
	if err != nil || len(ids) == 0 {
		return err
	}
	f.File.Item = domain.ItemRef{Kind: domain.KindMusic, ID: ids[0], Variant: domain.VariantTrack}
	fileID, err := upsertFile(ctx, tx, f.File)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, "UPDATE tracks SET file_id = ? WHERE id = ?", fileID, id); err != nil {
			return err
		}
	}
	return nil
}

// trackIDs finds the ids of the tracks at the keys that exist.
func trackIDs(ctx context.Context, tx *sql.Tx, albumID int64, keys []TrackKey) ([]int64, error) {
	var ids []int64
	for _, k := range keys {
		var id int64
		ok, err := found(tx.QueryRowContext(ctx, "SELECT id FROM tracks WHERE album_id = ? AND medium = ? AND number = ?",
			albumID, k.Medium, k.Number).Scan(&id))
		if err != nil {
			return nil, err
		}
		if ok {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// AlbumIDs maps the album MusicBrainz ids of an artist (by its MusicBrainz
// id) to album ids.
func (r Migration) AlbumIDs(ctx context.Context, artistMBID string) (map[string]int64, error) {
	out := map[string]int64{}
	scan := func(sc scanner) (domain.Album, error) {
		var a domain.Album
		return a, sc.Scan(&a.MBID, &a.ID)
	}
	err := eachRow(ctx, r.s.R(), scan, func(a domain.Album) error { out[a.MBID] = a.ID; return nil },
		`SELECT al.mbid, al.id FROM albums al JOIN artists ar ON ar.id = al.artist_id WHERE ar.mbid = ?`, artistMBID)
	return out, err
}
