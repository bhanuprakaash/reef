CREATE TABLE proof_repository (
  work_item_id INTEGER NOT NULL REFERENCES proof(work_item_id) ON DELETE CASCADE,
  workspace_id INTEGER NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  project_id INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  branch TEXT NOT NULL,
  head TEXT NOT NULL,
  base_ref TEXT NOT NULL,
  dir TEXT NOT NULL,
  dirty INTEGER NOT NULL,
  PRIMARY KEY (work_item_id, workspace_id)
);
