package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"nopsai/services/aaa/pkg/model"
)

func (a *App) GrantProductRole(ctx context.Context, input GrantProductRoleInput) (accessGrantRecord, error) {
	var record accessGrantRecord
	if a == nil || a.db == nil {
		return record, fmt.Errorf("database unavailable")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return record, err
	}
	defer tx.Rollback(ctx)

	subject, err := resolveAccessGrantSubject(ctx, tx, input.SubjectType, input.SubjectID)
	if err != nil {
		return record, err
	}
	roleName, err := normalizeProductRoleName(input.RoleName)
	if err != nil {
		return record, err
	}
	if locked, err := isDefaultAdminGrantSubject(ctx, tx, subject.Type, subject.ID); err != nil {
		return record, err
	} else if locked {
		return record, fmt.Errorf("cannot modify default admin role assignments")
	}
	resource, err := resolveAccessGrantResource(ctx, tx, input.ResourceType, input.ResourceID, true)
	if err != nil {
		return record, err
	}
	if err := validateGrantShape(roleName, resource, input.Inherit); err != nil {
		return record, err
	}
	if err := validateFolderOwnerGuard(ctx, tx, roleName, resource, 0); err != nil {
		return record, err
	}

	var existingID int64
	if err := tx.QueryRow(ctx, `
		SELECT id
		FROM access_grants
		WHERE subject_type = $1 AND subject_id = $2 AND resource_type = $3 AND resource_id = $4
	`, subject.Type, subject.ID, resource.Type, resource.ID).Scan(&existingID); err == nil {
		return record, fmt.Errorf("grant already exists")
	} else if !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
		return record, err
	}

	grantedBy := strings.TrimSpace(input.GrantedBy)
	err = tx.QueryRow(ctx, `
		INSERT INTO access_grants (
			subject_type, subject_id, subject_display, role_name,
			resource_type, resource_id, resource_display, inherit, granted_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at
	`, subject.Type, subject.ID, subject.Display, roleName, resource.Type, resource.ID, resource.Display, input.Inherit, grantedBy).Scan(&record.ID, &record.CreatedAt)
	if err != nil {
		return record, err
	}

	record.SubjectType = subject.Type
	record.SubjectID = subject.ID
	record.SubjectDisplay = subject.Display
	record.RoleName = roleName
	record.ResourceType = resource.Type
	record.ResourceID = resource.ID
	record.ResourceDisplay = resource.Display
	record.Inherit = input.Inherit
	record.GrantedBy = grantedBy

	if roleName == productRoleAdmin {
		if err := ensureUniqueAdminBinding(ctx, tx, subject); err != nil {
			return accessGrantRecord{}, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO auth_role_bindings (role_name, subject_type, subject_id)
			VALUES ($1, $2, $3)
		`, productRoleAdmin, subject.Type, subject.ID); err != nil {
			return accessGrantRecord{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return accessGrantRecord{}, err
		}
		return record, nil
	}

	actions := applicableProductRoleActions(roleName, resource.Type)
	for _, action := range actions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO resource_acl (
				resource_type, resource_id, subject_type, subject_id, access_grant_id, action, effect
			)
			VALUES ($1, $2, $3, $4, $5, $6, 'allow')
		`, resource.Type, resource.ID, subject.Type, subject.ID, record.ID, action); err != nil {
			return accessGrantRecord{}, err
		}
	}

	if roleName == productRoleOwner {
		if _, err := tx.Exec(ctx, `
			INSERT INTO resource_ownership (
				resource_type, resource_id, owner_subject_type, owner_subject_id, access_grant_id
			)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (resource_type, resource_id, owner_subject_type, owner_subject_id) DO NOTHING
		`, resource.Type, resource.ID, subject.Type, subject.ID, record.ID); err != nil {
			return accessGrantRecord{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return accessGrantRecord{}, err
	}
	return record, nil
}

func (a *App) deleteProductRoleGrant(ctx context.Context, grantID int64) (accessGrantRecord, error) {
	var record accessGrantRecord
	if a == nil || a.db == nil {
		return record, fmt.Errorf("database unavailable")
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return record, err
	}
	defer tx.Rollback(ctx)

	record, err = loadAccessGrantRecord(ctx, tx, grantID)
	if err != nil {
		return record, err
	}
	if locked, err := isDefaultAdminGrantSubject(ctx, tx, record.SubjectType, record.SubjectID); err != nil {
		return record, err
	} else if locked {
		return record, fmt.Errorf("cannot modify default admin role assignments")
	}
	if err := validateFolderOwnerGuard(ctx, tx, record.RoleName, accessGrantResource{
		Type:    record.ResourceType,
		ID:      record.ResourceID,
		Display: record.ResourceDisplay,
	}, record.ID); err != nil {
		return record, err
	}

	if record.RoleName == productRoleAdmin {
		if _, err := tx.Exec(ctx, `
			DELETE FROM auth_role_bindings
			WHERE role_name = $1 AND subject_type = $2 AND subject_id = (
				SELECT subject_id FROM access_grants WHERE id = $3
			)
		`, productRoleAdmin, record.SubjectType, record.ID); err != nil {
			return accessGrantRecord{}, err
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM access_grants WHERE id = $1`, grantID); err != nil {
		return accessGrantRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return accessGrantRecord{}, err
	}
	return record, nil
}

func deleteUserAccessArtifacts(ctx context.Context, runner execRunner, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	statements := []string{
		`DELETE FROM access_grants WHERE subject_type = 'user' AND subject_id = $1`,
		`DELETE FROM resource_acl WHERE subject_type = 'user' AND subject_id = $1`,
		`DELETE FROM resource_ownership WHERE owner_subject_type = 'user' AND owner_subject_id = $1`,
		`DELETE FROM auth_role_bindings WHERE subject_type = 'user' AND subject_id = $1`,
		`DELETE FROM user_roles WHERE user_id = $1::uuid`,
	}
	for _, stmt := range statements {
		if _, err := runner.Exec(ctx, stmt, userID); err != nil {
			return err
		}
	}
	return nil
}

func loadAccessGrantRecord(ctx context.Context, runner queryRunner, grantID int64) (accessGrantRecord, error) {
	var record accessGrantRecord
	err := runner.QueryRow(ctx, `
		SELECT
			id,
			subject_type,
			subject_id,
			subject_display,
			role_name,
			resource_type,
			resource_id,
			resource_display,
			inherit,
			granted_by,
			created_at,
			managed_by_config_repo,
			config_source_path,
			config_source_commit_sha
		FROM access_grants
		WHERE id = $1
	`, grantID).Scan(
		&record.ID,
		&record.SubjectType,
		&record.SubjectID,
		&record.SubjectDisplay,
		&record.RoleName,
		&record.ResourceType,
		&record.ResourceID,
		&record.ResourceDisplay,
		&record.Inherit,
		&record.GrantedBy,
		&record.CreatedAt,
		&record.ManagedByConfig,
		&record.ConfigSourcePath,
		&record.ConfigSourceCommitSHA,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return accessGrantRecord{}, fmt.Errorf("grant not found")
		}
		return accessGrantRecord{}, err
	}
	return record, nil
}

func isDefaultAdminGrantSubject(ctx context.Context, runner queryRunner, subjectType, subjectID string) (bool, error) {
	if model.NormalizeType(subjectType) != model.SubjectTypeUser {
		return false, nil
	}
	subjectID = strings.TrimSpace(subjectID)
	if subjectID == "" {
		return false, nil
	}

	var exists int
	err := runner.QueryRow(ctx, `
		SELECT 1
		FROM users
		WHERE id::text = $1
		  AND sub = $2
		  AND LOWER(provider) = 'local'
		LIMIT 1
	`, subjectID, defaultAdminSub).Scan(&exists)
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, err
	default:
		return true, nil
	}
}

func ensureUniqueAdminBinding(ctx context.Context, runner queryRunner, subject accessGrantSubject) error {
	var existing int
	err := runner.QueryRow(ctx, `
		SELECT 1
		FROM auth_role_bindings
		WHERE role_name = $1 AND subject_type = $2 AND subject_id = $3
		LIMIT 1
	`, productRoleAdmin, subject.Type, subject.ID).Scan(&existing)
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return err
	default:
		return fmt.Errorf("admin role is already bound to this subject")
	}
}

func validateGrantShape(roleName string, resource accessGrantResource, inherit bool) error {
	if roleName == productRoleAdmin {
		if resource.Type != grantResourcePlatform {
			return fmt.Errorf("admin can only be granted on platform scope")
		}
		return nil
	}

	if resource.Type == grantResourcePlatform {
		return fmt.Errorf("only admin can be granted on platform scope")
	}
	if resource.Type == grantResourceFolder && !inherit {
		return fmt.Errorf("folder grants must inherit")
	}
	return nil
}

func validateFolderOwnerGuard(ctx context.Context, runner queryRunner, roleName string, resource accessGrantResource, excludeGrantID int64) error {
	if resource.Type != grantResourceFolder {
		return nil
	}
	if resource.ID == generalGrantID {
		return nil
	}
	if roleName != productRoleOwner {
		return nil
	}

	var ownerCount int
	ownerResourceIDs := folderOwnerGuardResourceIDs(resource.ID)
	err := runner.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM access_grants
		WHERE resource_type = $1
		  AND resource_id = ANY($2)
		  AND role_name = $3
		  AND id <> $4
	`, grantResourceFolder, ownerResourceIDs, productRoleOwner, excludeGrantID).Scan(&ownerCount)
	if err != nil {
		return err
	}

	switch {
	case roleName == productRoleOwner && excludeGrantID > 0 && ownerCount == 0:
		return errEveryFolderMustRetainOwner
	default:
		return nil
	}
}

func validateFolderOwnerUpsert(ctx context.Context, runner queryRunner, previousRole, nextRole string, resource accessGrantResource, existingGrantID int64) error {
	previousRole = strings.ToLower(strings.TrimSpace(previousRole))
	nextRole = strings.ToLower(strings.TrimSpace(nextRole))
	if nextRole == productRoleOwner {
		return nil
	}
	if existingGrantID > 0 && previousRole == productRoleOwner {
		return validateFolderOwnerGuard(ctx, runner, productRoleOwner, resource, existingGrantID)
	}
	return validateFolderOwnerGuard(ctx, runner, nextRole, resource, 0)
}

func folderOwnerGuardResourceIDs(folderID string) []string {
	folderID = strings.Trim(strings.TrimSpace(folderID), "/")
	if folderID == "" {
		return nil
	}

	segments := strings.Split(folderID, "/")
	resourceIDs := make([]string, 0, len(segments))
	for size := len(segments); size >= 1; size-- {
		resourceID := strings.TrimSpace(strings.Join(segments[:size], "/"))
		if resourceID != "" {
			resourceIDs = append(resourceIDs, resourceID)
		}
	}
	return resourceIDs
}

func applicableProductRoleActions(roleName, resourceType string) []string {
	roleName = strings.ToLower(strings.TrimSpace(roleName))
	if _, ok := productRoleDefinitions[roleName]; !ok {
		return nil
	}
	actionCandidates := effectiveProductRoleActions(roleName)
	actions := make([]string, 0, len(actionCandidates))
	for _, action := range actionCandidates {
		if actionAppliesToGrantResource(action, resourceType) {
			actions = append(actions, action)
		}
	}
	return actions
}

func effectiveProductRoleActions(roleName string) []string {
	roleName = strings.ToLower(strings.TrimSpace(roleName))
	visited := make(map[string]bool)
	seenActions := make(map[string]struct{})
	var actions []string

	var visit func(string)
	visit = func(current string) {
		current = strings.ToLower(strings.TrimSpace(current))
		if current == "" || visited[current] {
			return
		}
		visited[current] = true
		for _, includedRole := range productRoleIncludes[current] {
			visit(includedRole)
		}
		definition, ok := productRoleDefinitions[current]
		if !ok {
			return
		}
		for _, action := range definition.Actions {
			action = strings.TrimSpace(action)
			if action == "" {
				continue
			}
			if _, exists := seenActions[action]; exists {
				continue
			}
			seenActions[action] = struct{}{}
			actions = append(actions, action)
		}
	}

	visit(roleName)
	return actions
}

func actionAppliesToGrantResource(action, resourceType string) bool {
	action = strings.TrimSpace(action)
	resourceType = strings.TrimSpace(resourceType)
	if action == "*" {
		return resourceType == grantResourcePlatform
	}

	switch resourceType {
	case grantResourceFolder:
		return !strings.HasPrefix(action, "iam.") &&
			!strings.HasPrefix(action, "audit.") &&
			!strings.HasPrefix(action, "system.")
	case grantResourcePipeline:
		return strings.HasPrefix(action, "pipeline.") || strings.HasPrefix(action, "pipeline_run.")
	case grantResourceRun:
		return strings.HasPrefix(action, "pipeline_run.")
	case grantResourceSchedule:
		return strings.HasPrefix(action, "pipeline_schedule.")
	case grantResourceRepo:
		return strings.HasPrefix(action, "repository.") ||
			strings.HasPrefix(action, "trigger.") ||
			strings.HasPrefix(action, "secret.") ||
			strings.HasPrefix(action, "variable.") ||
			strings.HasPrefix(action, "pipeline_run.")
	case grantResourceScope:
		return strings.HasPrefix(action, "scope.") ||
			strings.HasPrefix(action, "secret.") ||
			strings.HasPrefix(action, "variable.") ||
			strings.HasPrefix(action, "pipeline_run.")
	case grantResourceSecret:
		return strings.HasPrefix(action, "secret.")
	case grantResourceVariable:
		return strings.HasPrefix(action, "variable.")
	case grantResourceTrigger:
		return strings.HasPrefix(action, "trigger.")
	case grantResourceExternalTrigger:
		return strings.HasPrefix(action, "external_trigger.")
	case grantResourceStep:
		return strings.HasPrefix(action, "step.")
	case grantResourceRunner:
		return strings.HasPrefix(action, "runner.")
	case grantResourceConfig:
		return strings.HasPrefix(action, "config_repo.")
	case grantResourceKnowledgeContext:
		return strings.HasPrefix(action, "knowledge_context.")
	default:
		return false
	}
}
