PRAGMA foreign_keys = OFF;

CREATE TABLE space_new (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER REFERENCES project(id) ON DELETE CASCADE,
  work_item_id INTEGER REFERENCES work_item(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT '',
  paused TEXT NOT NULL DEFAULT '',
  token_budget INTEGER NOT NULL DEFAULT 1000000,
  team_id INTEGER REFERENCES team(id),
  closed_at TEXT,
  dismissed_message_id INTEGER,
  active_turn TEXT NOT NULL DEFAULT '',
  name_auto INTEGER NOT NULL DEFAULT 0,
  kind TEXT NOT NULL DEFAULT 'free' CHECK (kind IN ('work_item', 'free', 'conversation', 'consultation')),
  moved_to INTEGER REFERENCES space(id) ON DELETE SET NULL
);
INSERT INTO space_new SELECT id, project_id, work_item_id, name, created_at, role, paused,
  token_budget, team_id, closed_at, dismissed_message_id, active_turn, name_auto, kind, moved_to
  FROM space;
DROP TABLE space;
ALTER TABLE space_new RENAME TO space;
CREATE INDEX space_work_item ON space(work_item_id);
CREATE UNIQUE INDEX space_one_active ON space(work_item_id)
  WHERE work_item_id IS NOT NULL AND closed_at IS NULL;

CREATE TRIGGER pin_space_delete AFTER DELETE ON space
BEGIN
  DELETE FROM pin WHERE kind IN ('chat', 'space') AND target_id = old.id;
END;
