ALTER TABLE direct_message ADD COLUMN state TEXT NOT NULL DEFAULT 'waiting' CHECK (state IN ('', 'waiting', 'delivered', 'answered', 'failed', 'stopped'));
