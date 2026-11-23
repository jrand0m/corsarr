-- 0005: blocklist and per-indexer RSS cursors (advanced by automation).

CREATE TABLE blocklist (
    id        INTEGER PRIMARY KEY,
    item_kind TEXT NOT NULL,
    item_id   INTEGER NOT NULL,
    indexer   TEXT NOT NULL DEFAULT '',
    guid      TEXT NOT NULL DEFAULT '',
    info_hash TEXT NOT NULL DEFAULT '',
    title     TEXT NOT NULL,
    source    TEXT NOT NULL,
    reason    TEXT NOT NULL DEFAULT '',
    added_at  INTEGER NOT NULL,
    UNIQUE (item_kind, item_id, indexer, guid, info_hash, title)
) STRICT;
CREATE INDEX blocklist_item ON blocklist (item_kind, item_id);

CREATE TABLE feed_cursors (
    indexer    TEXT NOT NULL,
    kind       TEXT NOT NULL,
    cursor     TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (indexer, kind)
) STRICT;
