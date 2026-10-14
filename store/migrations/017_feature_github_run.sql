ALTER TABLE feature_github_check ADD COLUMN run_key TEXT NOT NULL DEFAULT '';
ALTER TABLE feature_github_check ADD COLUMN woken INTEGER NOT NULL DEFAULT 0;
