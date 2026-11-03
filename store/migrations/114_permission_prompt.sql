CREATE TABLE permission_prompt (
  id          INTEGER PRIMARY KEY,
  space_id    INTEGER NOT NULL REFERENCES space(id) ON DELETE CASCADE,
  bot_id      INTEGER NOT NULL REFERENCES bot(id) ON DELETE CASCADE,
  turn_id     INTEGER REFERENCES turn(id) ON DELETE CASCADE,
  request_id  TEXT NOT NULL,
  tool        TEXT NOT NULL,
  input       TEXT NOT NULL DEFAULT '{}',
  rule        TEXT NOT NULL DEFAULT '',
  path        TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'waiting' CHECK (status IN ('waiting', 'allowed', 'denied', 'expired')),
  answer      TEXT NOT NULL DEFAULT '' CHECK (answer IN ('', 'once', 'always', 'deny')),
  reason      TEXT NOT NULL DEFAULT '',
  granted     INTEGER NOT NULL DEFAULT 0 CHECK (granted IN (0, 1)),
  created_at  TEXT NOT NULL,
  answered_at TEXT
);
CREATE INDEX permission_prompt_space ON permission_prompt(space_id, status);
