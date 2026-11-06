CREATE TABLE workspace (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
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

CREATE UNIQUE INDEX workspace_item ON workspace(work_item_id, project_id) WHERE work_item_id IS NOT NULL;
CREATE UNIQUE INDEX workspace_fix ON workspace(space_id, name, project_id) WHERE work_item_id IS NULL;
CREATE INDEX workspace_space ON workspace(space_id);
