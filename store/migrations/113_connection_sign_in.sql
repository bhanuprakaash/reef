CREATE TABLE connection_sign_in (
  connection_id INTEGER NOT NULL REFERENCES connection(id) ON DELETE CASCADE,
  backend TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('signed_in', 'needs_sign_in', 'failed')),
  detail TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  PRIMARY KEY (connection_id, backend)
);
