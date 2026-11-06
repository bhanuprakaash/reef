UPDATE proposal SET status = 'rejected', decided_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE kind = 'wrap' AND status = 'pending';
ALTER TABLE proposal ADD COLUMN items TEXT NOT NULL DEFAULT '[]';
ALTER TABLE proposal DROP COLUMN notes;
ALTER TABLE proposal DROP COLUMN decisions;
UPDATE proposal SET retire = '[]';
