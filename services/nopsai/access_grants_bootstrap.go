package nopsai

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
		managed_by_identity_provider BOOLEAN NOT NULL DEFAULT FALSE,
		identity_provider_id TEXT NOT NULL DEFAULT '',
		external_group_name TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(subject_type, subject_id, resource_type, resource_id)
	)`,
	`ALTER TABLE access_grants ADD COLUMN IF NOT EXISTS managed_by_identity_provider BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE access_grants ADD COLUMN IF NOT EXISTS identity_provider_id TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE access_grants ADD COLUMN IF NOT EXISTS external_group_name TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE resource_acl ADD COLUMN IF NOT EXISTS access_grant_id BIGINT REFERENCES access_grants(id) ON DELETE CASCADE`,
	`ALTER TABLE resource_ownership ADD COLUMN IF NOT EXISTS access_grant_id BIGINT REFERENCES access_grants(id) ON DELETE CASCADE`,
	`ALTER TABLE auth_group_members DROP CONSTRAINT IF EXISTS auth_group_members_subject_type_check`,
	`ALTER TABLE auth_group_members ADD CONSTRAINT auth_group_members_subject_type_check CHECK (subject_type IN ('user', 'repository', 'trigger', 'service_account', 'internal_service'))`,
	`ALTER TABLE auth_group_members ADD COLUMN IF NOT EXISTS managed_by_identity_provider BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE auth_group_members ADD COLUMN IF NOT EXISTS identity_provider_id TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE auth_group_members ADD COLUMN IF NOT EXISTS external_group_name TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE auth_group_members ADD COLUMN IF NOT EXISTS auth_group_name TEXT NOT NULL DEFAULT ''`,
	`CREATE INDEX IF NOT EXISTS idx_auth_group_members_identity_provider ON auth_group_members(identity_provider_id, external_group_name) WHERE managed_by_identity_provider = TRUE`,
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
	`CREATE INDEX IF NOT EXISTS idx_access_grants_identity_provider ON access_grants(identity_provider_id, external_group_name) WHERE managed_by_identity_provider = TRUE`,
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
