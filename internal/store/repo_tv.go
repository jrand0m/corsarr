package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// TV stores series, seasons and episodes (single writer: the tv module).
type TV struct{ s *Store }

// TV returns the TV repository.
func (s *Store) TV() TV { return TV{s} }

const seriesCols = `id, tmdb_id, tvdb_id, imdb_id, title, sort_title, year, status, network, overview, genres,
	runtime_min, alt_titles, poster_url, root_folder, path, profile_key, tags, series_type, season_folders, monitor_new,
	monitored, added_at`

func scanSeries(sc scanner) (domain.Series, error) {
	var s domain.Series
	var genres, alt, tags, typ, monNew string
	var added int64
	err := sc.Scan(&s.ID, &s.TMDBID, &s.TVDBID, &s.IMDBID, &s.Title, &s.SortTitle, &s.Year, &s.Status, &s.Network,
		&s.Overview, &genres, &s.RuntimeMin, &alt, &s.PosterURL, &s.RootFolder, &s.Path, &s.ProfileKey, &tags, &typ,
		&s.SeasonFolders, &monNew, &s.Monitored, &added)
	s.Type, s.MonitorNew, s.AddedAt = domain.SeriesType(typ), domain.MonitorNew(monNew), fromMS(added)
	s.Genres, s.AltTitles, s.Tags = jsonList(genres), jsonList(alt), jsonList(tags)
	return s, err
}

// jsonList reads a JSON list written by this repository; never nil.
func jsonList(text string) []string {
	out := []string{}
	_ = json.Unmarshal([]byte(text), &out)
	return out
}

const episodeCols = `id, series_id, season, number, absolute, title, air_at, runtime_min, monitored, file_id, in_metadata`

func scanEpisode(sc scanner) (domain.Episode, error) {
	var e domain.Episode
	var air sql.NullInt64
	err := sc.Scan(&e.ID, &e.SeriesID, &e.Season, &e.Number, &e.Absolute, &e.Title, &air, &e.RuntimeMin, &e.Monitored,
		&e.FileID, &e.InMetadata)
	e.AirAt = ptrMS(air)
	return e, err
}

// AddSeries inserts a series with its seasons and episodes in one
// transaction, keyed by TMDB id; adding the same id again returns the
// existing series with created=false.
func (r TV) AddSeries(ctx context.Context, s domain.Series, seasons []domain.Season, eps []domain.Episode) (domain.Series, bool, error) {
	var created bool
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO series (tmdb_id, tvdb_id, imdb_id, title, sort_title, year, status,
			network, overview, genres, runtime_min, alt_titles, poster_url, root_folder, path, profile_key, tags,
			series_type, season_folders, monitor_new, monitored, added_at, refreshed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (tmdb_id) DO NOTHING`,
			s.TMDBID, s.TVDBID, s.IMDBID, s.Title, s.SortTitle, s.Year, s.Status, s.Network, s.Overview, jsonText(s.Genres),
			s.RuntimeMin, jsonText(s.AltTitles), s.PosterURL, s.RootFolder, s.Path, s.ProfileKey, jsonText(s.Tags),
			string(s.Type), s.SeasonFolders, string(s.MonitorNew), s.Monitored, ms(s.AddedAt), ms(s.AddedAt))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return nil
		}
		created = true
		id, _ := res.LastInsertId()
		return syncChildren(ctx, tx, id, seasons, eps, nil)
	})
	if err != nil {
		return domain.Series{}, false, err
	}
	out, err := scanSeries(r.s.R().QueryRowContext(ctx, "SELECT "+seriesCols+" FROM series WHERE tmdb_id = ?", s.TMDBID))
	return out, created, err
}

// syncChildren upserts seasons and episodes (by season and number) and
// deletes the listed episodes. An episode's file link and availability
// flag are owned by other writes and never touched here.
func syncChildren(ctx context.Context, tx *sql.Tx, seriesID int64, seasons []domain.Season, eps []domain.Episode, drop []int64) error {
	for _, se := range seasons {
		if _, err := tx.ExecContext(ctx, `INSERT INTO seasons (series_id, number, monitored) VALUES (?, ?, ?)
			ON CONFLICT (series_id, number) DO UPDATE SET monitored = excluded.monitored`, seriesID, se.Number, se.Monitored); err != nil {
			return err
		}
	}
	for _, e := range eps {
		if _, err := tx.ExecContext(ctx, `INSERT INTO episodes (series_id, season, number, absolute, title, air_at,
			runtime_min, monitored, in_metadata) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (series_id, season, number) DO UPDATE SET absolute = excluded.absolute, title = excluded.title,
			air_at = excluded.air_at, runtime_min = excluded.runtime_min, monitored = excluded.monitored,
			in_metadata = excluded.in_metadata`,
			seriesID, e.Season, e.Number, e.Absolute, e.Title, nullMS(e.AirAt), e.RuntimeMin, e.Monitored, e.InMetadata); err != nil {
			return err
		}
	}
	for _, id := range drop {
		if _, err := tx.ExecContext(ctx, "DELETE FROM episodes WHERE id = ? AND series_id = ? AND file_id = 0", id, seriesID); err != nil {
			return err
		}
	}
	return nil
}

// Sync writes a series' metadata and settings, upserts its seasons and
// episodes and drops the listed file-less episodes, all in one transaction.
func (r TV) Sync(ctx context.Context, s domain.Series, seasons []domain.Season, eps []domain.Episode, drop []int64) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE series SET tvdb_id = ?, imdb_id = ?, title = ?, sort_title = ?, year = ?,
			status = ?, network = ?, overview = ?, genres = ?, runtime_min = ?, alt_titles = ?, poster_url = ?,
			root_folder = ?, path = ?, profile_key = ?, tags = ?, series_type = ?, season_folders = ?, monitor_new = ?,
			monitored = ? WHERE id = ?`, s.TVDBID, s.IMDBID, s.Title, s.SortTitle, s.Year, s.Status, s.Network, s.Overview,
			jsonText(s.Genres), s.RuntimeMin, jsonText(s.AltTitles), s.PosterURL, s.RootFolder, s.Path, s.ProfileKey,
			jsonText(s.Tags), string(s.Type), s.SeasonFolders, string(s.MonitorNew), s.Monitored, s.ID); err != nil {
			return err
		}
		return syncChildren(ctx, tx, s.ID, seasons, eps, drop)
	})
}

// MarkRefreshed records a metadata refresh.
func (r TV) MarkRefreshed(ctx context.Context, id int64, at time.Time) error {
	return r.s.exec(ctx, "UPDATE series SET refreshed_at = ? WHERE id = ?", ms(at), id)
}

// RefreshedAt is when the series' metadata was last refreshed.
func (r TV) RefreshedAt(ctx context.Context, id int64) (time.Time, error) {
	var at int64
	err := r.s.R().QueryRowContext(ctx, "SELECT refreshed_at FROM series WHERE id = ?", id).Scan(&at)
	return fromMS(at), err
}

// Series returns a series by id.
func (r TV) Series(ctx context.Context, id int64) (domain.Series, bool, error) {
	s, err := scanSeries(r.s.R().QueryRowContext(ctx, "SELECT "+seriesCols+" FROM series WHERE id = ?", id))
	ok, err := found(err)
	return s, ok, err
}

// SeriesByTMDB returns the series with a TMDB id.
func (r TV) SeriesByTMDB(ctx context.Context, tmdbID int64) (domain.Series, bool, error) {
	s, err := scanSeries(r.s.R().QueryRowContext(ctx, "SELECT "+seriesCols+" FROM series WHERE tmdb_id = ?", tmdbID))
	ok, err := found(err)
	return s, ok, err
}

// ListSeries pages series by ascending id after cursor; next is 0 on the last page.
func (r TV) ListSeries(ctx context.Context, cursor int64, limit int) ([]domain.Series, int64, error) {
	out := []domain.Series{}
	err := eachRow(ctx, r.s.R(), scanSeries, func(s domain.Series) error { out = append(out, s); return nil },
		"SELECT "+seriesCols+" FROM series WHERE id > ? ORDER BY id LIMIT ?", cursor, limit)
	var next int64
	if len(out) == limit && limit > 0 {
		next = out[len(out)-1].ID
	}
	return out, next, err
}

// EachSeries streams every series in id order.
func (r TV) EachSeries(ctx context.Context, fn func(domain.Series) error) error {
	return eachRow(ctx, r.s.R(), scanSeries, fn, "SELECT "+seriesCols+" FROM series ORDER BY id")
}

// DeleteSeries removes a series with its seasons and episodes; twice is fine.
func (r TV) DeleteSeries(ctx context.Context, id int64) error {
	return r.s.exec(ctx, "DELETE FROM series WHERE id = ?", id)
}

// Seasons lists a series' seasons by number.
func (r TV) Seasons(ctx context.Context, seriesID int64) ([]domain.Season, error) {
	out := []domain.Season{}
	err := eachRow(ctx, r.s.R(), func(sc scanner) (domain.Season, error) {
		var se domain.Season
		err := sc.Scan(&se.SeriesID, &se.Number, &se.Monitored)
		return se, err
	}, func(se domain.Season) error { out = append(out, se); return nil },
		"SELECT series_id, number, monitored FROM seasons WHERE series_id = ? ORDER BY number", seriesID)
	return out, err
}

// Episodes lists a series' episodes by season and number.
func (r TV) Episodes(ctx context.Context, seriesID int64) ([]domain.Episode, error) {
	out := []domain.Episode{}
	err := eachRow(ctx, r.s.R(), scanEpisode, func(e domain.Episode) error { out = append(out, e); return nil },
		"SELECT "+episodeCols+" FROM episodes WHERE series_id = ? ORDER BY season, number", seriesID)
	return out, err
}

// Episode returns an episode by id.
func (r TV) Episode(ctx context.Context, id int64) (domain.Episode, bool, error) {
	e, err := scanEpisode(r.s.R().QueryRowContext(ctx, "SELECT "+episodeCols+" FROM episodes WHERE id = ?", id))
	ok, err := found(err)
	return e, ok, err
}

// EachEpisode streams every episode in id order.
func (r TV) EachEpisode(ctx context.Context, fn func(domain.Episode) error) error {
	return eachRow(ctx, r.s.R(), scanEpisode, fn, "SELECT "+episodeCols+" FROM episodes ORDER BY id")
}

// AiringBetween streams episodes whose air time is in [from, to).
func (r TV) AiringBetween(ctx context.Context, from, to time.Time, fn func(domain.Episode) error) error {
	return eachRow(ctx, r.s.R(), scanEpisode, fn, "SELECT "+episodeCols+
		" FROM episodes WHERE air_at >= ? AND air_at < ? ORDER BY air_at, id", ms(from), ms(to))
}

// PendingAvailability streams monitored, dated episodes whose item.available
// has not been published yet.
func (r TV) PendingAvailability(ctx context.Context, fn func(domain.Episode) error) error {
	return eachRow(ctx, r.s.R(), scanEpisode, fn, "SELECT "+episodeCols+
		" FROM episodes WHERE available_published = 0 AND monitored = 1 AND air_at IS NOT NULL ORDER BY id")
}

// SetAvailablePublished records whether item.available was published.
func (r TV) SetAvailablePublished(ctx context.Context, id int64, published bool) error {
	return r.s.exec(ctx, "UPDATE episodes SET available_published = ? WHERE id = ?", published, id)
}

// MarkAvailable records item.available as published for many episodes in
// one transaction (episodes already aired when their series is added).
func (r TV) MarkAvailable(ctx context.Context, ids []int64) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, "UPDATE episodes SET available_published = 1 WHERE id = ?", id); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetMonitored sets the monitored flag of episodes of one series; it returns
// how many changed.
func (r TV) SetMonitored(ctx context.Context, ids []int64, monitored bool) (int, error) {
	n := 0
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range ids {
			res, err := tx.ExecContext(ctx, "UPDATE episodes SET monitored = ? WHERE id = ? AND monitored != ?", monitored, id, monitored)
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

// SetFile links episodes to a library file (0 = none).
func (r TV) SetFile(ctx context.Context, ids []int64, fileID int64) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, "UPDATE episodes SET file_id = ? WHERE id = ?", fileID, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// eachRow runs a query and hands each scanned row to fn.
func eachRow[T any](ctx context.Context, db *sql.DB, scan func(scanner) (T, error), fn func(T) error, q string, args ...any) error {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return err
		}
		if err := fn(v); err != nil {
			return err
		}
	}
	return rows.Err()
}
