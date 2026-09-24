CREATE TABLE IF NOT EXISTS update_requests (
    id              TEXT PRIMARY KEY,
    pair            TEXT NOT NULL,
    status          TEXT NOT NULL,
    rate            NUMERIC,
    observed_at     TIMESTAMPTZ,
    error           TEXT NOT NULL DEFAULT '',
    attempts        INTEGER NOT NULL DEFAULT 0,
    idempotency_key TEXT,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,
    CONSTRAINT update_requests_status_check
        CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
);

CREATE INDEX IF NOT EXISTS update_requests_pending_created_idx
    ON update_requests (created_at)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS update_requests_processing_updated_idx
    ON update_requests (updated_at)
    WHERE status = 'processing';

CREATE UNIQUE INDEX IF NOT EXISTS update_requests_idempotency_key_idx
    ON update_requests (idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';

CREATE TABLE IF NOT EXISTS quotes (
    pair        TEXT PRIMARY KEY,
    rate        NUMERIC NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);
