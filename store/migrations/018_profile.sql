CREATE TABLE profile (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  name TEXT NOT NULL,
  occupation TEXT NOT NULL DEFAULT '',
  style TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);
