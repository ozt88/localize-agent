CREATE TABLE IF NOT EXISTS jobs (
    run_id text PRIMARY KEY,
    created_at timestamptz NOT NULL,
    total_ids integer NOT NULL,
    config_json jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS items (
    id text PRIMARY KEY,
    status text NOT NULL,
    ko_json jsonb,
    pack_json jsonb,
    attempts integer NOT NULL DEFAULT 0,
    last_error text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL,
    latency_ms double precision NOT NULL DEFAULT 0,
    source_hash text NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_items_status ON items(status);

CREATE TABLE IF NOT EXISTS pipeline_items (
    id text PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
    sort_index integer NOT NULL DEFAULT 0,
    state text NOT NULL,
    retry_count integer NOT NULL DEFAULT 0,
    score_final double precision NOT NULL DEFAULT -1,
    last_error text NOT NULL DEFAULT '',
    claimed_by text NOT NULL DEFAULT '',
    claimed_at timestamptz,
    lease_until timestamptz,
    updated_at timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_pipeline_items_state ON pipeline_items(state);
CREATE INDEX IF NOT EXISTS idx_pipeline_items_state_lease ON pipeline_items(state, lease_until);

CREATE TABLE IF NOT EXISTS pipeline_worker_stats (
    id bigserial PRIMARY KEY,
    worker_id text NOT NULL,
    role text NOT NULL,
    processed_count integer NOT NULL,
    elapsed_ms bigint NOT NULL,
    started_at timestamptz NOT NULL,
    finished_at timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_pipeline_worker_stats_role_finished
    ON pipeline_worker_stats(role, finished_at DESC);
