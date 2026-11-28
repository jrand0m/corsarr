package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Music stores artists, albums and tracks (single writer: the music module).
type Music struct{ s *Store }

// Music returns the music repository.
func (s *Store) Music() Music { return Music{s} }

const artistCols = `id, mbid, name, sort_name, disambiguation, artist_type, country, root_folder, path, profile_key,
	metadata_profile, monitor_new, monitored, tags, added_at`

func scanArtist(sc scanner) (domain.Artist, error) {
	var a domain.Artist
	var monNew, tags string
	var added int64
	err := sc.Scan(&a.ID, &a.MBID, &a.Name, &a.SortName, &a.Disambiguation, &a.Type, &a.Country, &a.RootFolder, &a.Path,
		&a.ProfileKey, &a.MetadataProfile, &monNew, &a.Monitored, &tags, &added)
	a.MonitorNew, a.Tags, a.AddedAt = domain.MonitorNewItems(monNew), jsonList(tags), fromMS(added)
	return a, err
}

const albumCols = `id, artist_id, mbid, title, primary_type, secondary_types, release_date, monitored, pin_reason, edition_mbid,
	edition_locked, medium_count, track_count, duration_sec`

func scanAlbum(sc scanner) (domain.Album, error) {
	var a domain.Album
	var typ, sec, pin string
	var date sql.NullInt64
	err := sc.Scan(&a.ID, &a.ArtistID, &a.MBID, &a.Title, &typ, &sec, &date, &a.Monitored, &pin, &a.EditionMBID,
		&a.EditionLocked, &a.MediumCount, &a.TrackCount, &a.DurationSec)
	a.PrimaryType, a.SecondaryTypes, a.ReleaseDate, a.PinReason = domain.AlbumType(typ), jsonList(sec), ptrMS(date), domain.PinReason(pin)
	return a, err
}

const trackCols = `id, album_id, recording_mbid, medium, medium_format, number, title, duration_sec, file_id`

func scanTrack(sc scanner) (domain.Track, error) {
	var t domain.Track
	err := sc.Scan(&t.ID, &t.AlbumID, &t.RecordingID, &t.Medium, &t.MediumFormat, &t.Number, &t.Title, &t.DurationSec, &t.FileID)
	return t, err
}

// AddArtist inserts an artist with its albums in one transaction, keyed by
// MusicBrainz id; adding the same id again returns the stored artist with
// created=false.
func (r Music) AddArtist(ctx context.Context, a domain.Artist, albums []domain.Album) (domain.Artist, bool, error) {
	var created bool
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO artists (mbid, name, sort_name, disambiguation, artist_type, country,
			root_folder, path, profile_key, metadata_profile, monitor_new, monitored, tags, added_at, refreshed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (mbid) DO NOTHING`,
			a.MBID, a.Name, a.SortName, a.Disambiguation, a.Type, a.Country, a.RootFolder, a.Path, a.ProfileKey,
			a.MetadataProfile, string(a.MonitorNew), a.Monitored, jsonText(a.Tags), ms(a.AddedAt), ms(a.AddedAt))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return nil
		}
		created = true
		id, _ := res.LastInsertId()
		return upsertAlbums(ctx, tx, id, albums)
	})
	if err != nil {
		return domain.Artist{}, false, err
	}
	out, err := scanArtist(r.s.R().QueryRowContext(ctx, "SELECT "+artistCols+" FROM artists WHERE mbid = ?", a.MBID))
	return out, created, err
}

// upsertAlbums writes album metadata and monitoring by MusicBrainz id; the
// edition, its tracks and the availability flag are owned by other writes.
func upsertAlbums(ctx context.Context, tx *sql.Tx, artistID int64, albums []domain.Album) error {
	for _, a := range albums {
		if _, err := tx.ExecContext(ctx, `INSERT INTO albums (artist_id, mbid, title, primary_type, secondary_types,
			release_date, monitored, pin_reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (artist_id, mbid) DO UPDATE SET title = excluded.title, primary_type = excluded.primary_type,
			secondary_types = excluded.secondary_types, release_date = excluded.release_date,
			monitored = excluded.monitored, pin_reason = excluded.pin_reason`,
			artistID, a.MBID, a.Title, string(a.PrimaryType), jsonText(a.SecondaryTypes), nullMS(a.ReleaseDate), a.Monitored,
			string(a.PinReason)); err != nil {
			return err
		}
	}
	return nil
}

// SyncArtist writes an artist's metadata and settings and upserts its albums
// in one transaction.
func (r Music) SyncArtist(ctx context.Context, a domain.Artist, albums []domain.Album) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE artists SET name = ?, sort_name = ?, disambiguation = ?, artist_type = ?,
			country = ?, root_folder = ?, path = ?, profile_key = ?, metadata_profile = ?, monitor_new = ?, monitored = ?,
			tags = ? WHERE id = ?`, a.Name, a.SortName, a.Disambiguation, a.Type, a.Country, a.RootFolder, a.Path, a.ProfileKey,
			a.MetadataProfile, string(a.MonitorNew), a.Monitored, jsonText(a.Tags), a.ID); err != nil {
			return err
		}
		return upsertAlbums(ctx, tx, a.ID, albums)
	})
}

// MarkRefreshed records a metadata refresh of an artist.
func (r Music) MarkRefreshed(ctx context.Context, id int64, at time.Time) error {
	return r.s.exec(ctx, "UPDATE artists SET refreshed_at = ? WHERE id = ?", ms(at), id)
}

// RefreshedAt is when the artist's metadata was last refreshed.
func (r Music) RefreshedAt(ctx context.Context, id int64) (time.Time, error) {
	var at int64
	err := r.s.R().QueryRowContext(ctx, "SELECT refreshed_at FROM artists WHERE id = ?", id).Scan(&at)
	return fromMS(at), err
}

// Artist returns an artist by id.
func (r Music) Artist(ctx context.Context, id int64) (domain.Artist, bool, error) {
	a, err := scanArtist(r.s.R().QueryRowContext(ctx, "SELECT "+artistCols+" FROM artists WHERE id = ?", id))
	ok, err := found(err)
	return a, ok, err
}

// ArtistByMBID returns the artist with a MusicBrainz id.
func (r Music) ArtistByMBID(ctx context.Context, mbid string) (domain.Artist, bool, error) {
	a, err := scanArtist(r.s.R().QueryRowContext(ctx, "SELECT "+artistCols+" FROM artists WHERE mbid = ?", mbid))
	ok, err := found(err)
	return a, ok, err
}

// ListArtists pages artists by ascending id after cursor; next is 0 on the last page.
func (r Music) ListArtists(ctx context.Context, cursor int64, limit int) ([]domain.Artist, int64, error) {
	out := []domain.Artist{}
	err := eachRow(ctx, r.s.R(), scanArtist, func(a domain.Artist) error { out = append(out, a); return nil },
		"SELECT "+artistCols+" FROM artists WHERE id > ? ORDER BY id LIMIT ?", cursor, limit)
	var next int64
	if len(out) == limit && limit > 0 {
		next = out[len(out)-1].ID
	}
	return out, next, err
}

// EachArtist streams every artist in id order.
func (r Music) EachArtist(ctx context.Context, fn func(domain.Artist) error) error {
	return eachRow(ctx, r.s.R(), scanArtist, fn, "SELECT "+artistCols+" FROM artists ORDER BY id")
}

// DeleteArtist removes an artist with its albums and tracks; twice is fine.
func (r Music) DeleteArtist(ctx context.Context, id int64) error {
	return r.s.exec(ctx, "DELETE FROM artists WHERE id = ?", id)
}

// Albums lists an artist's albums by release date, undated last.
func (r Music) Albums(ctx context.Context, artistID int64) ([]domain.Album, error) {
	out := []domain.Album{}
	err := eachRow(ctx, r.s.R(), scanAlbum, func(a domain.Album) error { out = append(out, a); return nil },
		"SELECT "+albumCols+" FROM albums WHERE artist_id = ? ORDER BY release_date IS NULL, release_date, id", artistID)
	return out, err
}

// Album returns an album by id.
func (r Music) Album(ctx context.Context, id int64) (domain.Album, bool, error) {
	a, err := scanAlbum(r.s.R().QueryRowContext(ctx, "SELECT "+albumCols+" FROM albums WHERE id = ?", id))
	ok, err := found(err)
	return a, ok, err
}

// SetAlbumsMonitored sets monitoring and the pin reason of albums; it
// returns how many changed.
func (r Music) SetAlbumsMonitored(ctx context.Context, ids []int64, monitored bool, pin domain.PinReason) (int, error) {
	n := 0
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range ids {
			res, err := tx.ExecContext(ctx, `UPDATE albums SET monitored = ?, pin_reason = ? WHERE id = ?
				AND (monitored != ? OR pin_reason != ?)`, monitored, string(pin), id, monitored, string(pin))
			if err != nil {
				return err
			}
			c, _ := res.RowsAffected()
			n += int(c)
		}
		return nil
	})
	return n, err
}

// SetEdition stores an album's selected edition and replaces its track
// list: tracks are matched by medium and number, so a track keeps its file
// link; tracks the new edition lacks are dropped unless they have a file.
func (r Music) SetEdition(ctx context.Context, a domain.Album, tracks []domain.Track) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE albums SET edition_mbid = ?, edition_locked = ?, medium_count = ?,
			track_count = ?, duration_sec = ? WHERE id = ?`, a.EditionMBID, a.EditionLocked, a.MediumCount, a.TrackCount,
			a.DurationSec, a.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE tracks SET number = -id WHERE album_id = ? AND file_id = 0", a.ID); err != nil {
			return err
		}
		for _, t := range tracks {
			if _, err := tx.ExecContext(ctx, `INSERT INTO tracks (album_id, recording_mbid, medium, medium_format, number, title,
				duration_sec) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (album_id, medium, number) DO UPDATE SET
				recording_mbid = excluded.recording_mbid, medium_format = excluded.medium_format, title = excluded.title,
				duration_sec = excluded.duration_sec`, a.ID, t.RecordingID, t.Medium, t.MediumFormat, t.Number, t.Title,
				t.DurationSec); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM tracks WHERE album_id = ? AND number < 0", a.ID)
		return err
	})
}

// Tracks lists an album's tracks by medium and number.
func (r Music) Tracks(ctx context.Context, albumID int64) ([]domain.Track, error) {
	out := []domain.Track{}
	err := eachRow(ctx, r.s.R(), scanTrack, func(t domain.Track) error { out = append(out, t); return nil },
		"SELECT "+trackCols+" FROM tracks WHERE album_id = ? ORDER BY medium, number", albumID)
	return out, err
}

// Track returns a track by id.
func (r Music) Track(ctx context.Context, id int64) (domain.Track, bool, error) {
	t, err := scanTrack(r.s.R().QueryRowContext(ctx, "SELECT "+trackCols+" FROM tracks WHERE id = ?", id))
	ok, err := found(err)
	return t, ok, err
}

// SetTrackFile links a track to a library file (0 = none).
func (r Music) SetTrackFile(ctx context.Context, trackID, fileID int64) error {
	return r.s.exec(ctx, "UPDATE tracks SET file_id = ? WHERE id = ?", fileID, trackID)
}

// PendingAvailability streams monitored albums whose item.available has not
// been published yet.
func (r Music) PendingAvailability(ctx context.Context, fn func(domain.Album) error) error {
	return eachRow(ctx, r.s.R(), scanAlbum, fn, "SELECT "+albumCols+
		" FROM albums WHERE available_published = 0 AND monitored = 1 ORDER BY id")
}

// MarkAvailable records item.available as published for albums.
func (r Music) MarkAvailable(ctx context.Context, ids []int64) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, "UPDATE albums SET available_published = 1 WHERE id = ?", id); err != nil {
				return err
			}
		}
		return nil
	})
}

// AlbumMBIDs lists the MusicBrainz id of every album in the library.
func (r Music) AlbumMBIDs(ctx context.Context) ([]string, error) {
	rows, err := r.s.R().QueryContext(ctx, "SELECT DISTINCT mbid FROM albums")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var mbid string
		if err := rows.Scan(&mbid); err != nil {
			return nil, err
		}
		out = append(out, mbid)
	}
	return out, rows.Err()
}
