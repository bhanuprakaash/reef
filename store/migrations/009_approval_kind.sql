ALTER TABLE approval ADD COLUMN kind TEXT NOT NULL DEFAULT 'gate' CHECK (kind IN ('gate', 'hold'));
