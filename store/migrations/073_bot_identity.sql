ALTER TABLE message ADD COLUMN bot_id INTEGER REFERENCES teammate(id);
ALTER TABLE turn ADD COLUMN bot_id INTEGER REFERENCES teammate(id);
ALTER TABLE token ADD COLUMN bot_id INTEGER REFERENCES teammate(id);
ALTER TABLE direct_message ADD COLUMN from_bot_id INTEGER REFERENCES teammate(id);
ALTER TABLE direct_message ADD COLUMN to_bot_id INTEGER REFERENCES teammate(id);
CREATE INDEX message_bot ON message(space_id, bot_id, id);
CREATE INDEX turn_bot ON turn(space_id, bot_id, id);
CREATE INDEX direct_message_from_bot ON direct_message(space_id, from_bot_id, id);
CREATE INDEX direct_message_to_bot ON direct_message(space_id, to_bot_id, id);
