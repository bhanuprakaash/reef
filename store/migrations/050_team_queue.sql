CREATE UNIQUE INDEX work_item_team_busy ON work_item(team_id)
  WHERE parent_id IS NULL AND status IN ('in_progress', 'needs_you');
