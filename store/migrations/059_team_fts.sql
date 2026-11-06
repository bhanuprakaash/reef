CREATE VIRTUAL TABLE team_fts USING fts5(
  responsibility, content='team', content_rowid='id', tokenize='porter unicode61'
);
INSERT INTO team_fts (team_fts) VALUES ('rebuild');

CREATE TRIGGER team_fts_insert AFTER INSERT ON team BEGIN
  INSERT INTO team_fts (rowid, responsibility) VALUES (new.id, new.responsibility);
END;

CREATE TRIGGER team_fts_delete AFTER DELETE ON team BEGIN
  INSERT INTO team_fts (team_fts, rowid, responsibility) VALUES ('delete', old.id, old.responsibility);
END;

CREATE TRIGGER team_fts_update AFTER UPDATE OF responsibility ON team BEGIN
  INSERT INTO team_fts (team_fts, rowid, responsibility) VALUES ('delete', old.id, old.responsibility);
  INSERT INTO team_fts (rowid, responsibility) VALUES (new.id, new.responsibility);
END;
