CREATE TABLE connection (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE,
  command    TEXT NOT NULL DEFAULT '',
  args       TEXT NOT NULL DEFAULT '[]',
  env        TEXT NOT NULL DEFAULT '{}',
  url        TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  CHECK ((command = '') != (url = ''))
);

ALTER TABLE bot DROP COLUMN mcp_servers;
ALTER TABLE bot ADD COLUMN connections TEXT NOT NULL DEFAULT '[]';
ALTER TABLE bot_session RENAME COLUMN mcp_servers TO connections;
