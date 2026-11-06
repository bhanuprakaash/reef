CREATE TABLE approval_new (
  id INTEGER PRIMARY KEY,
  channel_id INTEGER NOT NULL REFERENCES channel(id) ON DELETE CASCADE,
  from_role TEXT NOT NULL,
  to_role TEXT NOT NULL,
  message_id INTEGER REFERENCES message(id) ON DELETE CASCADE,
  direct_message_id INTEGER REFERENCES direct_message(id) ON DELETE CASCADE,
  text TEXT NOT NULL,
  parent INTEGER REFERENCES message(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
  kind TEXT NOT NULL DEFAULT 'gate' CHECK (kind IN ('gate', 'hold')),
  created_at TEXT NOT NULL
);
INSERT INTO approval_new (id, channel_id, from_role, to_role, message_id, direct_message_id, text, parent, status, kind, created_at)
SELECT id, channel_id, from_role, to_role, message_id, NULL, text, parent, status, kind, created_at FROM approval;
DROP TABLE approval;
ALTER TABLE approval_new RENAME TO approval;
CREATE INDEX approval_channel ON approval(channel_id, status);
