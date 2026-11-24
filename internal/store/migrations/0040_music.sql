-- 0040: music artists, albums (release groups) and the tracks of each
-- album's selected edition (single writer: the music module).
-- Times are unix milliseconds UTC; JSON lists as TEXT.

CREATE TABLE artists (
    id               INTEGER PRIMARY KEY,
    mbid             TEXT NOT NULL UNIQUE,
    name             TEXT NOT NULL,
    sort_name        TEXT NOT NULL DEFAULT '',
    disambiguation   TEXT NOT NULL DEFAULT '',
    artist_type      TEXT NOT NULL DEFAULT '',
    country          TEXT NOT NULL DEFAULT '',
    root_folder      TEXT NOT NULL,
    path             TEXT NOT NULL,
    profile_key      TEXT NOT NULL,
    metadata_profile TEXT NOT NULL,
    monitor_new      TEXT NOT NULL DEFAULT 'all' CHECK (monitor_new IN ('all', 'new', 'none')),
    monitored        INTEGER NOT NULL DEFAULT 1,
    tags             TEXT NOT NULL DEFAULT '[]',
    added_at         INTEGER NOT NULL,
    refreshed_at     INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE albums (
    id                  INTEGER PRIMARY KEY,
    artist_id           INTEGER NOT NULL REFERENCES artists (id) ON DELETE CASCADE,
    mbid                TEXT NOT NULL,
    title               TEXT NOT NULL,
    primary_type        TEXT NOT NULL DEFAULT 'album',
    secondary_types     TEXT NOT NULL DEFAULT '[]',
    release_date        INTEGER,
    monitored           INTEGER NOT NULL DEFAULT 0,
    pin_reason          TEXT NOT NULL DEFAULT '' CHECK (pin_reason IN ('', 'owner', 'list', 'import')),
    edition_mbid        TEXT NOT NULL DEFAULT '',
    edition_locked      INTEGER NOT NULL DEFAULT 0,
    medium_count        INTEGER NOT NULL DEFAULT 0,
    track_count         INTEGER NOT NULL DEFAULT 0,
    duration_sec        INTEGER NOT NULL DEFAULT 0,
    available_published INTEGER NOT NULL DEFAULT 0,
    UNIQUE (artist_id, mbid)
) STRICT;
CREATE INDEX albums_pending_available ON albums (available_published, monitored);
CREATE INDEX albums_release_date ON albums (release_date);

CREATE TABLE tracks (
    id             INTEGER PRIMARY KEY,
    album_id       INTEGER NOT NULL REFERENCES albums (id) ON DELETE CASCADE,
    recording_mbid TEXT NOT NULL DEFAULT '',
    medium         INTEGER NOT NULL,
    medium_format  TEXT NOT NULL DEFAULT '',
    number         INTEGER NOT NULL,
    title          TEXT NOT NULL DEFAULT '',
    duration_sec   INTEGER NOT NULL DEFAULT 0,
    file_id        INTEGER NOT NULL DEFAULT 0,
    UNIQUE (album_id, medium, number)
) STRICT;
