package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
	cfgMu        sync.RWMutex
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

func (s *Service) SetLocalEnabled(enabled bool) {
	if s == nil {
		return
	}
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	s.cfg.LocalEnabled = enabled
}

func (s *Service) localEnabled() bool {
	if s == nil {
		return false
	}
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg.LocalEnabled
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
	if !s.localEnabled() {
		return nil, fmt.Errorf("local authentication is disabled")
	}
	if s.local == nil {
		return nil, fmt.Errorf("local JWT signing is not configured")
	}
	if identifier == "" || password == "" {
		return nil, fmt.Errorf("username/email and password are required")
	}

	loginKey := canonicalLoginKey(identifier)
	if s.lockout != nil && s.lockout.IsLocked(loginKey) {
		return nil, fmt.Errorf("account temporarily locked due to failed attempts")
	}
	if s.rateLimiter != nil && !s.rateLimiter.Allow(loginKey, s.cfg.LoginRateLimit, time.Minute) {
		return nil, fmt.Errorf("too many login attempts, slow down")
	}
	if locked, err := s.persistentLoginLocked(ctx, loginKey); err != nil {
		return nil, err
	} else if locked {
		return nil, fmt.Errorf("account temporarily locked due to failed attempts")
	}
	if allowed, err := s.persistentLoginAttemptAllowed(ctx, loginKey); err != nil {
		return nil, err
	} else if !allowed {
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
		compareDummyPassword(password)
		s.recordPersistentLoginFailureBestEffort(ctx, loginKey)
		return nil, err
	}

	if status != "active" {
		compareDummyPassword(password)
		s.recordPersistentLoginFailureBestEffort(ctx, loginKey)
		return nil, fmt.Errorf("account disabled")
	}
	if provider != "local" {
		compareDummyPassword(password)
		s.recordPersistentLoginFailureBestEffort(ctx, loginKey)
		return nil, fmt.Errorf("password login is unavailable for this account")
	}
	if err := ComparePassword(passwordHash.String, password); err != nil {
		if locked, trackErr := s.recordPersistentLoginFailure(ctx, loginKey); trackErr != nil {
			return nil, trackErr
		} else if locked {
			return nil, fmt.Errorf("too many failed attempts, account temporarily locked")
		}
		if s.lockout != nil && s.lockout.RecordFailure(loginKey) {
			return nil, fmt.Errorf("too many failed attempts, account temporarily locked")
		}
		return nil, fmt.Errorf("invalid credentials")
	}

	if s.lockout != nil {
		s.lockout.Reset(loginKey)
	}
	if s.rateLimiter != nil {
		s.rateLimiter.Reset(loginKey)
	}
	if err := s.resetPersistentLoginTracking(ctx, loginKey); err != nil {
		return nil, err
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

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	hash := HashToken(rawRefresh)
	var (
		tokenID uuid.UUID
		userID  uuid.UUID
		expires time.Time
		revoked sql.NullTime
	)

	row := tx.QueryRow(ctx, `
		SELECT id, user_id, expires_at, revoked_at
		FROM refresh_tokens
		WHERE token_hash = $1
		FOR UPDATE
	`, hash)
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
	row = tx.QueryRow(ctx, `SELECT sub, email, provider, status, must_change_password FROM users WHERE id = $1`, userID)
	if err := row.Scan(&sub, &email, &provider, &status, &mustChange); err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, fmt.Errorf("account disabled")
	}

	roles, err := s.fetchRolesWithQuerier(ctx, tx, userID)
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

	tag, err := tx.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = NOW()
		WHERE id = $1 AND revoked_at IS NULL
	`, tokenID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, fmt.Errorf("refresh token already used")
	}

	newRefresh, err := s.persistRefreshTokenTx(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &LoginResult{
		AccessToken:        access,
		RefreshToken:       newRefresh,
		ExpiresAt:          exp,
		Claims:             claims,
		MustChangePassword: mustChange,
	}, nil
}

func (s *Service) IssueSessionForUser(ctx context.Context, userID uuid.UUID) (*LoginResult, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("user id is required")
	}
	if s.local == nil {
		return nil, fmt.Errorf("local JWT signing is not configured")
	}

	var sub string
	var email sql.NullString
	var provider string
	var status string
	var mustChange bool
	row := s.db.QueryRow(ctx, `SELECT sub, email, provider, status, must_change_password FROM users WHERE id = $1`, userID)
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
	refresh := ""
	if s.refreshTTL > 0 {
		refresh, err = s.persistRefreshToken(ctx, userID)
		if err != nil {
			return nil, err
		}
	}
	_, _ = s.db.Exec(ctx, `UPDATE users SET last_login = NOW() WHERE id = $1`, userID)

	return &LoginResult{
		AccessToken:        access,
		RefreshToken:       refresh,
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
	return s.fetchRolesWithQuerier(ctx, s.db, userID)
}

type roleQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (s *Service) fetchRolesWithQuerier(ctx context.Context, q roleQuerier, userID uuid.UUID) ([]string, error) {
	rows, err := q.Query(ctx, `
		SELECT role
		FROM user_roles
		WHERE user_id = $1
		UNION
		SELECT role_name
		FROM auth_role_bindings
		WHERE subject_type = 'user' AND subject_id = $2
		ORDER BY 1 ASC
	`, userID, userID.String())
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
	return s.persistRefreshTokenTx(ctx, s.db, userID)
}

type refreshTokenPersister interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func (s *Service) persistRefreshTokenTx(ctx context.Context, q refreshTokenPersister, userID uuid.UUID) (string, error) {
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
	_, err = q.Exec(ctx, `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, tokenID, userID, hash, expires)
	if err != nil {
		return "", err
	}
	return raw, nil
}

func (s *Service) persistentLoginLocked(ctx context.Context, loginKey string) (bool, error) {
	if s == nil || s.db == nil || s.cfg.LoginLockoutThresh <= 0 {
		return false, nil
	}
	var lockedUntil sql.NullTime
	err := s.db.QueryRow(ctx, `
		SELECT locked_until
		FROM auth_login_attempts
		WHERE key_hash = $1
	`, HashToken(loginKey)).Scan(&lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check login lockout: %w", err)
	}
	return lockedUntil.Valid && lockedUntil.Time.After(time.Now()), nil
}

func (s *Service) persistentLoginAttemptAllowed(ctx context.Context, loginKey string) (bool, error) {
	if s == nil || s.db == nil || s.cfg.LoginRateLimit <= 0 {
		return true, nil
	}
	var attemptCount int
	err := s.db.QueryRow(ctx, `
		INSERT INTO auth_login_attempts (key_hash, attempt_window_start, attempt_count, updated_at)
		VALUES ($1, NOW(), 1, NOW())
		ON CONFLICT (key_hash) DO UPDATE
		SET attempt_count = CASE
		      WHEN auth_login_attempts.attempt_window_start <= NOW() - ($2 * INTERVAL '1 second') THEN 1
		      ELSE auth_login_attempts.attempt_count + 1
		    END,
		    attempt_window_start = CASE
		      WHEN auth_login_attempts.attempt_window_start <= NOW() - ($2 * INTERVAL '1 second') THEN NOW()
		      ELSE auth_login_attempts.attempt_window_start
		    END,
		    updated_at = NOW()
		RETURNING attempt_count
	`, HashToken(loginKey), int(time.Minute.Seconds())).Scan(&attemptCount)
	if err != nil {
		return false, fmt.Errorf("record login attempt: %w", err)
	}
	return attemptCount <= s.cfg.LoginRateLimit, nil
}

func (s *Service) recordPersistentLoginFailure(ctx context.Context, loginKey string) (bool, error) {
	if s == nil || s.db == nil || s.cfg.LoginLockoutThresh <= 0 {
		return false, nil
	}
	windowSeconds := int(s.cfg.LoginLockoutWindow.Seconds())
	if windowSeconds <= 0 {
		windowSeconds = int((15 * time.Minute).Seconds())
	}
	var locked bool
	err := s.db.QueryRow(ctx, `
		INSERT INTO auth_login_attempts (key_hash, failure_window_start, failure_count, updated_at)
		VALUES ($1, NOW(), 1, NOW())
		ON CONFLICT (key_hash) DO UPDATE
		SET failure_count = CASE
		      WHEN auth_login_attempts.failure_window_start IS NULL
		        OR auth_login_attempts.failure_window_start <= NOW() - ($3 * INTERVAL '1 second') THEN 1
		      ELSE auth_login_attempts.failure_count + 1
		    END,
		    failure_window_start = CASE
		      WHEN auth_login_attempts.failure_window_start IS NULL
		        OR auth_login_attempts.failure_window_start <= NOW() - ($3 * INTERVAL '1 second') THEN NOW()
		      ELSE auth_login_attempts.failure_window_start
		    END,
		    locked_until = CASE
		      WHEN (
		        CASE
		          WHEN auth_login_attempts.failure_window_start IS NULL
		            OR auth_login_attempts.failure_window_start <= NOW() - ($3 * INTERVAL '1 second') THEN 1
		          ELSE auth_login_attempts.failure_count + 1
		        END
		      ) >= $2 THEN NOW() + ($3 * INTERVAL '1 second')
		      ELSE auth_login_attempts.locked_until
		    END,
		    updated_at = NOW()
		RETURNING locked_until IS NOT NULL AND locked_until > NOW()
	`, HashToken(loginKey), s.cfg.LoginLockoutThresh, windowSeconds).Scan(&locked)
	if err != nil {
		return false, fmt.Errorf("record login failure: %w", err)
	}
	return locked, nil
}

func (s *Service) recordPersistentLoginFailureBestEffort(ctx context.Context, loginKey string) {
	_, _ = s.recordPersistentLoginFailure(ctx, loginKey)
}

func (s *Service) resetPersistentLoginTracking(ctx context.Context, loginKey string) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(ctx, `DELETE FROM auth_login_attempts WHERE key_hash = $1`, HashToken(loginKey))
	if err != nil {
		return fmt.Errorf("reset login tracking: %w", err)
	}
	return nil
}

func canonicalLoginKey(identifier string) string {
	return strings.ToLower(strings.TrimSpace(identifier))
}

const dummyPasswordHash = "$2a$10$v2gKcaFneUWR4j8plbrgsOxPqg2U0o2vTkm8YZRs2ITo6lQ.6n5cS"

func compareDummyPassword(password string) {
	_ = ComparePassword(dummyPasswordHash, password)
}
