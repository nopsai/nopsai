CREATE TABLE runs (
    run_id UUID PRIMARY KEY,
    parent_run_id UUID NULL REFERENCES runs(run_id) ON DELETE SET NULL,
    pipeline_name VARCHAR(255),
    pipeline_definition TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
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

CREATE TABLE tasks (
    task_id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
    step_name VARCHAR(255) NOT NULL,
    task_name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    exit_code INT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    task_index INT NOT NULL,
    UNIQUE(run_id, step_name, task_name)
);

CREATE TABLE steps (
    step_id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    exit_code INT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    step_index INT NOT NULL,
    UNIQUE(run_id, name)
);

CREATE TABLE trigger_overrides (
    id SERIAL PRIMARY KEY,
    repository_name VARCHAR(255) UNIQUE NOT NULL,
    trigger_definition TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE pipelines (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    definition TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE reusable_steps (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    definition TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE secrets (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    value TEXT NOT NULL,
    environment VARCHAR(255),
    repository_name VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(name, repository_name, environment)
);

CREATE TABLE environments (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    value TEXT NOT NULL,
    repository_name VARCHAR(255),
    environment VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(name, repository_name, environment)
);