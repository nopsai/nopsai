package nopsai

import (
	"context"
	"fmt"
	"strings"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/credentials"
)

var credentialReferenceQueries = []struct {
	label string
	query string
}{
	{
		label: "LLM profile",
		query: `SELECT name
			FROM llm_profiles
			WHERE credential_ref = $1
			ORDER BY name`,
	},
	{
		label: "MCP server",
		query: `SELECT name
			FROM mcp_servers
			WHERE credential_ref = $1
			ORDER BY name`,
	},
	{
		label: "notification mail settings",
		query: `SELECT 'smtp'
			FROM notification_mail_settings
			WHERE smtp_password_credential_ref = $1`,
	},
	{
		label: "OIDC provider client",
		query: `SELECT id
			FROM auth_identity_providers
			WHERE client_credential_ref = $1
			ORDER BY id`,
	},
	{
		label: "OIDC entitlement admin client",
		query: `SELECT id
			FROM auth_identity_providers
			WHERE entitlement_sync->>'admin_client_credential_ref' = $1
			ORDER BY id`,
	},
	{
		label: "OIDC entitlement admin password",
		query: `SELECT id
			FROM auth_identity_providers
			WHERE entitlement_sync->>'admin_password_credential_ref' = $1
			ORDER BY id`,
	},
}

func (a *App) ensureCredentialReference(
	ctx context.Context,
	rawReference, kind, description, actor string,
) error {
	return a.ensureCredentialReferenceMetadata(ctx, rawReference, createCredentialInput{
		Kind:        kind,
		Description: description,
		Actor:       actor,
	})
}

func (a *App) ensureCredentialReferenceMetadata(
	ctx context.Context,
	rawReference string,
	input createCredentialInput,
) error {
	rawReference = strings.TrimSpace(rawReference)
	if rawReference == "" {
		return nil
	}
	ref, err := credentials.ParseReference(rawReference)
	if err != nil {
		return err
	}
	if a == nil || a.credentials == nil {
		return credentials.ErrUnavailable
	}
	input.Reference = ref
	_, err = a.credentials.EnsureMetadata(ctx, input)
	return err
}

func (a *App) ensureLLMProfileCredentialReferences(
	ctx context.Context,
	profiles map[string]config.LLMProfile,
	actor string,
) error {
	for name, profile := range config.NormalizeLLMProfiles(profiles) {
		if err := a.ensureCredentialReference(
			ctx,
			profile.CredentialRef,
			"api_key",
			"LLM API key for "+name,
			actor,
		); err != nil {
			return fmt.Errorf("LLM profile %q credential: %w", name, err)
		}
	}
	return nil
}

func (a *App) ensureMCPServerCredentialReferences(
	ctx context.Context,
	servers map[string]models.MCPServer,
	actor string,
) error {
	for name, server := range models.NormalizeMCPServers(servers) {
		if err := a.ensureCredentialReference(
			ctx,
			server.CredentialRef,
			"bearer_token",
			"MCP bearer token for "+name,
			actor,
		); err != nil {
			return fmt.Errorf("MCP server %q credential: %w", name, err)
		}
	}
	return nil
}

func (a *App) ensureOIDCProviderCredentialReferences(
	ctx context.Context,
	provider oidcProviderRecord,
	actor string,
) error {
	if err := a.ensureCredentialReference(
		ctx,
		provider.ClientCredentialRef,
		"client_secret",
		"OIDC client secret for "+provider.ID,
		actor,
	); err != nil {
		return err
	}
	if err := a.ensureCredentialReference(
		ctx,
		provider.EntitlementSync.AdminClientCredentialRef,
		"client_secret",
		"OIDC entitlement admin client secret for "+provider.ID,
		actor,
	); err != nil {
		return err
	}
	return a.ensureCredentialReference(
		ctx,
		provider.EntitlementSync.AdminPasswordCredentialRef,
		"password",
		"OIDC entitlement admin password for "+provider.ID,
		actor,
	)
}

func (a *App) credentialReferenceUsages(ctx context.Context, ref credentials.Reference) ([]string, error) {
	if a == nil {
		return nil, nil
	}
	reference := ref.String()
	usages := make([]string, 0, 4)
	if a.cfg != nil {
		a.cfgMu.RLock()
		if strings.TrimSpace(a.cfg.GitHubPrivateKeyCredentialRef) == reference {
			usages = append(usages, "GitHub App private key")
		}
		if strings.TrimSpace(a.cfg.GitHubWebhookCredentialRef) == reference {
			usages = append(usages, "GitHub webhook secret")
		}
		a.cfgMu.RUnlock()
	}
	if a.db == nil {
		return usages, nil
	}
	for _, referenceQuery := range credentialReferenceQueries {
		rows, err := a.db.Query(ctx, referenceQuery.query, reference)
		if err != nil {
			return nil, fmt.Errorf("check %s credential references: %w", referenceQuery.label, err)
		}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan %s credential reference: %w", referenceQuery.label, err)
			}
			usages = append(usages, fmt.Sprintf("%s %q", referenceQuery.label, name))
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read %s credential references: %w", referenceQuery.label, err)
		}
		rows.Close()
	}
	return usages, nil
}
