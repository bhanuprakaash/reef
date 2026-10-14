CREATE TABLE team (
  id   INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE
);
CREATE TABLE role (
  id              INTEGER PRIMARY KEY,
  team_id         INTEGER NOT NULL REFERENCES team(id) ON DELETE CASCADE,
  name            TEXT NOT NULL,
  persona         TEXT NOT NULL DEFAULT '',
  skills          TEXT NOT NULL DEFAULT '',
  tools           TEXT NOT NULL DEFAULT '[]',
  mcp_servers     TEXT NOT NULL DEFAULT '[]',
  backend         TEXT NOT NULL,
  permission_mode TEXT NOT NULL DEFAULT '',
  lead            INTEGER NOT NULL DEFAULT 0 CHECK (lead IN (0, 1)),
  UNIQUE (team_id, name)
);
CREATE UNIQUE INDEX role_one_lead ON role(team_id) WHERE lead = 1;
ALTER TABLE channel ADD COLUMN team_id INTEGER REFERENCES team(id);
INSERT INTO team (name) VALUES ('solo');
INSERT INTO role (team_id, name, backend, lead)
  SELECT id, 'lead', 'claude', 1 FROM team WHERE name = 'solo';
UPDATE channel SET team_id = (SELECT id FROM team WHERE name = 'solo');
