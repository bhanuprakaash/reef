PRAGMA foreign_keys = OFF;

CREATE TEMP TABLE work_item_seq AS SELECT seq FROM sqlite_sequence WHERE name = 'work_item';

CREATE TABLE work_item_new (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  parent_id INTEGER REFERENCES work_item(id) ON DELETE CASCADE,
  type TEXT NOT NULL DEFAULT 'feature',
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'backlog',
  owner_bot_id INTEGER REFERENCES bot(id) ON DELETE SET NULL,
  approved_at TEXT,
  approval_key TEXT UNIQUE,
  proposed_by TEXT NOT NULL DEFAULT '',
  queue_position INTEGER,
  token_budget INTEGER NOT NULL DEFAULT 1000000,
  created_at TEXT NOT NULL,
  closed_at TEXT
);
INSERT INTO work_item_new (id, project_id, parent_id, type, title, description, status, proposed_by,
  queue_position, token_budget, created_at, closed_at)
SELECT id, project_id, parent_id, type, title, description, status, proposed_by,
  queue_position, token_budget, created_at, closed_at FROM work_item;
DROP TABLE work_item;
ALTER TABLE work_item_new RENAME TO work_item;
INSERT INTO sqlite_sequence (name, seq)
  SELECT 'work_item', seq FROM work_item_seq
  WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'work_item');
UPDATE sqlite_sequence SET seq = MAX(seq, (SELECT seq FROM work_item_seq))
  WHERE name = 'work_item' AND EXISTS (SELECT 1 FROM work_item_seq);
DROP TABLE work_item_seq;
CREATE INDEX work_item_project ON work_item(project_id);
CREATE INDEX work_item_parent ON work_item(parent_id);

CREATE TABLE work_item_participant (
  work_item_id INTEGER NOT NULL REFERENCES work_item(id) ON DELETE CASCADE,
  bot_id INTEGER NOT NULL REFERENCES bot(id) ON DELETE CASCADE,
  PRIMARY KEY (work_item_id, bot_id)
);

CREATE TABLE work_item_target (
  work_item_id INTEGER NOT NULL REFERENCES work_item(id) ON DELETE CASCADE,
  project_id INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  authorized_by TEXT NOT NULL,
  origin_message_id INTEGER REFERENCES message(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (work_item_id, project_id)
);
