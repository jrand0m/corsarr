-- What the outbound client learned about each service's rate limit (one
-- document per host or plugin instance), so a restart does not relearn it.
CREATE TABLE rate_limits (
    key        TEXT PRIMARY KEY,
    doc        TEXT NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;
