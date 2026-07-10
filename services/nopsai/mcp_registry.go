package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/mcpregistry"
	"nopsai/services/nopsai/pkg/validation"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"
)

func (a *App) setMCPRegistry(servers map[string]models.MCPServer, profiles map[string]models.MCPProfile) config.Config {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	a.cfg.MCPServers = models.NormalizeMCPServers(servers)
	a.cfg.MCPProfiles = models.NormalizeMCPProfiles(profiles)
	return *a.cfg
}

func (a *App) mcpCatalogForValidation(cfg config.Config) map[string]validation.MCPProfileDefinition {
	profiles := cfg.EffectiveMCPProfiles()
	return mcpCatalogForProfiles(profiles)
}

func mcpCatalogForProfiles(profiles map[string]models.MCPProfile) map[string]validation.MCPProfileDefinition {
	catalog := make(map[string]validation.MCPProfileDefinition, len(profiles))
	for name, profile := range profiles {
		catalog[name] = validation.MCPProfileDefinition{
			Enabled:       profile.Enabled,
			AllowedScopes: append([]string(nil), profile.AllowedScopes...),
		}
	}
	return catalog
}

func (a *App) validatePipelineMCPProfiles(pipeline *models.Pipeline, scope string) error {
	return a.validatePipelineMCPProfilesForTeam(context.Background(), pipeline, scope, nil)
}

func (a *App) validatePipelineMCPProfilesForTeam(ctx context.Context, pipeline *models.Pipeline, scope string, teamID *int) error {
	if !models.PipelineLLMEnabled(pipeline) {
		return nil
	}
	cfg := a.getConfigSnapshot()
	profiles, err := a.effectiveMCPProfilesForTeam(ctx, cfg, teamID)
	if err != nil {
		return err
	}
	if err := validation.ValidatePipelineMCPProfiles(pipeline, validation.MCPProfileValidationOptions{
		Profiles: mcpCatalogForProfiles(profiles),
		Scope:    scope,
	}); err != nil {
		return err
	}
	return nil
}

func (a *App) loadOrSeedMCPRegistryConfig(ctx context.Context) error {
	if a == nil || a.db == nil {
		return nil
	}
	servers, profiles, found, err := a.loadMCPRegistryFromDB(ctx)
	if err != nil {
		return err
	}
	if found {
		a.setMCPRegistry(servers, profiles)
		return nil
	}
	cfg := a.getConfigSnapshot()
	servers = cfg.EffectiveMCPServers()
	profiles = cfg.EffectiveMCPProfiles()
	if len(servers) == 0 && len(profiles) == 0 {
		return nil
	}
	if err := a.persistMCPRegistryToDB(ctx, servers, profiles); err != nil {
		return err
	}
	a.setMCPRegistry(servers, profiles)
	return nil
}

func (a *App) loadMCPRegistryFromDB(ctx context.Context) (map[string]models.MCPServer, map[string]models.MCPProfile, bool, error) {
	if a == nil || a.db == nil {
		return nil, nil, false, nil
	}
	servers, err := a.loadMCPServersFromDB(ctx)
	if err != nil {
		return nil, nil, false, err
	}
	profiles, err := a.loadMCPProfilesFromDB(ctx)
	if err != nil {
		return nil, nil, false, err
	}
	return servers, profiles, len(servers) > 0 || len(profiles) > 0, nil
}

func (a *App) loadMCPServersFromDB(ctx context.Context) (map[string]models.MCPServer, error) {
	rows, err := a.db.Query(ctx, `
		SELECT name, display_name, enabled, provider, transport, url, auth_type, credential_ref,
			headers, timeout, allowed_scopes, last_test_status, last_test_message, last_tested_at,
			last_discovered_at, discovered_server_name, discovered_version, discovered_protocol
		FROM mcp_servers
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("load MCP servers: %w", err)
	}
	defer rows.Close()

	servers := map[string]models.MCPServer{}
	for rows.Next() {
		var (
			server         models.MCPServer
			headersRaw     []byte
			allowedRaw     []byte
			lastTested     sql.NullTime
			lastDiscovered sql.NullTime
		)
		if err := rows.Scan(
			&server.Name,
			&server.DisplayName,
			&server.Enabled,
			&server.Provider,
			&server.Transport,
			&server.URL,
			&server.AuthType,
			&server.CredentialRef,
			&headersRaw,
			&server.Timeout,
			&allowedRaw,
			&server.LastTestStatus,
			&server.LastTestMessage,
			&lastTested,
			&lastDiscovered,
			&server.DiscoveredServerName,
			&server.DiscoveredVersion,
			&server.DiscoveredProtocol,
		); err != nil {
			return nil, fmt.Errorf("scan MCP server: %w", err)
		}
		if len(headersRaw) > 0 {
			_ = json.Unmarshal(headersRaw, &server.Headers)
		}
		if len(allowedRaw) > 0 {
			_ = json.Unmarshal(allowedRaw, &server.AllowedScopes)
		}
		if lastTested.Valid {
			server.LastTestedAt = &lastTested.Time
		}
		if lastDiscovered.Valid {
			server.LastDiscoveredAt = &lastDiscovered.Time
		}
		server = models.NormalizeMCPServer(server)
		servers[server.Name] = server
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate MCP servers: %w", err)
	}
	return servers, nil
}

func (a *App) loadMCPProfilesFromDB(ctx context.Context) (map[string]models.MCPProfile, error) {
	rows, err := a.db.Query(ctx, `
		SELECT name, description, enabled, server_refs, allowed_scopes
		FROM mcp_profiles
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("load MCP profiles: %w", err)
	}
	defer rows.Close()

	profiles := map[string]models.MCPProfile{}
	for rows.Next() {
		var (
			profile    models.MCPProfile
			refsRaw    []byte
			allowedRaw []byte
		)
		if err := rows.Scan(&profile.Name, &profile.Description, &profile.Enabled, &refsRaw, &allowedRaw); err != nil {
			return nil, fmt.Errorf("scan MCP profile: %w", err)
		}
		if len(refsRaw) > 0 {
			_ = json.Unmarshal(refsRaw, &profile.ServerRefs)
		}
		if len(allowedRaw) > 0 {
			_ = json.Unmarshal(allowedRaw, &profile.AllowedScopes)
		}
		profile = models.NormalizeMCPProfile(profile)
		profiles[profile.Name] = profile
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate MCP profiles: %w", err)
	}
	return profiles, nil
}

func (a *App) loadMCPToolsByServer(ctx context.Context) (map[string][]models.MCPTool, error) {
	rows, err := a.db.Query(ctx, `
		SELECT server_name, name, description, input_schema, schema_hash, last_seen_at
		FROM mcp_tools
		ORDER BY server_name ASC, name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("load MCP tools: %w", err)
	}
	defer rows.Close()
	toolsByServer := map[string][]models.MCPTool{}
	for rows.Next() {
		var tool models.MCPTool
		if err := rows.Scan(&tool.ServerName, &tool.Name, &tool.Description, &tool.InputSchema, &tool.SchemaHash, &tool.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan MCP tool: %w", err)
		}
		toolsByServer[tool.ServerName] = append(toolsByServer[tool.ServerName], tool)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate MCP tools: %w", err)
	}
	return toolsByServer, nil
}

func (a *App) persistMCPRegistryToDB(ctx context.Context, servers map[string]models.MCPServer, profiles map[string]models.MCPProfile) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin MCP registry persistence: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := persistMCPRegistryToTx(ctx, tx, servers, profiles); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit MCP registry persistence: %w", err)
	}
	return nil
}

func persistMCPRegistryToTx(ctx context.Context, tx pgx.Tx, servers map[string]models.MCPServer, profiles map[string]models.MCPProfile) error {
	servers = models.NormalizeMCPServers(servers)
	profiles = models.NormalizeMCPProfiles(profiles)
	if err := mcpregistry.ValidateRegistryDefinition(servers, profiles); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM mcp_profiles`); err != nil {
		return fmt.Errorf("clear MCP profiles: %w", err)
	}

	serverNames := make([]string, 0, len(servers))
	for name := range servers {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)
	if len(serverNames) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM mcp_servers`); err != nil {
			return fmt.Errorf("clear MCP servers: %w", err)
		}
	} else if _, err := tx.Exec(ctx, `DELETE FROM mcp_servers WHERE NOT (name = ANY($1::text[]))`, serverNames); err != nil {
		return fmt.Errorf("prune MCP servers: %w", err)
	}

	for _, name := range serverNames {
		server := models.NormalizeMCPServer(servers[name])
		server.Name = name
		if err := upsertMCPServerTx(ctx, tx, server); err != nil {
			return err
		}
	}

	profileNames := make([]string, 0, len(profiles))
	for name := range profiles {
		profileNames = append(profileNames, name)
	}
	sort.Strings(profileNames)
	for _, name := range profileNames {
		profile := models.NormalizeMCPProfile(profiles[name])
		profile.Name = name
		if err := upsertMCPProfileTx(ctx, tx, profile); err != nil {
			return err
		}
	}
	return nil
}

func upsertMCPServerTx(ctx context.Context, tx pgx.Tx, server models.MCPServer) error {
	server = models.NormalizeMCPServer(server)
	if err := mcpregistry.ValidateServerDefinition(server); err != nil {
		return err
	}
	headersJSON, err := json.Marshal(server.Headers)
	if err != nil {
		return fmt.Errorf("encode MCP server headers: %w", err)
	}
	allowedJSON, err := json.Marshal(server.AllowedScopes)
	if err != nil {
		return fmt.Errorf("encode MCP server allowed scopes: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO mcp_servers (
			name, display_name, enabled, provider, transport, url, auth_type, credential_ref,
			headers, timeout, allowed_scopes, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11::jsonb, NOW())
		ON CONFLICT (name) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			enabled = EXCLUDED.enabled,
			provider = EXCLUDED.provider,
			transport = EXCLUDED.transport,
			url = EXCLUDED.url,
			auth_type = EXCLUDED.auth_type,
			credential_ref = EXCLUDED.credential_ref,
			headers = EXCLUDED.headers,
			timeout = EXCLUDED.timeout,
			allowed_scopes = EXCLUDED.allowed_scopes,
			updated_at = NOW()
	`, server.Name, server.DisplayName, server.Enabled, server.Provider, server.Transport, server.URL, server.AuthType, server.CredentialRef, string(headersJSON), server.Timeout, string(allowedJSON))
	if err != nil {
		return fmt.Errorf("persist MCP server %q: %w", server.Name, err)
	}
	return nil
}

func upsertMCPProfileTx(ctx context.Context, tx pgx.Tx, profile models.MCPProfile) error {
	profile = models.NormalizeMCPProfile(profile)
	refsJSON, err := json.Marshal(profile.ServerRefs)
	if err != nil {
		return fmt.Errorf("encode MCP profile server refs: %w", err)
	}
	allowedJSON, err := json.Marshal(profile.AllowedScopes)
	if err != nil {
		return fmt.Errorf("encode MCP profile allowed scopes: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO mcp_profiles (name, description, enabled, server_refs, allowed_scopes, updated_at)
		VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, NOW())
		ON CONFLICT (name) DO UPDATE SET
			description = EXCLUDED.description,
			enabled = EXCLUDED.enabled,
			server_refs = EXCLUDED.server_refs,
			allowed_scopes = EXCLUDED.allowed_scopes,
			updated_at = NOW()
	`, profile.Name, profile.Description, profile.Enabled, string(refsJSON), string(allowedJSON))
	if err != nil {
		return fmt.Errorf("persist MCP profile %q: %w", profile.Name, err)
	}
	return nil
}

func (a *App) refreshMCPRegistryFromDB(ctx context.Context) error {
	servers, profiles, _, err := a.loadMCPRegistryFromDB(ctx)
	if err != nil {
		return err
	}
	cfg := a.setMCPRegistry(servers, profiles)
	return a.persistMCPRegistryConfig(ctx, cfg)
}

func (a *App) persistMCPRegistryConfig(ctx context.Context, cfg config.Config) error {
	if err := a.ensureMCPServerCredentialReferences(ctx, cfg.EffectiveMCPServers(), credentialActorFromContext(ctx)); err != nil {
		return err
	}
	if a.configPath == "" {
		return nil
	}
	existing := map[string]interface{}{}
	if contents, err := os.ReadFile(a.configPath); err == nil {
		if len(contents) > 0 {
			_ = yaml.Unmarshal(contents, &existing)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	existing["mcp_servers"] = cfg.EffectiveMCPServers()
	existing["mcp_profiles"] = cfg.EffectiveMCPProfiles()
	contents, err := yaml.Marshal(existing)
	if err != nil {
		return err
	}
	return os.WriteFile(a.configPath, contents, 0o644)
}
