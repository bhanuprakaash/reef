ALTER TABLE role ADD COLUMN write INTEGER NOT NULL DEFAULT 0 CHECK (write IN (0, 1));
UPDATE role SET write = 1 WHERE team_id = (SELECT id FROM team WHERE name = 'solo');
