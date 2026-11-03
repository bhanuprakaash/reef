ALTER TABLE repo_request ADD COLUMN depends_on TEXT NOT NULL DEFAULT '[]';
