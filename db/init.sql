CREATE TABLE runs (
    run_id UUID PRIMARY KEY,
    pipeline_name VARCHAR(255),
    pipeline_definition TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    timeout_at TIMESTAMPTZ
);

CREATE TABLE steps (
    step_id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    exit_code INT,
    finished_at TIMESTAMPTZ,
    UNIQUE(run_id, name)
);