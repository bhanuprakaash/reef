CREATE TABLE bot_session (
  id              INTEGER PRIMARY KEY,
  space_id        INTEGER NOT NULL REFERENCES space(id) ON DELETE CASCADE,
  bot_id          INTEGER NOT NULL REFERENCES bot(id),
  backend         TEXT NOT NULL,
  provider        TEXT NOT NULL DEFAULT '',
  model           TEXT NOT NULL DEFAULT '',
  description     TEXT NOT NULL DEFAULT '',
  skills          TEXT NOT NULL DEFAULT '[]',
  tools           TEXT NOT NULL DEFAULT '[]',
  mcp_servers     TEXT NOT NULL DEFAULT '[]',
  write           INTEGER NOT NULL DEFAULT 0 CHECK (write IN (0, 1)),
  permission_mode TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'replaced', 'revoked')),
  created_at      TEXT NOT NULL,
  ended_at        TEXT
);
CREATE UNIQUE INDEX bot_session_active ON bot_session(space_id, bot_id) WHERE status = 'active';

ALTER TABLE turn ADD COLUMN bot_session_id INTEGER REFERENCES bot_session(id) ON DELETE SET NULL;
