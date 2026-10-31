CREATE TABLE team_member_new (
  id      INTEGER PRIMARY KEY,
  team_id INTEGER NOT NULL REFERENCES team(id) ON DELETE CASCADE,
  bot_id  INTEGER NOT NULL REFERENCES bot(id),
  lead    INTEGER NOT NULL DEFAULT 0 CHECK (lead IN (0, 1)),
  UNIQUE (team_id, bot_id)
);
INSERT INTO team_member_new (id, team_id, bot_id, lead)
  SELECT id, team_id, bot_id, lead FROM team_member;
DROP TABLE team_member;
ALTER TABLE team_member_new RENAME TO team_member;
CREATE UNIQUE INDEX team_member_one_lead ON team_member(team_id) WHERE lead = 1;

CREATE TABLE space_member_new (
  id       INTEGER PRIMARY KEY,
  space_id INTEGER NOT NULL REFERENCES space(id) ON DELETE CASCADE,
  bot_id   INTEGER NOT NULL REFERENCES bot(id),
  lead     INTEGER NOT NULL DEFAULT 0 CHECK (lead IN (0, 1)),
  UNIQUE (space_id, bot_id)
);
INSERT INTO space_member_new (id, space_id, bot_id, lead)
  SELECT id, space_id, bot_id, lead FROM space_member;
DROP TABLE space_member;
ALTER TABLE space_member_new RENAME TO space_member;
CREATE UNIQUE INDEX space_member_one_lead ON space_member(space_id) WHERE lead = 1;
