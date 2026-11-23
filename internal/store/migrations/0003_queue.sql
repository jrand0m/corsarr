-- 0003: grabs, downloads and their item links (single writer: queue).

CREATE TABLE grabs (
    id                  INTEGER PRIMARY KEY,
    kind                TEXT NOT NULL,
    item_refs           TEXT NOT NULL,
    release             TEXT NOT NULL,
    download_url_sealed TEXT NOT NULL DEFAULT '',
    decision            TEXT NOT NULL,
    origin              TEXT NOT NULL,
    downloader          TEXT NOT NULL,
    download_id         TEXT NOT NULL,
    guid                TEXT NOT NULL DEFAULT '',
    info_hash           TEXT NOT NULL DEFAULT '',
    title               TEXT NOT NULL,
    quality             TEXT NOT NULL DEFAULT '',
    sent_at             INTEGER NOT NULL
) STRICT;

CREATE TABLE downloads (
    id               INTEGER PRIMARY KEY,
    grab_id          INTEGER NOT NULL UNIQUE REFERENCES grabs (id),
    downloader       TEXT NOT NULL,
    download_id      TEXT NOT NULL,
    protocol         TEXT NOT NULL,
    kind             TEXT NOT NULL,
    title            TEXT NOT NULL,
    state            TEXT NOT NULL,
    progress         REAL NOT NULL DEFAULT 0,
    size             INTEGER NOT NULL DEFAULT 0,
    downloaded       INTEGER NOT NULL DEFAULT 0,
    speed            INTEGER NOT NULL DEFAULT 0,
    eta              INTEGER NOT NULL DEFAULT 0,
    seeders          INTEGER NOT NULL DEFAULT 0,
    peers            INTEGER NOT NULL DEFAULT 0,
    category         TEXT NOT NULL DEFAULT '',
    save_path        TEXT NOT NULL DEFAULT '',
    content_path     TEXT NOT NULL DEFAULT '',
    held             INTEGER NOT NULL DEFAULT 0,
    status_reason    TEXT NOT NULL DEFAULT '',
    payload_checked  INTEGER NOT NULL DEFAULT 0,
    added_at         INTEGER NOT NULL,
    completed_at     INTEGER,
    last_activity_at INTEGER
) STRICT;
-- (downloader, download id) is unique among downloads still alive.
CREATE UNIQUE INDEX downloads_live ON downloads (downloader, download_id)
    WHERE state NOT IN ('imported', 'removed', 'failed');
CREATE INDEX downloads_state ON downloads (state);

CREATE TABLE download_links (
    download_id INTEGER NOT NULL REFERENCES downloads (id) ON DELETE CASCADE,
    item_kind   TEXT NOT NULL,
    item_id     INTEGER NOT NULL,
    status      TEXT NOT NULL,
    PRIMARY KEY (download_id, item_kind, item_id)
) STRICT;
CREATE INDEX download_links_item ON download_links (item_kind, item_id);
