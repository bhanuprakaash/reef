ALTER TABLE channel ADD COLUMN role TEXT NOT NULL DEFAULT '';
ALTER TABLE channel ADD COLUMN backend TEXT NOT NULL DEFAULT '';
ALTER TABLE channel ADD COLUMN permission_mode TEXT NOT NULL DEFAULT '';
UPDATE channel SET
  role = COALESCE((SELECT role FROM turn WHERE channel_id = channel.id ORDER BY id DESC LIMIT 1), ''),
  backend = COALESCE((SELECT backend FROM turn WHERE channel_id = channel.id ORDER BY id DESC LIMIT 1), '');
