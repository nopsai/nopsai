package nopsai

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/pkg/auth"
)

func pruneManagedAccessConfiguration(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, plan accessSyncPlan, grantKeys map[resolvedAccessGrantKey]struct{}) error {
	if err := pruneManagedAccessGrants(ctx, tx, binding, grantKeys); err != nil {
		return err
	}
	if err := pruneManagedUsers(ctx, tx, binding.ID, plan.users); err != nil {
		return err
	}
	if err := pruneManagedServiceAccounts(ctx, tx, binding.ID, plan.serviceAccounts); err != nil {
		return err
	}
	if err := pruneManagedRoles(ctx, tx, binding.ID, plan); err != nil {
		return err
	}
	return nil
}

func clearResourceAccessOverridesForConfigSync(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository) error {
	rows, err := tx.Query(ctx, `
		SELECT resource_type, resource_id
		FROM resource_access_overrides
	`)
	if err != nil {
		return fmt.Errorf("failed to load resource access overrides: %w", err)
	}
	defer rows.Close()

	type overrideKey struct {
		resourceType string
		resourceID   string
	}
	var keys []overrideKey
	for rows.Next() {
		var key overrideKey
		if err := rows.Scan(&key.resourceType, &key.resourceID); err != nil {
			return err
		}
		if accessGrantResourceInConfigBindingScope(key.resourceType, key.resourceID, binding) {
			keys = append(keys, key)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, key := range keys {
		if _, err := tx.Exec(ctx, `
			DELETE FROM resource_access_overrides
			WHERE resource_type = $1 AND resource_id = $2
		`, key.resourceType, key.resourceID); err != nil {
			return fmt.Errorf("failed to clear resource access override for %s:%s: %w", key.resourceType, key.resourceID, err)
		}
	}
	return nil
}

func pruneManagedAccessGrants(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, keep map[resolvedAccessGrantKey]struct{}) error {
	rows, err := tx.Query(ctx, `
		SELECT id, subject_type, subject_id, role_name, resource_type, resource_id, resource_display, managed_by_config_repo
		FROM access_grants
		WHERE (managed_by_config_repo = TRUE AND config_repo_id = $1)
		   OR managed_by_config_repo = FALSE
	`, binding.ID)
	if err != nil {
		return fmt.Errorf("failed to load managed access grants for pruning: %w", err)
	}
	defer rows.Close()

	type grantRow struct {
		id              int64
		subjectType     string
		subjectID       string
		roleName        string
		resourceType    string
		resourceID      string
		resourceDisplay string
		managed         bool
	}
	var prune []grantRow
	for rows.Next() {
		var row grantRow
		if err := rows.Scan(&row.id, &row.subjectType, &row.subjectID, &row.roleName, &row.resourceType, &row.resourceID, &row.resourceDisplay, &row.managed); err != nil {
			return err
		}
		if !row.managed && !accessGrantResourceInConfigBindingScope(row.resourceType, row.resourceID, binding) {
			continue
		}
		key := resolvedAccessGrantKey{
			subjectType:  row.subjectType,
			subjectID:    row.subjectID,
			resourceType: row.resourceType,
			resourceID:   row.resourceID,
		}
		if _, ok := keep[key]; !ok {
			prune = append(prune, row)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, row := range prune {
		if row.roleName == productRoleAdmin {
			if _, err := tx.Exec(ctx, `
				DELETE FROM auth_role_bindings
				WHERE role_name = $1
				  AND subject_type = $2
				  AND subject_id = $3
				  AND (managed_by_config_repo = FALSE OR config_repo_id = $4)
			`, productRoleAdmin, row.subjectType, row.subjectID, binding.ID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM access_grants WHERE id = $1`, row.id); err != nil {
			return fmt.Errorf("failed to prune access grant: %w", err)
		}
	}
	return nil
}

func accessGrantResourceInConfigBindingScope(resourceType, resourceID string, binding models.ConfigRepository) bool {
	switch binding.ScopeType {
	case models.ConfigRepositoryScopeSystem:
		return true
	case models.ConfigRepositoryScopeFolder:
		return accessGrantResourceUnderBindingScope(resourceType, resourceID, binding.ScopeID)
	default:
		return false
	}
}

func pruneStaleAccessGrantsForManagedUsers(ctx context.Context, tx pgx.Tx, users map[string]storedAccessUser) error {
	for _, user := range users {
		labels := []string{strings.TrimSpace(user.sub), strings.TrimSpace(user.email)}
		filtered := labels[:0]
		for _, label := range labels {
			if label != "" {
				filtered = append(filtered, label)
			}
		}
		if len(filtered) == 0 {
			continue
		}

		var currentID string
		err := tx.QueryRow(ctx, `SELECT id::text FROM users WHERE sub = $1 LIMIT 1`, user.sub).Scan(&currentID)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id
			FROM access_grants
			WHERE subject_type = 'user'
			  AND subject_id != $1
			  AND subject_display = ANY($2)
		`, currentID, filtered)
		if err != nil {
			return err
		}
		var grantIDs []int64
		for rows.Next() {
			var grantID int64
			if err := rows.Scan(&grantID); err != nil {
				rows.Close()
				return err
			}
			grantIDs = append(grantIDs, grantID)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()

		for _, grantID := range grantIDs {
			if _, err := tx.Exec(ctx, `DELETE FROM access_grants WHERE id = $1`, grantID); err != nil {
				return err
			}
		}
	}
	return nil
}

func pruneManagedUsers(ctx context.Context, tx pgx.Tx, configRepoID int64, users map[string]storedAccessUser) error {
	query := `
		SELECT id::text
		FROM users
		WHERE managed_by_config_repo = TRUE
		  AND config_repo_id = $1
		  AND provider <> $2
	`
	args := []any{configRepoID, auth.ProviderServiceAccount}
	if len(users) > 0 {
		subs := make([]string, 0, len(users))
		for sub := range users {
			subs = append(subs, sub)
		}
		query += ` AND sub != ALL($3)`
		args = append(args, subs)
	}

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	var prunedUserIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			rows.Close()
			return err
		}
		prunedUserIDs = append(prunedUserIDs, userID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, userID := range prunedUserIDs {
		if err := deleteUserAccessArtifacts(ctx, tx, userID); err != nil {
			return err
		}
	}

	if len(users) == 0 {
		_, err := tx.Exec(ctx, `
			DELETE FROM users
			WHERE managed_by_config_repo = TRUE
			  AND config_repo_id = $1
			  AND provider <> $2
		`, configRepoID, auth.ProviderServiceAccount)
		return err
	}
	subs := make([]string, 0, len(users))
	for sub := range users {
		subs = append(subs, sub)
	}
	_, err = tx.Exec(ctx, `
		DELETE FROM users
		WHERE managed_by_config_repo = TRUE
		  AND config_repo_id = $1
		  AND provider <> $2
		  AND sub != ALL($3)
	`, configRepoID, auth.ProviderServiceAccount, subs)
	return err
}

func pruneManagedServiceAccounts(ctx context.Context, tx pgx.Tx, configRepoID int64, serviceAccounts map[string]storedAccessServiceAccount) error {
	query := `
		SELECT sub
		FROM users
		WHERE managed_by_config_repo = TRUE
		  AND config_repo_id = $1
		  AND provider = $2
	`
	args := []any{configRepoID, auth.ProviderServiceAccount}
	if len(serviceAccounts) > 0 {
		subs := make([]string, 0, len(serviceAccounts))
		for sub := range serviceAccounts {
			subs = append(subs, sub)
		}
		query += ` AND sub != ALL($3)`
		args = append(args, subs)
	}

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	var prunedServiceAccountSubs []string
	for rows.Next() {
		var sub string
		if err := rows.Scan(&sub); err != nil {
			rows.Close()
			return err
		}
		prunedServiceAccountSubs = append(prunedServiceAccountSubs, sub)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, sub := range prunedServiceAccountSubs {
		if err := deleteServiceAccountAccessArtifacts(ctx, tx, sub); err != nil {
			return err
		}
	}

	if len(serviceAccounts) == 0 {
		_, err := tx.Exec(ctx, `
			DELETE FROM users
			WHERE managed_by_config_repo = TRUE
			  AND config_repo_id = $1
			  AND provider = $2
		`, configRepoID, auth.ProviderServiceAccount)
		return err
	}
	subs := make([]string, 0, len(serviceAccounts))
	for sub := range serviceAccounts {
		subs = append(subs, sub)
	}
	_, err = tx.Exec(ctx, `
		DELETE FROM users
		WHERE managed_by_config_repo = TRUE
		  AND config_repo_id = $1
		  AND provider = $2
		  AND sub != ALL($3)
	`, configRepoID, auth.ProviderServiceAccount, subs)
	return err
}

func pruneManagedRoles(ctx context.Context, tx pgx.Tx, configRepoID int64, plan accessSyncPlan) error {
	keep := map[string]struct{}{}
	for name := range plan.roles {
		keep[name] = struct{}{}
	}
	for _, policy := range plan.policies {
		keep[policy.role] = struct{}{}
	}
	for _, roleBinding := range plan.roleBindings {
		keep[roleBinding.role] = struct{}{}
	}

	if len(keep) == 0 {
		_, err := tx.Exec(ctx, `
			DELETE FROM auth_roles
			WHERE managed_by_config_repo = TRUE
			  AND config_repo_id = $1
		`, configRepoID)
		return err
	}
	names := make([]string, 0, len(keep))
	for name := range keep {
		names = append(names, name)
	}
	_, err := tx.Exec(ctx, `
		DELETE FROM auth_roles
		WHERE managed_by_config_repo = TRUE
		  AND config_repo_id = $1
		  AND name != ALL($2)
	`, configRepoID, names)
	return err
}

func ensureGlobalConfigObjectWritable(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, tableName, resourceKind, resourceID, whereClause string, args ...any) error {
	query := fmt.Sprintf("SELECT config_repo_id, managed_by_config_repo FROM %s WHERE %s", tableName, whereClause)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var existingRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(&existingRepoID, &managed); err != nil {
			return err
		}
		if !managed {
			continue
		}
		if !existingRepoID.Valid {
			return fmt.Errorf("%s %s is already managed by an unknown config repository", resourceKind, resourceID)
		}
		if existingRepoID.Int64 != binding.ID {
			return fmt.Errorf("%s %s is already managed by config repository %d", resourceKind, resourceID, existingRepoID.Int64)
		}
	}
	return rows.Err()
}
