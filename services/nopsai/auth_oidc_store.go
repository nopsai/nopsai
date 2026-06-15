package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/services/nopsai/pkg/auth"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

func seedOIDCConfigProviders(ctx context.Context, db *pgxpool.Pool, cfg *config.Config) error {
	if db == nil || cfg == nil {
		return nil
	}
	oidc := cfg.EffectiveOIDCAuth()
	hasConfigAuth := cfg.Auth.LocalEnabled != nil ||
		oidc.Enabled ||
		oidc.AutoCreateUsers ||
		oidc.AllowEmailLinking ||
		strings.TrimSpace(oidc.DefaultRole) != "" ||
		len(oidc.DomainMapping) > 0 ||
		len(oidc.Providers) > 0
	if hasConfigAuth {
		settings := oidcSettings{
			LocalEnabled:      cfg.EffectiveAuthProviderLocalEnabled(),
			OIDCEnabled:       oidc.Enabled,
			AutoCreateUsers:   oidc.AutoCreateUsers,
			DefaultRole:       strings.TrimSpace(oidc.DefaultRole),
			AllowEmailLinking: oidc.AllowEmailLinking,
		}
		if err := upsertOIDCSettings(ctx, db, settings); err != nil {
			return err
		}
	}
	for id, providerCfg := range oidc.Providers {
		provider := oidcProviderRecord{
			ID:                    normalizeOIDCProviderID(id),
			Type:                  normalizeOIDCProviderType(providerCfg.Type),
			DisplayName:           firstNonEmpty(strings.TrimSpace(providerCfg.DisplayName), normalizeOIDCProviderID(id)),
			Issuer:                strings.TrimRight(strings.TrimSpace(providerCfg.Issuer), "/"),
			AuthorizationEndpoint: strings.TrimSpace(providerCfg.AuthorizationEndpoint),
			TokenEndpoint:         strings.TrimSpace(providerCfg.TokenEndpoint),
			JWKSURI:               strings.TrimSpace(providerCfg.JWKSURI),
			UserInfoEndpoint:      strings.TrimSpace(providerCfg.UserInfoEndpoint),
			ClientID:              strings.TrimSpace(providerCfg.ClientID),
			ClientSecret:          strings.TrimSpace(providerCfg.ClientSecret),
			Scopes:                normalizeOIDCScopes(providerCfg.Scopes),
			AllowedEmailDomains:   normalizeOIDCEmailDomains(providerCfg.AllowedEmailDomains),
			GroupClaim:            strings.TrimSpace(providerCfg.GroupClaim),
			RoleMapping:           normalizeOIDCRoleMapping(providerCfg.RoleMapping),
			GroupMapping:          normalizeOIDCGroupMapping(providerCfg.GroupMapping),
			BasicRoleMapping:      normalizeOIDCBasicRoleMapping(basicRoleMappingFromConfig(providerCfg.BasicRoleMapping)),
			EntitlementSync:       normalizeOIDCEntitlementSync(entitlementSyncFromConfig(providerCfg.EntitlementSync)),
			AutoCreateUsers:       providerCfg.AutoCreateUsers,
			DefaultRole:           strings.TrimSpace(providerCfg.DefaultRole),
			AllowEmailLinking:     providerCfg.AllowEmailLinking,
			Enabled:               providerCfg.Enabled == nil || *providerCfg.Enabled,
			ConfigSource:          authProviderSourceConfig,
		}
		if provider.ID == "" || provider.Issuer == "" || provider.ClientID == "" {
			continue
		}
		if err := upsertOIDCProvider(ctx, db, provider, true); err != nil {
			return err
		}
	}
	if len(oidc.DomainMapping) > 0 {
		if err := replaceOIDCDomainMappings(ctx, db, oidc.DomainMapping, authProviderSourceConfig); err != nil {
			return err
		}
	}
	return nil
}

func getOIDCSettings(ctx context.Context, db *pgxpool.Pool, cfg *config.Config) (oidcSettings, error) {
	defaults := oidcSettings{
		LocalEnabled:      true,
		OIDCEnabled:       false,
		AutoCreateUsers:   false,
		DefaultRole:       "",
		AllowEmailLinking: false,
	}
	if cfg != nil {
		oidc := cfg.EffectiveOIDCAuth()
		defaults.LocalEnabled = cfg.EffectiveAuthProviderLocalEnabled()
		defaults.OIDCEnabled = oidc.Enabled
		defaults.AutoCreateUsers = oidc.AutoCreateUsers
		defaults.DefaultRole = strings.TrimSpace(oidc.DefaultRole)
		defaults.AllowEmailLinking = oidc.AllowEmailLinking
	}
	if db == nil {
		return defaults, nil
	}
	var raw []byte
	err := db.QueryRow(ctx, `SELECT value FROM auth_settings WHERE key = $1`, authSettingsKeyOIDC).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	if len(raw) == 0 {
		return defaults, nil
	}
	var stored oidcSettings
	if err := json.Unmarshal(raw, &stored); err != nil {
		return defaults, err
	}
	stored.DefaultRole = strings.TrimSpace(stored.DefaultRole)
	return stored, nil
}

func upsertOIDCSettings(ctx context.Context, db *pgxpool.Pool, settings oidcSettings) error {
	if db == nil {
		return nil
	}
	settings.DefaultRole = strings.TrimSpace(settings.DefaultRole)
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `
		INSERT INTO auth_settings (key, value, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value,
		    updated_at = NOW()
	`, authSettingsKeyOIDC, raw)
	return err
}

func basicRoleMappingFromConfig(mapping map[string]config.OIDCBasicRoleGrantConfig) map[string]oidcBasicRoleGrantMapping {
	if len(mapping) == 0 {
		return nil
	}
	out := make(map[string]oidcBasicRoleGrantMapping, len(mapping))
	for group, grant := range mapping {
		out[group] = oidcBasicRoleGrantMapping{
			Role:         grant.Role,
			Resource:     grant.Resource,
			ResourceType: grant.ResourceType,
			ResourceID:   grant.ResourceID,
		}
	}
	return out
}

func entitlementSyncFromConfig(sync config.OIDCEntitlementSyncConfig) oidcEntitlementSyncConfig {
	return oidcEntitlementSyncConfig{
		Mode:               sync.Mode,
		AdminBaseURL:       sync.AdminBaseURL,
		Realm:              sync.Realm,
		AdminRealm:         sync.AdminRealm,
		AdminClientID:      sync.AdminClientID,
		AdminClientSecret:  sync.AdminClientSecret,
		AdminUsername:      sync.AdminUsername,
		AdminPassword:      sync.AdminPassword,
		ClientID:           sync.ClientID,
		TargetResourceType: sync.TargetResourceType,
		GroupPathPrefix:    sync.GroupPathPrefix,
	}
}

func listOIDCProviders(ctx context.Context, db *pgxpool.Pool, enabledOnly bool) ([]oidcProviderRecord, error) {
	if db == nil {
		return nil, nil
	}
	query := `
		SELECT id, type, display_name, issuer, authorization_endpoint, token_endpoint, jwks_uri, userinfo_endpoint,
		       client_id, client_secret, scopes, allowed_email_domains, group_claim, role_mapping, group_mapping, basic_role_mapping, entitlement_sync,
		       auto_create_users, default_role, allow_email_linking, enabled, config_source, created_at, updated_at
		FROM auth_identity_providers`
	if enabledOnly {
		query += ` WHERE enabled = TRUE`
	}
	query += ` ORDER BY display_name ASC, id ASC`
	rows, err := db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var providers []oidcProviderRecord
	for rows.Next() {
		provider, err := scanOIDCProvider(rows)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	return providers, rows.Err()
}

func getOIDCProvider(ctx context.Context, db *pgxpool.Pool, providerID string) (oidcProviderRecord, error) {
	providerID = normalizeOIDCProviderID(providerID)
	if providerID == "" {
		return oidcProviderRecord{}, fmt.Errorf("provider is required")
	}
	row := db.QueryRow(ctx, `
		SELECT id, type, display_name, issuer, authorization_endpoint, token_endpoint, jwks_uri, userinfo_endpoint,
		       client_id, client_secret, scopes, allowed_email_domains, group_claim, role_mapping, group_mapping, basic_role_mapping, entitlement_sync,
		       auto_create_users, default_role, allow_email_linking, enabled, config_source, created_at, updated_at
		FROM auth_identity_providers
		WHERE id = $1
	`, providerID)
	return scanOIDCProvider(row)
}

type oidcProviderScanner interface {
	Scan(dest ...any) error
}

func scanOIDCProvider(scanner oidcProviderScanner) (oidcProviderRecord, error) {
	var provider oidcProviderRecord
	var scopesJSON, domainsJSON, mappingJSON, groupMappingJSON, basicRoleMappingJSON, entitlementSyncJSON []byte
	var autoCreate, allowLink sql.NullBool
	if err := scanner.Scan(
		&provider.ID,
		&provider.Type,
		&provider.DisplayName,
		&provider.Issuer,
		&provider.AuthorizationEndpoint,
		&provider.TokenEndpoint,
		&provider.JWKSURI,
		&provider.UserInfoEndpoint,
		&provider.ClientID,
		&provider.ClientSecret,
		&scopesJSON,
		&domainsJSON,
		&provider.GroupClaim,
		&mappingJSON,
		&groupMappingJSON,
		&basicRoleMappingJSON,
		&entitlementSyncJSON,
		&autoCreate,
		&provider.DefaultRole,
		&allowLink,
		&provider.Enabled,
		&provider.ConfigSource,
		&provider.CreatedAt,
		&provider.UpdatedAt,
	); err != nil {
		return provider, err
	}
	_ = json.Unmarshal(scopesJSON, &provider.Scopes)
	_ = json.Unmarshal(domainsJSON, &provider.AllowedEmailDomains)
	_ = json.Unmarshal(mappingJSON, &provider.RoleMapping)
	_ = json.Unmarshal(groupMappingJSON, &provider.GroupMapping)
	_ = json.Unmarshal(basicRoleMappingJSON, &provider.BasicRoleMapping)
	_ = json.Unmarshal(entitlementSyncJSON, &provider.EntitlementSync)
	provider.Scopes = normalizeOIDCScopes(provider.Scopes)
	provider.AllowedEmailDomains = normalizeOIDCEmailDomains(provider.AllowedEmailDomains)
	provider.RoleMapping = normalizeOIDCRoleMapping(provider.RoleMapping)
	provider.GroupMapping = normalizeOIDCGroupMapping(provider.GroupMapping)
	provider.BasicRoleMapping = normalizeOIDCBasicRoleMapping(provider.BasicRoleMapping)
	provider.EntitlementSync = normalizeOIDCEntitlementSync(provider.EntitlementSync)
	if autoCreate.Valid {
		value := autoCreate.Bool
		provider.AutoCreateUsers = &value
	}
	if allowLink.Valid {
		value := allowLink.Bool
		provider.AllowEmailLinking = &value
	}
	return provider, nil
}

func upsertOIDCProvider(ctx context.Context, db *pgxpool.Pool, provider oidcProviderRecord, replaceSecret bool) error {
	if db == nil {
		return nil
	}
	provider.ID = normalizeOIDCProviderID(provider.ID)
	provider.Type = normalizeOIDCProviderType(provider.Type)
	provider.Issuer = strings.TrimRight(strings.TrimSpace(provider.Issuer), "/")
	provider.DisplayName = firstNonEmpty(strings.TrimSpace(provider.DisplayName), provider.ID)
	provider.Scopes = normalizeOIDCScopes(provider.Scopes)
	provider.AllowedEmailDomains = normalizeOIDCEmailDomains(provider.AllowedEmailDomains)
	provider.RoleMapping = normalizeOIDCRoleMapping(provider.RoleMapping)
	provider.GroupMapping = normalizeOIDCGroupMapping(provider.GroupMapping)
	provider.BasicRoleMapping = normalizeOIDCBasicRoleMapping(provider.BasicRoleMapping)
	provider.EntitlementSync = normalizeOIDCEntitlementSync(provider.EntitlementSync)
	provider.ConfigSource = firstNonEmpty(strings.TrimSpace(provider.ConfigSource), authProviderSourceDatabase)
	if provider.ID == "" || provider.Issuer == "" || provider.ClientID == "" {
		return fmt.Errorf("provider id, issuer, and client_id are required")
	}
	if !replaceSecret {
		existing, err := getOIDCProvider(ctx, db, provider.ID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			if provider.ClientSecret == "" {
				provider.ClientSecret = existing.ClientSecret
			}
			if provider.EntitlementSync.AdminClientSecret == "" {
				provider.EntitlementSync.AdminClientSecret = existing.EntitlementSync.AdminClientSecret
			}
			if provider.EntitlementSync.AdminPassword == "" {
				provider.EntitlementSync.AdminPassword = existing.EntitlementSync.AdminPassword
			}
		}
	}
	scopesJSON, _ := json.Marshal(provider.Scopes)
	domainsJSON, _ := json.Marshal(provider.AllowedEmailDomains)
	mappingJSON, _ := json.Marshal(provider.RoleMapping)
	groupMappingJSON, _ := json.Marshal(provider.GroupMapping)
	basicRoleMappingJSON, _ := json.Marshal(provider.BasicRoleMapping)
	entitlementSyncJSON, _ := json.Marshal(provider.EntitlementSync)
	args := []any{
		provider.ID,
		provider.Type,
		provider.DisplayName,
		provider.Issuer,
		provider.AuthorizationEndpoint,
		provider.TokenEndpoint,
		provider.JWKSURI,
		provider.UserInfoEndpoint,
		provider.ClientID,
		provider.ClientSecret,
		scopesJSON,
		domainsJSON,
		provider.GroupClaim,
		mappingJSON,
		groupMappingJSON,
		basicRoleMappingJSON,
		entitlementSyncJSON,
		provider.AutoCreateUsers,
		provider.DefaultRole,
		provider.AllowEmailLinking,
		provider.Enabled,
		provider.ConfigSource,
	}
	secretSet := `client_secret = EXCLUDED.client_secret`
	if !replaceSecret {
		secretSet = `client_secret = CASE WHEN EXCLUDED.client_secret <> '' THEN EXCLUDED.client_secret ELSE auth_identity_providers.client_secret END`
	}
	_, err := db.Exec(ctx, `
		INSERT INTO auth_identity_providers (
			id, type, display_name, issuer, authorization_endpoint, token_endpoint, jwks_uri, userinfo_endpoint,
			client_id, client_secret, scopes, allowed_email_domains, group_claim, role_mapping, group_mapping, basic_role_mapping, entitlement_sync,
			auto_create_users, default_role, allow_email_linking, enabled, config_source, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, NOW())
		ON CONFLICT (id) DO UPDATE
		SET type = EXCLUDED.type,
		    display_name = EXCLUDED.display_name,
		    issuer = EXCLUDED.issuer,
		    authorization_endpoint = EXCLUDED.authorization_endpoint,
		    token_endpoint = EXCLUDED.token_endpoint,
		    jwks_uri = EXCLUDED.jwks_uri,
		    userinfo_endpoint = EXCLUDED.userinfo_endpoint,
		    client_id = EXCLUDED.client_id,
		    `+secretSet+`,
		    scopes = EXCLUDED.scopes,
		    allowed_email_domains = EXCLUDED.allowed_email_domains,
		    group_claim = EXCLUDED.group_claim,
		    role_mapping = EXCLUDED.role_mapping,
		    group_mapping = EXCLUDED.group_mapping,
		    basic_role_mapping = EXCLUDED.basic_role_mapping,
		    entitlement_sync = EXCLUDED.entitlement_sync,
		    auto_create_users = EXCLUDED.auto_create_users,
		    default_role = EXCLUDED.default_role,
		    allow_email_linking = EXCLUDED.allow_email_linking,
		    enabled = EXCLUDED.enabled,
		    config_source = EXCLUDED.config_source,
		    updated_at = NOW()
	`, args...)
	return err
}

func deleteOIDCProvider(ctx context.Context, db *pgxpool.Pool, providerID string) error {
	providerID = normalizeOIDCProviderID(providerID)
	if providerID == "" {
		return fmt.Errorf("provider is required")
	}
	if _, err := db.Exec(ctx, `
		DELETE FROM access_grants
		WHERE managed_by_identity_provider = TRUE
		  AND identity_provider_id = $1
	`, providerID); err != nil {
		return err
	}
	tag, err := db.Exec(ctx, `DELETE FROM auth_identity_providers WHERE id = $1`, providerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func listOIDCDomainMappings(ctx context.Context, db *pgxpool.Pool) (map[string]string, error) {
	rows, err := db.Query(ctx, `SELECT domain, provider_id FROM auth_oidc_domain_mappings ORDER BY domain ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var domain, providerID string
		if err := rows.Scan(&domain, &providerID); err != nil {
			return nil, err
		}
		out[domain] = providerID
	}
	return out, rows.Err()
}

func replaceOIDCDomainMappings(ctx context.Context, db *pgxpool.Pool, mapping map[string]string, source string) error {
	mapping = normalizeOIDCDomainMappings(mapping)
	source = firstNonEmpty(strings.TrimSpace(source), authProviderSourceDatabase)
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM auth_oidc_domain_mappings WHERE config_source = $1`, source); err != nil {
		return err
	}
	domains := make([]string, 0, len(mapping))
	for domain := range mapping {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	for _, domain := range domains {
		if _, err := tx.Exec(ctx, `
			INSERT INTO auth_oidc_domain_mappings (domain, provider_id, config_source, updated_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (domain) DO UPDATE
			SET provider_id = EXCLUDED.provider_id,
			    config_source = EXCLUDED.config_source,
			    updated_at = NOW()
		`, domain, mapping[domain], source); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func upsertOIDCDomainMapping(ctx context.Context, db *pgxpool.Pool, domain, providerID, source string) error {
	domain = normalizeOIDCEmailDomain(domain)
	providerID = normalizeOIDCProviderID(providerID)
	source = firstNonEmpty(strings.TrimSpace(source), authProviderSourceDatabase)
	if domain == "" || providerID == "" {
		return nil
	}
	_, err := db.Exec(ctx, `
		INSERT INTO auth_oidc_domain_mappings (domain, provider_id, config_source, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (domain) DO UPDATE
		SET provider_id = EXCLUDED.provider_id,
		    config_source = EXCLUDED.config_source,
		    updated_at = NOW()
	`, domain, providerID, source)
	return err
}

func findOIDCProviderForEmail(ctx context.Context, db *pgxpool.Pool, email string) (oidcProviderRecord, bool, error) {
	domain := emailDomain(email)
	if domain == "" {
		return oidcProviderRecord{}, false, nil
	}
	var providerID string
	err := db.QueryRow(ctx, `
		SELECT provider_id
		FROM auth_oidc_domain_mappings
		WHERE domain = $1
	`, domain).Scan(&providerID)
	if err == nil {
		provider, err := getOIDCProvider(ctx, db, providerID)
		return provider, provider.Enabled, err
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
		return oidcProviderRecord{}, false, err
	}
	providers, err := listOIDCProviders(ctx, db, true)
	if err != nil {
		return oidcProviderRecord{}, false, err
	}
	for _, provider := range providers {
		if emailDomainAllowed(domain, provider.AllowedEmailDomains) {
			return provider, true, nil
		}
	}
	return oidcProviderRecord{}, false, nil
}

func createOIDCState(ctx context.Context, db *pgxpool.Pool, providerID, state, nonce, codeVerifier, returnTo string, expiresAt time.Time) error {
	_, err := db.Exec(ctx, `
		INSERT INTO auth_oidc_states (state_hash, provider_id, nonce_hash, code_verifier, return_to, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, auth.HashToken(state), providerID, auth.HashToken(nonce), codeVerifier, returnTo, expiresAt)
	return err
}

func consumeOIDCState(ctx context.Context, db *pgxpool.Pool, providerID, state string) (oidcStateRecord, error) {
	var record oidcStateRecord
	tx, err := db.Begin(ctx)
	if err != nil {
		return record, err
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, `
		SELECT provider_id, nonce_hash, code_verifier, return_to, expires_at
		FROM auth_oidc_states
		WHERE state_hash = $1
		  AND provider_id = $2
		  AND consumed_at IS NULL
		  AND expires_at > NOW()
		FOR UPDATE
	`, auth.HashToken(state), providerID)
	if err := row.Scan(&record.ProviderID, &record.NonceHash, &record.CodeVerifier, &record.ReturnTo, &record.ExpiresAt); err != nil {
		return record, err
	}
	if _, err := tx.Exec(ctx, `UPDATE auth_oidc_states SET consumed_at = NOW() WHERE state_hash = $1`, auth.HashToken(state)); err != nil {
		return record, err
	}
	return record, tx.Commit(ctx)
}

func createOIDCLoginCode(ctx context.Context, db *pgxpool.Pool, userID uuid.UUID, providerID, code, returnTo string, expiresAt time.Time) error {
	_, err := db.Exec(ctx, `
		INSERT INTO auth_login_codes (code_hash, user_id, provider_id, return_to, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, auth.HashToken(code), userID, providerID, returnTo, expiresAt)
	return err
}

func consumeOIDCLoginCode(ctx context.Context, db *pgxpool.Pool, code string) (uuid.UUID, string, error) {
	var userID uuid.UUID
	var returnTo string
	tx, err := db.Begin(ctx)
	if err != nil {
		return userID, returnTo, err
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, `
		SELECT user_id, return_to
		FROM auth_login_codes
		WHERE code_hash = $1
		  AND consumed_at IS NULL
		  AND expires_at > NOW()
		FOR UPDATE
	`, auth.HashToken(code))
	if err := row.Scan(&userID, &returnTo); err != nil {
		return userID, returnTo, err
	}
	if _, err := tx.Exec(ctx, `UPDATE auth_login_codes SET consumed_at = NOW() WHERE code_hash = $1`, auth.HashToken(code)); err != nil {
		return userID, returnTo, err
	}
	return userID, returnTo, tx.Commit(ctx)
}

func resolveOIDCUser(ctx context.Context, db *pgxpool.Pool, settings oidcSettings, provider oidcProviderRecord, identity oidcVerifiedIdentity) (oidcUserResolution, error) {
	var result oidcUserResolution
	if db == nil {
		return result, fmt.Errorf("database is required")
	}
	if identity.Subject == "" {
		return result, fmt.Errorf("oidc subject is required")
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)

	var existingUserID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT user_id
		FROM auth_external_identities
		WHERE provider_id = $1 AND issuer = $2 AND subject = $3
	`, provider.ID, identity.Issuer, identity.Subject).Scan(&existingUserID)
	if err == nil {
		result.UserID = existingUserID
		if err := touchExternalIdentity(ctx, tx, existingUserID, provider, identity); err != nil {
			return result, err
		}
		if err := syncOIDCRolesAndGroups(ctx, tx, existingUserID, provider, settings, identity); err != nil {
			return result, err
		}
		return result, tx.Commit(ctx)
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}

	linkByEmail := providerBool(provider.AllowEmailLinking, settings.AllowEmailLinking)
	if linkByEmail && identity.EmailVerified && identity.Email != "" {
		err = tx.QueryRow(ctx, `SELECT id FROM users WHERE LOWER(email) = LOWER($1) ORDER BY id ASC LIMIT 1`, identity.Email).Scan(&existingUserID)
		if err == nil {
			result.UserID = existingUserID
			result.Linked = true
			if err := insertExternalIdentity(ctx, tx, existingUserID, provider, identity); err != nil {
				return result, err
			}
			if err := pruneSupersededExternalIdentities(ctx, tx, existingUserID, provider, identity); err != nil {
				return result, err
			}
			if err := syncOIDCRolesAndGroups(ctx, tx, existingUserID, provider, settings, identity); err != nil {
				return result, err
			}
			return result, tx.Commit(ctx)
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
			return result, err
		}
	}

	autoCreate := providerBool(provider.AutoCreateUsers, settings.AutoCreateUsers)
	if !autoCreate {
		return result, fmt.Errorf("no linked Nopsai user exists for this identity")
	}
	if identity.Email == "" {
		return result, fmt.Errorf("identity email is required for automatic user creation")
	}
	var duplicate uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM users WHERE LOWER(email) = LOWER($1) ORDER BY id ASC LIMIT 1`, identity.Email).Scan(&duplicate)
	if err == nil {
		return result, fmt.Errorf("email is already owned by an existing account")
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}

	userID := uuid.New()
	sub := fmt.Sprintf("oidc:%s:%s", provider.ID, identity.Subject)
	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, sub, email, provider, password_hash, status, must_change_password)
		VALUES ($1, $2, $3, $4, NULL, 'active', FALSE)
	`, userID, sub, identity.Email, "oidc:"+provider.ID)
	if err != nil {
		return result, err
	}
	result.UserID = userID
	result.Created = true
	if err := insertExternalIdentity(ctx, tx, userID, provider, identity); err != nil {
		return result, err
	}
	if err := syncOIDCRolesAndGroups(ctx, tx, userID, provider, settings, identity); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

type oidcTx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func touchExternalIdentity(ctx context.Context, tx oidcTx, userID uuid.UUID, provider oidcProviderRecord, identity oidcVerifiedIdentity) error {
	_, err := tx.Exec(ctx, `
		UPDATE auth_external_identities
		SET email = $4,
		    email_verified = $5,
		    last_login_at = NOW()
		WHERE user_id = $1 AND provider_id = $2 AND subject = $3
	`, userID, provider.ID, identity.Subject, identity.Email, identity.EmailVerified)
	return err
}

func insertExternalIdentity(ctx context.Context, tx oidcTx, userID uuid.UUID, provider oidcProviderRecord, identity oidcVerifiedIdentity) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO auth_external_identities (id, user_id, provider_id, issuer, subject, email, email_verified, last_login_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (provider_id, issuer, subject) DO UPDATE
		SET user_id = EXCLUDED.user_id,
		    email = EXCLUDED.email,
		    email_verified = EXCLUDED.email_verified,
		    last_login_at = NOW()
	`, uuid.New(), userID, provider.ID, identity.Issuer, identity.Subject, identity.Email, identity.EmailVerified)
	return err
}

func pruneSupersededExternalIdentities(ctx context.Context, tx oidcTx, userID uuid.UUID, provider oidcProviderRecord, identity oidcVerifiedIdentity) error {
	if strings.TrimSpace(identity.Email) == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `
		DELETE FROM auth_external_identities
		WHERE user_id = $1
		  AND provider_id = $2
		  AND issuer = $3
		  AND LOWER(email) = LOWER($4)
		  AND subject <> $5
	`, userID, provider.ID, identity.Issuer, identity.Email, identity.Subject)
	return err
}

func reconcileOIDCAuthGroupMappings(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	rows, err := db.Query(ctx, `
		SELECT
			ei.user_id,
			ei.provider_id,
			COALESCE(eg.group_name, ''),
			ip.group_mapping
		FROM auth_external_identities ei
		JOIN auth_identity_providers ip ON ip.id = ei.provider_id
		LEFT JOIN auth_external_group_memberships eg
		  ON eg.user_id = ei.user_id AND eg.provider_id = ei.provider_id
		ORDER BY ei.user_id, ei.provider_id, eg.group_name
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type syncKey struct {
		userID     uuid.UUID
		providerID string
	}
	desiredByKey := map[syncKey]map[string]string{}
	for rows.Next() {
		var userID uuid.UUID
		var providerID, externalGroup string
		var rawMapping []byte
		if err := rows.Scan(&userID, &providerID, &externalGroup, &rawMapping); err != nil {
			return err
		}
		key := syncKey{userID: userID, providerID: strings.TrimSpace(providerID)}
		if _, ok := desiredByKey[key]; !ok {
			desiredByKey[key] = map[string]string{}
		}
		var mapping map[string]string
		_ = json.Unmarshal(rawMapping, &mapping)
		mapping = normalizeOIDCGroupMapping(mapping)
		if authGroup := strings.TrimSpace(mapping[strings.TrimSpace(externalGroup)]); authGroup != "" {
			desiredByKey[key][strings.TrimSpace(externalGroup)] = authGroup
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(desiredByKey) == 0 {
		return nil
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for key, desired := range desiredByKey {
		if err := syncOIDCAuthGroupMemberships(ctx, tx, key.userID, key.providerID, desired); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func reconcileOIDCBasicRoleMappings(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	rows, err := db.Query(ctx, `
		SELECT
			ei.user_id,
			ei.provider_id,
			COALESCE(eg.group_name, ''),
			ip.basic_role_mapping
		FROM auth_external_identities ei
		JOIN auth_identity_providers ip ON ip.id = ei.provider_id
		LEFT JOIN auth_external_group_memberships eg
		  ON eg.user_id = ei.user_id AND eg.provider_id = ei.provider_id
		ORDER BY ei.user_id, ei.provider_id, eg.group_name
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type syncKey struct {
		userID     uuid.UUID
		providerID string
	}
	type syncState struct {
		mapping map[string]oidcBasicRoleGrantMapping
		groups  []string
	}
	stateByKey := map[syncKey]*syncState{}
	for rows.Next() {
		var userID uuid.UUID
		var providerID, externalGroup string
		var rawMapping []byte
		if err := rows.Scan(&userID, &providerID, &externalGroup, &rawMapping); err != nil {
			return err
		}
		key := syncKey{userID: userID, providerID: strings.TrimSpace(providerID)}
		state := stateByKey[key]
		if state == nil {
			state = &syncState{}
			stateByKey[key] = state
		}
		if state.mapping == nil {
			_ = json.Unmarshal(rawMapping, &state.mapping)
			state.mapping = normalizeOIDCBasicRoleMapping(state.mapping)
		}
		externalGroup = strings.TrimSpace(externalGroup)
		if externalGroup != "" {
			state.groups = append(state.groups, externalGroup)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(stateByKey) == 0 {
		return nil
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for key, state := range stateByKey {
		if err := syncOIDCBasicRoleGrants(ctx, tx, key.userID, key.providerID, oidcBasicRoleGrantSetForGroups(state.mapping, state.groups)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func syncOIDCRolesAndGroups(ctx context.Context, tx oidcTx, userID uuid.UUID, provider oidcProviderRecord, settings oidcSettings, identity oidcVerifiedIdentity) error {
	groupSet := map[string]bool{}
	authGroupSet := map[string]string{}
	roleSet := oidcDesiredAccessRoleSet(provider, settings, identity)
	groups := identity.Groups
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		groupSet[group] = true
		if _, err := tx.Exec(ctx, `
			INSERT INTO auth_external_group_memberships (user_id, provider_id, group_name, last_seen_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (user_id, provider_id, group_name) DO UPDATE
			SET last_seen_at = NOW()
		`, userID, provider.ID, group); err != nil {
			return err
		}
		if authGroup := strings.TrimSpace(provider.GroupMapping[group]); authGroup != "" {
			authGroupSet[group] = authGroup
		}
	}
	if err := pruneStaleExternalGroupMemberships(ctx, tx, userID, provider.ID, groupSet); err != nil {
		return err
	}
	if err := syncOIDCAuthGroupMemberships(ctx, tx, userID, provider.ID, authGroupSet); err != nil {
		return err
	}
	desiredBasicRoles := oidcBasicRoleGrantSetForGroups(provider.BasicRoleMapping, groups)
	desiredBasicRoles = mergeOIDCBasicRoleGrantSets(desiredBasicRoles, oidcBasicRoleGrantSetFromGrants(identity.BasicRoles))
	if err := syncOIDCBasicRoleGrants(ctx, tx, userID, provider.ID, desiredBasicRoles); err != nil {
		return err
	}
	if err := pruneStaleExternalRoleAssignments(ctx, tx, userID, provider.ID, roleSet); err != nil {
		return err
	}
	for role := range roleSet {
		if _, err := tx.Exec(ctx, `
			INSERT INTO auth_roles (name, description)
			VALUES ($1, '')
			ON CONFLICT (name) DO NOTHING
		`, role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_roles (user_id, role)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, userID, role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO auth_external_role_assignments (user_id, provider_id, role_name, last_seen_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (user_id, provider_id, role_name) DO UPDATE
			SET last_seen_at = NOW()
		`, userID, provider.ID, role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO auth_role_bindings (role_name, subject_type, subject_id)
			VALUES ($1, 'user', $2)
			ON CONFLICT (role_name, subject_type, subject_id) DO NOTHING
		`, role, userID.String()); err != nil {
			return err
		}
	}
	return pruneNonExternalUserRoleAssignments(ctx, tx, userID)
}

func oidcDesiredAccessRoleSet(provider oidcProviderRecord, settings oidcSettings, identity oidcVerifiedIdentity) map[string]bool {
	roleSet := map[string]bool{}
	if defaultRole := firstNonEmpty(strings.TrimSpace(provider.DefaultRole), strings.TrimSpace(settings.DefaultRole)); defaultRole != "" {
		roleSet[defaultRole] = true
	}
	for _, group := range identity.Groups {
		if role := strings.TrimSpace(provider.RoleMapping[strings.TrimSpace(group)]); role != "" {
			roleSet[role] = true
		}
	}
	for _, role := range identity.AccessRoles {
		if normalized, ok := normalizeExternalAccessRoleName(role); ok {
			roleSet[normalized] = true
		}
	}
	return roleSet
}

type oidcDesiredBasicRoleGrant struct {
	ExternalGroup         string
	Role                  string
	ResourceType          string
	ResourceID            string
	Inherit               bool
	RequireResourceExists bool
}

func oidcBasicRoleGrantSetForGroups(mapping map[string]oidcBasicRoleGrantMapping, groups []string) map[string]oidcDesiredBasicRoleGrant {
	if len(mapping) == 0 || len(groups) == 0 {
		return nil
	}
	desired := map[string]oidcDesiredBasicRoleGrant{}
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		grant, ok := mapping[group]
		if !ok {
			continue
		}
		role, err := normalizeProductRoleName(grant.Role)
		if err != nil || role == productRoleAdmin {
			continue
		}
		resourceType := strings.TrimSpace(grant.ResourceType)
		resourceID := strings.TrimSpace(grant.ResourceID)
		if strings.TrimSpace(grant.Resource) != "" {
			if parsedType, parsedID, ok := strings.Cut(grant.Resource, ":"); ok {
				resourceType = strings.TrimSpace(parsedType)
				resourceID = strings.TrimSpace(parsedID)
			}
		}
		if resourceType == "" || resourceID == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(resourceType)) + ":" + strings.Trim(strings.TrimSpace(resourceID), "/")
		next := oidcDesiredBasicRoleGrant{
			ExternalGroup: group,
			Role:          role,
			ResourceType:  resourceType,
			ResourceID:    resourceID,
			Inherit:       true,
		}
		if existing, ok := desired[key]; ok && productRoleRank(existing.Role) >= productRoleRank(next.Role) {
			continue
		}
		desired[key] = next
	}
	if len(desired) == 0 {
		return nil
	}
	return desired
}

func oidcBasicRoleGrantSetFromGrants(grants []oidcDesiredBasicRoleGrant) map[string]oidcDesiredBasicRoleGrant {
	desired := map[string]oidcDesiredBasicRoleGrant{}
	for _, grant := range grants {
		role, err := normalizeProductRoleName(grant.Role)
		if err != nil || role == productRoleAdmin {
			continue
		}
		resourceType := strings.TrimSpace(grant.ResourceType)
		resourceID := strings.Trim(strings.TrimSpace(grant.ResourceID), "/")
		if resourceType == "" || resourceID == "" {
			continue
		}
		grant.Role = role
		grant.ResourceType = resourceType
		grant.ResourceID = resourceID
		grant.Inherit = true
		key := strings.ToLower(resourceType) + ":" + resourceID
		if existing, ok := desired[key]; ok && productRoleRank(existing.Role) >= productRoleRank(grant.Role) {
			continue
		}
		desired[key] = grant
	}
	if len(desired) == 0 {
		return nil
	}
	return desired
}

func mergeOIDCBasicRoleGrantSets(left, right map[string]oidcDesiredBasicRoleGrant) map[string]oidcDesiredBasicRoleGrant {
	if len(left) == 0 {
		return right
	}
	if len(right) == 0 {
		return left
	}
	out := make(map[string]oidcDesiredBasicRoleGrant, len(left)+len(right))
	for key, grant := range left {
		out[key] = grant
	}
	for key, grant := range right {
		if existing, ok := out[key]; ok && productRoleRank(existing.Role) >= productRoleRank(grant.Role) {
			continue
		}
		out[key] = grant
	}
	return out
}

func productRoleRank(role string) int {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case productRoleOwner:
		return 3
	case productRoleDeveloper:
		return 2
	case productRoleViewer:
		return 1
	default:
		return 0
	}
}

func syncOIDCBasicRoleGrants(ctx context.Context, tx oidcTx, userID uuid.UUID, providerID string, desired map[string]oidcDesiredBasicRoleGrant) error {
	providerID = strings.TrimSpace(providerID)
	subjectID := userID.String()
	desiredKeys := make([]string, 0, len(desired))
	for key := range desired {
		desiredKeys = append(desiredKeys, key)
	}
	sort.Strings(desiredKeys)

	keptGrantIDs := make([]int64, 0, len(desiredKeys))
	for _, key := range desiredKeys {
		grant := desired[key]
		grant.ExternalGroup = strings.TrimSpace(grant.ExternalGroup)
		roleName, err := normalizeProductRoleName(grant.Role)
		if err != nil || roleName == productRoleAdmin {
			continue
		}
		resource, err := resolveAccessGrantResource(ctx, tx, grant.ResourceType, grant.ResourceID, grant.RequireResourceExists)
		if err != nil {
			if grant.RequireResourceExists && strings.Contains(strings.ToLower(err.Error()), "not found") {
				log.Warn().
					Str("provider", providerID).
					Str("external_group", grant.ExternalGroup).
					Str("role", roleName).
					Str("resource_type", grant.ResourceType).
					Str("resource_id", grant.ResourceID).
					Msg("Skipping SSO basic role grant because the target resource does not exist.")
				continue
			}
			return fmt.Errorf("failed to resolve SSO basic role target %s:%s: %w", grant.ResourceType, grant.ResourceID, err)
		}
		if err := validateGrantShape(roleName, resource, grant.Inherit); err != nil {
			return err
		}
		var subjectDisplay string
		_ = tx.QueryRow(ctx, `
			SELECT COALESCE(NULLIF(email, ''), COALESCE(NULLIF(sub, ''), id::text))
			FROM users
			WHERE id = $1
		`, userID).Scan(&subjectDisplay)
		subjectDisplay = firstNonEmptyString(strings.TrimSpace(subjectDisplay), subjectID)

		var grantID int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO access_grants (
				subject_type, subject_id, subject_display, role_name,
				resource_type, resource_id, resource_display, inherit, granted_by,
				managed_by_identity_provider, identity_provider_id, external_group_name
			)
			VALUES ('user', $1, $2, $3, $4, $5, $6, $7, $8, TRUE, $9, $10)
			ON CONFLICT (subject_type, subject_id, resource_type, resource_id) DO UPDATE
			SET subject_display = EXCLUDED.subject_display,
			    role_name = EXCLUDED.role_name,
			    resource_display = EXCLUDED.resource_display,
			    inherit = EXCLUDED.inherit,
			    granted_by = EXCLUDED.granted_by,
			    managed_by_identity_provider = TRUE,
			    identity_provider_id = EXCLUDED.identity_provider_id,
			    external_group_name = EXCLUDED.external_group_name
			RETURNING id
		`, subjectID, subjectDisplay, roleName, resource.Type, resource.ID, resource.Display, grant.Inherit, "sso:"+providerID, providerID, grant.ExternalGroup).Scan(&grantID); err != nil {
			return err
		}
		if err := rebuildAccessGrantExpansion(ctx, tx, grantID, grantSubjectUser, subjectID, roleName, resource); err != nil {
			return err
		}
		keptGrantIDs = append(keptGrantIDs, grantID)
	}

	if len(keptGrantIDs) == 0 {
		_, err := tx.Exec(ctx, `
			DELETE FROM access_grants
			WHERE subject_type = 'user'
			  AND subject_id = $1
			  AND managed_by_identity_provider = TRUE
			  AND identity_provider_id = $2
		`, subjectID, providerID)
		return err
	}
	_, err := tx.Exec(ctx, `
		DELETE FROM access_grants
		WHERE subject_type = 'user'
		  AND subject_id = $1
		  AND managed_by_identity_provider = TRUE
		  AND identity_provider_id = $2
		  AND NOT (id = ANY($3::bigint[]))
	`, subjectID, providerID, keptGrantIDs)
	return err
}

func rebuildAccessGrantExpansion(ctx context.Context, tx oidcTx, grantID int64, subjectType, subjectID, roleName string, resource accessGrantResource) error {
	if _, err := tx.Exec(ctx, `DELETE FROM resource_acl WHERE access_grant_id = $1`, grantID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM resource_ownership WHERE access_grant_id = $1`, grantID); err != nil {
		return err
	}
	for _, action := range applicableProductRoleActions(roleName, resource.Type) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO resource_acl (
				resource_type, resource_id, subject_type, subject_id, access_grant_id, action, effect
			)
			VALUES ($1, $2, $3, $4, $5, $6, 'allow')
			ON CONFLICT (resource_type, resource_id, subject_type, subject_id, action, effect)
			DO UPDATE SET access_grant_id = EXCLUDED.access_grant_id
		`, resource.Type, resource.ID, subjectType, subjectID, grantID, action); err != nil {
			return err
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
		`, resource.Type, resource.ID, subjectType, subjectID, grantID); err != nil {
			return err
		}
	}
	return nil
}

func syncOIDCAuthGroupMemberships(ctx context.Context, tx oidcTx, userID uuid.UUID, providerID string, desired map[string]string) error {
	providerID = strings.TrimSpace(providerID)
	subjectID := userID.String()
	externalGroups := make([]string, 0, len(desired))
	for externalGroup := range desired {
		externalGroups = append(externalGroups, externalGroup)
	}
	sort.Strings(externalGroups)

	for _, externalGroup := range externalGroups {
		authGroupName := strings.TrimSpace(desired[externalGroup])
		if authGroupName == "" {
			continue
		}
		description := fmt.Sprintf("Managed by OIDC provider %s", providerID)
		if _, err := tx.Exec(ctx, `
			WITH group_record AS (
				INSERT INTO auth_groups (name, description)
				VALUES ($1, $2)
				ON CONFLICT (name) DO UPDATE
				SET updated_at = auth_groups.updated_at
				RETURNING id
			)
			INSERT INTO auth_group_members (
				group_id, subject_type, subject_id, managed_by_identity_provider,
				identity_provider_id, external_group_name, auth_group_name
			)
			SELECT id, 'user', $3, TRUE, $4, $5, $1
			FROM group_record
			ON CONFLICT (group_id, subject_type, subject_id) DO UPDATE
			SET managed_by_identity_provider = TRUE,
			    identity_provider_id = EXCLUDED.identity_provider_id,
			    external_group_name = EXCLUDED.external_group_name,
			    auth_group_name = EXCLUDED.auth_group_name
		`, authGroupName, description, subjectID, providerID, externalGroup); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM auth_group_members
			WHERE subject_type = 'user'
			  AND subject_id = $1
			  AND managed_by_identity_provider = TRUE
			  AND identity_provider_id = $2
			  AND external_group_name = $3
			  AND auth_group_name <> $4
		`, subjectID, providerID, externalGroup, authGroupName); err != nil {
			return err
		}
	}

	if len(externalGroups) == 0 {
		if _, err := tx.Exec(ctx, `
			DELETE FROM auth_group_members
			WHERE subject_type = 'user'
			  AND subject_id = $1
			  AND managed_by_identity_provider = TRUE
			  AND identity_provider_id = $2
		`, subjectID, providerID); err != nil {
			return err
		}
	} else if _, err := tx.Exec(ctx, `
		DELETE FROM auth_group_members
		WHERE subject_type = 'user'
		  AND subject_id = $1
		  AND managed_by_identity_provider = TRUE
		  AND identity_provider_id = $2
		  AND NOT (external_group_name = ANY($3::text[]))
	`, subjectID, providerID, externalGroups); err != nil {
		return err
	}

	_, err := tx.Exec(ctx, `
		DELETE FROM auth_group_members
		WHERE subject_type = 'user'
		  AND subject_id = $1
		  AND managed_by_identity_provider = FALSE
	`, subjectID)
	return err
}

func pruneStaleExternalGroupMemberships(ctx context.Context, tx oidcTx, userID uuid.UUID, providerID string, desired map[string]bool) error {
	rows, err := tx.Query(ctx, `
		SELECT group_name
		FROM auth_external_group_memberships
		WHERE user_id = $1 AND provider_id = $2
	`, userID, providerID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var stale []string
	for rows.Next() {
		var group string
		if err := rows.Scan(&group); err != nil {
			return err
		}
		if !desired[group] {
			stale = append(stale, group)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, group := range stale {
		if _, err := tx.Exec(ctx, `
			DELETE FROM auth_external_group_memberships
			WHERE user_id = $1 AND provider_id = $2 AND group_name = $3
		`, userID, providerID, group); err != nil {
			return err
		}
	}
	return nil
}

func pruneStaleExternalRoleAssignments(ctx context.Context, tx oidcTx, userID uuid.UUID, providerID string, desired map[string]bool) error {
	rows, err := tx.Query(ctx, `
		SELECT role_name
		FROM auth_external_role_assignments
		WHERE user_id = $1 AND provider_id = $2
	`, userID, providerID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var stale []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return err
		}
		if !desired[role] {
			stale = append(stale, role)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, role := range stale {
		if _, err := tx.Exec(ctx, `
			DELETE FROM auth_external_role_assignments
			WHERE user_id = $1 AND provider_id = $2 AND role_name = $3
		`, userID, providerID, role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM user_roles
			WHERE user_id = $1 AND role = $2
			  AND NOT EXISTS (
				SELECT 1
				FROM auth_external_role_assignments
				WHERE user_id = $1 AND role_name = $2
			  )
		`, userID, role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM auth_role_bindings
			WHERE role_name = $1 AND subject_type = 'user' AND subject_id = $2
			  AND NOT EXISTS (
				SELECT 1
				FROM auth_external_role_assignments
				WHERE user_id = $3 AND role_name = $1
			  )
		`, role, userID.String(), userID); err != nil {
			return err
		}
	}
	return nil
}

func pruneNonExternalUserRoleAssignments(ctx context.Context, tx oidcTx, userID uuid.UUID) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM user_roles ur
		WHERE ur.user_id = $1
		  AND NOT EXISTS (
			SELECT 1
			FROM auth_external_role_assignments er
			WHERE er.user_id = $1 AND er.role_name = ur.role
		  )
	`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM auth_role_bindings rb
		WHERE rb.subject_type = 'user'
		  AND rb.subject_id = $2
		  AND NOT EXISTS (
			SELECT 1
			FROM auth_external_role_assignments er
			WHERE er.user_id = $1 AND er.role_name = rb.role_name
		  )
	`, userID, userID.String()); err != nil {
		return err
	}
	return nil
}

func providerBool(value *bool, fallback bool) bool {
	if value != nil {
		return *value
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func emailDomain(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return normalizeOIDCEmailDomain(email[at+1:])
}

func emailDomainAllowed(domain string, allowed []string) bool {
	domain = normalizeOIDCEmailDomain(domain)
	if domain == "" {
		return false
	}
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if normalizeOIDCEmailDomain(candidate) == domain {
			return true
		}
	}
	return false
}
