DROP TABLE IF EXISTS work_item_github_check;
DROP TABLE IF EXISTS work_item_github;
ALTER TABLE repo_request ADD COLUMN review TEXT NOT NULL DEFAULT '';
ALTER TABLE repo_request ADD COLUMN checks TEXT NOT NULL DEFAULT '[]';
ALTER TABLE repo_request ADD COLUMN failure TEXT NOT NULL DEFAULT '';
ALTER TABLE repo_request ADD COLUMN failure_handled TEXT NOT NULL DEFAULT '';
ALTER TABLE repo_request ADD COLUMN checked_at TEXT NOT NULL DEFAULT '';
ALTER TABLE repo_request ADD COLUMN check_error TEXT NOT NULL DEFAULT '';
