-- 0007: notification outbox and media-server refresh requests.

CREATE TABLE notify_outbox (
    id         INTEGER PRIMARY KEY,
    event_id   TEXT NOT NULL,
    connection TEXT NOT NULL,
    event_type TEXT NOT NULL,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL,
    envelope   TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
    attempts   INTEGER NOT NULL DEFAULT 0,
    next_at    INTEGER NOT NULL,
    last_error TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    sent_at    INTEGER,
    UNIQUE (event_id, connection)
) STRICT;
CREATE INDEX notify_outbox_due ON notify_outbox (status, next_at);

-- Pending partial refreshes, deduplicated by (server, path); they survive restarts.
CREATE TABLE refresh_requests (
    server     TEXT NOT NULL,
    path       TEXT NOT NULL,
    attempts   INTEGER NOT NULL DEFAULT 0,
    next_at    INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (server, path)
) STRICT;
