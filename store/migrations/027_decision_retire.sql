ALTER TABLE decision ADD COLUMN retired_at TEXT;
ALTER TABLE decision ADD COLUMN replaced_by INTEGER REFERENCES decision(id);
ALTER TABLE proposal ADD COLUMN retire TEXT NOT NULL DEFAULT '[]';
