CREATE TABLE proof (
  work_item_id INTEGER PRIMARY KEY REFERENCES work_item(id) ON DELETE CASCADE,
  space_id INTEGER NOT NULL REFERENCES space(id) ON DELETE CASCADE,
  commands TEXT NOT NULL,
  screenshots TEXT NOT NULL,
  submitted_at TEXT NOT NULL,
  wrap_started INTEGER NOT NULL DEFAULT 0
);
