CREATE TABLE direct_message (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  channel_id INTEGER NOT NULL REFERENCES channel(id) ON DELETE CASCADE,
  chain_id INTEGER REFERENCES direct_message(id) ON DELETE CASCADE,
  reply_to_id INTEGER REFERENCES direct_message(id) ON DELETE CASCADE,
  from_role TEXT NOT NULL,
  to_role TEXT NOT NULL,
  body TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('sent', 'done', 'failed')),
  created_at TEXT NOT NULL
);
CREATE INDEX direct_message_channel ON direct_message(channel_id, id);
CREATE INDEX direct_message_chain ON direct_message(chain_id, id);
CREATE UNIQUE INDEX direct_message_done_response ON direct_message(reply_to_id) WHERE status = 'done';
ALTER TABLE turn ADD COLUMN direct_message_id INTEGER REFERENCES direct_message(id) ON DELETE SET NULL;
