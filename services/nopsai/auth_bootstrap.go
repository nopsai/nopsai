package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nopsai/config"
	aaastore "nopsai/services/aaa/pkg/store"
	"nopsai/services/nopsai/pkg/auth"
)

const bootstrapAdminProductionMinPasswordLength = 12

type bootstrapAdminCredentials struct {
	Email                string
	EmailConfigured      bool
	Password             string
	PasswordConfigured   bool
	AllowDefaultPassword bool
	MustChangePassword   bool
}

func ensureDefaultAdmin(ctx context.Context, db *pgxpool.Pool) error {
	return ensureBootstrapAdmin(ctx, db, nil)
}

func ensureBootstrapAdmin(ctx context.Context, db *pgxpool.Pool, cfg *config.Config) error {
	if db == nil {
		return nil
	}
	credentials, err := resolveBootstrapAdminCredentials(cfg)
	if err != nil {
		return err
	}
	requiresProductionGates := cfg != nil && cfg.RequiresProductionGates()

	var passwordHash string
	if credentials.PasswordConfigured {
		passwordHash, err = auth.HashPassword(credentials.Password)
		if err != nil {
			return fmt.Errorf("hash bootstrap admin password: %w", err)
		}
	}

	adminID := uuid.MustParse(defaultAdminID)
	var existingID uuid.UUID
	var provider sql.NullString
	var existingPasswordHash sql.NullString
	err = db.QueryRow(ctx, `
		SELECT id, provider, password_hash
		FROM users
		WHERE sub = $1
	`, defaultAdminSub).Scan(&existingID, &provider, &existingPasswordHash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if existingID == uuid.Nil {
		if !credentials.PasswordConfigured {
			if requiresProductionGates {
				return fmt.Errorf("production startup gates require NOPSAI_BOOTSTRAP_ADMIN_PASSWORD or NOPSAI_BOOTSTRAP_ADMIN_PASSWORD_FILE before creating the first administrator")
			}
			return fmt.Errorf("bootstrap admin password is required before the bootstrap administrator can be created")
		}
		_, err = db.Exec(ctx, `
			INSERT INTO users (id, sub, email, provider, password_hash, status, must_change_password)
			VALUES ($1, $2, $3, 'local', $4, 'active', $5)
			ON CONFLICT (sub) DO NOTHING
		`, adminID, defaultAdminSub, credentials.Email, passwordHash, credentials.MustChangePassword)
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
	needsPasswordRotation := shouldRotateBootstrapAdminPassword(existingPasswordHash)
	if existingID != uuid.Nil && needsPasswordRotation && !credentials.PasswordConfigured && requiresProductionGates {
		return fmt.Errorf("production startup gates reject the built-in admin password; set NOPSAI_BOOTSTRAP_ADMIN_PASSWORD or NOPSAI_BOOTSTRAP_ADMIN_PASSWORD_FILE to rotate %s", credentials.Email)
	}
	if existingID != uuid.Nil && credentials.EmailConfigured {
		if _, err := db.Exec(ctx, `UPDATE users SET email = $1 WHERE id = $2`, credentials.Email, existingID); err != nil {
			return err
		}
	}
	if existingID != uuid.Nil && credentials.PasswordConfigured && needsPasswordRotation {
		if _, err := db.Exec(ctx, `
			UPDATE users
			SET password_hash = $1, status = 'active', must_change_password = $2
			WHERE id = $3
		`, passwordHash, credentials.MustChangePassword, existingID); err != nil {
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

func resolveBootstrapAdminCredentials(cfg *config.Config) (bootstrapAdminCredentials, error) {
	admin := config.BootstrapAdminConfig{}
	requiresProductionGates := false
	if cfg != nil {
		admin = config.NormalizeBootstrapAdminConfig(cfg.BootstrapAdmin)
		requiresProductionGates = cfg.RequiresProductionGates()
	}

	email := defaultAdminEmail
	emailConfigured := admin.Email != ""
	if emailConfigured {
		normalizedEmail, err := normalizeOptionalEmail(admin.Email)
		if err != nil {
			return bootstrapAdminCredentials{}, fmt.Errorf("bootstrap admin email: %w", err)
		}
		if normalizedEmail == "" {
			return bootstrapAdminCredentials{}, fmt.Errorf("bootstrap admin email is required")
		}
		email = normalizedEmail
	}

	password := admin.Password
	passwordConfigured := password != ""
	passwordFileConfigured := admin.PasswordFile != ""
	if passwordConfigured && passwordFileConfigured {
		return bootstrapAdminCredentials{}, fmt.Errorf("configure either NOPSAI_BOOTSTRAP_ADMIN_PASSWORD or NOPSAI_BOOTSTRAP_ADMIN_PASSWORD_FILE, not both")
	}
	if passwordFileConfigured {
		raw, err := os.ReadFile(admin.PasswordFile)
		if err != nil {
			return bootstrapAdminCredentials{}, fmt.Errorf("read bootstrap admin password file: %w", err)
		}
		password = strings.TrimRight(string(raw), "\r\n")
		passwordConfigured = true
	}

	mustChangePassword := passwordConfigured
	if admin.MustChangePassword != nil {
		mustChangePassword = *admin.MustChangePassword
	}
	implicitDevelopmentDefault := false
	if !passwordConfigured && !requiresProductionGates {
		password = "admin"
		passwordConfigured = true
		mustChangePassword = true
		implicitDevelopmentDefault = true
	}

	credentials := bootstrapAdminCredentials{
		Email:                email,
		EmailConfigured:      emailConfigured,
		Password:             password,
		PasswordConfigured:   passwordConfigured,
		AllowDefaultPassword: admin.AllowDefaultPassword || implicitDevelopmentDefault,
		MustChangePassword:   mustChangePassword,
	}
	if err := validateBootstrapAdminCredentials(credentials, requiresProductionGates); err != nil {
		return bootstrapAdminCredentials{}, err
	}
	return credentials, nil
}

func validateBootstrapAdminCredentials(credentials bootstrapAdminCredentials, requiresProductionGates bool) error {
	if !credentials.PasswordConfigured {
		return nil
	}
	if strings.TrimSpace(credentials.Password) == "" {
		return fmt.Errorf("bootstrap admin password must not be empty")
	}
	if credentials.Password == "admin" {
		if requiresProductionGates {
			return fmt.Errorf("production startup gates reject the built-in admin password; choose a unique bootstrap admin password")
		}
		if !credentials.AllowDefaultPassword {
			return fmt.Errorf("bootstrap admin password cannot be 'admin' unless NOPSAI_BOOTSTRAP_ADMIN_ALLOW_DEFAULT_PASSWORD=true")
		}
	}
	if requiresProductionGates && len([]rune(credentials.Password)) < bootstrapAdminProductionMinPasswordLength {
		return fmt.Errorf("production bootstrap admin password must be at least %d characters", bootstrapAdminProductionMinPasswordLength)
	}
	return nil
}

func shouldRotateBootstrapAdminPassword(passwordHash sql.NullString) bool {
	if !passwordHash.Valid || strings.TrimSpace(passwordHash.String) == "" {
		return true
	}
	if passwordHash.String == defaultAdminPasswordHash {
		return true
	}
	return auth.ComparePassword(passwordHash.String, "admin") == nil
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
	case shouldRotateBootstrapAdminPassword(passwordHash):
		return defaultAdminPasswordDefault, nil
	default:
		return defaultAdminPasswordReady, nil
	}
}
