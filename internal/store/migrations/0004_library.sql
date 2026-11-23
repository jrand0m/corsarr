-- 0004: library files and import records (owner: importer).

CREATE TABLE library_files (
    id            INTEGER PRIMARY KEY,
    item_kind     TEXT NOT NULL,
    item_id       INTEGER NOT NULL,
    path          TEXT NOT NULL UNIQUE,
    size          INTEGER NOT NULL DEFAULT 0,
    quality       TEXT NOT NULL DEFAULT '',
    format_score  INTEGER NOT NULL DEFAULT 0,
    release_group TEXT NOT NULL DEFAULT '',
    revision      INTEGER NOT NULL DEFAULT 1,
    edition       TEXT NOT NULL DEFAULT '',
    added_at      INTEGER NOT NULL
) STRICT;
CREATE INDEX library_files_item ON library_files (item_kind, item_id);

-- One row per (download, source file): imports are idempotent.
CREATE TABLE import_records (
    download_id INTEGER NOT NULL,
    source_path TEXT NOT NULL,
    dest_path   TEXT NOT NULL,
    file_id     INTEGER NOT NULL DEFAULT 0,
    imported_at INTEGER NOT NULL,
    PRIMARY KEY (download_id, source_path)
) STRICT;
