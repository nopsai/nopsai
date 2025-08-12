CREATE TABLE runs (
    run_id UUID PRIMARY KEY,
    pipeline_name VARCHAR(255),
    pipeline_definition TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    timeout_at TIMESTAMPTZ,
    git_repo_owner VARCHAR(255),
    git_repo_name VARCHAR(255),
    git_clone_url VARCHAR(255),
    git_ssh_url VARCHAR(255),
    git_ref VARCHAR(255),
    git_commit_sha VARCHAR(255),
    git_commit_url TEXT,
    git_commit_message TEXT,
    git_commit_author_name VARCHAR(255),
    git_commit_author_email VARCHAR(255),
    git_commit_author_username VARCHAR(255),
    git_pusher_name VARCHAR(255),
    git_pusher_email VARCHAR(255),
    git_check_run_id BIGINT
);

CREATE TABLE steps (
    step_id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    exit_code INT,
    finished_at TIMESTAMPTZ,
    step_index INT NOT NULL,
    UNIQUE(run_id, name)
);

CREATE TABLE pipeline_overrides (
    id SERIAL PRIMARY KEY,
    repository_name VARCHAR(255) UNIQUE NOT NULL,
    pipeline_definition TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);