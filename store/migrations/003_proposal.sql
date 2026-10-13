CREATE TABLE proposal (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  channel_id INTEGER NOT NULL REFERENCES channel(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  brief TEXT NOT NULL,
  notes TEXT NOT NULL,
  decisions TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  created_at TEXT NOT NULL,
  decided_at TEXT
);
CREATE INDEX proposal_channel ON proposal(channel_id);
