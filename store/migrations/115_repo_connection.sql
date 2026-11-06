CREATE TABLE repo_connection (
  id         INTEGER PRIMARY KEY,
  provider   TEXT NOT NULL CHECK (provider IN ('github', 'gitlab')),
  host       TEXT NOT NULL,
  account    TEXT NOT NULL DEFAULT '',
  state      TEXT NOT NULL CHECK (state IN ('connected', 'disconnected')),
  reason     TEXT NOT NULL DEFAULT '',
  checked_at TEXT NOT NULL,
  UNIQUE (provider, host)
);
ALTER TABLE project ADD COLUMN repo_connection_id INTEGER;
ALTER TABLE project ADD COLUMN remote_path TEXT NOT NULL DEFAULT '';
