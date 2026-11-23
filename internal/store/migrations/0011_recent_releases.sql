-- 0011: bounded cache of releases shown by recent searches, so a manual grab
-- survives a restart (writer: automation). The download link is sealed.

CREATE TABLE recent_releases (
    id                  TEXT PRIMARY KEY,
    release             TEXT NOT NULL,
    download_url_sealed TEXT NOT NULL DEFAULT '',
    seen_at             INTEGER NOT NULL
) STRICT;
CREATE INDEX recent_releases_seen ON recent_releases (seen_at);
