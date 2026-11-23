-- 0010: metadata cache, one record per (source, kind, external id) (writer: the media-kind module).

CREATE TABLE metadata_cache (
    source       TEXT NOT NULL,
    kind         TEXT NOT NULL,
    external_id  TEXT NOT NULL,
    title        TEXT NOT NULL DEFAULT '',
    record       TEXT NOT NULL,
    fetched_at   INTEGER NOT NULL,
    next_refresh INTEGER NOT NULL,
    PRIMARY KEY (source, kind, external_id)
) STRICT;
