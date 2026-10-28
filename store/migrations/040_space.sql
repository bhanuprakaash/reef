ALTER TABLE channel RENAME TO space;
ALTER TABLE approval RENAME COLUMN channel_id TO space_id;
ALTER TABLE direct_message RENAME COLUMN channel_id TO space_id;
ALTER TABLE message RENAME COLUMN channel_id TO space_id;
ALTER TABLE proposal RENAME COLUMN channel_id TO space_id;
ALTER TABLE token RENAME COLUMN channel_id TO space_id;
ALTER TABLE turn RENAME COLUMN channel_id TO space_id;

DROP INDEX channel_project;
DROP INDEX channel_feature;
DROP INDEX approval_channel;
DROP INDEX direct_message_channel;
DROP INDEX message_channel;
DROP INDEX proposal_channel;
DROP INDEX token_channel;
DROP INDEX turn_channel;

CREATE INDEX space_project ON space(project_id);
CREATE INDEX space_feature ON space(feature_id);
CREATE INDEX approval_space ON approval(space_id, status);
CREATE INDEX direct_message_space ON direct_message(space_id, id);
CREATE INDEX message_space ON message(space_id, id);
CREATE INDEX proposal_space ON proposal(space_id);
CREATE INDEX token_space ON token(space_id);
CREATE INDEX turn_space ON turn(space_id);
