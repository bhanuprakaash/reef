CREATE TABLE skill (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL,
  source_kind TEXT NOT NULL CHECK (source_kind IN ('folder', 'git')),
  source TEXT NOT NULL,
  ref TEXT NOT NULL,
  dir TEXT NOT NULL,
  revision TEXT NOT NULL,
  version INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE skill_file (
  skill_id INTEGER NOT NULL REFERENCES skill(id) ON DELETE CASCADE,
  version INTEGER NOT NULL,
  path TEXT NOT NULL,
  body BLOB NOT NULL,
  PRIMARY KEY (skill_id, version, path)
);
