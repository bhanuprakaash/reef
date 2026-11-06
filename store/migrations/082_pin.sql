CREATE TABLE pin (
  kind TEXT NOT NULL CHECK (kind IN ('bot', 'chat', 'space')),
  target_id INTEGER NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (kind, target_id)
);

CREATE TRIGGER pin_bot_delete AFTER DELETE ON bot
BEGIN
  DELETE FROM pin WHERE kind = 'bot' AND target_id = old.id;
END;

CREATE TRIGGER pin_space_delete AFTER DELETE ON space
BEGIN
  DELETE FROM pin WHERE kind IN ('chat', 'space') AND target_id = old.id;
END;
