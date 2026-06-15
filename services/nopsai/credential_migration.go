package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nopsai/config"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/pkg/store"
)

const credentialMigrationActor = "system:credential-migration"

type credentialImporter struct {
	service *credentialService
	store   store.CredentialStore
}

func migrateLegacyCredentialSources(ctx context.Context, db *pgxpool.Pool, cfg *config.Config) error {
	if db == nil || cfg == nil {
		return nil
	}
	codec, err := credentials.NewEnvelopeCodec(cfg.MasterKey)
	if err != nil {
		return err
	}
	credentialStore := store.NewPGStore(db)
	service, err := newCredentialService(credentialStore, codec, nil)
	if err != nil {
		return err
	}
	importer := credentialImporter{service: service, store: credentialStore}
	if err := migrateLegacyOIDCCredentials(ctx, db, cfg, importer); err != nil {
		return fmt.Errorf("migrate OIDC credentials: %w", err)
	}
	if err := migrateLegacyMailCredential(ctx, db, importer); err != nil {
		return fmt.Errorf("migrate mail credential: %w", err)
	}
	if err := migrateLegacyLLMCredentials(ctx, db, cfg, importer); err != nil {
		return fmt.Errorf("migrate LLM credentials: %w", err)
	}
	if err := migrateLegacyMCPCredentials(ctx, db, cfg, importer); err != nil {
		return fmt.Errorf("migrate MCP credentials: %w", err)
	}
	if err := migrateLegacyGitHubCredentials(ctx, cfg, importer); err != nil {
		return fmt.Errorf("migrate GitHub credentials: %w", err)
	}
	return nil
}

func (i credentialImporter) ensure(
	ctx context.Context,
	rawReference, kind, description, plaintext string,
) (string, error) {
	rawReference = strings.TrimSpace(rawReference)
	if rawReference == "" {
		return "", nil
	}
	ref, err := credentials.ParseReference(rawReference)
	if err != nil {
		return "", err
	}
	existing, err := i.store.GetCredentialByReference(ctx, ref)
	switch {
	case err == nil:
		if existing.Kind != kind {
			return "", fmt.Errorf("%s already exists with kind %q", rawReference, existing.Kind)
		}
		if existing.HasValue() || strings.TrimSpace(plaintext) == "" {
			return ref.String(), nil
		}
		_, err = i.service.PutValue(ctx, existing.ID, []byte(plaintext), credentialMigrationActor)
		return ref.String(), err
	case !errors.Is(err, credentials.ErrNotFound):
		return "", err
	}
	_, err = i.service.Create(ctx, createCredentialInput{
		Reference:   ref,
		Kind:        kind,
		Description: description,
		Value:       []byte(plaintext),
		Actor:       credentialMigrationActor,
	})
	return ref.String(), err
}

func migrateLegacyOIDCCredentials(
	ctx context.Context,
	db *pgxpool.Pool,
	cfg *config.Config,
	importer credentialImporter,
) error {
	hasLegacyColumn, err := databaseColumnExists(ctx, db, "auth_identity_providers", "client_secret")
	if err != nil {
		return err
	}
	if hasLegacyColumn {
		rows, err := db.Query(ctx, `
			SELECT id, client_secret, client_credential_ref, entitlement_sync
			FROM auth_identity_providers
		`)
		if err != nil {
			return err
		}
		type legacyOIDCProvider struct {
			id                  string
			clientSecret        string
			clientCredentialRef string
			entitlementJSON     []byte
		}
		var providers []legacyOIDCProvider
		for rows.Next() {
			var provider legacyOIDCProvider
			if err := rows.Scan(
				&provider.id,
				&provider.clientSecret,
				&provider.clientCredentialRef,
				&provider.entitlementJSON,
			); err != nil {
				rows.Close()
				return err
			}
			providers = append(providers, provider)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, provider := range providers {
			clientRef, entitlementJSON, err := migrateOIDCProviderCredentialValues(
				ctx,
				importer,
				provider.id,
				provider.clientCredentialRef,
				provider.clientSecret,
				provider.entitlementJSON,
			)
			if err != nil {
				return err
			}
			if _, err := db.Exec(ctx, `
				UPDATE auth_identity_providers
				SET client_credential_ref = $2, client_secret = '', entitlement_sync = $3, updated_at = NOW()
				WHERE id = $1
			`, provider.id, clientRef, entitlementJSON); err != nil {
				return err
			}
		}
	}

	normalized := config.NormalizeAuthConfig(cfg.Auth)
	for providerID, provider := range normalized.OIDC.Providers {
		clientRef := strings.TrimSpace(provider.ClientCredentialRef)
		if clientRef == "" && provider.LegacyClientSecret != "" {
			clientRef = oidcCredentialReference(providerID, "client-secret")
		}
		if clientRef != "" {
			clientRef, err = importer.ensure(ctx, clientRef, "client_secret", "OIDC client secret for "+providerID, provider.LegacyClientSecret)
			if err != nil {
				return err
			}
		}
		provider.ClientCredentialRef = clientRef
		provider.LegacyClientSecret = ""

		sync := provider.EntitlementSync
		if sync.AdminClientCredentialRef == "" && sync.LegacyAdminClientSecret != "" {
			sync.AdminClientCredentialRef = oidcCredentialReference(providerID, "admin-client-secret")
		}
		if sync.AdminClientCredentialRef != "" {
			sync.AdminClientCredentialRef, err = importer.ensure(
				ctx,
				sync.AdminClientCredentialRef,
				"client_secret",
				"OIDC entitlement admin client secret for "+providerID,
				sync.LegacyAdminClientSecret,
			)
			if err != nil {
				return err
			}
		}
		if sync.AdminPasswordCredentialRef == "" && sync.LegacyAdminPassword != "" {
			sync.AdminPasswordCredentialRef = oidcCredentialReference(providerID, "admin-password")
		}
		if sync.AdminPasswordCredentialRef != "" {
			sync.AdminPasswordCredentialRef, err = importer.ensure(
				ctx,
				sync.AdminPasswordCredentialRef,
				"password",
				"OIDC entitlement admin password for "+providerID,
				sync.LegacyAdminPassword,
			)
			if err != nil {
				return err
			}
		}
		sync.LegacyAdminClientSecret = ""
		sync.LegacyAdminPassword = ""
		provider.EntitlementSync = sync
		normalized.OIDC.Providers[providerID] = provider
	}
	cfg.Auth = normalized
	return nil
}

func migrateOIDCProviderCredentialValues(
	ctx context.Context,
	importer credentialImporter,
	providerID, clientRef, legacyClientSecret string,
	entitlementJSON []byte,
) (string, []byte, error) {
	var err error
	if clientRef == "" && strings.TrimSpace(legacyClientSecret) != "" {
		clientRef = oidcCredentialReference(providerID, "client-secret")
	}
	if clientRef != "" {
		clientRef, err = importer.ensure(ctx, clientRef, "client_secret", "OIDC client secret for "+providerID, legacyClientSecret)
		if err != nil {
			return "", nil, err
		}
	}

	entitlement := map[string]any{}
	if len(entitlementJSON) > 0 {
		if err := json.Unmarshal(entitlementJSON, &entitlement); err != nil {
			return "", nil, err
		}
	}
	adminClientSecret := stringMapValue(entitlement, "admin_client_secret")
	adminClientRef := stringMapValue(entitlement, "admin_client_credential_ref")
	if adminClientRef == "" && adminClientSecret != "" {
		adminClientRef = oidcCredentialReference(providerID, "admin-client-secret")
	}
	if adminClientRef != "" {
		adminClientRef, err = importer.ensure(ctx, adminClientRef, "client_secret", "OIDC entitlement admin client secret for "+providerID, adminClientSecret)
		if err != nil {
			return "", nil, err
		}
		entitlement["admin_client_credential_ref"] = adminClientRef
	}
	delete(entitlement, "admin_client_secret")

	adminPassword := stringMapValue(entitlement, "admin_password")
	adminPasswordRef := stringMapValue(entitlement, "admin_password_credential_ref")
	if adminPasswordRef == "" && adminPassword != "" {
		adminPasswordRef = oidcCredentialReference(providerID, "admin-password")
	}
	if adminPasswordRef != "" {
		adminPasswordRef, err = importer.ensure(ctx, adminPasswordRef, "password", "OIDC entitlement admin password for "+providerID, adminPassword)
		if err != nil {
			return "", nil, err
		}
		entitlement["admin_password_credential_ref"] = adminPasswordRef
	}
	delete(entitlement, "admin_password")

	updatedEntitlement, err := json.Marshal(entitlement)
	return clientRef, updatedEntitlement, err
}

func migrateLegacyMailCredential(ctx context.Context, db *pgxpool.Pool, importer credentialImporter) error {
	hasLegacyColumn, err := databaseColumnExists(ctx, db, "notification_mail_settings", "smtp_password_secret_ref")
	if err != nil || !hasLegacyColumn {
		return err
	}
	var legacyRef, credentialRef string
	err = db.QueryRow(ctx, `
		SELECT smtp_password_secret_ref, smtp_password_credential_ref
		FROM notification_mail_settings
		WHERE id = TRUE
	`).Scan(&legacyRef, &credentialRef)
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	legacyRef = strings.TrimSpace(legacyRef)
	if credentialRef == "" && legacyRef != "" {
		if strings.HasPrefix(strings.ToLower(legacyRef), credentials.Scheme+"://") {
			credentialRef = legacyRef
		} else {
			credentialRef = "credential://system/mail/smtp-password"
		}
	}
	if credentialRef != "" {
		legacyValue := ""
		if legacyRef != "" && !strings.HasPrefix(strings.ToLower(legacyRef), credentials.Scheme+"://") {
			legacyValue = os.Getenv(legacyRef)
		}
		credentialRef, err = importer.ensure(ctx, credentialRef, "password", "SMTP authentication password", legacyValue)
		if err != nil {
			return err
		}
	}
	_, err = db.Exec(ctx, `
		UPDATE notification_mail_settings
		SET smtp_password_credential_ref = $1, smtp_password_secret_ref = '', updated_at = NOW()
		WHERE id = TRUE
	`, credentialRef)
	return err
}

func migrateLegacyLLMCredentials(
	ctx context.Context,
	db *pgxpool.Pool,
	cfg *config.Config,
	importer credentialImporter,
) error {
	hasLegacyColumn, err := databaseColumnExists(ctx, db, "llm_profiles", "api_key_secret")
	if err != nil {
		return err
	}
	if hasLegacyColumn {
		rows, err := db.Query(ctx, `SELECT name, api_key_secret, credential_ref FROM llm_profiles`)
		if err != nil {
			return err
		}
		type legacyLLMProfile struct {
			name          string
			legacyRef     string
			credentialRef string
		}
		var profiles []legacyLLMProfile
		for rows.Next() {
			var profile legacyLLMProfile
			if err := rows.Scan(&profile.name, &profile.legacyRef, &profile.credentialRef); err != nil {
				rows.Close()
				return err
			}
			profiles = append(profiles, profile)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, profile := range profiles {
			credentialRef, err := migrateEnvironmentCredentialReference(
				ctx,
				importer,
				profile.credentialRef,
				profile.legacyRef,
				"credential://system/llm/"+credentialReferenceSegment(profile.name),
				"api_key",
				"LLM API key for "+profile.name,
			)
			if err != nil {
				return err
			}
			if _, err := db.Exec(ctx, `
				UPDATE llm_profiles
				SET credential_ref = $2, api_key_secret = '', updated_at = NOW()
				WHERE name = $1
			`, profile.name, credentialRef); err != nil {
				return err
			}
		}
	}

	profiles := cfg.EffectiveLLMProfiles()
	for name, profile := range profiles {
		profile.CredentialRef, err = migrateEnvironmentCredentialReference(
			ctx,
			importer,
			profile.CredentialRef,
			profile.LegacyAPIKeySecret,
			"credential://system/llm/"+credentialReferenceSegment(name),
			"api_key",
			"LLM API key for "+name,
		)
		if err != nil {
			return err
		}
		profile.LegacyAPIKeySecret = ""
		profiles[name] = profile
	}
	cfg.LLMProfiles = profiles
	return nil
}

func migrateLegacyMCPCredentials(
	ctx context.Context,
	db *pgxpool.Pool,
	cfg *config.Config,
	importer credentialImporter,
) error {
	hasLegacyColumn, err := databaseColumnExists(ctx, db, "mcp_servers", "auth_secret")
	if err != nil {
		return err
	}
	if hasLegacyColumn {
		rows, err := db.Query(ctx, `SELECT name, auth_secret, credential_ref FROM mcp_servers`)
		if err != nil {
			return err
		}
		type legacyMCPServer struct {
			name          string
			legacyRef     string
			credentialRef string
		}
		var servers []legacyMCPServer
		for rows.Next() {
			var server legacyMCPServer
			if err := rows.Scan(&server.name, &server.legacyRef, &server.credentialRef); err != nil {
				rows.Close()
				return err
			}
			servers = append(servers, server)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, server := range servers {
			credentialRef, err := migrateEnvironmentCredentialReference(
				ctx,
				importer,
				server.credentialRef,
				server.legacyRef,
				"credential://system/mcp/"+credentialReferenceSegment(server.name),
				"bearer_token",
				"MCP bearer token for "+server.name,
			)
			if err != nil {
				return err
			}
			if _, err := db.Exec(ctx, `
				UPDATE mcp_servers
				SET credential_ref = $2, auth_secret = '', updated_at = NOW()
				WHERE name = $1
			`, server.name, credentialRef); err != nil {
				return err
			}
		}
	}

	servers := cfg.EffectiveMCPServers()
	for name, server := range servers {
		server.CredentialRef, err = migrateEnvironmentCredentialReference(
			ctx,
			importer,
			server.CredentialRef,
			server.LegacyAuthSecret,
			"credential://system/mcp/"+credentialReferenceSegment(name),
			"bearer_token",
			"MCP bearer token for "+name,
		)
		if err != nil {
			return err
		}
		server.LegacyAuthSecret = ""
		servers[name] = server
	}
	cfg.MCPServers = servers
	return nil
}

func migrateEnvironmentCredentialReference(
	ctx context.Context,
	importer credentialImporter,
	credentialRef, legacyRef, defaultRef, kind, description string,
) (string, error) {
	credentialRef = strings.TrimSpace(credentialRef)
	legacyRef = strings.TrimSpace(legacyRef)
	if credentialRef == "" && legacyRef != "" {
		if strings.HasPrefix(strings.ToLower(legacyRef), credentials.Scheme+"://") {
			credentialRef = legacyRef
		} else {
			credentialRef = defaultRef
		}
	}
	if credentialRef == "" {
		return "", nil
	}
	legacyValue := ""
	if legacyRef != "" && !strings.HasPrefix(strings.ToLower(legacyRef), credentials.Scheme+"://") {
		legacyValue = os.Getenv(legacyRef)
	}
	return importer.ensure(ctx, credentialRef, kind, description, legacyValue)
}

func migrateLegacyGitHubCredentials(
	ctx context.Context,
	cfg *config.Config,
	importer credentialImporter,
) error {
	githubConfigured := strings.TrimSpace(cfg.GitHubAppID) != "" || strings.TrimSpace(cfg.GitHubInstallID) != ""
	webhookRef := strings.TrimSpace(cfg.GitHubWebhookCredentialRef)
	if webhookRef == "" && (githubConfigured || strings.TrimSpace(cfg.LegacyGitHubWebhookSecret) != "") {
		webhookRef = "credential://system/github/webhook-secret"
	}
	if webhookRef != "" {
		var err error
		webhookRef, err = importer.ensure(
			ctx,
			webhookRef,
			"webhook_secret",
			"GitHub App webhook verification secret",
			cfg.LegacyGitHubWebhookSecret,
		)
		if err != nil {
			return err
		}
	}

	privateKey := strings.TrimSpace(cfg.LegacyGitHubPrivateKey)
	if privateKey == "" && strings.TrimSpace(cfg.LegacyGitHubPrivateKeyPath) != "" {
		contents, err := os.ReadFile(strings.TrimSpace(cfg.LegacyGitHubPrivateKeyPath))
		if err == nil {
			privateKey = string(contents)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	privateKey = strings.ReplaceAll(privateKey, `\n`, "\n")
	privateKeyRef := strings.TrimSpace(cfg.GitHubPrivateKeyCredentialRef)
	if privateKeyRef == "" && (githubConfigured || privateKey != "") {
		privateKeyRef = "credential://system/github/app-private-key"
	}
	if privateKeyRef != "" {
		var err error
		privateKeyRef, err = importer.ensure(
			ctx,
			privateKeyRef,
			"private_key",
			"GitHub App private key",
			privateKey,
		)
		if err != nil {
			return err
		}
	}

	cfg.GitHubWebhookCredentialRef = webhookRef
	cfg.GitHubPrivateKeyCredentialRef = privateKeyRef
	cfg.LegacyGitHubWebhookSecret = ""
	cfg.LegacyGitHubPrivateKey = ""
	cfg.LegacyGitHubPrivateKeyPath = ""
	return nil
}

func oidcCredentialReference(providerID, suffix string) string {
	return "credential://system/oidc/" + normalizeOIDCProviderID(providerID) + "/" + suffix
}

func credentialReferenceSegment(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var builder strings.Builder
	lastDash := false
	for _, char := range raw {
		valid := char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '.' || char == '_' || char == '-'
		if valid {
			builder.WriteRune(char)
			lastDash = char == '-'
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func stringMapValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func databaseColumnExists(ctx context.Context, db *pgxpool.Pool, table, column string) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = $1
			  AND column_name = $2
		)
	`, table, column).Scan(&exists)
	return exists, err
}
