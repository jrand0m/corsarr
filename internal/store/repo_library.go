package store

import (
	"context"
	"database/sql"

	"github.com/jrand0m/corsarr/internal/domain"
)

// Library stores library files and import records (writer: importer).
type Library struct{ s *Store }

// Library returns the library repository.
func (s *Store) Library() Library { return Library{s} }

// SaveFile upserts a library file by path and returns it with its id.
func (r Library) SaveFile(ctx context.Context, f domain.LibraryFile) (domain.LibraryFile, error) {
	err := r.s.Write(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `INSERT INTO library_files (item_kind, item_id, path, size, quality, format_score,
			release_group, revision, edition, added_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (path) DO UPDATE SET item_kind = excluded.item_kind, item_id = excluded.item_id, size = excluded.size,
			quality = excluded.quality, format_score = excluded.format_score, release_group = excluded.release_group,
			revision = excluded.revision, edition = excluded.edition RETURNING id`,
			string(f.Item.Kind), f.Item.ID, f.Path, f.Size, f.Quality, f.FormatScore, f.Group, f.Revision, f.Edition,
			ms(f.AddedAt)).Scan(&f.ID)
	})
	return f, err
}

const fileCols = "id, item_kind, item_id, path, size, quality, format_score, release_group, revision, edition, added_at"

func scanFile(sc scanner) (domain.LibraryFile, error) {
	var f domain.LibraryFile
	var kind string
	var added int64
	err := sc.Scan(&f.ID, &kind, &f.Item.ID, &f.Path, &f.Size, &f.Quality, &f.FormatScore, &f.Group, &f.Revision, &f.Edition, &added)
	f.Item.Kind, f.AddedAt = domain.MediaKind(kind), fromMS(added)
	return f, err
}

// File returns a library file by id.
func (r Library) File(ctx context.Context, id int64) (domain.LibraryFile, bool, error) {
	f, err := scanFile(r.s.R().QueryRowContext(ctx, "SELECT "+fileCols+" FROM library_files WHERE id = ?", id))
	ok, err := found(err)
	return f, ok, err
}

// FilesOf lists the library files of an item.
func (r Library) FilesOf(ctx context.Context, item domain.ItemRef) ([]domain.LibraryFile, error) {
	rows, err := r.s.R().QueryContext(ctx, "SELECT "+fileCols+" FROM library_files WHERE item_kind = ? AND item_id = ? ORDER BY id",
		string(item.Kind), item.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.LibraryFile
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// SetPath records that a library file moved; setting it twice is fine.
func (r Library) SetPath(ctx context.Context, id int64, path string) error {
	return r.s.exec(ctx, "UPDATE library_files SET path = ? WHERE id = ?", path, id)
}

// DeleteFile removes a library file record; deleting twice is fine.
func (r Library) DeleteFile(ctx context.Context, id int64) error {
	return r.s.exec(ctx, "DELETE FROM library_files WHERE id = ?", id)
}

// ImportRecord returns the destination of an already imported (download, file).
func (r Library) ImportRecord(ctx context.Context, downloadID int64, source string) (string, int64, bool, error) {
	var dest string
	var fileID int64
	ok, err := found(r.s.R().QueryRowContext(ctx, "SELECT dest_path, file_id FROM import_records WHERE download_id = ? AND source_path = ?",
		downloadID, source).Scan(&dest, &fileID))
	return dest, fileID, ok, err
}

// RecordImport remembers an imported (download, file); a repeat is a no-op.
func (r Library) RecordImport(ctx context.Context, downloadID int64, source, dest string, fileID int64) error {
	return r.s.exec(ctx, `INSERT OR IGNORE INTO import_records (download_id, source_path, dest_path, file_id, imported_at)
		VALUES (?, ?, ?, ?, ?)`, downloadID, source, dest, fileID, ms(now()))
}
