-- 0060: migration from an existing installation: runs with their reports,
-- provenance of every adopted object (re-runs converge) and mode overrides
-- that a migration or the cutover panel sets when the config file is silent.
-- Times are unix milliseconds UTC.

CREATE TABLE migration_runs (
    id          INTEGER PRIMARY KEY,
    mode        TEXT NOT NULL CHECK (mode IN ('plan', 'apply')),
    sources     TEXT NOT NULL DEFAULT '[]',
    status      TEXT NOT NULL CHECK (status IN ('running', 'done', 'failed')),
    counts      TEXT NOT NULL DEFAULT '{}',
    report      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    started_at  INTEGER NOT NULL,
    finished_at INTEGER
) STRICT;

-- One row per adopted source object. source_hash is the source content at
-- the last apply, target_hash the corsarr object as that apply wrote it: a
-- later difference tells "source changed" from "edited in corsarr".
CREATE TABLE migration_provenance (
    source_app  TEXT NOT NULL,
    instance    TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_id   TEXT NOT NULL,
    target_kind TEXT NOT NULL,
    target_id   INTEGER NOT NULL,
    source_hash TEXT NOT NULL,
    target_hash TEXT NOT NULL,
    imported_at INTEGER NOT NULL,
    PRIMARY KEY (source_app, instance, object_type, object_id)
) STRICT;

-- Modes chosen outside the config file. A mode the config file sets wins.
CREATE TABLE mode_overrides (
    scope  TEXT NOT NULL CHECK (scope IN ('kind', 'rule')),
    name   TEXT NOT NULL,
    mode   TEXT NOT NULL,
    set_at INTEGER NOT NULL,
    PRIMARY KEY (scope, name)
) STRICT;
