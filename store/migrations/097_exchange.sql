PRAGMA foreign_keys = OFF;

CREATE TABLE direct_message_new (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  space_id INTEGER NOT NULL REFERENCES "space"(id) ON DELETE CASCADE,
  chain_id INTEGER REFERENCES direct_message(id) ON DELETE CASCADE,
  reply_to_id INTEGER REFERENCES direct_message(id) ON DELETE CASCADE,
  from_role TEXT NOT NULL,
  to_role TEXT NOT NULL,
  body TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('sent', 'done', 'failed')),
  created_at TEXT NOT NULL,
  source_message_id INTEGER REFERENCES message(id) ON DELETE SET NULL,
  state TEXT NOT NULL DEFAULT 'waiting' CHECK (state IN ('', 'waiting', 'delivered', 'answered', 'failed', 'stopped', 'cancelled')),
  from_bot_id INTEGER REFERENCES "bot"(id),
  to_bot_id INTEGER REFERENCES "bot"(id),
  origin_space_id INTEGER REFERENCES "space"(id) ON DELETE SET NULL,
  origin_message_id INTEGER REFERENCES message(id) ON DELETE SET NULL,
  kind TEXT NOT NULL DEFAULT 'request' CHECK (kind IN ('request', 'question', 'guidance')),
  request_key TEXT
);
INSERT INTO direct_message_new (id, space_id, chain_id, reply_to_id, from_role, to_role, body, status,
  created_at, source_message_id, state, from_bot_id, to_bot_id)
SELECT id, space_id, chain_id, reply_to_id, from_role, to_role, body, status,
  created_at, source_message_id, state, from_bot_id, to_bot_id FROM direct_message;
DROP TABLE direct_message;
ALTER TABLE direct_message_new RENAME TO direct_message;
CREATE INDEX direct_message_chain ON direct_message(chain_id, id);
CREATE UNIQUE INDEX direct_message_done_response ON direct_message(reply_to_id) WHERE status = 'done';
CREATE INDEX direct_message_space ON direct_message(space_id, id);
CREATE INDEX direct_message_from_bot ON direct_message(space_id, from_bot_id, id);
CREATE INDEX direct_message_to_bot ON direct_message(space_id, to_bot_id, id);
CREATE INDEX direct_message_origin ON direct_message(origin_space_id, id);
CREATE UNIQUE INDEX direct_message_request_key ON direct_message(request_key) WHERE request_key IS NOT NULL;

ALTER TABLE message ADD COLUMN exchange_id INTEGER REFERENCES direct_message(id) ON DELETE SET NULL;
