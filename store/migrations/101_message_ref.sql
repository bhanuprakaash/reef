CREATE TABLE message_ref (
  message_id INTEGER NOT NULL REFERENCES message(id) ON DELETE CASCADE,
  space_id INTEGER NOT NULL,
  PRIMARY KEY (message_id, space_id)
);
CREATE INDEX message_ref_space ON message_ref(space_id);
