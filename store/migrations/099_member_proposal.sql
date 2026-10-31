CREATE TABLE member_proposal (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  space_id INTEGER NOT NULL REFERENCES space(id) ON DELETE CASCADE,
  bot_id INTEGER NOT NULL REFERENCES bot(id) ON DELETE CASCADE,
  proposed_by TEXT NOT NULL,
  reason TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
  created_at TEXT NOT NULL,
  decided_at TEXT
);
CREATE UNIQUE INDEX member_proposal_pending ON member_proposal(space_id, bot_id) WHERE status = 'pending';
