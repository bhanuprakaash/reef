CREATE TABLE space_proposal (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  proposed_by TEXT NOT NULL,
  name TEXT NOT NULL,
  bot_ids TEXT NOT NULL,
  coordinator_bot_id INTEGER NOT NULL,
  created_at TEXT NOT NULL
);
