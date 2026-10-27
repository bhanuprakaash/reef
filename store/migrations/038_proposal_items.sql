ALTER TABLE proposal ADD COLUMN items TEXT NOT NULL DEFAULT '[]';
ALTER TABLE proposal DROP COLUMN notes;
ALTER TABLE proposal DROP COLUMN decisions;
UPDATE proposal SET retire = '[]';
