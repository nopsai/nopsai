CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE groups (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    kind TEXT NOT NULL DEFAULT 'group' CHECK (kind IN ('group', 'app')),
    description TEXT NOT NULL DEFAULT '',
    repo_url TEXT NOT NULL DEFAULT '',
    repository_full_name TEXT NOT NULL DEFAULT '',
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
    failure_reason TEXT,
    trigger_source TEXT,
    requested_by_type TEXT,
    requested_by_id TEXT,
    effective_subject_type TEXT,
    effective_subject_id TEXT,
    authorization_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb
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

CREATE TABLE pipeline_run_checkpoints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES pipeline_runs(run_id) ON DELETE CASCADE,
    step_name TEXT NOT NULL,
    execution_history TEXT NOT NULL DEFAULT '',
    pipeline_definition TEXT NOT NULL DEFAULT '',
    variables JSONB NOT NULL DEFAULT '{}'::jsonb,
    workspace_archive BYTEA,
    workspace_archive_format TEXT NOT NULL DEFAULT 'tar.gz',
    shared_volume_name TEXT NOT NULL DEFAULT '',
    runner_id TEXT NOT NULL DEFAULT '',
    completed_tasks JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE pipeline_approvals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES pipeline_runs(run_id) ON DELETE CASCADE,
    step_name TEXT NOT NULL,
    task_name TEXT NOT NULL,
    approval_type TEXT NOT NULL,
    assigned_groups JSONB NOT NULL DEFAULT '[]'::jsonb,
    allow_self_approval BOOLEAN NOT NULL DEFAULT FALSE,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    requested_by_type TEXT NOT NULL DEFAULT '',
    requested_by_id TEXT NOT NULL DEFAULT '',
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decided_by_type TEXT NOT NULL DEFAULT '',
    decided_by_id TEXT NOT NULL DEFAULT '',
    decided_by_email TEXT NOT NULL DEFAULT '',
    decided_at TIMESTAMPTZ,
    decision_comment TEXT NOT NULL DEFAULT '',
    checkpoint_id UUID REFERENCES pipeline_run_checkpoints(id) ON DELETE SET NULL,
    UNIQUE(run_id, step_name)
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
    visibility TEXT NOT NULL DEFAULT 'group' CHECK (visibility IN ('group', 'restricted', 'workspace')),
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
    write_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    write_branch TEXT NOT NULL DEFAULT '',
    visibility TEXT NOT NULL DEFAULT 'group' CHECK (visibility IN ('group', 'restricted', 'workspace')),
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
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
    visibility TEXT NOT NULL DEFAULT 'group' CHECK (visibility IN ('group', 'restricted', 'workspace')),
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (path, name)
);

CREATE TABLE pipeline_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    path TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    pipeline_path TEXT NOT NULL DEFAULT '',
    pipeline_name TEXT NOT NULL,
    pipeline_version TEXT NOT NULL DEFAULT 'latest',
    schedule_kind TEXT NOT NULL DEFAULT 'cron' CHECK (schedule_kind IN ('cron', 'once')),
    cron_expression TEXT NOT NULL,
    run_at TIMESTAMPTZ,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    scope TEXT NOT NULL DEFAULT '',
    run_group_path TEXT NOT NULL DEFAULT '',
    variables JSONB NOT NULL DEFAULT '{}'::jsonb,
    next_run_at TIMESTAMPTZ,
    last_run_at TIMESTAMPTZ,
    last_run_id UUID REFERENCES pipeline_runs(run_id) ON DELETE SET NULL,
    last_status TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'database',
    visibility TEXT NOT NULL DEFAULT 'group' CHECK (visibility IN ('group', 'restricted', 'workspace')),
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    created_by TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(path, name)
);

ALTER TABLE pipeline_runs
    ADD COLUMN schedule_id UUID REFERENCES pipeline_schedules(id) ON DELETE SET NULL;

CREATE TABLE steps (
    id SERIAL PRIMARY KEY,
    path TEXT NOT NULL DEFAULT '',
    name VARCHAR(255) NOT NULL,
    definition TEXT NOT NULL,
    source VARCHAR(32) NOT NULL DEFAULT 'database',
    visibility TEXT NOT NULL DEFAULT 'group' CHECK (visibility IN ('group', 'restricted', 'workspace')),
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (path, name)
);

CREATE TABLE knowledge_contexts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind TEXT NOT NULL,
    group_path TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'database',
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(kind, group_path, name)
);

CREATE TABLE secrets (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    value TEXT,
    scope VARCHAR(255) NOT NULL DEFAULT 'default' CONSTRAINT secrets_scope_not_empty CHECK (BTRIM(scope) <> ''),
    repository_name VARCHAR(255),
    source VARCHAR(32) NOT NULL DEFAULT 'database',
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE NULLS NOT DISTINCT (name, repository_name, scope)
);

CREATE TABLE credentials (
    id UUID PRIMARY KEY,
    namespace TEXT NOT NULL DEFAULT 'system',
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'disabled')),
    active_version INTEGER NOT NULL DEFAULT 0 CHECK (active_version >= 0),
    next_version INTEGER NOT NULL DEFAULT 1 CHECK (next_version > 0),
    expires_at TIMESTAMPTZ,
    last_rotated_at TIMESTAMPTZ,
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(namespace, name)
);

CREATE TABLE credential_versions (
    credential_id UUID NOT NULL REFERENCES credentials(id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK (version > 0),
    ciphertext BYTEA NOT NULL,
    wrapped_data_key BYTEA NOT NULL,
    encryption_key_id TEXT NOT NULL,
    encryption_format_version INTEGER NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    activated_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    PRIMARY KEY (credential_id, version)
);

CREATE TABLE credential_access_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    credential_id UUID NOT NULL REFERENCES credentials(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    consumer_service TEXT NOT NULL,
    purpose TEXT NOT NULL,
    subject_type TEXT NOT NULL DEFAULT '',
    subject_id TEXT NOT NULL DEFAULT '',
    correlation_id TEXT NOT NULL DEFAULT '',
    success BOOLEAN NOT NULL,
    error_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_credentials_status ON credentials(status);
CREATE INDEX idx_credentials_config_repo ON credentials(config_repo_id);
CREATE INDEX idx_credential_access_logs_credential_created
    ON credential_access_logs(credential_id, created_at DESC);
CREATE INDEX idx_credential_access_logs_consumer_created
    ON credential_access_logs(consumer_service, created_at DESC);

CREATE TABLE variables (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    value TEXT NOT NULL,
    repository_name VARCHAR(255),
    scope VARCHAR(255) NOT NULL DEFAULT 'default' CONSTRAINT variables_scope_not_empty CHECK (BTRIM(scope) <> ''),
    source VARCHAR(32) NOT NULL DEFAULT 'database',
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE NULLS NOT DISTINCT (name, repository_name, scope)
);

CREATE TABLE llm_profile_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE llm_profiles (
    name TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    model TEXT NOT NULL DEFAULT '',
    base_url TEXT NOT NULL DEFAULT '',
    credential_ref TEXT NOT NULL DEFAULT '',
    allowed_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    reasoning TEXT NOT NULL DEFAULT '',
    thinking BOOLEAN,
    timeout_seconds INTEGER NOT NULL DEFAULT 0,
    max_tokens INTEGER NOT NULL DEFAULT 0,
    temperature DOUBLE PRECISION,
    extra JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE agent_profile_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE agent_profiles (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    instructions TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    source TEXT NOT NULL DEFAULT 'ui',
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agent_profiles_source ON agent_profiles(source);
CREATE INDEX idx_agent_profiles_config_repo ON agent_profiles(config_repo_id);

CREATE TABLE mcp_servers (
    name TEXT PRIMARY KEY,
    display_name TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    provider TEXT NOT NULL DEFAULT '',
    transport TEXT NOT NULL DEFAULT 'streamable_http',
    url TEXT NOT NULL DEFAULT '',
    auth_type TEXT NOT NULL DEFAULT 'none',
    credential_ref TEXT NOT NULL DEFAULT '',
    headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    timeout TEXT NOT NULL DEFAULT '30s',
    allowed_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    last_test_status TEXT NOT NULL DEFAULT '',
    last_test_message TEXT NOT NULL DEFAULT '',
    last_tested_at TIMESTAMPTZ,
    last_discovered_at TIMESTAMPTZ,
    discovered_server_name TEXT NOT NULL DEFAULT '',
    discovered_version TEXT NOT NULL DEFAULT '',
    discovered_protocol TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE mcp_tools (
    server_name TEXT NOT NULL REFERENCES mcp_servers(name) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    input_schema TEXT NOT NULL DEFAULT '{}',
    schema_hash TEXT NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (server_name, name)
);

CREATE TABLE mcp_profiles (
    name TEXT PRIMARY KEY,
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    server_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    allowed_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE resource_visibility (
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    visibility TEXT NOT NULL DEFAULT 'group' CHECK (visibility IN ('group', 'restricted', 'workspace')),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (resource_type, resource_id)
);
CREATE TABLE pipeline_run_logs (
    id SERIAL PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES pipeline_runs(run_id) ON DELETE CASCADE,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    line TEXT NOT NULL
);

CREATE INDEX idx_pipeline_run_logs_run_id ON pipeline_run_logs(run_id);

CREATE TABLE pipeline_run_knowledge_contexts (
    id BIGSERIAL PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES pipeline_runs(run_id) ON DELETE CASCADE,
    knowledge_context_id UUID REFERENCES knowledge_contexts(id) ON DELETE SET NULL,
    kind TEXT NOT NULL,
    group_path TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    ref TEXT NOT NULL DEFAULT '',
    path TEXT NOT NULL DEFAULT '',
    required BOOLEAN NOT NULL DEFAULT FALSE,
    source TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    resolved_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_pipeline_run_knowledge_contexts_run_id ON pipeline_run_knowledge_contexts(run_id);

CREATE TABLE users (
    id UUID PRIMARY KEY,
    sub TEXT UNIQUE NOT NULL,
    email TEXT,
    provider TEXT NOT NULL DEFAULT 'local',
    password_hash TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
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

CREATE TABLE external_triggers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    pipeline TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT '',
    run_group_path TEXT NOT NULL DEFAULT '',
    allowed_callers JSONB NOT NULL DEFAULT '[]'::jsonb,
    variable_mapping JSONB NOT NULL DEFAULT '{}'::jsonb,
    payload_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
    rate_limit JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    source TEXT NOT NULL DEFAULT 'database',
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_external_triggers_pipeline ON external_triggers(pipeline);
CREATE INDEX idx_external_triggers_enabled ON external_triggers(enabled);
CREATE INDEX idx_external_triggers_last_used_at ON external_triggers(last_used_at DESC);
CREATE INDEX idx_external_triggers_config_repo ON external_triggers(config_repo_id);

CREATE TABLE external_trigger_invocations (
    id UUID PRIMARY KEY,
    trigger_id TEXT NOT NULL REFERENCES external_triggers(id) ON DELETE CASCADE,
    caller_type TEXT NOT NULL,
    caller_id TEXT NOT NULL,
    status TEXT NOT NULL,
    run_id UUID REFERENCES pipeline_runs(run_id) ON DELETE SET NULL,
    idempotency_key TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL DEFAULT '',
    source_ip TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    error TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX idx_external_trigger_invocations_active_idempotency
    ON external_trigger_invocations(trigger_id, caller_type, caller_id, idempotency_key)
    WHERE idempotency_key <> '' AND status IN ('pending', 'queued');
CREATE INDEX idx_external_trigger_invocations_trigger_created
    ON external_trigger_invocations(trigger_id, created_at DESC);
CREATE INDEX idx_external_trigger_invocations_run
    ON external_trigger_invocations(run_id);

CREATE TABLE git_webhook_sources (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    auth_mode TEXT NOT NULL,
    credential_ref TEXT NOT NULL DEFAULT '',
    repository_allowlist JSONB NOT NULL DEFAULT '[]'::jsonb,
    rate_limit JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    source TEXT NOT NULL DEFAULT 'database',
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_git_webhook_sources_enabled ON git_webhook_sources(enabled);
CREATE INDEX idx_git_webhook_sources_provider ON git_webhook_sources(provider);
CREATE INDEX idx_git_webhook_sources_config_repo ON git_webhook_sources(config_repo_id);

CREATE TABLE git_webhook_deliveries (
    id UUID PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES git_webhook_sources(id) ON DELETE CASCADE,
    delivery_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    event_type TEXT NOT NULL DEFAULT '',
    repository_full_name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    run_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    error TEXT NOT NULL DEFAULT '',
    source_ip TEXT NOT NULL DEFAULT '',
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    UNIQUE(source_id, delivery_id)
);

CREATE INDEX idx_git_webhook_deliveries_source_received
    ON git_webhook_deliveries(source_id, received_at DESC);
CREATE INDEX idx_git_webhook_deliveries_repository
    ON git_webhook_deliveries(repository_full_name, received_at DESC);
CREATE INDEX idx_git_webhook_deliveries_status
    ON git_webhook_deliveries(status, received_at DESC);

CREATE TABLE notification_mail_settings (
    id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    from_address TEXT NOT NULL DEFAULT '',
    smtp_host TEXT NOT NULL DEFAULT '',
    smtp_port INTEGER NOT NULL DEFAULT 587,
    smtp_start_tls BOOLEAN NOT NULL DEFAULT TRUE,
    smtp_username TEXT NOT NULL DEFAULT '',
    smtp_password_credential_ref TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'database',
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE notification_routes (
    id BIGSERIAL PRIMARY KEY,
    group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    definition JSONB NOT NULL DEFAULT '{}'::jsonb,
    source TEXT NOT NULL DEFAULT 'database',
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    updated_by TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(group_id)
);

CREATE TABLE notification_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID REFERENCES pipeline_runs(run_id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    channel TEXT NOT NULL,
    recipient TEXT NOT NULL,
    status TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    dedupe_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ,
    UNIQUE(dedupe_key)
);

CREATE INDEX idx_notification_routes_group ON notification_routes(group_id);
CREATE INDEX idx_notification_routes_config_repo ON notification_routes(config_repo_id);
CREATE INDEX idx_notification_deliveries_run ON notification_deliveries(run_id);
CREATE INDEX idx_notification_deliveries_status ON notification_deliveries(status, created_at DESC);

CREATE TABLE ai_usage_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID REFERENCES pipeline_runs(run_id) ON DELETE SET NULL,
    step_name TEXT NOT NULL DEFAULT '',
    task_name TEXT NOT NULL DEFAULT '',
    pipeline_path TEXT NOT NULL DEFAULT '',
    pipeline_name TEXT NOT NULL DEFAULT '',
    group_id INTEGER REFERENCES groups(id) ON DELETE SET NULL,
    feature TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    llm_profile TEXT NOT NULL DEFAULT '',
    prompt_tokens BIGINT NOT NULL DEFAULT 0,
    completion_tokens BIGINT NOT NULL DEFAULT 0,
    total_tokens BIGINT NOT NULL DEFAULT 0,
    input_cost_usd NUMERIC(18, 8) NOT NULL DEFAULT 0,
    output_cost_usd NUMERIC(18, 8) NOT NULL DEFAULT 0,
    total_cost_usd NUMERIC(18, 8) NOT NULL DEFAULT 0,
    requested_by_type TEXT NOT NULL DEFAULT '',
    requested_by_id TEXT NOT NULL DEFAULT '',
    effective_subject_type TEXT NOT NULL DEFAULT '',
    effective_subject_id TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ai_usage_events_run ON ai_usage_events(run_id);
CREATE INDEX idx_ai_usage_events_pipeline_created ON ai_usage_events(pipeline_path, pipeline_name, created_at DESC);
CREATE INDEX idx_ai_usage_events_group_created ON ai_usage_events(group_id, created_at DESC);
CREATE INDEX idx_ai_usage_events_feature_created ON ai_usage_events(feature, created_at DESC);
CREATE INDEX idx_ai_usage_events_subject_created ON ai_usage_events(effective_subject_type, effective_subject_id, created_at DESC);

CREATE TABLE runner_metric_snapshots (
    id BIGSERIAL PRIMARY KEY,
    runner_id TEXT NOT NULL,
    runtime TEXT NOT NULL DEFAULT '',
    namespace TEXT NOT NULL DEFAULT '',
    node TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    capacity INTEGER NOT NULL DEFAULT 0,
    active_jobs INTEGER NOT NULL DEFAULT 0,
    inflight_jobs INTEGER NOT NULL DEFAULT 0,
    queued_jobs INTEGER NOT NULL DEFAULT 0,
    allow_dispatch BOOLEAN NOT NULL DEFAULT TRUE,
    sampled_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_runner_metric_snapshots_sampled ON runner_metric_snapshots(sampled_at DESC);
CREATE INDEX idx_runner_metric_snapshots_runner_sampled ON runner_metric_snapshots(runner_id, sampled_at DESC);

CREATE TABLE pipeline_run_usage_summary (
    run_id UUID PRIMARY KEY REFERENCES pipeline_runs(run_id) ON DELETE CASCADE,
    total_runtime_seconds BIGINT NOT NULL DEFAULT 0,
    runner_cost_usd NUMERIC(18, 8) NOT NULL DEFAULT 0,
    ai_prompt_tokens BIGINT NOT NULL DEFAULT 0,
    ai_completion_tokens BIGINT NOT NULL DEFAULT 0,
    ai_total_tokens BIGINT NOT NULL DEFAULT 0,
    ai_cost_usd NUMERIC(18, 8) NOT NULL DEFAULT 0,
    total_cost_usd NUMERIC(18, 8) NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_pipeline_run_usage_summary_total_cost ON pipeline_run_usage_summary(total_cost_usd DESC);

CREATE TABLE monitoring_saved_views (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    owner_subject_type TEXT NOT NULL DEFAULT '',
    owner_subject_id TEXT NOT NULL DEFAULT '',
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'group', 'workspace')),
    group_id INTEGER REFERENCES groups(id) ON DELETE SET NULL,
    filters JSONB NOT NULL DEFAULT '{}'::jsonb,
    columns JSONB NOT NULL DEFAULT '[]'::jsonb,
    source TEXT NOT NULL DEFAULT 'database',
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_monitoring_saved_views_owner ON monitoring_saved_views(owner_subject_type, owner_subject_id);
CREATE INDEX idx_monitoring_saved_views_config_repo ON monitoring_saved_views(config_repo_id);

CREATE TABLE monitoring_alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    owner_subject_type TEXT NOT NULL DEFAULT '',
    owner_subject_id TEXT NOT NULL DEFAULT '',
    visibility TEXT NOT NULL DEFAULT 'workspace' CHECK (visibility IN ('private', 'group', 'workspace')),
    severity TEXT NOT NULL DEFAULT 'warning',
    metric TEXT NOT NULL,
    comparator TEXT NOT NULL DEFAULT 'gt',
    threshold NUMERIC(18, 8) NOT NULL DEFAULT 0,
    window_seconds INTEGER NOT NULL DEFAULT 3600,
    filters JSONB NOT NULL DEFAULT '{}'::jsonb,
    notification_route_id BIGINT REFERENCES notification_routes(id) ON DELETE SET NULL,
    source TEXT NOT NULL DEFAULT 'database',
    config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
    config_source_path TEXT NOT NULL DEFAULT '',
    config_source_commit_sha TEXT NOT NULL DEFAULT '',
    managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_monitoring_alert_rules_enabled ON monitoring_alert_rules(enabled);
CREATE INDEX idx_monitoring_alert_rules_owner ON monitoring_alert_rules(owner_subject_type, owner_subject_id);
CREATE INDEX idx_monitoring_alert_rules_config_repo ON monitoring_alert_rules(config_repo_id);

CREATE TABLE monitoring_alert_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id UUID REFERENCES monitoring_alert_rules(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'firing',
    value NUMERIC(18, 8) NOT NULL DEFAULT 0,
    message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_monitoring_alert_events_rule_created ON monitoring_alert_events(rule_id, created_at DESC);
CREATE INDEX idx_monitoring_alert_events_status_created ON monitoring_alert_events(status, created_at DESC);

CREATE TABLE monitoring_recommendations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fingerprint TEXT NOT NULL UNIQUE,
    category TEXT NOT NULL DEFAULT '',
    severity TEXT NOT NULL DEFAULT 'info',
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'acknowledged', 'resolved')),
    message TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

CREATE INDEX idx_monitoring_recommendations_status_seen ON monitoring_recommendations(status, last_seen_at DESC);

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

CREATE TABLE personal_access_tokens (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT UNIQUE NOT NULL,
    token_suffix TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_personal_access_tokens_user ON personal_access_tokens(user_id);
CREATE INDEX idx_personal_access_tokens_expiry ON personal_access_tokens(expires_at);

CREATE TABLE service_account_tokens (
    id UUID PRIMARY KEY,
    service_account_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT UNIQUE NOT NULL,
    token_suffix TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_service_account_tokens_account ON service_account_tokens(service_account_id);
CREATE INDEX idx_service_account_tokens_expiry ON service_account_tokens(expires_at);

CREATE TABLE auth_groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT UNIQUE NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE auth_group_members (
    group_id UUID NOT NULL REFERENCES auth_groups(id) ON DELETE CASCADE,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('user', 'repository', 'trigger', 'service_account', 'internal_service')),
    subject_id TEXT NOT NULL,
    managed_by_identity_provider BOOLEAN NOT NULL DEFAULT FALSE,
    identity_provider_id TEXT NOT NULL DEFAULT '',
    external_group_name TEXT NOT NULL DEFAULT '',
    auth_group_name TEXT NOT NULL DEFAULT '',
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
    subject_type TEXT NOT NULL CHECK (subject_type IN ('user', 'auth_group', 'repository', 'trigger', 'service_account', 'internal_service')),
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
    subject_type TEXT NOT NULL CHECK (subject_type IN ('user', 'auth_group', 'group', 'repository', 'trigger', 'service_account', 'internal_service')),
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
    subject_type TEXT NOT NULL CHECK (subject_type IN ('user', 'auth_group', 'repository', 'trigger', 'service_account', 'internal_service')),
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
    owner_subject_type TEXT NOT NULL CHECK (owner_subject_type IN ('user', 'auth_group', 'repository', 'trigger', 'service_account', 'internal_service')),
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

CREATE TABLE setup_state (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE data_backups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    backup_type TEXT NOT NULL CHECK (backup_type IN ('full', 'runs', 'logs')),
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'success', 'failure')),
    file_path TEXT NOT NULL DEFAULT '',
    file_name TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT 'application/gzip',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    checksum_sha256 TEXT NOT NULL DEFAULT '',
    requested_by TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE TABLE data_cleanup_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    target TEXT NOT NULL CHECK (target IN ('runs', 'logs')),
    mode TEXT NOT NULL CHECK (mode IN ('keep_last', 'older_than_days', 'all_terminal_runs', 'all_logs')),
    keep_last INT NOT NULL DEFAULT 0,
    older_than_days INT NOT NULL DEFAULT 0,
    backup_before_cleanup BOOLEAN NOT NULL DEFAULT TRUE,
    cron_expression TEXT NOT NULL DEFAULT '0 2 * * 0',
    timezone TEXT NOT NULL DEFAULT 'UTC',
    next_run_at TIMESTAMPTZ,
    last_run_at TIMESTAMPTZ,
    last_job_id UUID,
    last_status TEXT NOT NULL DEFAULT '',
    last_deleted_counts JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_error TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE data_cleanup_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id UUID REFERENCES data_cleanup_schedules(id) ON DELETE SET NULL,
    trigger_type TEXT NOT NULL DEFAULT 'manual' CHECK (trigger_type IN ('manual', 'scheduled')),
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'success', 'failure')),
    target TEXT NOT NULL CHECK (target IN ('runs', 'logs')),
    mode TEXT NOT NULL CHECK (mode IN ('keep_last', 'older_than_days', 'all_terminal_runs', 'all_logs')),
    keep_last INT NOT NULL DEFAULT 0,
    older_than_days INT NOT NULL DEFAULT 0,
    backup_before_cleanup BOOLEAN NOT NULL DEFAULT FALSE,
    backup_id UUID REFERENCES data_backups(id) ON DELETE SET NULL,
    requested_by TEXT NOT NULL DEFAULT '',
    preview_counts JSONB NOT NULL DEFAULT '{}'::jsonb,
    deleted_counts JSONB NOT NULL DEFAULT '{}'::jsonb,
    error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE data_cleanup_schedules
    ADD CONSTRAINT data_cleanup_schedules_last_job_id_fkey
    FOREIGN KEY (last_job_id) REFERENCES data_cleanup_jobs(id) ON DELETE SET NULL;

CREATE INDEX idx_auth_group_members_subject ON auth_group_members(subject_type, subject_id);
CREATE INDEX idx_auth_group_members_identity_provider ON auth_group_members(identity_provider_id, external_group_name) WHERE managed_by_identity_provider = TRUE;
CREATE INDEX idx_auth_role_bindings_subject ON auth_role_bindings(subject_type, subject_id);
CREATE INDEX idx_auth_role_permissions_role_name ON auth_role_permissions(role_name);
CREATE INDEX idx_auth_role_permissions_resource_lookup ON auth_role_permissions(resource_type, resource_id, action);
CREATE INDEX idx_access_grants_subject_lookup ON access_grants(subject_type, subject_id);
CREATE INDEX idx_access_grants_resource_lookup ON access_grants(resource_type, resource_id);
CREATE INDEX idx_resource_acl_resource_lookup ON resource_acl(resource_type, resource_id, action);
CREATE INDEX idx_resource_acl_subject_lookup ON resource_acl(subject_type, subject_id);
CREATE INDEX idx_authz_decision_logs_created_at ON authz_decision_logs(created_at);
CREATE INDEX idx_authz_decision_logs_request_id ON authz_decision_logs(request_id);
CREATE INDEX idx_groups_kind ON groups(kind);
CREATE INDEX idx_groups_repository_full_name ON groups(repository_full_name) WHERE repository_full_name <> '';
CREATE UNIQUE INDEX idx_groups_repository_full_name_unique ON groups(LOWER(repository_full_name)) WHERE repository_full_name <> '';
CREATE INDEX idx_config_repositories_scope ON config_repositories(scope_type, scope_id);
CREATE INDEX idx_config_repositories_config_repo_id ON config_repositories(config_repo_id);
CREATE INDEX idx_pipelines_config_repo_id ON pipelines(config_repo_id);
CREATE INDEX idx_pipeline_schedules_config_repo_id ON pipeline_schedules(config_repo_id);
CREATE INDEX idx_pipeline_schedules_next_run ON pipeline_schedules(enabled, next_run_at);
CREATE INDEX idx_pipeline_schedules_pipeline ON pipeline_schedules(pipeline_path, pipeline_name);
CREATE INDEX idx_pipeline_runs_schedule_id ON pipeline_runs(schedule_id);
CREATE INDEX idx_pipeline_run_checkpoints_run ON pipeline_run_checkpoints(run_id, created_at DESC);
CREATE INDEX idx_pipeline_approvals_run ON pipeline_approvals(run_id, status, requested_at DESC);
CREATE INDEX idx_steps_config_repo_id ON steps(config_repo_id);
CREATE INDEX idx_triggers_config_repo_id ON triggers(config_repo_id);
CREATE INDEX idx_variables_config_repo_id ON variables(config_repo_id);
CREATE INDEX idx_secrets_config_repo_id ON secrets(config_repo_id);
CREATE INDEX idx_data_backups_created_at ON data_backups(created_at DESC);
CREATE INDEX idx_data_backups_status ON data_backups(status, created_at DESC);
CREATE INDEX idx_data_cleanup_jobs_created_at ON data_cleanup_jobs(created_at DESC);
CREATE INDEX idx_data_cleanup_jobs_schedule_id ON data_cleanup_jobs(schedule_id, created_at DESC);
CREATE INDEX idx_data_cleanup_schedules_next_run ON data_cleanup_schedules(enabled, next_run_at);

-- Seed default admin user with password 'admin' (change after first login).
INSERT INTO users (id, sub, email, provider, password_hash, status, must_change_password)
VALUES (
    '00000000-0000-0000-0000-00000000000a',
    'admin',
    'admin@example.com',
    'local',
    '$2a$10$ueFOcGRKCWDeOaTwy1hmQ.WjQ70Yu8JJLcl8ZvJprx7HPKArt8ESC',
    'active',
    TRUE
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
