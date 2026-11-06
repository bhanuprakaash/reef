ALTER TABLE direct_message ADD COLUMN source_message_id INTEGER REFERENCES message(id) ON DELETE SET NULL;
