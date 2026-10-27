ALTER TABLE teammate ADD COLUMN colour TEXT NOT NULL DEFAULT '';

UPDATE teammate SET face = 'jellyfish' WHERE name = 'lead' AND face = '';
