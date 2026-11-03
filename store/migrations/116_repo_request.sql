CREATE TABLE repo_request (
  id INTEGER PRIMARY KEY,
  provider TEXT NOT NULL,
  host TEXT NOT NULL,
  path TEXT NOT NULL,
  number INTEGER NOT NULL,
  url TEXT NOT NULL,
  state TEXT NOT NULL,
  head TEXT NOT NULL,
  base TEXT NOT NULL,
  workspace_id INTEGER NOT NULL REFERENCES workspace(id),
  work_item_id INTEGER REFERENCES work_item(id),
  space_id INTEGER REFERENCES space(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(host, path, number)
);
