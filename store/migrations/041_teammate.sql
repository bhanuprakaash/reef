DROP TABLE role;

CREATE TABLE teammate (
  id              INTEGER PRIMARY KEY,
  name            TEXT NOT NULL UNIQUE,
  persona         TEXT NOT NULL DEFAULT '',
  skills          TEXT NOT NULL DEFAULT '[]',
  backend         TEXT NOT NULL,
  model           TEXT NOT NULL DEFAULT '',
  provider        TEXT NOT NULL DEFAULT '',
  face            TEXT NOT NULL DEFAULT '',
  tools           TEXT NOT NULL DEFAULT '[]',
  mcp_servers     TEXT NOT NULL DEFAULT '[]',
  write           INTEGER NOT NULL DEFAULT 0 CHECK (write IN (0, 1)),
  permission_mode TEXT NOT NULL DEFAULT ''
);

CREATE TABLE team_member (
  id              INTEGER PRIMARY KEY,
  team_id         INTEGER NOT NULL REFERENCES team(id) ON DELETE CASCADE,
  teammate_id     INTEGER NOT NULL REFERENCES teammate(id),
  name            TEXT NOT NULL,
  lead            INTEGER NOT NULL DEFAULT 0 CHECK (lead IN (0, 1)),
  tools           TEXT NOT NULL DEFAULT '[]',
  mcp_servers     TEXT NOT NULL DEFAULT '[]',
  write           INTEGER NOT NULL DEFAULT 0 CHECK (write IN (0, 1)),
  permission_mode TEXT NOT NULL DEFAULT '',
  UNIQUE (team_id, teammate_id),
  UNIQUE (team_id, name)
);
CREATE UNIQUE INDEX team_member_one_lead ON team_member(team_id) WHERE lead = 1;

INSERT INTO teammate (name, backend, write) VALUES ('lead', 'claude', 1);
INSERT INTO team_member (team_id, teammate_id, name, lead, write)
  SELECT team.id, teammate.id, 'lead', 1, 1 FROM team, teammate
  WHERE team.name = 'solo' AND teammate.name = 'lead';
