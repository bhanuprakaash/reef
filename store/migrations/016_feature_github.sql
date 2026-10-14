CREATE TABLE feature_github (
  feature_id INTEGER PRIMARY KEY REFERENCES feature(id) ON DELETE CASCADE,
  pr_number INTEGER NOT NULL,
  pr_state TEXT NOT NULL,
  pr_url TEXT NOT NULL DEFAULT ''
);
CREATE TABLE feature_github_check (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  feature_id INTEGER NOT NULL REFERENCES feature(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  notified INTEGER NOT NULL DEFAULT 0,
  UNIQUE(feature_id, name)
);
