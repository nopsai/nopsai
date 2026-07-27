package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgconn"
)

type PolicyRunner interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

type ConfigMetadata struct {
	RepoID     int64
	SourcePath string
	CommitSHA  string
}

type RolePermission struct {
	RoleName     string
	ResourceType string
	ResourceID   string
	Action       string
	Effect       string
}

type RoleBinding struct {
	RoleName    string
	SubjectType string
	SubjectID   string
}

type ExternalBindingMetadata struct {
	ProviderID      string
	ExternalGroupID string
	ExternalRoleID  string
}

type ResourceACL struct {
	ResourceType  string
	ResourceID    string
	SubjectType   string
	SubjectID     string
	Action        string
	Effect        string
	AccessGrantID *int64
}

func EnsureRole(ctx context.Context, runner PolicyRunner, name, description string) error {
	_, err := runner.Exec(ctx, `
		INSERT INTO auth_roles (name, description)
		VALUES ($1, $2)
		ON CONFLICT (name) DO NOTHING
	`, name, description)
	return err
}

func UpsertRoleDescription(ctx context.Context, runner PolicyRunner, name, description string) error {
	_, err := runner.Exec(ctx, `
		INSERT INTO auth_roles (name, description)
		VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE
		SET description = EXCLUDED.description,
		    updated_at = NOW()
	`, name, description)
	return err
}

func UpsertManagedRole(ctx context.Context, runner PolicyRunner, name, description string, metadata ConfigMetadata) error {
	_, err := runner.Exec(ctx, `
		INSERT INTO auth_roles (
			name, description,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
		)
		VALUES ($1, $2, $3, $4, $5, TRUE)
		ON CONFLICT (name) DO UPDATE SET
			description = EXCLUDED.description,
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE,
			updated_at = NOW()
	`, name, description, metadata.RepoID, metadata.SourcePath, metadata.CommitSHA)
	return err
}

func DeleteManagedRolesByConfig(ctx context.Context, runner PolicyRunner, configRepoID int64) (pgconn.CommandTag, error) {
	return runner.Exec(ctx, `
		DELETE FROM auth_roles
		WHERE managed_by_config_repo = TRUE
		  AND config_repo_id = $1
	`, configRepoID)
}

func DeleteManagedRolesByConfigExcept(ctx context.Context, runner PolicyRunner, configRepoID int64, keepNames []string) (pgconn.CommandTag, error) {
	return runner.Exec(ctx, `
		DELETE FROM auth_roles
		WHERE managed_by_config_repo = TRUE
		  AND config_repo_id = $1
		  AND name != ALL($2)
	`, configRepoID, keepNames)
}

func DeleteEmptyRole(ctx context.Context, runner PolicyRunner, name string) error {
	_, err := runner.Exec(ctx, `
		DELETE FROM auth_roles
		WHERE name = $1
		  AND NOT EXISTS (SELECT 1 FROM auth_role_permissions WHERE role_name = $1)
		  AND NOT EXISTS (SELECT 1 FROM auth_role_bindings WHERE role_name = $1)
	`, name)
	return err
}

func InsertRolePermission(ctx context.Context, runner PolicyRunner, permission RolePermission) error {
	_, err := runner.Exec(ctx, `
		INSERT INTO auth_role_permissions (role_name, resource_type, resource_id, action, effect)
		VALUES ($1, $2, $3, $4, $5)
	`, permission.RoleName, permission.ResourceType, permission.ResourceID, permission.Action, permission.Effect)
	return err
}

func EnsureRolePermission(ctx context.Context, runner PolicyRunner, permission RolePermission) error {
	_, err := runner.Exec(ctx, `
		INSERT INTO auth_role_permissions (role_name, resource_type, resource_id, action, effect)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (role_name, resource_type, resource_id, action, effect) DO NOTHING
	`, permission.RoleName, permission.ResourceType, permission.ResourceID, permission.Action, permission.Effect)
	return err
}

func UpsertManagedRolePermission(ctx context.Context, runner PolicyRunner, permission RolePermission, metadata ConfigMetadata) error {
	_, err := runner.Exec(ctx, `
		INSERT INTO auth_role_permissions (
			role_name, resource_type, resource_id, action, effect,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, TRUE)
		ON CONFLICT (role_name, resource_type, resource_id, action, effect) DO UPDATE SET
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE
	`, permission.RoleName, permission.ResourceType, permission.ResourceID, permission.Action, permission.Effect, metadata.RepoID, metadata.SourcePath, metadata.CommitSHA)
	return err
}

func DeleteRolePermissionsForRoles(ctx context.Context, runner PolicyRunner, roleNames []string) error {
	_, err := runner.Exec(ctx, `DELETE FROM auth_role_permissions WHERE role_name = ANY($1)`, roleNames)
	return err
}

func DeleteManagedRolePermissionsByConfig(ctx context.Context, runner PolicyRunner, configRepoID int64) error {
	_, err := runner.Exec(ctx, `DELETE FROM auth_role_permissions WHERE managed_by_config_repo = TRUE AND config_repo_id = $1`, configRepoID)
	return err
}

func DeleteRolePermission(ctx context.Context, runner PolicyRunner, permission RolePermission) (pgconn.CommandTag, error) {
	return runner.Exec(ctx, `
		DELETE FROM auth_role_permissions
		WHERE role_name = $1 AND resource_type = $2 AND resource_id = $3 AND action = $4 AND effect = $5
	`, permission.RoleName, permission.ResourceType, permission.ResourceID, permission.Action, permission.Effect)
}

func CreateRoleBinding(ctx context.Context, runner PolicyRunner, binding RoleBinding) error {
	_, err := runner.Exec(ctx, `
		INSERT INTO auth_role_bindings (role_name, subject_type, subject_id, source)
		VALUES ($1, $2, $3, 'local')
	`, binding.RoleName, binding.SubjectType, binding.SubjectID)
	return err
}

func EnsureRoleBinding(ctx context.Context, runner PolicyRunner, binding RoleBinding) error {
	_, err := runner.Exec(ctx, `
		INSERT INTO auth_role_bindings (
			role_name, subject_type, subject_id, source, provider_id, external_group_id, external_role_id
		)
		VALUES ($1, $2, $3, 'local', '', '', '')
		ON CONFLICT (role_name, subject_type, subject_id, source, provider_id, external_group_id, external_role_id) DO NOTHING
	`, binding.RoleName, binding.SubjectType, binding.SubjectID)
	return err
}

func EnsureExternalRoleBinding(ctx context.Context, runner PolicyRunner, binding RoleBinding, metadata ExternalBindingMetadata) error {
	_, err := runner.Exec(ctx, `
		INSERT INTO auth_role_bindings (
			role_name, subject_type, subject_id, source, provider_id, external_group_id, external_role_id
		)
		VALUES ($1, $2, $3, 'idp', $4, $5, $6)
		ON CONFLICT (role_name, subject_type, subject_id, source, provider_id, external_group_id, external_role_id) DO NOTHING
	`, binding.RoleName, binding.SubjectType, binding.SubjectID, metadata.ProviderID, metadata.ExternalGroupID, metadata.ExternalRoleID)
	return err
}

func UpsertManagedRoleBinding(ctx context.Context, runner PolicyRunner, binding RoleBinding, metadata ConfigMetadata) error {
	_, err := runner.Exec(ctx, `
		INSERT INTO auth_role_bindings (
			role_name, subject_type, subject_id,
			source, provider_id, external_group_id, external_role_id,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
		)
		VALUES ($1, $2, $3, 'local', '', '', '', $4, $5, $6, TRUE)
		ON CONFLICT (role_name, subject_type, subject_id, source, provider_id, external_group_id, external_role_id) DO UPDATE SET
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE
	`, binding.RoleName, binding.SubjectType, binding.SubjectID, metadata.RepoID, metadata.SourcePath, metadata.CommitSHA)
	return err
}

func DeleteManagedRoleBindingsByConfig(ctx context.Context, runner PolicyRunner, configRepoID int64) error {
	_, err := runner.Exec(ctx, `DELETE FROM auth_role_bindings WHERE managed_by_config_repo = TRUE AND config_repo_id = $1`, configRepoID)
	return err
}

func DeleteRoleBinding(ctx context.Context, runner PolicyRunner, binding RoleBinding) (pgconn.CommandTag, error) {
	return runner.Exec(ctx, `
		DELETE FROM auth_role_bindings
		WHERE role_name = $1 AND subject_type = $2 AND subject_id = $3
		  AND COALESCE(source, 'local') = 'local'
	`, binding.RoleName, binding.SubjectType, binding.SubjectID)
}

func DeleteExternalRoleBinding(ctx context.Context, runner PolicyRunner, binding RoleBinding, metadata ExternalBindingMetadata) (pgconn.CommandTag, error) {
	return runner.Exec(ctx, `
		DELETE FROM auth_role_bindings
		WHERE role_name = $1
		  AND subject_type = $2
		  AND subject_id = $3
		  AND source = 'idp'
		  AND provider_id = $4
		  AND external_group_id = $5
		  AND external_role_id = $6
	`, binding.RoleName, binding.SubjectType, binding.SubjectID, metadata.ProviderID, metadata.ExternalGroupID, metadata.ExternalRoleID)
}

func DeleteRoleBindingForConfigScope(ctx context.Context, runner PolicyRunner, binding RoleBinding, configRepoID int64) error {
	_, err := runner.Exec(ctx, `
		DELETE FROM auth_role_bindings
		WHERE role_name = $1
		  AND subject_type = $2
		  AND subject_id = $3
		  AND COALESCE(source, 'local') = 'local'
		  AND (managed_by_config_repo = FALSE OR config_repo_id = $4)
	`, binding.RoleName, binding.SubjectType, binding.SubjectID, configRepoID)
	return err
}

func DeleteSubjectRoleBindings(ctx context.Context, runner PolicyRunner, subjectType, subjectID string) error {
	_, err := runner.Exec(ctx, `
		DELETE FROM auth_role_bindings
		WHERE subject_type = $1 AND subject_id = $2
	`, subjectType, subjectID)
	return err
}

func InsertResourceACL(ctx context.Context, runner PolicyRunner, acl ResourceACL) error {
	accessGrantID := nullableAccessGrantID(acl.AccessGrantID)
	_, err := runner.Exec(ctx, `
		INSERT INTO resource_acl (
			resource_type, resource_id, subject_type, subject_id, access_grant_id, action, effect
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, acl.ResourceType, acl.ResourceID, acl.SubjectType, acl.SubjectID, accessGrantID, acl.Action, acl.Effect)
	return err
}

func UpsertResourceACL(ctx context.Context, runner PolicyRunner, acl ResourceACL) error {
	accessGrantID := nullableAccessGrantID(acl.AccessGrantID)
	_, err := runner.Exec(ctx, `
		INSERT INTO resource_acl (
			resource_type, resource_id, subject_type, subject_id, access_grant_id, action, effect
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (resource_type, resource_id, subject_type, subject_id, action, effect)
		DO UPDATE SET access_grant_id = EXCLUDED.access_grant_id
	`, acl.ResourceType, acl.ResourceID, acl.SubjectType, acl.SubjectID, accessGrantID, acl.Action, acl.Effect)
	return err
}

func DeleteResourceACLByAccessGrantID(ctx context.Context, runner PolicyRunner, accessGrantID int64) error {
	_, err := runner.Exec(ctx, `DELETE FROM resource_acl WHERE access_grant_id = $1`, accessGrantID)
	return err
}

func DeleteResourceACLBySubject(ctx context.Context, runner PolicyRunner, subjectType, subjectID string) error {
	_, err := runner.Exec(ctx, `DELETE FROM resource_acl WHERE subject_type = $1 AND subject_id = $2`, subjectType, subjectID)
	return err
}

func nullableAccessGrantID(accessGrantID *int64) any {
	if accessGrantID == nil {
		return nil
	}
	return *accessGrantID
}
