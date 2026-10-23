CREATE TABLE IF NOT EXISTS memory (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL,
    body TEXT NOT NULL,
    why TEXT NOT NULL DEFAULT '',
    scope_kind TEXT NOT NULL,
    scope_id TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('candidate', 'accepted', 'retired')),
    retired_at TEXT,
    replaced_by INTEGER,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS memory_scope ON memory (scope_kind, scope_id, status);

CREATE TABLE IF NOT EXISTS memory_link (
    from_id INTEGER NOT NULL,
    rel TEXT NOT NULL CHECK (rel IN ('about', 'source', 'replaces', 'related')),
    to_kind TEXT NOT NULL,
    to_id TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS memory_link_from ON memory_link (from_id);
CREATE INDEX IF NOT EXISTS memory_link_to ON memory_link (to_kind, to_id);
