PRAGMA foreign_keys = OFF;

CREATE TABLE skill_new (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL,
  source_kind TEXT NOT NULL CHECK (source_kind IN ('folder', 'git', 'reef')),
  source TEXT NOT NULL,
  ref TEXT NOT NULL,
  dir TEXT NOT NULL,
  revision TEXT NOT NULL,
  version INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
INSERT INTO skill_new SELECT id, name, description, source_kind, source, ref, dir, revision, version,
  created_at, updated_at FROM skill;
DROP TABLE skill;
ALTER TABLE skill_new RENAME TO skill;

CREATE TABLE skill_draft (
  id INTEGER PRIMARY KEY,
  space_id INTEGER NOT NULL REFERENCES space(id) ON DELETE CASCADE,
  bot_id INTEGER NOT NULL REFERENCES bot(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  files TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'saved', 'rejected')),
  skill_id INTEGER REFERENCES skill(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  decided_at TEXT
);
