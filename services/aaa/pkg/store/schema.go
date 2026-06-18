package store

import (
	"context"
	"errors"
	"fmt"
)

const defaultAdminUserID = "00000000-0000-0000-0000-00000000000a"

var aaaSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS auth_groups (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name TEXT UNIQUE NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS auth_group_members (
		group_id UUID NOT NULL REFERENCES auth_groups(id) ON DELETE CASCADE,
		subject_type TEXT NOT NULL CHECK (subject_type IN ('user', 'repository', 'trigger', 'service_account', 'internal_service')),
		subject_id TEXT NOT NULL,
		managed_by_identity_provider BOOLEAN NOT NULL DEFAULT FALSE,
		identity_provider_id TEXT NOT NULL DEFAULT '',
		external_group_name TEXT NOT NULL DEFAULT '',
		auth_group_name TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (group_id, subject_type, subject_id)
	)`,
	`ALTER TABLE auth_group_members ADD COLUMN IF NOT EXISTS managed_by_identity_provider BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE auth_group_members ADD COLUMN IF NOT EXISTS identity_provider_id TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE auth_group_members ADD COLUMN IF NOT EXISTS external_group_name TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE auth_group_members ADD COLUMN IF NOT EXISTS auth_group_name TEXT NOT NULL DEFAULT ''`,
	`CREATE TABLE IF NOT EXISTS auth_roles (
		name TEXT PRIMARY KEY,
		description TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS auth_role_bindings (
		id BIGSERIAL PRIMARY KEY,
		role_name TEXT NOT NULL REFERENCES auth_roles(name) ON DELETE CASCADE,
		subject_type TEXT NOT NULL CHECK (subject_type IN ('user', 'auth_group', 'repository', 'trigger', 'service_account', 'internal_service')),
		subject_id TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(role_name, subject_type, subject_id)
	)`,
	`CREATE TABLE IF NOT EXISTS auth_role_permissions (
		id BIGSERIAL PRIMARY KEY,
		role_name TEXT NOT NULL REFERENCES auth_roles(name) ON DELETE CASCADE,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL DEFAULT '*',
		action TEXT NOT NULL,
		effect TEXT NOT NULL CHECK (effect IN ('allow', 'deny')),
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(role_name, resource_type, resource_id, action, effect)
	)`,
	`CREATE TABLE IF NOT EXISTS access_grants (
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
	)`,
	`CREATE TABLE IF NOT EXISTS resource_acl (
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
	)`,
	`CREATE TABLE IF NOT EXISTS resource_ownership (
		id BIGSERIAL PRIMARY KEY,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		owner_subject_type TEXT NOT NULL CHECK (owner_subject_type IN ('user', 'auth_group', 'repository', 'trigger', 'service_account', 'internal_service')),
		owner_subject_id TEXT NOT NULL,
		access_grant_id BIGINT REFERENCES access_grants(id) ON DELETE CASCADE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(resource_type, resource_id, owner_subject_type, owner_subject_id)
	)`,
	`ALTER TABLE resource_acl ADD COLUMN IF NOT EXISTS access_grant_id BIGINT REFERENCES access_grants(id) ON DELETE CASCADE`,
	`ALTER TABLE resource_ownership ADD COLUMN IF NOT EXISTS access_grant_id BIGINT REFERENCES access_grants(id) ON DELETE CASCADE`,
	`ALTER TABLE auth_group_members DROP CONSTRAINT IF EXISTS auth_group_members_subject_type_check`,
	`ALTER TABLE auth_group_members ADD CONSTRAINT auth_group_members_subject_type_check CHECK (subject_type IN ('user', 'repository', 'trigger', 'service_account', 'internal_service'))`,
	`ALTER TABLE auth_role_bindings DROP CONSTRAINT IF EXISTS auth_role_bindings_subject_type_check`,
	`ALTER TABLE auth_role_bindings ADD CONSTRAINT auth_role_bindings_subject_type_check CHECK (subject_type IN ('user', 'auth_group', 'repository', 'trigger', 'service_account', 'internal_service'))`,
	`ALTER TABLE access_grants DROP CONSTRAINT IF EXISTS access_grants_subject_type_check`,
	`ALTER TABLE access_grants ADD CONSTRAINT access_grants_subject_type_check CHECK (subject_type IN ('user', 'auth_group', 'group', 'repository', 'trigger', 'service_account', 'internal_service'))`,
	`ALTER TABLE resource_acl DROP CONSTRAINT IF EXISTS resource_acl_subject_type_check`,
	`ALTER TABLE resource_acl ADD CONSTRAINT resource_acl_subject_type_check CHECK (subject_type IN ('user', 'auth_group', 'repository', 'trigger', 'service_account', 'internal_service'))`,
	`ALTER TABLE resource_ownership DROP CONSTRAINT IF EXISTS resource_ownership_owner_subject_type_check`,
	`ALTER TABLE resource_ownership ADD CONSTRAINT resource_ownership_owner_subject_type_check CHECK (owner_subject_type IN ('user', 'auth_group', 'repository', 'trigger', 'service_account', 'internal_service'))`,
	`CREATE TABLE IF NOT EXISTS authz_decision_logs (
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
	)`,
	`CREATE INDEX IF NOT EXISTS idx_auth_group_members_subject ON auth_group_members(subject_type, subject_id)`,
	`CREATE INDEX IF NOT EXISTS idx_auth_group_members_identity_provider ON auth_group_members(identity_provider_id, external_group_name) WHERE managed_by_identity_provider = TRUE`,
	`CREATE INDEX IF NOT EXISTS idx_auth_role_bindings_subject ON auth_role_bindings(subject_type, subject_id)`,
	`CREATE INDEX IF NOT EXISTS idx_auth_role_permissions_role_name ON auth_role_permissions(role_name)`,
	`CREATE INDEX IF NOT EXISTS idx_auth_role_permissions_resource_lookup ON auth_role_permissions(resource_type, resource_id, action)`,
	`CREATE INDEX IF NOT EXISTS idx_access_grants_subject_lookup ON access_grants(subject_type, subject_id)`,
	`CREATE INDEX IF NOT EXISTS idx_access_grants_resource_lookup ON access_grants(resource_type, resource_id)`,
	`CREATE INDEX IF NOT EXISTS idx_resource_acl_resource_lookup ON resource_acl(resource_type, resource_id, action)`,
	`CREATE INDEX IF NOT EXISTS idx_resource_acl_subject_lookup ON resource_acl(subject_type, subject_id)`,
	`CREATE INDEX IF NOT EXISTS idx_authz_decision_logs_created_at ON authz_decision_logs(created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_authz_decision_logs_request_id ON authz_decision_logs(request_id)`,
	`INSERT INTO auth_roles (name, description)
	VALUES
		('nopsai-admin', 'Default platform administrator'),
		('dispatcher-internal', 'Internal dispatcher service permissions')
	ON CONFLICT (name) DO NOTHING`,
	`INSERT INTO auth_role_bindings (role_name, subject_type, subject_id)
	VALUES
		('nopsai-admin', 'user', '` + defaultAdminUserID + `'),
		('dispatcher-internal', 'internal_service', 'dispatcher')
	ON CONFLICT (role_name, subject_type, subject_id) DO NOTHING`,
	`INSERT INTO auth_role_permissions (role_name, resource_type, resource_id, action, effect)
	VALUES
		('nopsai-admin', '*', '*', '*', 'allow'),
		('dispatcher-internal', 'pipeline', '*', 'pipeline.read', 'allow'),
		('dispatcher-internal', 'pipeline', '*', 'pipeline.execute', 'allow'),
		('dispatcher-internal', 'pipeline_run', '*', 'pipeline_run.read', 'allow'),
		('dispatcher-internal', 'pipeline_run', '*', 'pipeline_run.update_status', 'allow'),
		('dispatcher-internal', 'pipeline_run', '*', 'pipeline_run.write_logs', 'allow'),
		('dispatcher-internal', 'pipeline_run', '*', 'pipeline_run.finalize', 'allow'),
		('dispatcher-internal', 'pipeline_run', '*', 'pipeline_run.task_update', 'allow')
	ON CONFLICT (role_name, resource_type, resource_id, action, effect) DO NOTHING`,
}

var aaaConfigMetadataForeignKeyStatements = []string{
	`DO $$
	BEGIN
		IF to_regclass('config_repositories') IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'auth_roles_config_repo_id_fkey') THEN
			ALTER TABLE auth_roles
			ADD CONSTRAINT auth_roles_config_repo_id_fkey
			FOREIGN KEY (config_repo_id) REFERENCES config_repositories(id) ON DELETE SET NULL;
		END IF;
	END $$`,
	`DO $$
	BEGIN
		IF to_regclass('config_repositories') IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'auth_role_bindings_config_repo_id_fkey') THEN
			ALTER TABLE auth_role_bindings
			ADD CONSTRAINT auth_role_bindings_config_repo_id_fkey
			FOREIGN KEY (config_repo_id) REFERENCES config_repositories(id) ON DELETE SET NULL;
		END IF;
	END $$`,
	`DO $$
	BEGIN
		IF to_regclass('config_repositories') IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'auth_role_permissions_config_repo_id_fkey') THEN
			ALTER TABLE auth_role_permissions
			ADD CONSTRAINT auth_role_permissions_config_repo_id_fkey
			FOREIGN KEY (config_repo_id) REFERENCES config_repositories(id) ON DELETE SET NULL;
		END IF;
	END $$`,
}

var aaaConfigMetadataSchemaStatements = []string{
	`ALTER TABLE auth_roles ADD COLUMN IF NOT EXISTS config_repo_id BIGINT`,
	`ALTER TABLE auth_roles ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE auth_roles ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE auth_roles ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE auth_role_bindings ADD COLUMN IF NOT EXISTS config_repo_id BIGINT`,
	`ALTER TABLE auth_role_bindings ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE auth_role_bindings ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE auth_role_bindings ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE auth_role_permissions ADD COLUMN IF NOT EXISTS config_repo_id BIGINT`,
	`ALTER TABLE auth_role_permissions ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE auth_role_permissions ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE auth_role_permissions ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`CREATE INDEX IF NOT EXISTS idx_auth_roles_config_repo_id ON auth_roles(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_auth_role_bindings_config_repo_id ON auth_role_bindings(config_repo_id)`,
	`CREATE INDEX IF NOT EXISTS idx_auth_role_permissions_config_repo_id ON auth_role_permissions(config_repo_id)`,
}

func (s *PGStore) EnsureSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("store is not configured")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin aaa schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range aaaSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply aaa schema statement %d: %w", idx+1, err)
		}
	}
	if err := EnsurePolicyConfigMetadataSchema(ctx, tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit aaa schema transaction: %w", err)
	}
	return nil
}

func EnsurePolicyConfigMetadataSchema(ctx context.Context, runner PolicyRunner) error {
	for idx, stmt := range aaaConfigMetadataSchemaStatements {
		if _, err := runner.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply aaa config metadata column statement %d: %w", idx+1, err)
		}
	}
	for idx, stmt := range aaaConfigMetadataForeignKeyStatements {
		if _, err := runner.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply aaa config metadata schema statement %d: %w", idx+1, err)
		}
	}
	return nil
}
