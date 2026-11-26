-- 0090: stable integer ids for the compatibility API (writer: legacy). Clients
-- of that API address quality profiles, root folders, tags and indexers by
-- integer; corsarr names them by string key. Rows are never renumbered, so an
-- id a client stored keeps its meaning across restarts. A tag row also keeps
-- a tag a client created before any item carries it.

CREATE TABLE legacy_ids (
    space TEXT NOT NULL,
    key   TEXT NOT NULL,
    id    INTEGER NOT NULL,
    PRIMARY KEY (space, key),
    UNIQUE (space, id)
) STRICT;
