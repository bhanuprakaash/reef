CREATE TABLE turn_checkpoint (
  turn_id INTEGER NOT NULL REFERENCES turn(id) ON DELETE CASCADE,
  workspace_id INTEGER NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  ref TEXT NOT NULL,
  PRIMARY KEY (turn_id, workspace_id)
);

ALTER TABLE turn DROP COLUMN checkpoint;
