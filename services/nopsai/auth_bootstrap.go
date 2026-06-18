package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	aaastore "nopsai/services/aaa/pkg/store"
)

func ensureDefaultAdmin(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	adminID := uuid.MustParse(defaultAdminID)
	var existingID uuid.UUID
	var provider sql.NullString
	err := db.QueryRow(ctx, `SELECT id, provider FROM users WHERE sub = $1`, defaultAdminSub).Scan(&existingID, &provider)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if existingID == uuid.Nil {
		_, err = db.Exec(ctx, `
			INSERT INTO users (id, sub, email, provider, password_hash, status)
			VALUES ($1, $2, $3, 'local', $4, 'active')
			ON CONFLICT (sub) DO NOTHING
		`, adminID, defaultAdminSub, defaultAdminEmail, defaultAdminPasswordHash)
		if err != nil {
			return err
		}
		if err := db.QueryRow(ctx, `SELECT id FROM users WHERE sub = $1`, defaultAdminSub).Scan(&existingID); err != nil {
			return err
		}
	} else if provider.Valid && provider.String != "local" {
		if _, err := db.Exec(ctx, `UPDATE users SET provider = 'local' WHERE id = $1`, existingID); err != nil {
			return err
		}
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO user_roles (user_id, role)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, existingID, defaultAdminRole); err != nil {
		return err
	}
	if err := aaastore.EnsureRole(ctx, db, defaultAdminRole, "Default platform administrator"); err != nil {
		return err
	}
	if err := aaastore.EnsureRoleBinding(ctx, db, aaastore.RoleBinding{
		RoleName:    defaultAdminRole,
		SubjectType: "user",
		SubjectID:   existingID.String(),
	}); err != nil {
		return err
	}
	if err := aaastore.EnsureRolePermission(ctx, db, aaastore.RolePermission{
		RoleName:     defaultAdminRole,
		ResourceType: "*",
		ResourceID:   "*",
		Action:       "*",
		Effect:       "allow",
	}); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO role_permissions (role, name, obj, act)
		SELECT $1, 'All access', '/*', '.*'
		WHERE NOT EXISTS (
			SELECT 1 FROM role_permissions WHERE role = $1 AND obj = '/*' AND act = '.*'
		)
	`, defaultAdminRole); err != nil {
		return err
	}
	return nil
}

func ensureNoDefaultAdminPassword(ctx context.Context, db *pgxpool.Pool) error {
	state, err := defaultAdminPasswordState(ctx, db)
	if err != nil {
		return err
	}
	switch state {
	case defaultAdminPasswordMissing:
		return fmt.Errorf("production startup gates require a pre-provisioned administrator; refusing to seed default admin credentials")
	case defaultAdminPasswordDefault:
		return fmt.Errorf("production startup gates reject the default admin password; rotate admin@example.com before enabling production gates")
	default:
		return nil
	}
}

type defaultAdminPasswordStatus string

const (
	defaultAdminPasswordReady   defaultAdminPasswordStatus = "ready"
	defaultAdminPasswordMissing defaultAdminPasswordStatus = "missing"
	defaultAdminPasswordDefault defaultAdminPasswordStatus = "default"
)

func defaultAdminPasswordState(ctx context.Context, db *pgxpool.Pool) (defaultAdminPasswordStatus, error) {
	if db == nil {
		return defaultAdminPasswordReady, nil
	}
	var passwordHash sql.NullString
	err := db.QueryRow(ctx, `SELECT password_hash FROM users WHERE sub = $1`, defaultAdminSub).Scan(&passwordHash)
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, sql.ErrNoRows):
		return defaultAdminPasswordMissing, nil
	case err != nil:
		return defaultAdminPasswordMissing, err
	case passwordHash.Valid && passwordHash.String == defaultAdminPasswordHash:
		return defaultAdminPasswordDefault, nil
	default:
		return defaultAdminPasswordReady, nil
	}
}
