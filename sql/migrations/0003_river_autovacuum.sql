-- +goose Up
-- Create river_job table baseline if not yet initialized by river migration
CREATE TABLE IF NOT EXISTS river_job (
    id BIGSERIAL PRIMARY KEY,
    args JSONB NOT NULL,
    attempt SMALLINT NOT NULL DEFAULT 0,
    attempted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    errors JSONB[],
    finalized_at TIMESTAMPTZ,
    kind TEXT NOT NULL,
    max_attempts SMALLINT NOT NULL DEFAULT 10,
    metadata JSONB NOT NULL DEFAULT '{}',
    priority SMALLINT NOT NULL DEFAULT 1,
    queue TEXT NOT NULL DEFAULT 'default',
    state TEXT NOT NULL DEFAULT 'available',
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tags TEXT[] NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_river_job_kind ON river_job (kind);
CREATE INDEX IF NOT EXISTS idx_river_job_state ON river_job (state);

-- Apply aggressive autovacuum tuning to prevent outbox table dead tuple bloat
ALTER TABLE river_job SET (
    autovacuum_vacuum_scale_factor = 0.01,
    autovacuum_vacuum_threshold = 500,
    autovacuum_vacuum_cost_limit = 2000
);

-- +goose Down
DROP TABLE IF EXISTS river_job;
