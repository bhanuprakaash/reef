ALTER TABLE approval ADD COLUMN from_bot_id INTEGER REFERENCES teammate(id);
ALTER TABLE approval ADD COLUMN to_bot_id INTEGER REFERENCES teammate(id);
