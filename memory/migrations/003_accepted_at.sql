ALTER TABLE memory ADD COLUMN accepted_at TEXT;

UPDATE memory SET accepted_at = created_at WHERE status IN ('accepted', 'retired');
