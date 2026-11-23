-- 0020: TV series, seasons and episodes (single writer: the tv module).
-- Times are unix milliseconds UTC; JSON lists as TEXT.

CREATE TABLE series (
    id             INTEGER PRIMARY KEY,
    tmdb_id        INTEGER NOT NULL UNIQUE,
    tvdb_id        INTEGER NOT NULL DEFAULT 0,
    imdb_id        TEXT NOT NULL DEFAULT '',
    title          TEXT NOT NULL,
    sort_title     TEXT NOT NULL DEFAULT '',
    year           INTEGER NOT NULL DEFAULT 0,
    status         TEXT NOT NULL DEFAULT '',
    network        TEXT NOT NULL DEFAULT '',
    overview       TEXT NOT NULL DEFAULT '',
    genres         TEXT NOT NULL DEFAULT '[]',
    runtime_min    INTEGER NOT NULL DEFAULT 0,
    alt_titles     TEXT NOT NULL DEFAULT '[]',
    poster_url     TEXT NOT NULL DEFAULT '',
    root_folder    TEXT NOT NULL,
    path           TEXT NOT NULL,
    profile_key    TEXT NOT NULL,
    tags           TEXT NOT NULL DEFAULT '[]',
    series_type    TEXT NOT NULL DEFAULT 'standard' CHECK (series_type IN ('standard', 'anime', 'daily')),
    season_folders INTEGER NOT NULL DEFAULT 1,
    monitor_new    TEXT NOT NULL DEFAULT 'all' CHECK (monitor_new IN ('all', 'none')),
    monitored      INTEGER NOT NULL DEFAULT 1,
    added_at       INTEGER NOT NULL,
    refreshed_at   INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE seasons (
    series_id INTEGER NOT NULL REFERENCES series (id) ON DELETE CASCADE,
    number    INTEGER NOT NULL,
    monitored INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (series_id, number)
) STRICT;

CREATE TABLE episodes (
    id                  INTEGER PRIMARY KEY,
    series_id           INTEGER NOT NULL REFERENCES series (id) ON DELETE CASCADE,
    season              INTEGER NOT NULL,
    number              INTEGER NOT NULL,
    absolute            INTEGER NOT NULL DEFAULT 0,
    title               TEXT NOT NULL DEFAULT '',
    air_at              INTEGER,
    runtime_min         INTEGER NOT NULL DEFAULT 0,
    monitored           INTEGER NOT NULL DEFAULT 0,
    file_id             INTEGER NOT NULL DEFAULT 0,
    in_metadata         INTEGER NOT NULL DEFAULT 1,
    available_published INTEGER NOT NULL DEFAULT 0,
    UNIQUE (series_id, season, number)
) STRICT;
CREATE INDEX episodes_pending_available ON episodes (available_published, monitored);
CREATE INDEX episodes_air_at ON episodes (air_at);
