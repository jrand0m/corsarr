-- 0002: movies. Times are unix milliseconds UTC; JSON lists as TEXT.

CREATE TABLE movies (
    id                  INTEGER PRIMARY KEY,
    tmdb_id             INTEGER NOT NULL UNIQUE,
    imdb_id             TEXT NOT NULL DEFAULT '',
    title               TEXT NOT NULL,
    original_title      TEXT NOT NULL DEFAULT '',
    year                INTEGER NOT NULL DEFAULT 0,
    alt_titles          TEXT NOT NULL DEFAULT '[]',
    runtime_min         INTEGER NOT NULL DEFAULT 0,
    in_cinemas          INTEGER,
    digital             INTEGER,
    physical            INTEGER,
    monitored           INTEGER NOT NULL DEFAULT 1,
    min_availability    TEXT NOT NULL DEFAULT 'released' CHECK (min_availability IN ('announced', 'inCinemas', 'released')),
    profile_key         TEXT NOT NULL,
    root_folder         TEXT NOT NULL,
    path                TEXT NOT NULL,
    file_id             INTEGER NOT NULL DEFAULT 0,
    available_published INTEGER NOT NULL DEFAULT 0,
    added_at            INTEGER NOT NULL
) STRICT;
CREATE INDEX movies_pending_available ON movies (available_published, monitored);
