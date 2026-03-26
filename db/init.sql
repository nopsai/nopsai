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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE pipelines (
    id SERIAL PRIMARY KEY,
    path TEXT NOT NULL DEFAULT '',
    name VARCHAR(255) NOT NULL,
    version VARCHAR(255) NOT NULL DEFAULT 'latest',
    definition TEXT NOT NULL,
    source VARCHAR(32) NOT NULL DEFAULT 'database',
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

CREATE TABLE tenants (
    id UUID PRIMARY KEY,
    name TEXT UNIQUE NOT NULL
);

CREATE TABLE user_tenant_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    PRIMARY KEY (user_id, tenant_id, role)
);

CREATE TABLE role_permissions (
    role TEXT NOT NULL,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    obj TEXT NOT NULL,
    act TEXT NOT NULL
);

CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_sub TEXT,
    actor_email TEXT,
    provider TEXT,
    tenant_id UUID,
    action TEXT,
    resource TEXT,
    result TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_tenant ON audit_logs(tenant_id);
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

INSERT INTO tenants (id, name)
VALUES ('00000000-0000-0000-0000-000000000001', 'default')
ON CONFLICT (name) DO NOTHING;

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

INSERT INTO user_tenant_roles (user_id, tenant_id, role)
VALUES (
    '00000000-0000-0000-0000-00000000000a',
    '00000000-0000-0000-0000-000000000001',
    'nopsai-admin'
)
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role, tenant_id, obj, act)
SELECT 'nopsai-admin', '00000000-0000-0000-0000-000000000001', '/*', '.*'
WHERE NOT EXISTS (
    SELECT 1 FROM role_permissions WHERE role = 'nopsai-admin' AND tenant_id = '00000000-0000-0000-0000-000000000001' AND obj = '/*' AND act = '.*'
);
