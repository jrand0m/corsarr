-- 0070: indexer instances added from the web UI (writer: indexers). Instances
-- from the config file are not stored here; the config wins on a shared key.
-- Secret setting values are sealed with secret.key.

CREATE TABLE indexer_instances (
    key        TEXT PRIMARY KEY,
    plugin_id  TEXT NOT NULL,
    name       TEXT NOT NULL DEFAULT '',
    enabled    INTEGER NOT NULL DEFAULT 1,
    settings   TEXT NOT NULL DEFAULT '{}',
    secrets    TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;
