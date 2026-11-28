-- Custom formats and custom quality profiles made in the web UI or by a
-- migration, one JSON document each (writer: the profile service).
CREATE TABLE custom_formats (
    key        TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    doc        TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE TABLE quality_profiles (
    key        TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    doc        TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;
