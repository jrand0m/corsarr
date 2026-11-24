package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jrand0m/corsarr/internal/domain"
)

// ── migration runs, provenance and adoption ──────────────────────────────────

// MigrationRun is one plan or apply of a migration.
type MigrationRun struct {
	ID         int64          `json:"id"`
	Mode       string         `json:"mode"` // plan | apply
	Sources    []string       `json:"sources"`
	Status     string         `json:"status"` // running | done | failed
	Counts     map[string]int `json:"counts"`
	Error      string         `json:"error,omitempty"`
	StartedAt  time.Time      `json:"startedAt"`
	FinishedAt *time.Time     `json:"finishedAt,omitempty"`
}

// ProvenanceKey names one source object.
type ProvenanceKey struct {
	App, Instance, ObjectType, ObjectID string
}

// Provenance links a source object to the corsarr object it became.
type Provenance struct {
	Key        ProvenanceKey
	TargetKind string
	TargetID   int64
	SourceHash string
	TargetHash string
	ImportedAt time.Time
}

// Migration stores runs and provenance and adopts library objects; each
// adopt is one transaction, so an interrupted run stops at a parent boundary.
type Migration struct{ s *Store }

// Migration returns the migration repository.
func (s *Store) Migration() Migration { return Migration{s} }

// StartRun opens a run and returns its id.
func (r Migration) StartRun(ctx context.Context, mode string, sources []string, at time.Time) (int64, error) {
	var id int64
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `INSERT INTO migration_runs (mode, sources, status, started_at) VALUES (?, ?, 'running', ?)
			RETURNING id`, mode, jsonText(sources), ms(at)).Scan(&id)
	})
	return id, err
}

// FinishRun closes a run with its counts, report and error text.
func (r Migration) FinishRun(ctx context.Context, id int64, counts map[string]int, report []byte, errText string, at time.Time) error {
	status := "done"
	if errText != "" {
		status = "failed"
	}
	return r.s.exec(ctx, `UPDATE migration_runs SET status = ?, counts = ?, report = ?, error = ?, finished_at = ? WHERE id = ?`,
		status, jsonText(counts), string(report), errText, ms(at), id)
}

const runCols = "id, mode, sources, status, counts, error, started_at, finished_at"

func scanRun(sc scanner) (MigrationRun, error) {
	var run MigrationRun
	var sources, counts string
	var started int64
	var finished sql.NullInt64
	err := sc.Scan(&run.ID, &run.Mode, &sources, &run.Status, &counts, &run.Error, &started, &finished)
	run.Sources = jsonList(sources)
	_ = json.Unmarshal([]byte(counts), &run.Counts)
	run.StartedAt, run.FinishedAt = fromMS(started), ptrMS(finished)
	return run, err
}

// Runs lists the newest runs first.
func (r Migration) Runs(ctx context.Context, limit int) ([]MigrationRun, error) {
	var out []MigrationRun
	err := eachRow(ctx, r.s.R(), scanRun, func(run MigrationRun) error { out = append(out, run); return nil },
		"SELECT "+runCols+" FROM migration_runs ORDER BY id DESC LIMIT ?", limit)
	return out, err
}

// Report returns a run's stored report.
func (r Migration) Report(ctx context.Context, id int64) ([]byte, bool, error) {
	var report string
	ok, err := found(r.s.R().QueryRowContext(ctx, "SELECT report FROM migration_runs WHERE id = ?", id).Scan(&report))
	return []byte(report), ok, err
}

// Provenance returns every provenance row of one source instance.
func (r Migration) Provenance(ctx context.Context, app, instance string) (map[ProvenanceKey]Provenance, error) {
	out := map[ProvenanceKey]Provenance{}
	scan := func(sc scanner) (Provenance, error) {
		var p Provenance
		var at int64
		err := sc.Scan(&p.Key.App, &p.Key.Instance, &p.Key.ObjectType, &p.Key.ObjectID, &p.TargetKind, &p.TargetID,
			&p.SourceHash, &p.TargetHash, &at)
		p.ImportedAt = fromMS(at)
		return p, err
	}
	err := eachRow(ctx, r.s.R(), scan, func(p Provenance) error { out[p.Key] = p; return nil },
		`SELECT source_app, instance, object_type, object_id, target_kind, target_id, source_hash, target_hash, imported_at
		FROM migration_provenance WHERE source_app = ? AND instance = ?`, app, instance)
	return out, err
}

// Record upserts one provenance row (blocklist and history imports).
func (r Migration) Record(ctx context.Context, p Provenance) error {
	return r.s.Write(ctx, func(tx *sql.Tx) error { return putProvenance(ctx, tx, p) })
}

func putProvenance(ctx context.Context, tx *sql.Tx, p Provenance) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO migration_provenance (source_app, instance, object_type, object_id, target_kind,
		target_id, source_hash, target_hash, imported_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (source_app, instance, object_type, object_id) DO UPDATE SET target_kind = excluded.target_kind,
		target_id = excluded.target_id, source_hash = excluded.source_hash, target_hash = excluded.target_hash,
		imported_at = excluded.imported_at`, p.Key.App, p.Key.Instance, p.Key.ObjectType, p.Key.ObjectID, p.TargetKind,
		p.TargetID, p.SourceHash, p.TargetHash, ms(p.ImportedAt))
	return err
}

// LibraryIndex is what the library already holds, keyed for joining.
type LibraryIndex struct {
	SeriesByTMDB map[int64]int64  // tmdb id → series id
	MovieByTMDB  map[int64]int64  // tmdb id → movie id
	FilePaths    map[string]int64 // linked file path → file id
}

// Index reads the join keys of the library.
func (r Migration) Index(ctx context.Context) (LibraryIndex, error) {
	ix := LibraryIndex{SeriesByTMDB: map[int64]int64{}, MovieByTMDB: map[int64]int64{}, FilePaths: map[string]int64{}}
	pair := func(sc scanner) ([2]int64, error) { var p [2]int64; return p, sc.Scan(&p[0], &p[1]) }
	err := errors.Join(
		eachRow(ctx, r.s.R(), pair, func(p [2]int64) error { ix.SeriesByTMDB[p[0]] = p[1]; return nil }, "SELECT tmdb_id, id FROM series"),
		eachRow(ctx, r.s.R(), pair, func(p [2]int64) error { ix.MovieByTMDB[p[0]] = p[1]; return nil }, "SELECT tmdb_id, id FROM movies"),
		eachRow(ctx, r.s.R(), func(sc scanner) (domain.LibraryFile, error) {
			var f domain.LibraryFile
			return f, sc.Scan(&f.Path, &f.ID)
		}, func(f domain.LibraryFile) error { ix.FilePaths[f.Path] = f.ID; return nil }, "SELECT path, id FROM library_files"),
	)
	return ix, err
}

// EpisodeKey is an episode's place in its series.
type EpisodeKey struct{ Season, Number int }

// EpisodeFile is one adopted file and the episodes it holds.
type EpisodeFile struct {
	File     domain.LibraryFile
	Episodes []EpisodeKey
}

// AdoptSeries creates the series (or updates the one prov.TargetID names),
// upserts its seasons and episodes, links the adopted files where they are
// and records provenance, all in one transaction. It returns the series id.
func (r Migration) AdoptSeries(ctx context.Context, s domain.Series, seasons []domain.Season, eps []domain.Episode,
	files []EpisodeFile, prov Provenance) (int64, error) {
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		id, err := writeSeries(ctx, tx, s, prov.TargetID)
		if err != nil {
			return err
		}
		if err := syncChildren(ctx, tx, id, seasons, eps, nil); err != nil {
			return err
		}
		for _, f := range files {
			if err := linkEpisodeFile(ctx, tx, id, f); err != nil {
				return err
			}
		}
		prov.TargetKind, prov.TargetID = string(domain.KindTV), id
		return putProvenance(ctx, tx, prov)
	})
	return prov.TargetID, err
}

func writeSeries(ctx context.Context, tx *sql.Tx, s domain.Series, id int64) (int64, error) {
	if id > 0 {
		_, err := tx.ExecContext(ctx, `UPDATE series SET profile_key = ?, tags = ?, series_type = ?, season_folders = ?,
			monitor_new = ?, monitored = ?, root_folder = ?, path = ? WHERE id = ?`, s.ProfileKey, jsonText(s.Tags), string(s.Type),
			s.SeasonFolders, string(s.MonitorNew), s.Monitored, s.RootFolder, s.Path, id)
		return id, err
	}
	err := tx.QueryRowContext(ctx, `INSERT INTO series (tmdb_id, tvdb_id, imdb_id, title, sort_title, year, status, network,
		runtime_min, root_folder, path, profile_key, tags, series_type, season_folders, monitor_new, monitored, added_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		s.TMDBID, s.TVDBID, s.IMDBID, s.Title, s.SortTitle, s.Year, s.Status, s.Network, s.RuntimeMin, s.RootFolder, s.Path,
		s.ProfileKey, jsonText(s.Tags), string(s.Type), s.SeasonFolders, string(s.MonitorNew), s.Monitored, ms(s.AddedAt)).Scan(&id)
	return id, err
}

func linkEpisodeFile(ctx context.Context, tx *sql.Tx, seriesID int64, f EpisodeFile) error {
	if len(f.Episodes) == 0 {
		return nil
	}
	var first int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM episodes WHERE series_id = ? AND season = ? AND number = ?",
		seriesID, f.Episodes[0].Season, f.Episodes[0].Number).Scan(&first); err != nil {
		return err
	}
	f.File.Item = domain.ItemRef{Kind: domain.KindTV, ID: first}
	fileID, err := upsertFile(ctx, tx, f.File)
	if err != nil {
		return err
	}
	for _, k := range f.Episodes {
		if _, err := tx.ExecContext(ctx, "UPDATE episodes SET file_id = ? WHERE series_id = ? AND season = ? AND number = ?",
			fileID, seriesID, k.Season, k.Number); err != nil {
			return err
		}
	}
	return nil
}

func upsertFile(ctx context.Context, tx *sql.Tx, f domain.LibraryFile) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `INSERT INTO library_files (item_kind, item_id, path, size, quality, format_score,
		release_group, revision, edition, added_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (path) DO UPDATE SET size = excluded.size RETURNING id`,
		string(f.Item.Kind), f.Item.ID, f.Path, f.Size, f.Quality, f.FormatScore, f.Group, f.Revision, f.Edition, ms(f.AddedAt)).Scan(&id)
	return id, err
}

// AdoptMovie creates the movie (or updates the one prov.TargetID names),
// links its adopted file where it is and records provenance in one
// transaction. It returns the movie id.
func (r Migration) AdoptMovie(ctx context.Context, m domain.Movie, file *domain.LibraryFile, prov Provenance) (int64, error) {
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		id, err := writeMovie(ctx, tx, m, prov.TargetID)
		if err != nil {
			return err
		}
		if file != nil {
			file.Item = domain.ItemRef{Kind: domain.KindMovie, ID: id}
			fileID, err := upsertFile(ctx, tx, *file)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "UPDATE movies SET file_id = ? WHERE id = ?", fileID, id); err != nil {
				return err
			}
		}
		prov.TargetKind, prov.TargetID = string(domain.KindMovie), id
		return putProvenance(ctx, tx, prov)
	})
	return prov.TargetID, err
}

func writeMovie(ctx context.Context, tx *sql.Tx, m domain.Movie, id int64) (int64, error) {
	if id > 0 {
		_, err := tx.ExecContext(ctx, `UPDATE movies SET monitored = ?, min_availability = ?, profile_key = ?, root_folder = ?,
			path = ? WHERE id = ?`, m.Monitored, string(m.MinAvailability), m.ProfileKey, m.RootFolder, m.Path, id)
		return id, err
	}
	err := tx.QueryRowContext(ctx, `INSERT INTO movies (tmdb_id, imdb_id, title, original_title, year, runtime_min, in_cinemas,
		digital, physical, monitored, min_availability, profile_key, root_folder, path, added_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		m.TMDBID, m.IMDBID, m.Title, m.OriginalTitle, m.Year, m.RuntimeMin, nullMS(m.InCinemas), nullMS(m.Digital),
		nullMS(m.Physical), m.Monitored, string(m.MinAvailability), m.ProfileKey, m.RootFolder, m.Path, ms(m.AddedAt)).Scan(&id)
	return id, err
}

// EpisodeIDs maps episode places of a series (by TMDB id) to episode ids.
func (r Migration) EpisodeIDs(ctx context.Context, tmdbID int64) (map[EpisodeKey]int64, error) {
	out := map[EpisodeKey]int64{}
	scan := func(sc scanner) ([3]int64, error) { var v [3]int64; return v, sc.Scan(&v[0], &v[1], &v[2]) }
	err := eachRow(ctx, r.s.R(), scan, func(v [3]int64) error {
		out[EpisodeKey{Season: int(v[0]), Number: int(v[1])}] = v[2]
		return nil
	}, `SELECT e.season, e.number, e.id FROM episodes e JOIN series s ON s.id = e.series_id WHERE s.tmdb_id = ?`, tmdbID)
	return out, err
}

// ── mode overrides ───────────────────────────────────────────────────────────

// Override scopes.
const (
	ScopeKind = "kind"
	ScopeRule = "rule"
)

// ModeOverride is a mode chosen outside the config file.
type ModeOverride struct {
	Scope string    `json:"scope"`
	Name  string    `json:"name"` // media kind, rule id or "*" for every rule
	Mode  string    `json:"mode"`
	SetAt time.Time `json:"setAt"`
}

// ModeOverrides stores mode overrides (writers: migration apply and the cutover API).
type ModeOverrides struct{ s *Store }

// ModeOverrides returns the override repository.
func (s *Store) ModeOverrides() ModeOverrides { return ModeOverrides{s} }

// List returns every override.
func (r ModeOverrides) List(ctx context.Context) ([]ModeOverride, error) {
	var out []ModeOverride
	scan := func(sc scanner) (ModeOverride, error) {
		var o ModeOverride
		var at int64
		err := sc.Scan(&o.Scope, &o.Name, &o.Mode, &at)
		o.SetAt = fromMS(at)
		return o, err
	}
	err := eachRow(ctx, r.s.R(), scan, func(o ModeOverride) error { out = append(out, o); return nil },
		"SELECT scope, name, mode, set_at FROM mode_overrides ORDER BY scope, name")
	return out, err
}

// Set upserts an override.
func (r ModeOverrides) Set(ctx context.Context, o ModeOverride) error {
	return r.s.exec(ctx, `INSERT INTO mode_overrides (scope, name, mode, set_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (scope, name) DO UPDATE SET mode = excluded.mode, set_at = excluded.set_at`, o.Scope, o.Name, o.Mode, ms(o.SetAt))
}
