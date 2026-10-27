UPDATE space SET team_id = NULL WHERE team_id NOT IN (SELECT team_id FROM team_member);
DELETE FROM team WHERE id NOT IN (SELECT team_id FROM team_member);
