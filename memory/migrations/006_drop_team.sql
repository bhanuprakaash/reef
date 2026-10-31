DELETE FROM memory_link WHERE from_id IN (SELECT id FROM memory WHERE scope_kind = 'team')
    OR (to_kind = 'memory' AND to_id IN (SELECT CAST(id AS TEXT) FROM memory WHERE scope_kind = 'team'));
UPDATE memory SET replaced_by = NULL
    WHERE replaced_by IN (SELECT id FROM memory WHERE scope_kind = 'team');
DELETE FROM memory WHERE scope_kind = 'team';
