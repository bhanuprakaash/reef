CREATE TABLE work_item_target_new (
  work_item_id INTEGER NOT NULL REFERENCES work_item(id) ON DELETE CASCADE,
  project_id INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
  proposed_by TEXT NOT NULL DEFAULT '',
  authorized_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  PRIMARY KEY (work_item_id, project_id)
);
INSERT INTO work_item_target_new (work_item_id, project_id, proposed_by, authorized_by, created_at)
SELECT work_item_id, project_id,
  CASE WHEN authorized_by LIKE 'proposed by %' THEN substr(authorized_by, 13) ELSE '' END,
  CASE WHEN authorized_by LIKE 'proposed by %' THEN '' ELSE authorized_by END,
  created_at FROM work_item_target;
DROP TABLE work_item_target;
ALTER TABLE work_item_target_new RENAME TO work_item_target;
