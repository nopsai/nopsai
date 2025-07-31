CREATE TABLE runs (
    run_id UUID PRIMARY KEY,
    pipeline_definition TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE TABLE steps (
    step_id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
    step_index INT NOT NULL,
    name VARCHAR(255) NOT NULL,
    goal TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    action_taken TEXT,
    execution_log TEXT,
    exit_code INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    UNIQUE(run_id, step_index)
);