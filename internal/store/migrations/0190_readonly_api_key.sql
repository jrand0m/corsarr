-- A second, read-only API key for assistants (MCP); absent until issued.
CREATE TABLE api_key_readonly (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    key_sealed TEXT NOT NULL,
    created_at INTEGER NOT NULL
) STRICT;
