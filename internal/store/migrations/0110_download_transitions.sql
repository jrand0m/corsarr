-- 0110: the state history of every download (writer: queue). One row per
-- accepted transition, so a download's detail can show how it got where it is.

CREATE TABLE download_transitions (
    id          INTEGER PRIMARY KEY,
    download_id INTEGER NOT NULL REFERENCES downloads (id),
    from_state  TEXT NOT NULL,
    to_state    TEXT NOT NULL,
    actor       TEXT NOT NULL,
    reason      TEXT NOT NULL DEFAULT '',
    at          INTEGER NOT NULL
) STRICT;

CREATE INDEX download_transitions_download ON download_transitions (download_id, id);
