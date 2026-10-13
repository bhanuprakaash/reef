CREATE TABLE gate (
  id        INTEGER PRIMARY KEY,
  team_id   INTEGER NOT NULL REFERENCES team(id) ON DELETE CASCADE,
  from_role TEXT NOT NULL,
  to_role   TEXT NOT NULL,
  enabled   INTEGER NOT NULL CHECK (enabled IN (0, 1)),
  UNIQUE (team_id, from_role, to_role)
);
CREATE TABLE approval (
  id         INTEGER PRIMARY KEY,
  channel_id INTEGER NOT NULL REFERENCES channel(id) ON DELETE CASCADE,
  from_role  TEXT NOT NULL,
  to_role    TEXT NOT NULL,
  message_id INTEGER NOT NULL REFERENCES message(id) ON DELETE CASCADE,
  text       TEXT NOT NULL,
  parent     INTEGER REFERENCES message(id) ON DELETE CASCADE,
  status     TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
  created_at TEXT NOT NULL
);
CREATE INDEX approval_channel ON approval(channel_id, status);
