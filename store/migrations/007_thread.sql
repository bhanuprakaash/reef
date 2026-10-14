ALTER TABLE message ADD COLUMN parent_id INTEGER REFERENCES message(id);
