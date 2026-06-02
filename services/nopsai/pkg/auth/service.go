package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	LocalEnabled       bool
	SigningKey         string
	JWTIssuer          string
	JWTAudience        string
	AccessTTL          time.Duration
	RefreshTTL         time.Duration
	LoginRateLimit     int
	LoginLockoutThresh int
	LoginLockoutWindow time.Duration
}

type Service struct {
	db           *pgxpool.Pool
	local        *LocalJWTService
	cfg          Config
	rateLimiter  *RateLimiter
	lockout      *LockoutTracker
	refreshTTL   time.Duration
	refreshTable string
}

type LoginResult struct {
	AccessToken        string    `json:"access_token"`
	RefreshToken       string    `json:"refresh_token,omitempty"`
	ExpiresAt          time.Time `json:"expires_at"`
	Claims             *Claims   `json:"claims"`
	MustChangePassword bool      `json:"must_change_password,omitempty"`
}

func NewService(ctx context.Context, db *pgxpool.Pool, cfg Config) (*Service, error) {
	_ = ctx
	if db == nil {
		return nil, fmt.Errorf("db is required")
	}
	s := &Service{
		db:           db,
		cfg:          cfg,
		rateLimiter:  NewRateLimiter(),
		lockout:      NewLockoutTracker(cfg.LoginLockoutThresh, cfg.LoginLockoutWindow),
		refreshTTL:   cfg.RefreshTTL,
		refreshTable: "refresh_tokens",
	}

	if cfg.SigningKey != "" {
		accessTTL := cfg.AccessTTL
		if accessTTL == 0 {
			accessTTL = time.Hour
		}
		s.local = NewLocalJWTService([]byte(cfg.SigningKey), cfg.JWTIssuer, cfg.JWTAudience, accessTTL)
	}
	return s, nil
}

func (s *Service) AuthenticateToken(ctx context.Context, raw string) (*Claims, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty token")
	}

	if s.local != nil {
		if claims, err := s.local.ParseAndValidate(raw); err == nil {
			return claims, nil
		}
	}
	if strings.HasPrefix(raw, PersonalAccessTokenPrefix) {
		if claims, err := s.authenticatePersonalAccessToken(ctx, raw); err == nil {
			return claims, nil
		}
	}
	if strings.HasPrefix(raw, ServiceAccountTokenPrefix) {
		if claims, err := s.authenticateServiceAccountToken(ctx, raw); err == nil {
			return claims, nil
		}
	}

	return nil, fmt.Errorf("token could not be verified")
}

func (s *Service) authenticatePersonalAccessToken(ctx context.Context, raw string) (*Claims, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("token store is not configured")
	}
	hash := HashToken(raw)
	var (
		tokenID uuid.UUID
		userID  uuid.UUID
		sub     string
		email   sql.NullString
		status  string
	)
	row := s.db.QueryRow(ctx, `
		SELECT pat.id, pat.user_id, u.sub, u.email, u.status
		FROM personal_access_tokens pat
		JOIN users u ON u.id = pat.user_id
		WHERE pat.token_hash = $1
		  AND pat.revoked_at IS NULL
		  AND (pat.expires_at IS NULL OR pat.expires_at > NOW())
	`, hash)
	if err := row.Scan(&tokenID, &userID, &sub, &email, &status); err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, fmt.Errorf("account disabled")
	}

	roles, err := s.fetchRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	_, _ = s.db.Exec(ctx, `
		UPDATE personal_access_tokens
		SET last_used_at = NOW()
		WHERE id = $1
		  AND (last_used_at IS NULL OR last_used_at < NOW() - INTERVAL '1 minute')
	`, tokenID)

	return &Claims{
		Sub:      sub,
		Email:    email.String,
		Provider: ProviderPersonalAccessToken,
		Roles:    roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: sub,
			Issuer:  s.cfg.JWTIssuer,
		},
	}, nil
}

func (s *Service) authenticateServiceAccountToken(ctx context.Context, raw string) (*Claims, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("token store is not configured")
	}
	hash := HashToken(raw)
	var (
		tokenID uuid.UUID
		userID  uuid.UUID
		sub     string
		email   sql.NullString
		status  string
	)
	row := s.db.QueryRow(ctx, `
		SELECT sat.id, sat.service_account_id, u.sub, u.email, u.status
		FROM service_account_tokens sat
		JOIN users u ON u.id = sat.service_account_id
		WHERE sat.token_hash = $1
		  AND sat.revoked_at IS NULL
		  AND (sat.expires_at IS NULL OR sat.expires_at > NOW())
		  AND u.provider = $2
	`, hash, ProviderServiceAccount)
	if err := row.Scan(&tokenID, &userID, &sub, &email, &status); err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, fmt.Errorf("account disabled")
	}

	roles, err := s.fetchServiceAccountRoles(ctx, sub)
	if err != nil {
		return nil, err
	}
	_, _ = s.db.Exec(ctx, `
		UPDATE service_account_tokens
		SET last_used_at = NOW()
		WHERE id = $1
		  AND (last_used_at IS NULL OR last_used_at < NOW() - INTERVAL '1 minute')
	`, tokenID)

	return &Claims{
		Sub:      sub,
		Email:    email.String,
		Provider: ProviderServiceAccountToken,
		Roles:    roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: sub,
			Issuer:  s.cfg.JWTIssuer,
		},
	}, nil
}

func (s *Service) LoginLocal(ctx context.Context, identifier, password string) (*LoginResult, error) {
	if !s.cfg.LocalEnabled {
		return nil, fmt.Errorf("local authentication is disabled")
	}
	if s.local == nil {
		return nil, fmt.Errorf("local JWT signing is not configured")
	}
	if identifier == "" || password == "" {
		return nil, fmt.Errorf("username/email and password are required")
	}

	if s.lockout != nil && s.lockout.IsLocked(identifier) {
		return nil, fmt.Errorf("account temporarily locked due to failed attempts")
	}
	if s.rateLimiter != nil && !s.rateLimiter.Allow(identifier, s.cfg.LoginRateLimit, time.Minute) {
		return nil, fmt.Errorf("too many login attempts, slow down")
	}

	var (
		userID       uuid.UUID
		sub          string
		email        sql.NullString
		provider     string
		passwordHash sql.NullString
		status       string
		mustChange   bool
	)

	if err := s.lookupLoginUser(ctx, identifier, &userID, &sub, &email, &provider, &passwordHash, &status, &mustChange); err != nil {
		return nil, err
	}

	if status != "active" {
		return nil, fmt.Errorf("account disabled")
	}
	if provider != "local" {
		return nil, fmt.Errorf("password login is unavailable for this account")
	}
	if err := ComparePassword(passwordHash.String, password); err != nil {
		if s.lockout != nil && s.lockout.RecordFailure(identifier) {
			return nil, fmt.Errorf("too many failed attempts, account temporarily locked")
		}
		return nil, fmt.Errorf("invalid credentials")
	}

	if s.lockout != nil {
		s.lockout.Reset(identifier)
	}
	if s.rateLimiter != nil {
		s.rateLimiter.Reset(identifier)
	}

	roles, err := s.fetchRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("fetch roles: %w", err)
	}

	claims := &Claims{
		Sub:      sub,
		Email:    email.String,
		Provider: "local",
		Roles:    roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: sub,
			Issuer:  s.cfg.JWTIssuer,
		},
	}

	accessToken, exp, err := s.local.MintAccessToken(ctx, *claims)
	if err != nil {
		return nil, err
	}
	refreshToken := ""
	if s.refreshTTL > 0 {
		refreshToken, err = s.persistRefreshToken(ctx, userID)
		if err != nil {
			return nil, err
		}
	}

	_, _ = s.db.Exec(ctx, `UPDATE users SET last_login = NOW() WHERE id = $1`, userID)

	return &LoginResult{
		AccessToken:        accessToken,
		RefreshToken:       refreshToken,
		ExpiresAt:          exp,
		Claims:             claims,
		MustChangePassword: mustChange,
	}, nil
}

func (s *Service) Refresh(ctx context.Context, rawRefresh string) (*LoginResult, error) {
	if rawRefresh == "" {
		return nil, fmt.Errorf("refresh token required")
	}
	if s.local == nil {
		return nil, fmt.Errorf("local JWT signing is not configured")
	}

	hash := HashToken(rawRefresh)
	var (
		tokenID uuid.UUID
		userID  uuid.UUID
		expires time.Time
		revoked sql.NullTime
	)

	row := s.db.QueryRow(ctx, `SELECT id, user_id, expires_at, revoked_at FROM refresh_tokens WHERE token_hash = $1`, hash)
	if err := row.Scan(&tokenID, &userID, &expires, &revoked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("invalid refresh token")
		}
		return nil, err
	}
	if revoked.Valid || time.Now().After(expires) {
		return nil, fmt.Errorf("refresh token expired or revoked")
	}

	var sub string
	var email sql.NullString
	var provider string
	var status string
	var mustChange bool
	row = s.db.QueryRow(ctx, `SELECT sub, email, provider, status, must_change_password FROM users WHERE id = $1`, userID)
	if err := row.Scan(&sub, &email, &provider, &status, &mustChange); err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, fmt.Errorf("account disabled")
	}

	roles, err := s.fetchRoles(ctx, userID)
	if err != nil {
		return nil, err
	}

	claims := &Claims{
		Sub:      sub,
		Email:    email.String,
		Provider: provider,
		Roles:    roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: sub,
			Issuer:  s.cfg.JWTIssuer,
		},
	}

	access, exp, err := s.local.MintAccessToken(ctx, *claims)
	if err != nil {
		return nil, err
	}

	newRefresh, err := s.persistRefreshToken(ctx, userID)
	if err != nil {
		return nil, err
	}
	_, _ = s.db.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = NOW() WHERE id = $1`, tokenID)

	return &LoginResult{
		AccessToken:        access,
		RefreshToken:       newRefresh,
		ExpiresAt:          exp,
		Claims:             claims,
		MustChangePassword: mustChange,
	}, nil
}

func (s *Service) Logout(ctx context.Context, rawRefresh string) error {
	rawRefresh = strings.TrimSpace(rawRefresh)
	if rawRefresh == "" {
		return fmt.Errorf("refresh token required")
	}
	hash := HashToken(rawRefresh)
	_, err := s.db.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = NOW() WHERE token_hash = $1`, hash)
	return err
}

func (s *Service) lookupLoginUser(
	ctx context.Context,
	identifier string,
	userID *uuid.UUID,
	sub *string,
	email *sql.NullString,
	provider *string,
	passwordHash *sql.NullString,
	status *string,
	mustChangePassword *bool,
) error {
	row := s.db.QueryRow(ctx, `
		SELECT id, sub, email, provider, password_hash, status, must_change_password
		FROM users
		WHERE sub = $1
	`, identifier)
	if err := row.Scan(userID, sub, email, provider, passwordHash, status, mustChangePassword); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, sub, email, provider, password_hash, status, must_change_password
		FROM users
		WHERE LOWER(email) = LOWER($1)
		ORDER BY id ASC
		LIMIT 2
	`, identifier)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return fmt.Errorf("invalid credentials")
	}
	if err := rows.Scan(userID, sub, email, provider, passwordHash, status, mustChangePassword); err != nil {
		return err
	}
	if rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return fmt.Errorf("account lookup is ambiguous; contact an administrator")
	}
	return rows.Err()
}

func (s *Service) fetchRoles(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT role FROM user_roles WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []string
	seen := make(map[string]struct{})
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	return roles, nil
}

func (s *Service) fetchServiceAccountRoles(ctx context.Context, serviceAccountID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT role_name
		FROM auth_role_bindings
		WHERE subject_type = 'service_account' AND subject_id = $1
	`, serviceAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []string
	seen := make(map[string]struct{})
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	return roles, nil
}

func (s *Service) persistRefreshToken(ctx context.Context, userID uuid.UUID) (string, error) {
	if s.refreshTTL <= 0 {
		return "", nil
	}
	raw, err := GenerateRefreshToken()
	if err != nil {
		return "", err
	}
	hash := HashToken(raw)
	tokenID := uuid.New()
	expires := time.Now().Add(s.refreshTTL)
	_, err = s.db.Exec(ctx, `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, tokenID, userID, hash, expires)
	if err != nil {
		return "", err
	}
	return raw, nil
}
