ALTER TABLE proposal ADD COLUMN bot_id INTEGER REFERENCES teammate(id);
