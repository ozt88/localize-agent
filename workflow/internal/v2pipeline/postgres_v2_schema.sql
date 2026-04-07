CREATE TABLE IF NOT EXISTS pipeline_items_v2 (
    id TEXT PRIMARY KEY,
    sort_index INTEGER NOT NULL DEFAULT 0,
    source_file TEXT NOT NULL DEFAULT '',
    knot TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT '',
    speaker TEXT NOT NULL DEFAULT '',
    choice TEXT NOT NULL DEFAULT '',
    gate TEXT NOT NULL DEFAULT '',
    source_raw TEXT NOT NULL,
    source_hash TEXT NOT NULL UNIQUE,
    has_tags BOOLEAN NOT NULL DEFAULT FALSE,
    state TEXT NOT NULL,
    ko_raw TEXT,
    ko_formatted TEXT,
    translate_attempts INTEGER NOT NULL DEFAULT 0,
    format_attempts INTEGER NOT NULL DEFAULT 0,
    score_attempts INTEGER NOT NULL DEFAULT 0,
    score_final REAL NOT NULL DEFAULT -1,
    failure_type TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    attempt_log JSONB,
    claimed_by TEXT NOT NULL DEFAULT '',
    claimed_at TIMESTAMPTZ,
    lease_until TIMESTAMPTZ,
    batch_id TEXT NOT NULL DEFAULT '',
    parent_choice_text TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_pv2_state ON pipeline_items_v2(state);
CREATE INDEX IF NOT EXISTS idx_pv2_state_lease ON pipeline_items_v2(state, lease_until);
CREATE INDEX IF NOT EXISTS idx_pv2_source_hash ON pipeline_items_v2(source_hash);
CREATE INDEX IF NOT EXISTS idx_pv2_batch ON pipeline_items_v2(batch_id);

-- Retranslation support (Phase 06)
ALTER TABLE pipeline_items_v2 ADD COLUMN IF NOT EXISTS retranslation_gen INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_pv2_score ON pipeline_items_v2(score_final) WHERE state = 'done';

CREATE TABLE IF NOT EXISTS retranslation_snapshots (
    id TEXT NOT NULL,
    gen INTEGER NOT NULL,
    ko_raw TEXT,
    ko_formatted TEXT,
    score_final REAL,
    snapshot_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, gen)
);
