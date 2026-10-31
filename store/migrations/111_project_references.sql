PRAGMA foreign_keys = OFF;

CREATE TEMP TABLE workspace_seq AS SELECT seq FROM sqlite_sequence WHERE name = 'workspace';

CREATE TABLE workspace_new (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL,
  work_item_id INTEGER REFERENCES work_item(id) ON DELETE CASCADE,
  space_id INTEGER REFERENCES space(id) ON DELETE CASCADE,
  name TEXT NOT NULL DEFAULT '',
  dir TEXT NOT NULL UNIQUE,
  branch TEXT NOT NULL,
  base TEXT NOT NULL DEFAULT '',
  base_ref TEXT NOT NULL DEFAULT '',
  base_source TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT 'creating' CHECK (state IN ('creating', 'ready', 'paused', 'removed')),
  note TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
INSERT INTO workspace_new SELECT id, project_id, work_item_id, space_id, name, dir, branch, base, base_ref,
  base_source, state, note, created_at, updated_at FROM workspace;
DROP TABLE workspace;
ALTER TABLE workspace_new RENAME TO workspace;
INSERT INTO sqlite_sequence (name, seq)
  SELECT 'workspace', seq FROM workspace_seq
  WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'workspace');
UPDATE sqlite_sequence SET seq = MAX(seq, (SELECT seq FROM workspace_seq))
  WHERE name = 'workspace' AND EXISTS (SELECT 1 FROM workspace_seq);
DROP TABLE workspace_seq;
CREATE UNIQUE INDEX workspace_item ON workspace(work_item_id, project_id) WHERE work_item_id IS NOT NULL;
CREATE UNIQUE INDEX workspace_fix ON workspace(space_id, name, project_id) WHERE work_item_id IS NULL;
CREATE INDEX workspace_space ON workspace(space_id);

CREATE TABLE work_item_target_new (
  work_item_id INTEGER NOT NULL REFERENCES work_item(id) ON DELETE CASCADE,
  project_id INTEGER NOT NULL,
  authorized_by TEXT NOT NULL,
  origin_message_id INTEGER REFERENCES message(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (work_item_id, project_id)
);
INSERT INTO work_item_target_new SELECT work_item_id, project_id, authorized_by, origin_message_id, created_at
  FROM work_item_target;
DROP TABLE work_item_target;
ALTER TABLE work_item_target_new RENAME TO work_item_target;

CREATE TABLE proof_repository_new (
  work_item_id INTEGER NOT NULL REFERENCES proof(work_item_id) ON DELETE CASCADE,
  workspace_id INTEGER NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  project_id INTEGER NOT NULL,
  branch TEXT NOT NULL,
  head TEXT NOT NULL,
  base_ref TEXT NOT NULL,
  dir TEXT NOT NULL,
  dirty INTEGER NOT NULL,
  PRIMARY KEY (work_item_id, workspace_id)
);
INSERT INTO proof_repository_new SELECT work_item_id, workspace_id, project_id, branch, head, base_ref, dir, dirty
  FROM proof_repository;
DROP TABLE proof_repository;
ALTER TABLE proof_repository_new RENAME TO proof_repository;
