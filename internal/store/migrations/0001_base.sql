-- 0001: kernel base tables. Times are unix milliseconds UTC.

CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE TABLE admin_user (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    username      TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
) STRICT;

CREATE TABLE sessions (
    token_hash   TEXT PRIMARY KEY,
    created_at   INTEGER NOT NULL,
    expires_at   INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL
) STRICT;
CREATE INDEX sessions_expires ON sessions (expires_at);

CREATE TABLE api_key (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    key_sealed TEXT NOT NULL,
    created_at INTEGER NOT NULL
) STRICT;

CREATE TABLE history (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id    TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL,
    occurred_at INTEGER NOT NULL,
    actor       TEXT NOT NULL,
    media_kind  TEXT NOT NULL DEFAULT '',
    imported    INTEGER NOT NULL DEFAULT 0,
    envelope    TEXT NOT NULL
) STRICT;
CREATE INDEX history_occurred ON history (occurred_at);
CREATE INDEX history_type ON history (type, id);

CREATE TABLE jobs (
    name         TEXT PRIMARY KEY,
    enabled      INTEGER NOT NULL DEFAULT 1,
    interval_ms  INTEGER NOT NULL,
    last_start   INTEGER,
    last_finish  INTEGER,
    last_result  TEXT NOT NULL DEFAULT '',
    last_error   TEXT NOT NULL DEFAULT '',
    failures     INTEGER NOT NULL DEFAULT 0,
    next_run     INTEGER
) STRICT;

CREATE TABLE integration_health (
    type                 TEXT NOT NULL,
    key                  TEXT NOT NULL,
    status               TEXT NOT NULL,
    since                INTEGER NOT NULL,
    last_ok_at           INTEGER,
    last_error_class     TEXT NOT NULL DEFAULT '',
    last_error_message   TEXT NOT NULL DEFAULT '',
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    next_retry_at        INTEGER,
    fix_hint             TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (type, key)
) STRICT;

CREATE TABLE root_folders (
    id           INTEGER PRIMARY KEY,
    path         TEXT NOT NULL UNIQUE,
    kind         TEXT NOT NULL CHECK (kind IN ('tv', 'movie', 'music', 'book')),
    free_space   INTEGER NOT NULL DEFAULT 0,
    accessible   INTEGER NOT NULL DEFAULT 0,
    managed      INTEGER NOT NULL DEFAULT 0,
    last_checked INTEGER
) STRICT;
