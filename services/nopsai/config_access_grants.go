package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
	aaastore "nopsai/services/aaa/pkg/store"
	"nopsai/services/nopsai/internal/configsync"
)

func syncResourceVisibilities(ctx context.Context, tx pgx.Tx, plan accessSyncPlan, details map[string]int) error {
	for _, access := range plan.resourceAccess {
		if !access.visibilitySet {
			continue
		}
		resource, err := resolveConfigSyncGrantResource(ctx, tx, access.resourceType, access.resourceID)
		if err != nil {
			return fmt.Errorf("failed to resolve resource access target %s:%s: %w", access.resourceType, access.resourceID, err)
		}
		if err := validateResourceVisibilityPolicy(resource.Type, access.visibility); err != nil {
			return err
		}
		if err := setResourceVisibilityWithRunner(ctx, tx, resource, access.visibility); err != nil {
			return fmt.Errorf("failed to sync resource visibility for %s:%s: %w", resource.Type, resource.ID, err)
		}
		details["resource_access_synced"]++
	}
	return nil
}

func (a *App) upsertManagedProductRoleGrant(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, grant storedAccessGrant, commitSHA string) (resolvedAccessGrantKey, error) {
	subject, err := resolveAccessGrantSubject(ctx, tx, grant.subjectType, grant.subjectID)
	if err != nil {
		return resolvedAccessGrantKey{}, fmt.Errorf("failed to resolve grant subject %s:%s: %w", grant.subjectType, grant.subjectID, err)
	}
	roleName, err := normalizeProductRoleName(grant.role)
	if err != nil {
		return resolvedAccessGrantKey{}, err
	}
	if locked, err := isDefaultAdminGrantSubject(ctx, tx, subject.Type, subject.ID); err != nil {
		return resolvedAccessGrantKey{}, err
	} else if locked {
		return resolvedAccessGrantKey{}, fmt.Errorf("cannot modify default admin role assignments")
	}
	if locked, err := isExternallyManagedUserSubject(ctx, tx, subject.Type, subject.ID); err != nil {
		return resolvedAccessGrantKey{}, err
	} else if locked {
		return resolvedAccessGrantKey{}, errExternallyManagedUserRoleAssignments
	}
	resource, err := resolveConfigSyncGrantResource(ctx, tx, grant.resourceType, grant.resourceID)
	if err != nil {
		return resolvedAccessGrantKey{}, fmt.Errorf("failed to resolve grant resource %s:%s: %w", grant.resourceType, grant.resourceID, err)
	}
	if err := validateGrantShape(roleName, resource, grant.inherit); err != nil {
		return resolvedAccessGrantKey{}, err
	}

	resourceScope := resource.ID
	if resource.Type == grantResourceSecret || resource.Type == grantResourceVariable {
		repoName, scope, _ := model.ParseNamedResourceID(resource.ID)
		resourceScope = firstNonEmptyString(repoName, scope)
	}
	writable, err := ensureAccessGrantConfigWritable(ctx, tx, binding, resourceScope, subject.Type, subject.ID, resource.Type, resource.ID)
	if err != nil {
		return resolvedAccessGrantKey{}, err
	}
	if !writable {
		return resolvedAccessGrantKey{
			subjectType:  subject.Type,
			subjectID:    subject.ID,
			resourceType: resource.Type,
			resourceID:   resource.ID,
		}, nil
	}

	var existingID int64
	var previousRole string
	err = tx.QueryRow(ctx, `
		SELECT id, role_name
		FROM access_grants
		WHERE subject_type = $1 AND subject_id = $2 AND resource_type = $3 AND resource_id = $4
	`, subject.Type, subject.ID, resource.Type, resource.ID).Scan(&existingID, &previousRole)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
		return resolvedAccessGrantKey{}, err
	}

	grantedBy := "config-repo"
	if existingID == 0 {
		if err := tx.QueryRow(ctx, `
			INSERT INTO access_grants (
				subject_type, subject_id, subject_display, role_name,
				resource_type, resource_id, resource_display, inherit, granted_by,
				config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, TRUE)
			RETURNING id
		`, subject.Type, subject.ID, subject.Display, roleName, resource.Type, resource.ID, resource.Display, grant.inherit, grantedBy, binding.ID, grant.sourcePath, commitSHA).Scan(&existingID); err != nil {
			return resolvedAccessGrantKey{}, fmt.Errorf("failed to insert access grant: %w", err)
		}
	} else {
		if err := aaastore.DeleteResourceACLByAccessGrantID(ctx, tx, existingID); err != nil {
			return resolvedAccessGrantKey{}, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM resource_ownership WHERE access_grant_id = $1`, existingID); err != nil {
			return resolvedAccessGrantKey{}, err
		}
		if previousRole == productRoleAdmin {
			if err := aaastore.DeleteRoleBindingForConfigScope(ctx, tx, aaastore.RoleBinding{
				RoleName:    productRoleAdmin,
				SubjectType: subject.Type,
				SubjectID:   subject.ID,
			}, binding.ID); err != nil {
				return resolvedAccessGrantKey{}, err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE access_grants
			SET subject_display = $1,
				role_name = $2,
				resource_display = $3,
				inherit = $4,
				granted_by = $5,
				config_repo_id = $6,
				config_source_path = $7,
				config_source_commit_sha = $8,
				managed_by_config_repo = TRUE
			WHERE id = $9
		`, subject.Display, roleName, resource.Display, grant.inherit, grantedBy, binding.ID, grant.sourcePath, commitSHA, existingID); err != nil {
			return resolvedAccessGrantKey{}, fmt.Errorf("failed to update access grant: %w", err)
		}
	}

	if roleName == productRoleAdmin {
		if err := aaastore.UpsertManagedRoleBinding(ctx, tx, aaastore.RoleBinding{
			RoleName:    productRoleAdmin,
			SubjectType: subject.Type,
			SubjectID:   subject.ID,
		}, aaastore.ConfigMetadata{RepoID: binding.ID, SourcePath: grant.sourcePath, CommitSHA: commitSHA}); err != nil {
			return resolvedAccessGrantKey{}, err
		}
		return resolvedAccessGrantKey{subjectType: subject.Type, subjectID: subject.ID, resourceType: resource.Type, resourceID: resource.ID}, nil
	}

	for _, action := range applicableProductRoleActions(roleName, resource.Type) {
		if err := aaastore.UpsertResourceACL(ctx, tx, aaastore.ResourceACL{
			ResourceType:  resource.Type,
			ResourceID:    resource.ID,
			SubjectType:   subject.Type,
			SubjectID:     subject.ID,
			AccessGrantID: &existingID,
			Action:        action,
			Effect:        "allow",
		}); err != nil {
			return resolvedAccessGrantKey{}, err
		}
	}
	if roleName == productRoleOwner {
		if _, err := tx.Exec(ctx, `
			INSERT INTO resource_ownership (
				resource_type, resource_id, owner_subject_type, owner_subject_id, access_grant_id
			)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (resource_type, resource_id, owner_subject_type, owner_subject_id)
			DO UPDATE SET access_grant_id = EXCLUDED.access_grant_id
		`, resource.Type, resource.ID, subject.Type, subject.ID, existingID); err != nil {
			return resolvedAccessGrantKey{}, err
		}
	}
	return resolvedAccessGrantKey{subjectType: subject.Type, subjectID: subject.ID, resourceType: resource.Type, resourceID: resource.ID}, nil
}

func (a *App) upsertManagedResourceUseGrant(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, grant storedAccessGrant, commitSHA string) (resolvedAccessGrantKey, error) {
	subject, err := resolveResourceUseGrantSubject(ctx, tx, grant.subjectType, grant.subjectID)
	if err != nil {
		return resolvedAccessGrantKey{}, fmt.Errorf("failed to resolve resource access subject %s:%s: %w", grant.subjectType, grant.subjectID, err)
	}
	if locked, err := isDefaultAdminGrantSubject(ctx, tx, subject.Type, subject.ID); err != nil {
		return resolvedAccessGrantKey{}, err
	} else if locked {
		return resolvedAccessGrantKey{}, fmt.Errorf("cannot modify default admin role assignments")
	}
	resource, err := resolveConfigSyncGrantResource(ctx, tx, grant.resourceType, grant.resourceID)
	if err != nil {
		return resolvedAccessGrantKey{}, fmt.Errorf("failed to resolve resource access target %s:%s: %w", grant.resourceType, grant.resourceID, err)
	}
	actions, err := normalizeUseGrantActions(resource.Type, grant.actions)
	if err != nil {
		return resolvedAccessGrantKey{}, err
	}

	resourceScope := resource.ID
	if resource.Type == grantResourceSecret || resource.Type == grantResourceVariable {
		repoName, scope, _ := model.ParseNamedResourceID(resource.ID)
		resourceScope = firstNonEmptyString(repoName, scope)
	}
	writable, err := ensureAccessGrantConfigWritable(ctx, tx, binding, resourceScope, subject.Type, subject.ID, resource.Type, resource.ID)
	if err != nil {
		return resolvedAccessGrantKey{}, err
	}
	resolvedKey := resolvedAccessGrantKey{
		subjectType:  subject.Type,
		subjectID:    subject.ID,
		resourceType: resource.Type,
		resourceID:   resource.ID,
	}
	if !writable {
		return resolvedKey, nil
	}

	var existingID int64
	var previousRole string
	err = tx.QueryRow(ctx, `
		SELECT id, role_name
		FROM access_grants
		WHERE subject_type = $1 AND subject_id = $2 AND resource_type = $3 AND resource_id = $4
	`, subject.Type, subject.ID, resource.Type, resource.ID).Scan(&existingID, &previousRole)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
		return resolvedAccessGrantKey{}, err
	}

	grantedBy := "config-repo"
	if existingID == 0 {
		if err := tx.QueryRow(ctx, `
			INSERT INTO access_grants (
				subject_type, subject_id, subject_display, role_name,
				resource_type, resource_id, resource_display, inherit, granted_by,
				config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE, $8, $9, $10, $11, TRUE)
			RETURNING id
		`, subject.Type, subject.ID, subject.Display, customUseGrantRole, resource.Type, resource.ID, resource.Display, grantedBy, binding.ID, grant.sourcePath, commitSHA).Scan(&existingID); err != nil {
			return resolvedAccessGrantKey{}, fmt.Errorf("failed to insert resource access grant: %w", err)
		}
	} else {
		if err := aaastore.DeleteResourceACLByAccessGrantID(ctx, tx, existingID); err != nil {
			return resolvedAccessGrantKey{}, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM resource_ownership WHERE access_grant_id = $1`, existingID); err != nil {
			return resolvedAccessGrantKey{}, err
		}
		if previousRole == productRoleAdmin {
			if err := aaastore.DeleteRoleBindingForConfigScope(ctx, tx, aaastore.RoleBinding{
				RoleName:    productRoleAdmin,
				SubjectType: subject.Type,
				SubjectID:   subject.ID,
			}, binding.ID); err != nil {
				return resolvedAccessGrantKey{}, err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE access_grants
			SET subject_display = $1,
				role_name = $2,
				resource_display = $3,
				inherit = FALSE,
				granted_by = $4,
				config_repo_id = $5,
				config_source_path = $6,
				config_source_commit_sha = $7,
				managed_by_config_repo = TRUE
			WHERE id = $8
		`, subject.Display, customUseGrantRole, resource.Display, grantedBy, binding.ID, grant.sourcePath, commitSHA, existingID); err != nil {
			return resolvedAccessGrantKey{}, fmt.Errorf("failed to update resource access grant: %w", err)
		}
	}

	if subject.Type != grantSubjectTeam {
		for _, action := range actions {
			if err := aaastore.UpsertResourceACL(ctx, tx, aaastore.ResourceACL{
				ResourceType:  resource.Type,
				ResourceID:    resource.ID,
				SubjectType:   subject.Type,
				SubjectID:     subject.ID,
				AccessGrantID: &existingID,
				Action:        action,
				Effect:        "allow",
			}); err != nil {
				return resolvedAccessGrantKey{}, err
			}
		}
	}

	return resolvedKey, nil
}

func ensureAccessGrantConfigWritable(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, resourceScope, subjectType, subjectID, resourceType, resourceID string) (bool, error) {
	displayID := subjectType + ":" + subjectID + " " + resourceType + ":" + resourceID
	var existingRepoID sql.NullInt64
	var managed bool
	err := tx.QueryRow(ctx, `
		SELECT config_repo_id, managed_by_config_repo
		FROM access_grants
		WHERE subject_type = $1 AND subject_id = $2 AND resource_type = $3 AND resource_id = $4
		LIMIT 1
	`, subjectType, subjectID, resourceType, resourceID).Scan(&existingRepoID, &managed)
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, sql.ErrNoRows):
		return true, nil
	case err != nil:
		return false, err
	}
	if !managed {
		return true, nil
	}
	if !existingRepoID.Valid {
		return false, fmt.Errorf("access grant %s is already managed by an unknown config repository", displayID)
	}
	if existingRepoID.Int64 == binding.ID {
		return true, nil
	}

	existing, err := loadConfigRepositoryByID(ctx, tx, existingRepoID.Int64)
	if err != nil {
		return false, err
	}
	if configsync.CanRepositoryWriteOver(binding, existing, resourceScope) {
		return true, nil
	}
	if configsync.RepositoryShadowsCurrent(existing, binding, resourceScope) {
		return false, nil
	}

	return false, fmt.Errorf("access grant %s is already managed by config repository %d", displayID, existingRepoID.Int64)
}
