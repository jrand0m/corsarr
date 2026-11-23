-- 0006: kernel rule runner storage.

CREATE TABLE rule_runs (
    id          INTEGER PRIMARY KEY,
    rule_id     TEXT NOT NULL,
    mode        TEXT NOT NULL CHECK (mode IN ('off', 'dry-run', 'live')),
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    counts      TEXT NOT NULL DEFAULT '{}'
) STRICT;

CREATE TABLE rule_actions (
    id             INTEGER PRIMARY KEY,
    run_id         INTEGER NOT NULL REFERENCES rule_runs (id),
    rule_id        TEXT NOT NULL,
    mode           TEXT NOT NULL,
    candidate      TEXT NOT NULL,
    classification TEXT NOT NULL,
    evidence       TEXT NOT NULL DEFAULT '{}',
    ops            TEXT NOT NULL DEFAULT '[]',
    undo           TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL,
    error          TEXT NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL,
    acted_at       INTEGER,
    undone_at      INTEGER
) STRICT;
CREATE INDEX rule_actions_rule ON rule_actions (rule_id, id);
