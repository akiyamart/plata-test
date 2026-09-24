ALTER TABLE update_requests
    ADD COLUMN IF NOT EXISTS correlation_id TEXT NOT NULL DEFAULT '';
