CREATE TABLE memory_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT NOT NULL,
    body TEXT NOT NULL,
    why TEXT NOT NULL DEFAULT '',
    scope_kind TEXT NOT NULL,
    scope_id TEXT NOT NULL DEFAULT '',
    author TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('candidate', 'accepted', 'retired', 'rejected')),
    retired_at TEXT,
    replaced_by INTEGER,
    created_at TEXT NOT NULL,
    vector BLOB
);

INSERT INTO memory_new (id, kind, body, why, scope_kind, scope_id, author, role, status, retired_at, replaced_by, created_at, vector)
SELECT id, kind, body, why, scope_kind, scope_id, author, role, status, retired_at, replaced_by, created_at, vector FROM memory;

DROP TABLE memory;

ALTER TABLE memory_new RENAME TO memory;

CREATE INDEX memory_scope ON memory (scope_kind, scope_id, status);

CREATE TRIGGER memory_fts_insert AFTER INSERT ON memory BEGIN
    INSERT INTO memory_fts (rowid, body, why) VALUES (new.id, new.body, new.why);
END;

CREATE TRIGGER memory_fts_delete AFTER DELETE ON memory BEGIN
    INSERT INTO memory_fts (memory_fts, rowid, body, why) VALUES ('delete', old.id, old.body, old.why);
END;

CREATE TRIGGER memory_fts_update AFTER UPDATE OF body, why ON memory BEGIN
    INSERT INTO memory_fts (memory_fts, rowid, body, why) VALUES ('delete', old.id, old.body, old.why);
    INSERT INTO memory_fts (rowid, body, why) VALUES (new.id, new.body, new.why);
END;
