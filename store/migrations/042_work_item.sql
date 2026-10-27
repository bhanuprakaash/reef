DROP TABLE task;
DROP TABLE feature_github_check;
DROP TABLE feature_github;

ALTER TABLE feature RENAME TO work_item;
DELETE FROM work_item;
DELETE FROM space WHERE feature_id IS NULL;
CREATE TEMP TABLE work_item_seq AS SELECT seq FROM sqlite_sequence WHERE name = 'work_item';

CREATE TABLE work_item_new (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  parent_id INTEGER REFERENCES work_item(id) ON DELETE CASCADE,
  type TEXT NOT NULL DEFAULT 'feature',
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'backlog',
  team_id INTEGER REFERENCES team(id) ON DELETE SET NULL,
  proposed_by TEXT NOT NULL DEFAULT '',
  suggested_team_id INTEGER REFERENCES team(id) ON DELETE SET NULL,
  queue_position INTEGER,
  token_budget INTEGER NOT NULL DEFAULT 1000000,
  created_at TEXT NOT NULL,
  closed_at TEXT
);
DROP TABLE work_item;
ALTER TABLE work_item_new RENAME TO work_item;
INSERT INTO sqlite_sequence (name, seq)
  SELECT 'work_item', seq FROM work_item_seq
  WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'work_item');
DROP TABLE work_item_seq;
CREATE INDEX work_item_project ON work_item(project_id);
CREATE INDEX work_item_parent ON work_item(parent_id);

CREATE TABLE acceptance_line (
  work_item_id INTEGER NOT NULL REFERENCES work_item(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  text TEXT NOT NULL,
  checked INTEGER NOT NULL DEFAULT 0,
  note TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (work_item_id, position)
);

CREATE TABLE work_item_github (
  work_item_id INTEGER PRIMARY KEY REFERENCES work_item(id) ON DELETE CASCADE,
  pr_number INTEGER NOT NULL,
  pr_state TEXT NOT NULL,
  pr_url TEXT NOT NULL DEFAULT ''
);
CREATE TABLE work_item_github_check (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  work_item_id INTEGER NOT NULL REFERENCES work_item(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  run_key TEXT NOT NULL,
  woken INTEGER NOT NULL DEFAULT 0,
  UNIQUE(work_item_id, name, run_key)
);

ALTER TABLE brief RENAME COLUMN feature_id TO work_item_id;
ALTER TABLE role_notes RENAME COLUMN feature_id TO work_item_id;
ALTER TABLE decision RENAME COLUMN feature_id TO work_item_id;
ALTER TABLE space RENAME COLUMN feature_id TO work_item_id;

DROP INDEX brief_feature;
DROP INDEX decision_feature;
DROP INDEX space_feature;
CREATE INDEX brief_work_item ON brief(work_item_id);
CREATE INDEX decision_work_item ON decision(work_item_id);
CREATE INDEX space_work_item ON space(work_item_id);
