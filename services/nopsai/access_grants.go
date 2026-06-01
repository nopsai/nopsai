package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"nopsai/pkg/httpapi"
	"nopsai/services/aaa/pkg/model"
)

const (
	productRoleViewer    = "viewer"
	productRoleDeveloper = "developer"
	productRoleOwner     = "owner"
	productRoleAdmin     = "admin"

	grantResourceFolder           = "folder"
	grantResourceTeam             = "team"
	grantResourcePipeline         = "pipeline"
	grantResourceRun              = "pipeline_run"
	grantResourceSchedule         = "pipeline_schedule"
	grantResourceTrigger          = "trigger"
	grantResourceSecret           = "secret"
	grantResourceVariable         = "variable"
	grantResourceScope            = "scope"
	grantResourceRepo             = "repository"
	grantResourceStep             = "step"
	grantResourceRunner           = "runner"
	grantResourceConfig           = "config_repo"
	grantResourceKnowledgeContext = "knowledge_context"
	grantResourceCompany          = "company"
	grantResourcePlatform         = "platform"

	grantSubjectService        = "service"
	grantSubjectUser           = "user"
	grantSubjectGroup          = "group"
	grantSubjectRepository     = "repository"
	grantSubjectTrigger        = "trigger"
	grantSubjectServiceAccount = "service_account"

	platformGrantID = "default"
	generalGrantID  = model.FolderGeneralID
)

var errEveryFolderMustRetainOwner = errors.New("every folder must retain at least one owner")

type productRoleDefinition struct {
	Description string
	Actions     []string
}

type accessGrantRecord struct {
	ID                           int64
	SubjectType                  string
	SubjectID                    string
	SubjectDisplay               string
	RoleName                     string
	ResourceType                 string
	ResourceID                   string
	ResourceDisplay              string
	Inherit                      bool
	GrantedBy                    string
	CreatedAt                    time.Time
	ManagedByConfig              bool
	ConfigSourcePath             string
	ConfigSourceCommitSHA        string
	InheritedFromResourceType    string
	InheritedFromResourceID      string
	InheritedFromResourceDisplay string
}

type GrantProductRoleInput struct {
	SubjectType  string
	SubjectID    string
	RoleName     string
	ResourceType string
	ResourceID   string
	Inherit      bool
	GrantedBy    string
}

type createAccessGrantRequest struct {
	SubjectType  string `json:"subject_type"`
	SubjectID    string `json:"subject_id"`
	Role         string `json:"role"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Inherit      *bool  `json:"inherit"`
}

type accessGrantResponse struct {
	ID                        string    `json:"id"`
	SubjectType               string    `json:"subject_type"`
	SubjectID                 string    `json:"subject_id"`
	SubjectDisplay            string    `json:"subject_display,omitempty"`
	Role                      string    `json:"role"`
	ResourceType              string    `json:"resource_type"`
	ResourceID                string    `json:"resource_id"`
	Inherit                   bool      `json:"inherit"`
	GrantedBy                 string    `json:"granted_by,omitempty"`
	CreatedAt                 time.Time `json:"created_at,omitempty"`
	ManagedByConfigRepo       bool      `json:"managed_by_config_repo"`
	ConfigSourcePath          string    `json:"config_source_path,omitempty"`
	ConfigSourceCommitSHA     string    `json:"config_source_commit_sha,omitempty"`
	Source                    string    `json:"source"`
	InheritedFromResourceType string    `json:"inherited_from_resource_type,omitempty"`
	InheritedFromResourceID   string    `json:"inherited_from_resource_id,omitempty"`
	InheritedFromResource     string    `json:"inherited_from_resource,omitempty"`
}

type effectivePermissionResponse struct {
	Allowed              bool           `json:"allowed"`
	Action               string         `json:"action"`
	Resource             string         `json:"resource"`
	Reason               string         `json:"reason"`
	MatchedRole          string         `json:"matched_role,omitempty"`
	MatchedSubject       string         `json:"matched_subject,omitempty"`
	MatchedResource      string         `json:"matched_resource,omitempty"`
	Inherited            bool           `json:"inherited"`
	SourceParentResource string         `json:"source_parent_resource,omitempty"`
	LowLevelPermission   string         `json:"low_level_permission,omitempty"`
	MatchedPolicy        map[string]any `json:"matched_policy,omitempty"`
}

type groupPathRecord struct {
	ID          int
	Name        string
	ParentID    *int
	Description string
	Path        string
}

type accessGrantSubject struct {
	Type    string
	ID      string
	Display string
}

type accessGrantResource struct {
	Type    string
	ID      string
	Display string
}

type queryRunner interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type execRunner interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

var productRoleDefinitions = map[string]productRoleDefinition{
	productRoleViewer: {
		Description: "Read-only access to folders, pipelines, runs, schedules, triggers, steps, scope metadata, repositories, secrets, and variables.",
		Actions: []string{
			"folder.list",
			"folder.read",
			"pipeline.list",
			"pipeline.read",
			"pipeline_run.list",
			"pipeline_run.read",
			"pipeline_run.read_logs",
			"pipeline_schedule.list",
			"pipeline_schedule.read",
			"trigger.read",
			"secret.list_metadata",
			"variable.list_metadata",
			"scope.read",
			"repository.read",
			"step.read",
			"config_repo.read",
			"knowledge_context.read",
		},
	},
	productRoleDeveloper: {
		Description: "Viewer access plus non-destructive create, update, and execution capabilities.",
		Actions: []string{
			"folder.list",
			"folder.read",
			"pipeline.list",
			"pipeline.read",
			"pipeline_run.list",
			"pipeline_run.read",
			"pipeline_run.read_logs",
			"trigger.read",
			"secret.list_metadata",
			"variable.list_metadata",
			"scope.read",
			"repository.read",
			"step.read",
			"config_repo.read",
			"knowledge_context.read",
			"pipeline.create",
			"pipeline.update",
			"pipeline.execute",
			"pipeline.use",
			"pipeline_run.rerun",
			"pipeline_run.cancel",
			"pipeline_schedule.create",
			"pipeline_schedule.update",
			"pipeline_schedule.execute",
			"trigger.update",
			"secret.use",
			"secret.write_value",
			"variable.use",
			"variable.write_value",
			"scope.use",
			"scope.update",
			"repository.update",
			"step.create",
			"step.update",
			"step.use",
			"runner.use",
			"config_repo.use",
			"knowledge_context.use",
		},
	},
	productRoleOwner: {
		Description: "Developer access plus deletes, secret reads, and permission management inside the owned scope.",
		Actions: []string{
			"folder.list",
			"folder.read",
			"pipeline.list",
			"pipeline.read",
			"pipeline_run.list",
			"pipeline_run.read",
			"pipeline_run.read_logs",
			"trigger.read",
			"secret.list_metadata",
			"variable.list_metadata",
			"scope.read",
			"repository.read",
			"step.read",
			"config_repo.read",
			"knowledge_context.read",
			"pipeline.create",
			"pipeline.update",
			"pipeline.execute",
			"pipeline.use",
			"pipeline_run.rerun",
			"pipeline_run.cancel",
			"pipeline_run.finalize",
			"pipeline_run.write_logs",
			"pipeline_run.task_update",
			"trigger.update",
			"secret.use",
			"secret.write_value",
			"variable.read_value",
			"variable.use",
			"variable.write_value",
			"scope.use",
			"scope.update",
			"repository.update",
			"step.create",
			"step.update",
			"step.use",
			"runner.use",
			"config_repo.use",
			"knowledge_context.use",
			"knowledge_context.create",
			"knowledge_context.update",
			"knowledge_context.delete",
			"knowledge_context.manage_access",
			"folder.create",
			"folder.update",
			"folder.move",
			"folder.delete",
			"folder.manage_acl",
			"config_repo.manage",
			"config_repo.sync",
			"pipeline.delete",
			"pipeline.manage_acl",
			"pipeline_run.delete",
			"pipeline_schedule.delete",
			"pipeline_schedule.manage_acl",
			"trigger.delete",
			"trigger.manage_acl",
			"secret.delete",
			"secret.read_value",
			"secret.manage_acl",
			"variable.delete",
			"variable.manage_acl",
			"scope.delete",
			"scope.manage_acl",
			"repository.delete",
			"repository.manage_acl",
			"step.delete",
			"step.manage_acl",
		},
	},
	productRoleAdmin: {
		Description: "Platform-wide administrator.",
		Actions:     []string{"*"},
	},
}

var productRoleIncludes = map[string][]string{
	productRoleDeveloper: {productRoleViewer},
	productRoleOwner:     {productRoleDeveloper},
}

var accessGrantSchemaStatements = []string{
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
	`CREATE INDEX IF NOT EXISTS idx_access_grants_subject_lookup ON access_grants(subject_type, subject_id)`,
	`CREATE INDEX IF NOT EXISTS idx_access_grants_resource_lookup ON access_grants(resource_type, resource_id)`,
}

func ensureProductAccessBootstrap(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	if err := ensureAccessGrantSchema(ctx, db); err != nil {
		return err
	}
	if err := seedProductRoleTemplates(ctx, db); err != nil {
		return err
	}
	return reconcileProductAccessGrants(ctx, db)
}

func ensureAccessGrantSchema(ctx context.Context, db *pgxpool.Pool) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, stmt := range accessGrantSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func seedProductRoleTemplates(ctx context.Context, db *pgxpool.Pool) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	roleNames := []string{productRoleViewer, productRoleDeveloper, productRoleOwner, productRoleAdmin}
	for _, roleName := range roleNames {
		definition := productRoleDefinitions[roleName]
		if _, err := tx.Exec(ctx, `
			INSERT INTO auth_roles (name, description)
			VALUES ($1, $2)
			ON CONFLICT (name) DO UPDATE
			SET description = EXCLUDED.description,
			    updated_at = NOW()
		`, roleName, definition.Description); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM auth_role_permissions WHERE role_name = ANY($1)`, roleNames); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role = ANY($1)`, roleNames); err != nil {
		return err
	}

	for _, roleName := range roleNames {
		for _, action := range effectiveProductRoleActions(roleName) {
			resourceType := "*"
			resourceID := "*"
			if roleName != productRoleAdmin {
				resourceType = "*"
				resourceID = "*"
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO auth_role_permissions (role_name, resource_type, resource_id, action, effect)
				VALUES ($1, $2, $3, $4, 'allow')
			`, roleName, resourceType, resourceID, action); err != nil {
				return err
			}

			objectValue := formatAdminPermissionObject(resourceType, resourceID)
			actionValue := formatAdminPermissionAction("allow", action)
			displayName := adminPermissionDisplayName("", objectValue, actionValue)
			if roleName == productRoleAdmin && action == "*" {
				displayName = "All access"
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO role_permissions (role, name, obj, act)
				VALUES ($1, $2, $3, $4)
			`, roleName, displayName, objectValue, actionValue); err != nil {
				return err
			}
		}
	}

	return tx.Commit(ctx)
}

func reconcileProductAccessGrants(ctx context.Context, db *pgxpool.Pool) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, subject_type, subject_id, role_name, resource_type, resource_id
		FROM access_grants
		ORDER BY id ASC
	`)
	if err != nil {
		return err
	}

	type grantRow struct {
		id           int64
		subjectType  string
		subjectID    string
		roleName     string
		resourceType string
		resourceID   string
	}

	var grants []grantRow
	for rows.Next() {
		var grant grantRow
		if err := rows.Scan(
			&grant.id,
			&grant.subjectType,
			&grant.subjectID,
			&grant.roleName,
			&grant.resourceType,
			&grant.resourceID,
		); err != nil {
			rows.Close()
			return err
		}
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, grant := range grants {
		if _, ok := productRoleDefinitions[grant.roleName]; !ok {
			continue
		}

		if _, err := tx.Exec(ctx, `DELETE FROM resource_acl WHERE access_grant_id = $1`, grant.id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM resource_ownership WHERE access_grant_id = $1`, grant.id); err != nil {
			return err
		}

		if grant.roleName == productRoleAdmin {
			if _, err := tx.Exec(ctx, `
				INSERT INTO auth_role_bindings (role_name, subject_type, subject_id)
				VALUES ($1, $2, $3)
				ON CONFLICT (role_name, subject_type, subject_id) DO NOTHING
			`, productRoleAdmin, grant.subjectType, grant.subjectID); err != nil {
				return err
			}
			continue
		}

		for _, action := range applicableProductRoleActions(grant.roleName, grant.resourceType) {
			if _, err := tx.Exec(ctx, `
				INSERT INTO resource_acl (
					resource_type, resource_id, subject_type, subject_id, access_grant_id, action, effect
				)
				VALUES ($1, $2, $3, $4, $5, $6, 'allow')
				ON CONFLICT (resource_type, resource_id, subject_type, subject_id, action, effect)
				DO UPDATE SET access_grant_id = EXCLUDED.access_grant_id
			`, grant.resourceType, grant.resourceID, grant.subjectType, grant.subjectID, grant.id, action); err != nil {
				return err
			}
		}

		if grant.roleName == productRoleOwner {
			if _, err := tx.Exec(ctx, `
				INSERT INTO resource_ownership (
					resource_type, resource_id, owner_subject_type, owner_subject_id, access_grant_id
				)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (resource_type, resource_id, owner_subject_type, owner_subject_id)
				DO UPDATE SET access_grant_id = EXCLUDED.access_grant_id
			`, grant.resourceType, grant.resourceID, grant.subjectType, grant.subjectID, grant.id); err != nil {
				return err
			}
		}
	}

	return tx.Commit(ctx)
}

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

func normalizeProductRoleName(raw string) (string, error) {
	roleName := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := productRoleDefinitions[roleName]; !ok {
		return "", fmt.Errorf("role must be one of viewer, developer, owner, admin")
	}
	return roleName, nil
}

func normalizeAccessGrantSubjectType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case grantSubjectUser:
		return model.SubjectTypeUser, nil
	case model.SubjectTypeAuthGroup, "group":
		return model.SubjectTypeAuthGroup, nil
	case grantSubjectRepository:
		return model.SubjectTypeRepository, nil
	case grantSubjectTrigger:
		return model.SubjectTypeTrigger, nil
	case grantSubjectServiceAccount:
		return model.SubjectTypeServiceAccount, nil
	case grantSubjectService, model.SubjectTypeInternalService:
		return model.SubjectTypeInternalService, nil
	default:
		return "", fmt.Errorf("subject_type must be user, auth_group, repository, trigger, service_account, or internal_service")
	}
}

func normalizeAccessGrantResourceType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case grantResourceFolder, grantResourceTeam:
		return grantResourceFolder, nil
	case grantResourcePipeline:
		return grantResourcePipeline, nil
	case grantResourceRun:
		return grantResourceRun, nil
	case grantResourceSchedule:
		return grantResourceSchedule, nil
	case grantResourceTrigger:
		return grantResourceTrigger, nil
	case grantResourceSecret:
		return grantResourceSecret, nil
	case grantResourceVariable:
		return grantResourceVariable, nil
	case grantResourceScope:
		return grantResourceScope, nil
	case grantResourceRepo:
		return grantResourceRepo, nil
	case grantResourceStep:
		return grantResourceStep, nil
	case grantResourceRunner:
		return grantResourceRunner, nil
	case grantResourceConfig:
		return grantResourceConfig, nil
	case grantResourceKnowledgeContext:
		return grantResourceKnowledgeContext, nil
	case grantResourceCompany, grantResourcePlatform:
		return grantResourcePlatform, nil
	default:
		return "", fmt.Errorf("unsupported resource_type")
	}
}

func resolveAccessGrantSubject(ctx context.Context, runner queryRunner, rawType, rawID string) (accessGrantSubject, error) {
	subjectType, err := normalizeAccessGrantSubjectType(rawType)
	if err != nil {
		return accessGrantSubject{}, err
	}
	rawID = strings.TrimSpace(rawID)
	if rawID == "" {
		return accessGrantSubject{}, fmt.Errorf("subject_id is required")
	}

	switch subjectType {
	case model.SubjectTypeUser:
		return resolveAccessGrantUser(ctx, runner, rawID)
	case model.SubjectTypeAuthGroup:
		return resolveAccessGrantAuthGroup(ctx, runner, rawID)
	case model.SubjectTypeRepository, model.SubjectTypeTrigger, model.SubjectTypeServiceAccount:
		return resolveAccessGrantNamedSubject(subjectType, rawID)
	case model.SubjectTypeInternalService:
		return resolveAccessGrantService(ctx, runner, rawID)
	default:
		return accessGrantSubject{}, fmt.Errorf("unsupported subject_type")
	}
}

func resolveAccessGrantUser(ctx context.Context, runner queryRunner, rawID string) (accessGrantSubject, error) {
	var subject accessGrantSubject
	query := `
		SELECT id::text, COALESCE(NULLIF(sub, ''), COALESCE(email, id::text))
		FROM users
		WHERE %s
		LIMIT 1
	`

	var (
		lookup string
		args   []any
	)
	if _, err := uuid.Parse(rawID); err == nil {
		lookup = "id::text = $1"
		args = []any{rawID}
	} else if strings.Contains(rawID, "@") {
		lookup = "LOWER(email) = LOWER($1)"
		args = []any{rawID}
	} else {
		lookup = "sub = $1"
		args = []any{rawID}
	}

	err := runner.QueryRow(ctx, fmt.Sprintf(query, lookup), args...).Scan(&subject.ID, &subject.Display)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return accessGrantSubject{}, fmt.Errorf("subject not found")
		}
		return accessGrantSubject{}, err
	}
	subject.Type = model.SubjectTypeUser
	return subject, nil
}

func resolveAccessGrantAuthGroup(ctx context.Context, runner queryRunner, rawID string) (accessGrantSubject, error) {
	groupID := strings.TrimSpace(rawID)
	if groupID == "" {
		return accessGrantSubject{}, fmt.Errorf("subject_id is required")
	}

	var subject accessGrantSubject
	query := `
		SELECT id::text, name
		FROM auth_groups
		WHERE %s
		LIMIT 1
	`
	var (
		lookup string
		args   []any
	)
	if _, err := uuid.Parse(groupID); err == nil {
		lookup = "id::text = $1"
		args = []any{groupID}
	} else {
		lookup = "name = $1"
		args = []any{groupID}
	}

	err := runner.QueryRow(ctx, fmt.Sprintf(query, lookup), args...).Scan(&subject.ID, &subject.Display)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return accessGrantSubject{}, fmt.Errorf("subject not found")
		}
		return accessGrantSubject{}, err
	}
	subject.Type = model.SubjectTypeAuthGroup
	return subject, nil
}

func resolveAccessGrantNamedSubject(subjectType, rawID string) (accessGrantSubject, error) {
	subjectID := strings.Trim(strings.TrimSpace(rawID), "/")
	if subjectID == "" {
		return accessGrantSubject{}, fmt.Errorf("subject_id is required")
	}
	for _, prefix := range []string{
		model.SubjectTypeRepository + ":",
		model.SubjectTypeTrigger + ":",
		model.SubjectTypeServiceAccount + ":",
	} {
		if strings.HasPrefix(strings.ToLower(subjectID), prefix) {
			subjectID = strings.TrimSpace(subjectID[len(prefix):])
			break
		}
	}
	subjectID = strings.Trim(strings.TrimSpace(subjectID), "/")
	if subjectID == "" {
		return accessGrantSubject{}, fmt.Errorf("subject_id is required")
	}
	return accessGrantSubject{
		Type:    subjectType,
		ID:      subjectID,
		Display: subjectID,
	}, nil
}

func resolveAccessGrantService(ctx context.Context, runner queryRunner, rawID string) (accessGrantSubject, error) {
	serviceID := strings.TrimSpace(rawID)
	if serviceID == "" {
		return accessGrantSubject{}, fmt.Errorf("subject_id is required")
	}
	var exists int
	err := runner.QueryRow(ctx, `
		SELECT 1
		WHERE $1 = 'dispatcher'
		   OR EXISTS (
				SELECT 1
				FROM auth_role_bindings
				WHERE subject_type = 'internal_service' AND subject_id = $1
		   )
	`, serviceID).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return accessGrantSubject{}, fmt.Errorf("subject not found")
		}
		return accessGrantSubject{}, err
	}
	return accessGrantSubject{
		Type:    model.SubjectTypeInternalService,
		ID:      serviceID,
		Display: serviceID,
	}, nil
}

func resolveAccessGrantResource(ctx context.Context, runner queryRunner, rawType, rawID string, requireExists bool) (accessGrantResource, error) {
	resourceType, err := normalizeAccessGrantResourceType(rawType)
	if err != nil {
		return accessGrantResource{}, err
	}
	rawID = strings.TrimSpace(rawID)

	switch resourceType {
	case grantResourcePlatform:
		return accessGrantResource{
			Type:    grantResourcePlatform,
			ID:      platformGrantID,
			Display: "platform",
		}, nil
	case grantResourceFolder:
		return resolveAccessGrantFolder(ctx, runner, rawID, requireExists)
	case grantResourcePipeline:
		return resolvePipelineOrStepGrantResource(ctx, runner, grantResourcePipeline, rawID, requireExists, "pipelines")
	case grantResourceRun:
		if rawID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		if requireExists && rawID != "*" {
			var exists int
			err := runner.QueryRow(ctx, `SELECT 1 FROM pipeline_runs WHERE run_id::text = $1 LIMIT 1`, rawID).Scan(&exists)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
					return accessGrantResource{}, fmt.Errorf("resource not found")
				}
				return accessGrantResource{}, err
			}
		}
		return accessGrantResource{Type: grantResourceRun, ID: rawID, Display: rawID}, nil
	case grantResourceSchedule:
		return resolveScheduleGrantResource(ctx, runner, rawID, requireExists)
	case grantResourceStep:
		return resolvePipelineOrStepGrantResource(ctx, runner, grantResourceStep, rawID, requireExists, "steps")
	case grantResourceTrigger:
		if rawID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		if requireExists {
			var exists int
			err := runner.QueryRow(ctx, `SELECT 1 FROM triggers WHERE repository_name = $1 LIMIT 1`, rawID).Scan(&exists)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
					return accessGrantResource{}, fmt.Errorf("resource not found")
				}
				return accessGrantResource{}, err
			}
		}
		return accessGrantResource{Type: grantResourceTrigger, ID: rawID, Display: rawID}, nil
	case grantResourceScope:
		scopeID, scopeLookup, scopeDisplay := normalizeScopeGrantResourceID(rawID)
		if scopeDisplay == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		if requireExists && !isDefaultScopeGrantResource(scopeID, scopeDisplay) {
			var exists int
			err := runner.QueryRow(ctx, `
				SELECT 1
				FROM (
					SELECT scope FROM secrets WHERE scope = $1
					UNION
					SELECT scope FROM variables WHERE scope = $1
				) scopes
				LIMIT 1
			`, scopeLookup).Scan(&exists)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
					return accessGrantResource{}, fmt.Errorf("resource not found")
				}
				return accessGrantResource{}, err
			}
		}
		return accessGrantResource{Type: grantResourceScope, ID: scopeID, Display: scopeDisplay}, nil
	case grantResourceRepo:
		if rawID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		if requireExists {
			var exists int
			err := runner.QueryRow(ctx, `
				SELECT 1
				FROM (
					SELECT name AS value FROM groups WHERE name = $1
					UNION
					SELECT repository_name AS value FROM triggers WHERE repository_name = $1
					UNION
					SELECT repository_name AS value FROM secrets WHERE repository_name = $1
					UNION
					SELECT repository_name AS value FROM variables WHERE repository_name = $1
				) repositories
				LIMIT 1
			`, rawID).Scan(&exists)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
					return accessGrantResource{}, fmt.Errorf("resource not found")
				}
				return accessGrantResource{}, err
			}
		}
		return accessGrantResource{Type: grantResourceRepo, ID: rawID, Display: rawID}, nil
	case grantResourceRunner, grantResourceConfig:
		if rawID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		resourceID := strings.Trim(strings.TrimSpace(rawID), "/")
		if resourceID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		return accessGrantResource{Type: resourceType, ID: resourceID, Display: resourceID}, nil
	case grantResourceKnowledgeContext:
		resourceID := strings.Trim(strings.TrimSpace(rawID), "/")
		if resourceID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		if requireExists && resourceID != "*" {
			kind, group, name, err := splitKnowledgeContextIdentifier(resourceID)
			if err != nil {
				return accessGrantResource{}, err
			}
			var exists int
			err = runner.QueryRow(ctx, `
				SELECT 1
				FROM knowledge_contexts
				WHERE kind = $1 AND group_path = $2 AND name = $3
				LIMIT 1
			`, kind, group, name).Scan(&exists)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
					return accessGrantResource{}, fmt.Errorf("resource not found")
				}
				return accessGrantResource{}, err
			}
		}
		return accessGrantResource{Type: resourceType, ID: resourceID, Display: resourceID}, nil
	case grantResourceSecret, grantResourceVariable:
		if rawID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		resourceID := runtimeNamedResourceIDForResource(rawID)
		if requireExists {
			tableName := grantResourceSecret + "s"
			if resourceType == grantResourceVariable {
				tableName = grantResourceVariable + "s"
			}
			var exists int
			err := runner.QueryRow(ctx, fmt.Sprintf(`SELECT 1 FROM %s WHERE %s LIMIT 1`, tableName, namedResourceWhereClause(resourceID)), namedResourceWhereArgs(resourceID)...).Scan(&exists)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
					return accessGrantResource{}, fmt.Errorf("resource not found")
				}
				return accessGrantResource{}, err
			}
		}
		return accessGrantResource{Type: resourceType, ID: resourceID, Display: resourceID}, nil
	default:
		return accessGrantResource{}, fmt.Errorf("unsupported resource_type")
	}
}

func isDefaultScopeGrantResource(id, display string) bool {
	return strings.TrimSpace(id) == "" && strings.EqualFold(strings.TrimSpace(display), "default")
}

func normalizeScopeGrantResourceID(rawID string) (id, lookup, display string) {
	rawID = strings.Trim(strings.TrimSpace(rawID), "/")
	switch strings.ToLower(rawID) {
	case "", "default":
		return "", "", "default"
	default:
		return rawID, rawID, rawID
	}
}

func resolveAccessGrantFolder(ctx context.Context, runner queryRunner, rawID string, requireExists bool) (accessGrantResource, error) {
	rawID = strings.TrimSpace(rawID)
	if isGeneralGrantResourceID(rawID) {
		return accessGrantResource{
			Type:    grantResourceFolder,
			ID:      generalGrantID,
			Display: "general",
		}, nil
	}
	if rawID == "" {
		return accessGrantResource{}, fmt.Errorf("resource_id is required")
	}

	if numericID, err := strconv.Atoi(rawID); err == nil {
		pathRecords, loadErr := loadGroupPathRecords(ctx, runner)
		if loadErr != nil {
			return accessGrantResource{}, loadErr
		}
		record, ok := pathRecords[numericID]
		if !ok {
			return accessGrantResource{}, fmt.Errorf("resource not found")
		}
		return accessGrantResource{
			Type:    grantResourceFolder,
			ID:      record.Path,
			Display: "/" + record.Path,
		}, nil
	}

	normalized := strings.Trim(strings.TrimSpace(rawID), "/")
	if normalized == "" {
		return accessGrantResource{}, fmt.Errorf("resource_id is required")
	}
	if requireExists {
		pathRecords, err := loadGroupPathRecords(ctx, runner)
		if err != nil {
			return accessGrantResource{}, err
		}
		for _, record := range pathRecords {
			if record.Path == normalized {
				return accessGrantResource{
					Type:    grantResourceFolder,
					ID:      normalized,
					Display: "/" + normalized,
				}, nil
			}
		}
		return accessGrantResource{}, fmt.Errorf("resource not found")
	}

	return accessGrantResource{
		Type:    grantResourceFolder,
		ID:      normalized,
		Display: "/" + normalized,
	}, nil
}

func resolvePipelineOrStepGrantResource(ctx context.Context, runner queryRunner, resourceType, rawID string, requireExists bool, tableName string) (accessGrantResource, error) {
	resourceID := strings.Trim(strings.TrimSpace(rawID), "/")
	if resourceID == "" {
		return accessGrantResource{}, fmt.Errorf("resource_id is required")
	}
	if requireExists {
		pathPart, namePart := model.SplitPipelineID(resourceID)
		query := fmt.Sprintf(`SELECT 1 FROM %s WHERE path = $1 AND name = $2 LIMIT 1`, tableName)
		var exists int
		err := runner.QueryRow(ctx, query, pathPart, namePart).Scan(&exists)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
				return accessGrantResource{}, fmt.Errorf("resource not found")
			}
			return accessGrantResource{}, err
		}
	}
	return accessGrantResource{Type: resourceType, ID: resourceID, Display: resourceID}, nil
}

func namedResourceWhereClause(resourceID string) string {
	repoName, scope, _ := model.ParseNamedResourceID(resourceID)
	storageScope := runtimeScopeForStorage(scope)
	switch {
	case repoName != "":
		return "name = $1 AND repository_name = $2 AND " + runtimeScopeEqualsSQL("scope", 3, storageScope)
	case scope != "":
		return "name = $1 AND repository_name IS NULL AND " + runtimeScopeEqualsSQL("scope", 2, storageScope)
	default:
		return "name = $1 AND repository_name IS NULL AND " + runtimeScopeEqualsSQL("scope", 2, storageScope)
	}
}

func namedResourceWhereArgs(resourceID string) []any {
	repoName, scope, name := model.ParseNamedResourceID(resourceID)
	storageScope := runtimeScopeForStorage(scope)
	switch {
	case repoName != "" && scope != "":
		return []any{name, repoName, storageScope}
	case repoName != "":
		return []any{name, repoName, storageScope}
	case scope != "":
		return []any{name, storageScope}
	default:
		return []any{name, storageScope}
	}
}

func loadGroupPathRecords(ctx context.Context, runner queryRunner) (map[int]groupPathRecord, error) {
	rows, err := runner.Query(ctx, `SELECT id, name, parent_id, COALESCE(description, '') FROM groups`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make(map[int]groupPathRecord)
	for rows.Next() {
		var (
			record      groupPathRecord
			parentIDSQL sql.NullInt32
		)
		if err := rows.Scan(&record.ID, &record.Name, &parentIDSQL, &record.Description); err != nil {
			return nil, err
		}
		record.Name = strings.Trim(strings.TrimSpace(record.Name), "/")
		if parentIDSQL.Valid {
			parent := int(parentIDSQL.Int32)
			record.ParentID = &parent
		}
		records[record.ID] = record
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	cache := make(map[int]string, len(records))
	var buildPath func(int) (string, error)
	buildPath = func(id int) (string, error) {
		if path, ok := cache[id]; ok {
			return path, nil
		}
		record, ok := records[id]
		if !ok {
			return "", fmt.Errorf("group %d not found", id)
		}
		if record.ParentID == nil {
			cache[id] = record.Name
			return cache[id], nil
		}
		parentPath, err := buildPath(*record.ParentID)
		if err != nil {
			return "", err
		}
		cache[id] = strings.Trim(strings.TrimSpace(parentPath+"/"+record.Name), "/")
		return cache[id], nil
	}

	for id, record := range records {
		path, err := buildPath(id)
		if err != nil {
			return nil, err
		}
		record.Path = path
		records[id] = record
	}
	return records, nil
}

func (a *App) folderGrantResourceByGroupID(ctx context.Context, groupID int) (accessGrantResource, error) {
	if a == nil || a.db == nil {
		return accessGrantResource{}, fmt.Errorf("database unavailable")
	}
	return resolveAccessGrantFolder(ctx, a.db, strconv.Itoa(groupID), true)
}

func (a *App) folderPathRecords(ctx context.Context) (map[int]groupPathRecord, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	return loadGroupPathRecords(ctx, a.db)
}

func authorizeGrantOperation(ctx context.Context, subject model.Subject, resource accessGrantResource, roleName string, checker func(context.Context, model.Subject, string, model.ResourceRef, map[string]any) (model.Decision, error), requestContext map[string]any) error {
	if roleName == productRoleAdmin || resource.Type == grantResourcePlatform {
		decision, err := checker(ctx, subject, "iam.admin", model.ResourceRef{Type: "iam", ID: "admin"}, requestContext)
		if err != nil {
			return err
		}
		if !decision.Allowed {
			return fmt.Errorf("forbidden")
		}
		return nil
	}

	action, resourceRef, err := managementActionForGrantResource(resource)
	if err != nil {
		return err
	}
	decision, err := checker(ctx, subject, action, resourceRef, requestContext)
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return fmt.Errorf("forbidden")
	}
	return nil
}

func managementActionForGrantResource(resource accessGrantResource) (string, model.ResourceRef, error) {
	switch resource.Type {
	case grantResourceFolder:
		return "folder.manage_acl", model.ResourceRef{Type: grantResourceFolder, ID: resource.ID}, nil
	case grantResourcePipeline:
		return "pipeline.manage_acl", model.ResourceRef{Type: grantResourcePipeline, ID: resource.ID}, nil
	case grantResourceSchedule:
		return "pipeline_schedule.manage_acl", model.ResourceRef{Type: grantResourceSchedule, ID: resource.ID}, nil
	case grantResourceTrigger:
		return "trigger.manage_acl", model.ResourceRef{Type: grantResourceTrigger, ID: resource.ID}, nil
	case grantResourceSecret:
		return "secret.manage_acl", model.ResourceRef{Type: grantResourceSecret, ID: resource.ID}, nil
	case grantResourceVariable:
		return "variable.manage_acl", model.ResourceRef{Type: grantResourceVariable, ID: resource.ID}, nil
	case grantResourceScope:
		return "scope.manage_acl", model.ResourceRef{Type: grantResourceScope, ID: resource.ID}, nil
	case grantResourceRepo:
		return "repository.manage_acl", model.ResourceRef{Type: grantResourceRepo, ID: resource.ID}, nil
	case grantResourceStep:
		return "step.manage_acl", model.ResourceRef{Type: grantResourceStep, ID: resource.ID}, nil
	case grantResourceKnowledgeContext:
		return "knowledge_context.manage_access", model.ResourceRef{Type: grantResourceKnowledgeContext, ID: resource.ID}, nil
	case grantResourceRunner:
		return "system.update", model.ResourceRef{Type: "dispatcher", ID: "runners"}, nil
	case grantResourceConfig:
		return "config_repo.manage", model.ResourceRef{Type: grantResourceConfig, ID: resource.ID}, nil
	default:
		return "", model.ResourceRef{}, fmt.Errorf("unsupported grant resource")
	}
}

func (a *App) handleCreateAccessGrant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req createAccessGrantRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	roleName, err := normalizeProductRoleName(req.Role)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resource, err := resolveAccessGrantResource(r.Context(), a.db, req.ResourceType, req.ResourceID, true)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}

	inherit := resource.Type == grantResourceFolder
	if req.Inherit != nil {
		inherit = *req.Inherit
	}
	if err := authorizeGrantOperation(r.Context(), subject, resource, roleName, a.aaaCheck, a.aaaRequestContext(r)); err != nil {
		if err.Error() == "forbidden" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	record, err := a.GrantProductRole(r.Context(), GrantProductRoleInput{
		SubjectType:  req.SubjectType,
		SubjectID:    req.SubjectID,
		RoleName:     roleName,
		ResourceType: resource.Type,
		ResourceID:   resource.ID,
		Inherit:      inherit,
		GrantedBy:    firstNonEmptyString(subject.Sub, subject.ID),
	})
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case strings.Contains(err.Error(), "already exists"):
			status = http.StatusConflict
		case strings.Contains(err.Error(), "not found"):
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}

	_ = httpapi.WriteJSON(w, http.StatusCreated, accessGrantResponseFromRecord(record))
}

func (a *App) handleListAccessGrants(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	resourceType := strings.TrimSpace(r.URL.Query().Get("resource_type"))
	resourceID := strings.TrimSpace(r.URL.Query().Get("resource_id"))
	if resourceType == "" && resourceID == "" {
		decision, err := a.aaaCheck(r.Context(), subject, "iam.admin", model.ResourceRef{Type: "iam", ID: "admin"}, a.aaaRequestContext(r))
		if err != nil {
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return
		}
		if !decision.Allowed {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	} else {
		resource, err := resolveAccessGrantResource(r.Context(), a.db, resourceType, resourceID, true)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		if err := authorizeGrantOperation(r.Context(), subject, resource, productRoleOwner, a.aaaCheck, a.aaaRequestContext(r)); err != nil {
			if err.Error() == "forbidden" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return
		}
	}

	query := `
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
		WHERE ($1 = '' OR resource_type = $1)
		  AND ($2 = '' OR resource_id = $2)
		  AND ($3 = '' OR role_name = $3)
		ORDER BY resource_type ASC, resource_id ASC, subject_type ASC, subject_display ASC
	`

	roleFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("role")))
	rows, err := a.db.Query(r.Context(), query, normalizedOrEmpty(resourceType, resourceID), normalizedResourceIDOrEmpty(resourceType, resourceID), roleFilter)
	if err != nil {
		http.Error(w, "failed to list grants", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	records := make([]accessGrantResponse, 0)
	for rows.Next() {
		var record accessGrantRecord
		if err := rows.Scan(
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
		); err != nil {
			http.Error(w, "failed to read grants", http.StatusInternalServerError)
			return
		}
		records = append(records, accessGrantResponseFromRecord(record))
	}

	_ = httpapi.WriteJSON(w, http.StatusOK, records)
}

func normalizedOrEmpty(resourceType, resourceID string) string {
	if strings.TrimSpace(resourceType) == "" || strings.TrimSpace(resourceID) == "" {
		return ""
	}
	value, err := normalizeAccessGrantResourceType(resourceType)
	if err != nil {
		return ""
	}
	return value
}

func normalizedResourceIDOrEmpty(resourceType, resourceID string) string {
	if strings.TrimSpace(resourceType) == "" || strings.TrimSpace(resourceID) == "" {
		return ""
	}
	resource, err := resolveAccessGrantResource(context.Background(), &noopQueryRunner{}, resourceType, resourceID, false)
	if err != nil {
		return ""
	}
	return resource.ID
}

type noopQueryRunner struct{}

func (n *noopQueryRunner) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, fmt.Errorf("unsupported")
}
func (n *noopQueryRunner) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, fmt.Errorf("unsupported")
}
func (n *noopQueryRunner) QueryRow(context.Context, string, ...any) pgx.Row { return nil }

func (a *App) handleDeleteAccessGrant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	grantID, err := parseAccessGrantID(r.PathValue("grantID"))
	if err != nil {
		http.Error(w, "invalid grant id", http.StatusBadRequest)
		return
	}

	record, err := loadAccessGrantRecord(r.Context(), a.db, grantID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	resource := accessGrantResource{
		Type:    record.ResourceType,
		ID:      record.ResourceID,
		Display: record.ResourceDisplay,
	}
	if err := authorizeGrantOperation(r.Context(), subject, resource, record.RoleName, a.aaaCheck, a.aaaRequestContext(r)); err != nil {
		if err.Error() == "forbidden" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	if _, err := a.deleteProductRoleGrant(r.Context(), grantID); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleGetEffectivePermissions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	action := strings.TrimSpace(r.URL.Query().Get("action"))
	resourceType := strings.TrimSpace(r.URL.Query().Get("resource_type"))
	resourceID := strings.TrimSpace(r.URL.Query().Get("resource_id"))
	if err := httpapi.ValidateRequired(
		httpapi.RequiredString("action", action),
		httpapi.RequiredString("resource_type", resourceType),
		httpapi.RequiredString("resource_id", resourceID),
	); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resource, err := resolveAccessGrantResource(r.Context(), a.db, resourceType, resourceID, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	decision, err := a.aaaCheck(r.Context(), subject, action, model.ResourceRef{Type: resource.Type, ID: resource.ID}, a.aaaRequestContext(r))
	if err != nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	response := buildEffectivePermissionResponse(action, resource, decision)
	_ = httpapi.WriteJSON(w, http.StatusOK, response)
}

func buildEffectivePermissionResponse(action string, resource accessGrantResource, decision model.Decision) effectivePermissionResponse {
	resp := effectivePermissionResponse{
		Allowed:       decision.Allowed,
		Action:        action,
		Resource:      formatResourceLabel(resource.Type, resource.DisplayOrID()),
		Reason:        strings.TrimSpace(decision.Reason),
		Inherited:     strings.Contains(decision.Reason, "inheritance"),
		MatchedPolicy: decision.MatchedPolicy,
	}

	if decision.MatchedPolicy == nil {
		return resp
	}

	if roleName, _ := decision.MatchedPolicy["role_name"].(string); strings.TrimSpace(roleName) != "" {
		resp.MatchedRole = strings.TrimSpace(roleName)
	}
	if subjectType, _ := decision.MatchedPolicy["subject_type"].(string); strings.TrimSpace(subjectType) != "" {
		subjectID, _ := decision.MatchedPolicy["subject_id"].(string)
		resp.MatchedSubject = formatSubjectLabel(subjectType, subjectID)
	}
	if matchedResourceType, _ := decision.MatchedPolicy["resource_type"].(string); strings.TrimSpace(matchedResourceType) != "" {
		matchedResourceID, _ := decision.MatchedPolicy["resource_id"].(string)
		resp.MatchedResource = formatResourceLabel(matchedResourceType, matchedResourceID)
		if resp.Inherited {
			resp.SourceParentResource = resp.MatchedResource
		}
	}
	if matchedAction, _ := decision.MatchedPolicy["action"].(string); strings.TrimSpace(matchedAction) != "" {
		resp.LowLevelPermission = matchedAction
	}

	resp.Reason = buildHumanReadableDecisionReason(resp, decision)
	return resp
}

func buildHumanReadableDecisionReason(resp effectivePermissionResponse, decision model.Decision) string {
	if resp.MatchedRole == "" || resp.MatchedSubject == "" || resp.MatchedResource == "" {
		return strings.TrimSpace(decision.Reason)
	}
	if resp.Inherited {
		return fmt.Sprintf("%s has %s on %s, inherited by %s", resp.MatchedSubject, resp.MatchedRole, resp.MatchedResource, resp.Resource)
	}
	return fmt.Sprintf("%s has %s on %s", resp.MatchedSubject, resp.MatchedRole, resp.MatchedResource)
}

func accessGrantResponseFromRecord(record accessGrantRecord) accessGrantResponse {
	source := "ui"
	if record.ManagedByConfig {
		source = "gitops"
	}
	inheritedFromResourceID := externalGrantResourceID(record.InheritedFromResourceType, record.InheritedFromResourceDisplay, record.InheritedFromResourceID)
	inheritedFromResource := ""
	if record.InheritedFromResourceType != "" && inheritedFromResourceID != "" {
		inheritedFromResource = formatResourceLabel(record.InheritedFromResourceType, inheritedFromResourceID)
	}
	return accessGrantResponse{
		ID:                        formatAccessGrantID(record.ID),
		SubjectType:               record.SubjectType,
		SubjectID:                 record.SubjectID,
		SubjectDisplay:            record.SubjectDisplay,
		Role:                      record.RoleName,
		ResourceType:              record.ResourceType,
		ResourceID:                externalGrantResourceID(record.ResourceType, record.ResourceDisplay, record.ResourceID),
		Inherit:                   record.Inherit,
		GrantedBy:                 record.GrantedBy,
		CreatedAt:                 record.CreatedAt,
		ManagedByConfigRepo:       record.ManagedByConfig,
		ConfigSourcePath:          record.ConfigSourcePath,
		ConfigSourceCommitSHA:     record.ConfigSourceCommitSHA,
		Source:                    source,
		InheritedFromResourceType: record.InheritedFromResourceType,
		InheritedFromResourceID:   inheritedFromResourceID,
		InheritedFromResource:     inheritedFromResource,
	}
}

func externalGrantResourceID(resourceType, display, internalID string) string {
	if resourceType == grantResourceFolder && internalID == generalGrantID {
		return "general"
	}
	if strings.TrimSpace(display) != "" {
		return display
	}
	if resourceType == grantResourceFolder {
		return "/" + strings.Trim(strings.TrimSpace(internalID), "/")
	}
	if resourceType == grantResourcePlatform {
		return "platform"
	}
	return internalID
}

func formatAccessGrantID(id int64) string {
	return fmt.Sprintf("grant_%d", id)
}

func parseAccessGrantID(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "grant_")
	return strconv.ParseInt(raw, 10, 64)
}

func formatSubjectLabel(subjectType, subjectID string) string {
	subjectID = strings.TrimSpace(subjectID)
	switch subjectType {
	case grantSubjectGroup:
		return "group " + subjectID
	case model.SubjectTypeAuthGroup:
		return "group " + subjectID
	case model.SubjectTypeRepository:
		return "repository " + subjectID
	case model.SubjectTypeTrigger:
		return "trigger " + subjectID
	case model.SubjectTypeServiceAccount:
		return "service account " + subjectID
	case model.SubjectTypeInternalService:
		return "service " + subjectID
	default:
		return "user " + subjectID
	}
}

func formatResourceLabel(resourceType, resourceID string) string {
	resourceType = strings.TrimSpace(resourceType)
	resourceID = strings.TrimSpace(resourceID)
	if resourceType == grantResourceSecret || resourceType == grantResourceVariable {
		if label := formatNamedResourceLabel(resourceType, resourceID); label != "" {
			return label
		}
	}
	if resourceType == grantResourceScope && resourceID == "" {
		resourceID = "default"
	}
	if resourceType == grantResourceFolder && resourceID == generalGrantID {
		resourceID = "general"
	}
	if resourceType == grantResourceFolder && resourceID != "" && resourceID != "general" && !strings.HasPrefix(resourceID, "/") {
		resourceID = "/" + strings.Trim(resourceID, "/")
	}
	return resourceType + ":" + resourceID
}

func formatNamedResourceLabel(resourceType, resourceID string) string {
	repoName, scope, name := model.ParseNamedResourceID(resourceID)
	if strings.TrimSpace(name) == "" {
		return ""
	}
	if strings.TrimSpace(scope) == "" {
		scope = "default"
	}
	parts := []string{
		"name=" + strings.TrimSpace(name),
		"scope=" + strings.Trim(strings.TrimSpace(scope), "/"),
	}
	if repoName = strings.TrimSpace(repoName); repoName != "" {
		parts = append(parts, "repo="+repoName)
	}
	return strings.TrimSpace(resourceType) + ":" + strings.Join(parts, " ")
}

func isGeneralGrantResourceID(raw string) bool {
	switch strings.ToLower(strings.Trim(strings.TrimSpace(raw), "/")) {
	case "", ".", "general", strings.ToLower(generalGrantID):
		return strings.TrimSpace(raw) != ""
	default:
		return false
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func (r accessGrantResource) DisplayOrID() string {
	return firstNonEmptyString(r.Display, r.ID)
}
