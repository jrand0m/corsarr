-- 0260: artwork cache index (writer: the image cache) and artwork on movies and series.
-- variant 0 is the stored original; other values are the width of an on-demand smaller copy.
-- A row with format '' and miss_until in the future remembers that the provider has no such image.

CREATE TABLE image_cache (
    key         TEXT NOT NULL,
    variant     INTEGER NOT NULL DEFAULT 0,
    origin      TEXT NOT NULL,
    role        TEXT NOT NULL CHECK (role IN ('poster', 'backdrop')),
    format      TEXT NOT NULL DEFAULT '' CHECK (format IN ('', 'jpeg', 'png')),
    bytes       INTEGER NOT NULL DEFAULT 0,
    width       INTEGER NOT NULL DEFAULT 0,
    height      INTEGER NOT NULL DEFAULT 0,
    quality     INTEGER NOT NULL DEFAULT 0,
    last_access INTEGER NOT NULL,
    miss_until  INTEGER NOT NULL DEFAULT 0,
    created_at  INTEGER NOT NULL,
    PRIMARY KEY (key, variant)
) STRICT;
CREATE INDEX image_cache_lru ON image_cache (last_access);

ALTER TABLE movies ADD COLUMN poster_url TEXT NOT NULL DEFAULT '';
ALTER TABLE movies ADD COLUMN fanart_url TEXT NOT NULL DEFAULT '';
ALTER TABLE series ADD COLUMN fanart_url TEXT NOT NULL DEFAULT '';

-- Movies added before artwork was stored take the poster their cached provider record already holds.
UPDATE movies SET poster_url = COALESCE((
    SELECT json_extract(m.record, '$.posterUrl') FROM metadata_cache m
    WHERE m.source = 'tmdb' AND m.kind = 'movie' AND m.external_id = CAST(movies.tmdb_id AS TEXT)
), '');
