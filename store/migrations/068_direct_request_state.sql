ALTER TABLE direct_message ADD COLUMN state TEXT NOT NULL DEFAULT 'waiting' CHECK (state IN ('', 'waiting', 'delivered', 'answered', 'failed', 'stopped'));
UPDATE direct_message SET state = '' WHERE reply_to_id IS NOT NULL;
UPDATE direct_message SET state = coalesce(
  (SELECT CASE WHEN r.status = 'done' THEN 'answered' WHEN r.body = 'stopped' THEN 'stopped' ELSE 'failed' END
   FROM direct_message r WHERE r.reply_to_id = direct_message.id),
  'answered')
WHERE reply_to_id IS NULL;
