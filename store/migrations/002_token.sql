CREATE TABLE token (
  token TEXT PRIMARY KEY,
  channel_id INTEGER NOT NULL REFERENCES channel(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX token_channel ON token(channel_id);
