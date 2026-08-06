package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
	aaastore "nopsai/services/aaa/pkg/store"
	"nopsai/services/nopsai/pkg/auth"
)

func (a *App) syncAccessConfiguration(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, plan accessSyncPlan, commitSHA string, details map[string]int) error {
	if err := resetManagedAccessLinks(ctx, tx, binding.ID); err != nil {
		return err
	}

	for _, user := range plan.users {
		if err := upsertAccessUser(ctx, tx, binding, user, commitSHA); err != nil {
			return err
		}
		details["access_users_synced"]++
	}
	if err := pruneStaleAccessGrantsForManagedUsers(ctx, tx, plan.users); err != nil {
		return err
	}
	for _, serviceAccount := range plan.serviceAccounts {
		if err := upsertAccessServiceAccount(ctx, tx, binding, serviceAccount, commitSHA); err != nil {
			return err
		}
		details["access_service_accounts_synced"]++
	}
	for _, role := range plan.roles {
		if err := upsertAccessRole(ctx, tx, binding, role, commitSHA); err != nil {
			return err
		}
		details["access_roles_synced"]++
	}
	for _, policy := range plan.policies {
		if err := upsertAccessPolicy(ctx, tx, binding, policy, commitSHA); err != nil {
			return err
		}
		details["access_policies_synced"]++
	}
	for _, roleBinding := range plan.roleBindings {
		if err := upsertAccessRoleBinding(ctx, tx, binding, roleBinding, commitSHA); err != nil {
			return err
		}
		details["access_role_bindings_synced"]++
	}

	if err := syncResourceVisibilities(ctx, tx, plan, details); err != nil {
		return err
	}

	grantKeys, err := a.syncAccessGrants(ctx, tx, binding, plan, commitSHA, details)
	if err != nil {
		return err
	}
	if err := pruneManagedAccessConfiguration(ctx, tx, binding, plan, grantKeys); err != nil {
		return err
	}
	if err := clearResourceAccessOverridesForConfigSync(ctx, tx, binding); err != nil {
		return err
	}
	return nil
}

func resetManagedAccessLinks(ctx context.Context, tx pgx.Tx, configRepoID int64) error {
	statements := []string{
		`DELETE FROM user_roles WHERE managed_by_config_repo = TRUE AND config_repo_id = $1`,
		`DELETE FROM role_permissions WHERE managed_by_config_repo = TRUE AND config_repo_id = $1`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(ctx, stmt, configRepoID); err != nil {
			return fmt.Errorf("failed to reset managed access links: %w", err)
		}
	}
	if err := aaastore.DeleteManagedRoleBindingsByConfig(ctx, tx, configRepoID); err != nil {
		return fmt.Errorf("failed to reset managed access links: %w", err)
	}
	if err := aaastore.DeleteManagedRolePermissionsByConfig(ctx, tx, configRepoID); err != nil {
		return fmt.Errorf("failed to reset managed access links: %w", err)
	}
	return nil
}

func upsertAccessUser(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, user storedAccessUser, commitSHA string) error {
	if err := ensureGlobalConfigObjectWritable(ctx, tx, binding, "users", "user", user.sub, "sub = $1", user.sub); err != nil {
		return err
	}
	passwordHash := strings.TrimSpace(user.passwordHash)
	if passwordHash == "" && strings.TrimSpace(user.password) != "" {
		hashed, err := auth.HashPassword(user.password)
		if err != nil {
			return fmt.Errorf("failed to hash password for user %q: %w", user.sub, err)
		}
		passwordHash = hashed
	}
	userID := uuid.New()
	if user.id != "" {
		parsedID, err := uuid.Parse(user.id)
		if err != nil {
			return fmt.Errorf("user %q has invalid id: %w", user.sub, err)
		}
		userID = parsedID
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO users (
			id, sub, email, provider, password_hash, status,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
		)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8, $9, TRUE)
		ON CONFLICT (sub) DO UPDATE SET
			email = EXCLUDED.email,
			provider = EXCLUDED.provider,
			password_hash = COALESCE(EXCLUDED.password_hash, users.password_hash),
			status = EXCLUDED.status,
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE
	`, userID, user.sub, user.email, user.provider, passwordHash, user.status, binding.ID, user.sourcePath, commitSHA)
	if err != nil {
		return fmt.Errorf("failed to upsert user %q: %w", user.sub, err)
	}
	return nil
}

func upsertAccessServiceAccount(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, serviceAccount storedAccessServiceAccount, commitSHA string) error {
	if err := ensureGlobalConfigObjectWritable(ctx, tx, binding, "users", "service account", serviceAccount.sub, "sub = $1", serviceAccount.sub); err != nil {
		return err
	}
	if err := ensureServiceAccountSubWritable(ctx, tx, serviceAccount.sub); err != nil {
		return err
	}
	serviceAccountID := uuid.New()
	if serviceAccount.id != "" {
		parsedID, err := uuid.Parse(serviceAccount.id)
		if err != nil {
			return fmt.Errorf("service account %q has invalid id: %w", serviceAccount.sub, err)
		}
		serviceAccountID = parsedID
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO users (
			id, sub, email, provider, password_hash, status, must_change_password,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
		)
		VALUES ($1, $2, $3, $4, NULL, $5, FALSE, $6, $7, $8, TRUE)
		ON CONFLICT (sub) DO UPDATE SET
			email = EXCLUDED.email,
			provider = EXCLUDED.provider,
			password_hash = NULL,
			status = EXCLUDED.status,
			must_change_password = FALSE,
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE
	`, serviceAccountID, serviceAccount.sub, serviceAccount.email, auth.ProviderServiceAccount, serviceAccount.status, binding.ID, serviceAccount.sourcePath, commitSHA)
	if err != nil {
		return fmt.Errorf("failed to upsert service account %q: %w", serviceAccount.sub, err)
	}
	return nil
}

func ensureServiceAccountSubWritable(ctx context.Context, tx pgx.Tx, sub string) error {
	var provider string
	err := tx.QueryRow(ctx, `SELECT provider FROM users WHERE sub = $1 LIMIT 1`, sub).Scan(&provider)
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return err
	case provider == auth.ProviderServiceAccount:
		return nil
	default:
		return fmt.Errorf("service account sub %q is already used by a %q user", sub, provider)
	}
}

func upsertAccessRole(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, role storedAccessRole, commitSHA string) error {
	if err := ensureGlobalConfigObjectWritable(ctx, tx, binding, "auth_roles", "role", role.name, "name = $1", role.name); err != nil {
		return err
	}
	if err := aaastore.UpsertManagedRole(ctx, tx, role.name, role.description, aaastore.ConfigMetadata{
		RepoID:     binding.ID,
		SourcePath: role.sourcePath,
		CommitSHA:  commitSHA,
	}); err != nil {
		return fmt.Errorf("failed to upsert role %q: %w", role.name, err)
	}
	return nil
}

func upsertAccessPolicy(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, policy storedAccessPolicy, commitSHA string) error {
	if err := ensureAccessRolePrepared(ctx, tx, binding, policy.role, policy.sourcePath, commitSHA); err != nil {
		return err
	}
	if err := ensureGlobalConfigObjectWritable(ctx, tx, binding, "auth_role_permissions", "role policy", policy.role, "role_name = $1 AND resource_type = $2 AND resource_id = $3 AND action = $4 AND effect = $5", policy.role, policy.resourceType, policy.resourceID, policy.action, policy.effect); err != nil {
		return err
	}
	if err := aaastore.UpsertManagedRolePermission(ctx, tx, aaastore.RolePermission{
		RoleName:     policy.role,
		ResourceType: policy.resourceType,
		ResourceID:   policy.resourceID,
		Action:       policy.action,
		Effect:       policy.effect,
	}, aaastore.ConfigMetadata{RepoID: binding.ID, SourcePath: policy.sourcePath, CommitSHA: commitSHA}); err != nil {
		return fmt.Errorf("failed to upsert role policy for %q: %w", policy.role, err)
	}

	objectValue := formatAdminPermissionObject(policy.resourceType, policy.resourceID)
	actionValue := formatAdminPermissionAction(policy.effect, policy.action)
	displayName := adminPermissionDisplayName(policy.name, objectValue, actionValue)
	if err := ensureGlobalConfigObjectWritable(ctx, tx, binding, "role_permissions", "role policy metadata", policy.role, "role = $1 AND obj = $2 AND act = $3", policy.role, objectValue, actionValue); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role = $1 AND obj = $2 AND act = $3`, policy.role, objectValue, actionValue); err != nil {
		return fmt.Errorf("failed to refresh role policy metadata: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO role_permissions (
			role, name, obj, act,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, TRUE)
	`, policy.role, displayName, objectValue, actionValue, binding.ID, policy.sourcePath, commitSHA); err != nil {
		return fmt.Errorf("failed to upsert role policy metadata for %q: %w", policy.role, err)
	}
	return nil
}

func upsertAccessRoleBinding(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, roleBinding storedAccessRoleBinding, commitSHA string) error {
	if err := ensureAccessRolePrepared(ctx, tx, binding, roleBinding.role, roleBinding.sourcePath, commitSHA); err != nil {
		return err
	}
	subject, err := resolveAccessGrantSubject(ctx, tx, roleBinding.subjectType, roleBinding.subjectID)
	if err != nil {
		return fmt.Errorf("failed to resolve role binding subject %s:%s: %w", roleBinding.subjectType, roleBinding.subjectID, err)
	}
	if locked, err := isDefaultAdminGrantSubject(ctx, tx, subject.Type, subject.ID); err != nil {
		return err
	} else if locked {
		return fmt.Errorf("cannot modify default admin role assignments")
	}
	if err := ensureGlobalConfigObjectWritable(ctx, tx, binding, "auth_role_bindings", "role binding", roleBinding.role, "role_name = $1 AND subject_type = $2 AND subject_id = $3", roleBinding.role, subject.Type, subject.ID); err != nil {
		return err
	}
	if err := aaastore.UpsertManagedRoleBinding(ctx, tx, aaastore.RoleBinding{
		RoleName:    roleBinding.role,
		SubjectType: subject.Type,
		SubjectID:   subject.ID,
	}, aaastore.ConfigMetadata{RepoID: binding.ID, SourcePath: roleBinding.sourcePath, CommitSHA: commitSHA}); err != nil {
		return fmt.Errorf("failed to upsert role binding for %q: %w", roleBinding.role, err)
	}
	if subject.Type == model.SubjectTypeUser {
		if err := ensureGlobalConfigObjectWritable(ctx, tx, binding, "user_roles", "user role", roleBinding.role, "user_id = $1 AND role = $2", subject.ID, roleBinding.role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_roles (
				user_id, role,
				config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
			)
			VALUES ($1, $2, $3, $4, $5, TRUE)
			ON CONFLICT (user_id, role) DO UPDATE SET
				config_repo_id = EXCLUDED.config_repo_id,
				config_source_path = EXCLUDED.config_source_path,
				config_source_commit_sha = EXCLUDED.config_source_commit_sha,
				managed_by_config_repo = TRUE
		`, subject.ID, roleBinding.role, binding.ID, roleBinding.sourcePath, commitSHA); err != nil {
			return fmt.Errorf("failed to upsert legacy user role for %q: %w", roleBinding.role, err)
		}
	}
	return nil
}

func ensureAccessRolePrepared(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, roleName, sourcePath, commitSHA string) error {
	roleName = strings.TrimSpace(roleName)
	if roleName == "" {
		return fmt.Errorf("role is required")
	}
	var exists int
	err := tx.QueryRow(ctx, `SELECT 1 FROM auth_roles WHERE name = $1 LIMIT 1`, roleName).Scan(&exists)
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, sql.ErrNoRows):
		if isProtectedAdminRoleName(roleName) {
			return fmt.Errorf("default role %q is not available", roleName)
		}
		return upsertAccessRole(ctx, tx, binding, storedAccessRole{name: roleName, sourcePath: sourcePath}, commitSHA)
	case err != nil:
		return err
	default:
		return nil
	}
}

func (a *App) syncAccessGrants(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, plan accessSyncPlan, commitSHA string, details map[string]int) (map[resolvedAccessGrantKey]struct{}, error) {
	keep := map[resolvedAccessGrantKey]struct{}{}
	for _, grant := range plan.grants {
		if grant.role != productRoleOwner {
			continue
		}
		key, err := a.upsertManagedProductRoleGrant(ctx, tx, binding, grant, commitSHA)
		if err != nil {
			return nil, err
		}
		keep[key] = struct{}{}
		details["access_grants_synced"]++
	}
	for _, grant := range plan.grants {
		if grant.role == productRoleOwner || grant.role == customUseGrantRole {
			continue
		}
		key, err := a.upsertManagedProductRoleGrant(ctx, tx, binding, grant, commitSHA)
		if err != nil {
			return nil, err
		}
		keep[key] = struct{}{}
		details["access_grants_synced"]++
	}
	for _, grant := range plan.grants {
		if grant.role != customUseGrantRole {
			continue
		}
		key, err := a.upsertManagedResourceUseGrant(ctx, tx, binding, plan, grant, commitSHA)
		if err != nil {
			return nil, err
		}
		keep[key] = struct{}{}
		details["access_grants_synced"]++
	}
	return keep, nil
}

func resolveConfigSyncGrantResource(ctx context.Context, runner queryRunner, resourceType, resourceID string) (accessGrantResource, error) {
	return resolveAccessGrantResource(ctx, runner, resourceType, resourceID, false)
}
