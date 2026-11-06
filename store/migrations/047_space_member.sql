CREATE TABLE space_member (
  id              INTEGER PRIMARY KEY,
  space_id        INTEGER NOT NULL REFERENCES space(id) ON DELETE CASCADE,
  teammate_id     INTEGER NOT NULL REFERENCES teammate(id),
  name            TEXT NOT NULL,
  lead            INTEGER NOT NULL DEFAULT 0 CHECK (lead IN (0, 1)),
  tools           TEXT NOT NULL DEFAULT '[]',
  mcp_servers     TEXT NOT NULL DEFAULT '[]',
  write           INTEGER NOT NULL DEFAULT 0 CHECK (write IN (0, 1)),
  permission_mode TEXT NOT NULL DEFAULT '',
  UNIQUE (space_id, teammate_id),
  UNIQUE (space_id, name)
);
CREATE UNIQUE INDEX space_member_one_lead ON space_member(space_id) WHERE lead = 1;

INSERT INTO space_member (space_id, teammate_id, name, lead, tools, mcp_servers, write, permission_mode)
  SELECT space.id, member.teammate_id, member.name, member.lead, member.tools, member.mcp_servers,
         member.write, member.permission_mode
  FROM space JOIN team_member member ON member.team_id = space.team_id;

ALTER TABLE space ADD COLUMN kind TEXT NOT NULL DEFAULT 'free'
  CHECK (kind IN ('work_item', 'free', 'conversation'));
UPDATE space SET kind = 'work_item' WHERE work_item_id IS NOT NULL;

UPDATE space SET closed_at = created_at
  WHERE work_item_id IS NOT NULL AND closed_at IS NULL
    AND id <> (SELECT MAX(s.id) FROM space s WHERE s.work_item_id = space.work_item_id AND s.closed_at IS NULL);
CREATE UNIQUE INDEX space_one_active ON space(work_item_id)
  WHERE work_item_id IS NOT NULL AND closed_at IS NULL;
