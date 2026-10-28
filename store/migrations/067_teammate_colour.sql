ALTER TABLE teammate ADD COLUMN colour TEXT NOT NULL DEFAULT '';

UPDATE teammate SET face = CASE name
  WHEN 'lead' THEN 'jellyfish'
  WHEN 'architect' THEN 'whale'
  WHEN 'developer' THEN 'penguin'
  WHEN 'tester' THEN 'shark'
END
WHERE face = '' AND name IN ('lead', 'architect', 'developer', 'tester');
