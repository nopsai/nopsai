CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE groups (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    parent_id INTEGER REFERENCES groups(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(name)
);

CREATE TABLE pipeline_runs (
    run_id UUID PRIMARY KEY,
    parent_run_id UUID NULL REFERENCES pipeline_runs(run_id) ON DELETE SET NULL,
    parent_step_name VARCHAR(255),
    trigger_event_id VARCHAR(255),
    pipeline_name VARCHAR(255),
    pipeline_path TEXT NOT NULL DEFAULT '',
    pipeline_version VARCHAR(255) NOT NULL DEFAULT 'latest',
    pipeline_definition TEXT,
    pipeline_source VARCHAR(20),
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
    git_target_ref VARCHAR(255),
    git_commit_sha VARCHAR(255),
    git_commit_url TEXT,
    git_commit_message TEXT,
    git_commit_author_name VARCHAR(255),
    git_commit_author_email VARCHAR(255),
    git_commit_author_username VARCHAR(255),
    git_pusher_name VARCHAR(255),
    git_pusher_email VARCHAR(255),
    git_check_run_id BIGINT,
    group_id INTEGER REFERENCES groups(id) ON DELETE SET NULL,
    scope VARCHAR(255),
    failure_reason TEXT
);

CREATE TABLE task_runs (
    task_id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES pipeline_runs(run_id) ON DELETE CASCADE,
    step_name VARCHAR(255) NOT NULL,
    task_name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    exit_code INT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    task_index INT NOT NULL,
    UNIQUE(run_id, step_name, task_name)
);

CREATE TABLE step_runs (
    step_id UUID PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES pipeline_runs(run_id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    exit_code INT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    step_index INT NOT NULL,
    UNIQUE(run_id, name)
);

CREATE TABLE triggers (
    id SERIAL PRIMARY KEY,
    repository_name VARCHAR(255) UNIQUE NOT NULL,
    trigger_definition TEXT NOT NULL,
    source VARCHAR(32) NOT NULL DEFAULT 'database',
    config_repo_id BIGINT,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS config_repositories (
    id BIGSERIAL PRIMARY KEY,
    scope_type TEXT NOT NULL CHECK (scope_type IN ('folder', 'system')),
    scope_id TEXT NOT NULL,
    repo_url TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT 'main',
    base_path TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_sync_status TEXT NOT NULL DEFAULT '',
    last_sync_message TEXT NOT NULL DEFAULT '',
    last_sync_started_at TIMESTAMPTZ,
    last_sync_completed_at TIMESTAMPTZ,
    last_sync_commit_sha TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(scope_type, scope_id),
    UNIQUE(repo_url, branch, base_path)
);

ALTER TABLE triggers
    ADD CONSTRAINT triggers_config_repo_id_fkey
    FOREIGN KEY (config_repo_id) REFERENCES config_repositories(id) ON DELETE SET NULL;

CREATE TABLE pipelines (
    id SERIAL PRIMARY KEY,
    path TEXT NOT NULL DEFAULT '',
    name VARCHAR(255) NOT NULL,
    version VARCHAR(255) NOT NULL DEFAULT 'latest',
    definition TEXT NOT NULL,
    source VARCHAR(32) NOT NULL DEFAULT 'database',
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (path, name)
);

CREATE TABLE steps (
    id SERIAL PRIMARY KEY,
    path TEXT NOT NULL DEFAULT '',
    name VARCHAR(255) NOT NULL,
    definition TEXT NOT NULL,
    source VARCHAR(32) NOT NULL DEFAULT 'database',
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (path, name)
);

CREATE TABLE secrets (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    value TEXT NOT NULL,
    scope VARCHAR(255),
    repository_name VARCHAR(255),
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE NULLS NOT DISTINCT (name, repository_name, scope)
);

CREATE TABLE variables (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    value TEXT NOT NULL,
    repository_name VARCHAR(255),
    scope VARCHAR(255),
    source VARCHAR(32) NOT NULL DEFAULT 'database',
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE NULLS NOT DISTINCT (name, repository_name, scope)
);
CREATE TABLE pipeline_run_logs (
    id SERIAL PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES pipeline_runs(run_id) ON DELETE CASCADE,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    line TEXT NOT NULL
);

CREATE INDEX idx_pipeline_run_logs_run_id ON pipeline_run_logs(run_id);

CREATE TABLE users (
    id UUID PRIMARY KEY,
    sub TEXT UNIQUE NOT NULL,
    email TEXT,
    provider TEXT NOT NULL DEFAULT 'local',
    password_hash TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    last_login TIMESTAMPTZ
);

CREATE TABLE user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    PRIMARY KEY (user_id, role)
);

CREATE TABLE role_permissions (
    role TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    obj TEXT NOT NULL,
    act TEXT NOT NULL
);

CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_sub TEXT,
    actor_email TEXT,
    provider TEXT,
    action TEXT,
    resource TEXT,
    result TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at);

CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_expiry ON refresh_tokens(expires_at);

CREATE TABLE auth_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT UNIQUE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE auth_group_members (
    group_id UUID NOT NULL REFERENCES auth_groups(id) ON DELETE CASCADE,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('user', 'internal_service')),
    subject_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, subject_type, subject_id)
);

CREATE TABLE auth_roles (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE auth_role_bindings (
    id BIGSERIAL PRIMARY KEY,
    role_name TEXT NOT NULL REFERENCES auth_roles(name) ON DELETE CASCADE,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('user', 'auth_group', 'internal_service')),
    subject_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(role_name, subject_type, subject_id)
);

CREATE TABLE auth_role_permissions (
    id BIGSERIAL PRIMARY KEY,
    role_name TEXT NOT NULL REFERENCES auth_roles(name) ON DELETE CASCADE,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL DEFAULT '*',
    action TEXT NOT NULL,
    effect TEXT NOT NULL CHECK (effect IN ('allow', 'deny')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(role_name, resource_type, resource_id, action, effect)
);

CREATE TABLE access_grants (
    id BIGSERIAL PRIMARY KEY,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('user', 'auth_group', 'internal_service')),
    subject_id TEXT NOT NULL,
    subject_display TEXT NOT NULL DEFAULT '',
    role_name TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    resource_display TEXT NOT NULL DEFAULT '',
    inherit BOOLEAN NOT NULL DEFAULT TRUE,
    granted_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(subject_type, subject_id, resource_type, resource_id)
);

CREATE TABLE resource_acl (
    id BIGSERIAL PRIMARY KEY,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('user', 'auth_group', 'internal_service')),
    subject_id TEXT NOT NULL,
    access_grant_id BIGINT REFERENCES access_grants(id) ON DELETE CASCADE,
    action TEXT NOT NULL,
    effect TEXT NOT NULL CHECK (effect IN ('allow', 'deny')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(resource_type, resource_id, subject_type, subject_id, action, effect)
);

CREATE TABLE resource_ownership (
    id BIGSERIAL PRIMARY KEY,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    owner_subject_type TEXT NOT NULL CHECK (owner_subject_type IN ('user', 'auth_group', 'internal_service')),
    owner_subject_id TEXT NOT NULL,
    access_grant_id BIGINT REFERENCES access_grants(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(resource_type, resource_id, owner_subject_type, owner_subject_id)
);

CREATE TABLE authz_decision_logs (
    id BIGSERIAL PRIMARY KEY,
    request_id TEXT,
    subject_type TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    allowed BOOLEAN NOT NULL,
    reason TEXT NOT NULL,
    matched_policy JSONB,
    sensitive BOOLEAN NOT NULL DEFAULT FALSE,
    context JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_auth_group_members_subject ON auth_group_members(subject_type, subject_id);
CREATE INDEX idx_auth_role_bindings_subject ON auth_role_bindings(subject_type, subject_id);
CREATE INDEX idx_auth_role_permissions_role_name ON auth_role_permissions(role_name);
CREATE INDEX idx_auth_role_permissions_resource_lookup ON auth_role_permissions(resource_type, resource_id, action);
CREATE INDEX idx_access_grants_subject_lookup ON access_grants(subject_type, subject_id);
CREATE INDEX idx_access_grants_resource_lookup ON access_grants(resource_type, resource_id);
CREATE INDEX idx_resource_acl_resource_lookup ON resource_acl(resource_type, resource_id, action);
CREATE INDEX idx_resource_acl_subject_lookup ON resource_acl(subject_type, subject_id);
CREATE INDEX idx_authz_decision_logs_created_at ON authz_decision_logs(created_at);
CREATE INDEX idx_authz_decision_logs_request_id ON authz_decision_logs(request_id);
CREATE INDEX idx_config_repositories_scope ON config_repositories(scope_type, scope_id);
CREATE INDEX idx_pipelines_config_repo_id ON pipelines(config_repo_id);
CREATE INDEX idx_steps_config_repo_id ON steps(config_repo_id);
CREATE INDEX idx_triggers_config_repo_id ON triggers(config_repo_id);
CREATE INDEX idx_variables_config_repo_id ON variables(config_repo_id);
CREATE INDEX idx_secrets_config_repo_id ON secrets(config_repo_id);

-- Seed default admin user with password 'admin' (change after first login).
INSERT INTO users (id, sub, email, provider, password_hash, status)
VALUES (
    '00000000-0000-0000-0000-00000000000a',
    'admin',
    'admin@example.com',
    'local',
    '$2a$10$ueFOcGRKCWDeOaTwy1hmQ.WjQ70Yu8JJLcl8ZvJprx7HPKArt8ESC',
    'active'
)
ON CONFLICT (sub) DO NOTHING;

INSERT INTO user_roles (user_id, role)
VALUES (
    '00000000-0000-0000-0000-00000000000a',
    'nopsai-admin'
)
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role, name, obj, act)
SELECT 'nopsai-admin', 'All access', '/*', '.*'
WHERE NOT EXISTS (
    SELECT 1 FROM role_permissions WHERE role = 'nopsai-admin' AND obj = '/*' AND act = '.*'
);

INSERT INTO auth_roles (name, description)
VALUES
    ('nopsai-admin', 'Default platform administrator'),
    ('dispatcher-internal', 'Internal dispatcher service permissions')
ON CONFLICT (name) DO NOTHING;

INSERT INTO auth_role_bindings (role_name, subject_type, subject_id)
VALUES
    ('nopsai-admin', 'user', '00000000-0000-0000-0000-00000000000a'),
    ('dispatcher-internal', 'internal_service', 'dispatcher')
ON CONFLICT (role_name, subject_type, subject_id) DO NOTHING;

INSERT INTO auth_role_permissions (role_name, resource_type, resource_id, action, effect)
VALUES
    ('nopsai-admin', '*', '*', '*', 'allow'),
    -- Dispatcher still orchestrates pipeline fetch, child pipeline execution, and run status polling.
    ('dispatcher-internal', 'pipeline', '*', 'pipeline.read', 'allow'),
    ('dispatcher-internal', 'pipeline', '*', 'pipeline.execute', 'allow'),
    ('dispatcher-internal', 'pipeline_run', '*', 'pipeline_run.read', 'allow'),
    ('dispatcher-internal', 'pipeline_run', '*', 'pipeline_run.update_status', 'allow'),
    ('dispatcher-internal', 'pipeline_run', '*', 'pipeline_run.write_logs', 'allow'),
    ('dispatcher-internal', 'pipeline_run', '*', 'pipeline_run.finalize', 'allow'),
    ('dispatcher-internal', 'pipeline_run', '*', 'pipeline_run.task_update', 'allow')
ON CONFLICT (role_name, resource_type, resource_id, action, effect) DO NOTHING;
