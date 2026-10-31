ALTER TABLE teammate RENAME TO bot;
ALTER TABLE team_member RENAME COLUMN teammate_id TO bot_id;
ALTER TABLE space_member RENAME COLUMN teammate_id TO bot_id;
ALTER TABLE bot RENAME COLUMN persona TO description;
