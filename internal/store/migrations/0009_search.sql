-- 0009: per-item search state and the per-indexer query log (writer: automation).

CREATE TABLE search_state (
    item_kind     TEXT NOT NULL,
    item_id       INTEGER NOT NULL,
    last_searched INTEGER,
    next_eligible INTEGER,
    misses        INTEGER NOT NULL DEFAULT 0,
    last_result   TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (item_kind, item_id)
) STRICT;

-- One row per query sent to an indexer; limits count a sliding window over it.
CREATE TABLE indexer_queries (
    indexer TEXT NOT NULL,
    at      INTEGER NOT NULL
) STRICT;
CREATE INDEX indexer_queries_window ON indexer_queries (indexer, at);
