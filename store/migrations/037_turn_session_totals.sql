ALTER TABLE turn ADD COLUMN session_cost_usd REAL;
ALTER TABLE turn ADD COLUMN session_input_tokens INTEGER;
ALTER TABLE turn ADD COLUMN session_output_tokens INTEGER;
ALTER TABLE turn ADD COLUMN session_cache_read_tokens INTEGER;
ALTER TABLE turn ADD COLUMN session_cache_write_tokens INTEGER;
