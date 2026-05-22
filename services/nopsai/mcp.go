package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/mcpclient"
	"nopsai/pkg/models"
	"nopsai/services/nopsai/pkg/validation"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"
)

const mcpRegistryRuntimeEnv = "NOPSAI_MCP_REGISTRY"

var mcpNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

var mcpSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS mcp_servers (
		name TEXT PRIMARY KEY,
		display_name TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		provider TEXT NOT NULL DEFAULT '',
		transport TEXT NOT NULL DEFAULT 'streamable_http',
		url TEXT NOT NULL DEFAULT '',
		auth_type TEXT NOT NULL DEFAULT 'none',
		auth_secret TEXT NOT NULL DEFAULT '',
		headers JSONB NOT NULL DEFAULT '{}'::jsonb,
		timeout TEXT NOT NULL DEFAULT '30s',
		allowed_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
		last_test_status TEXT NOT NULL DEFAULT '',
		last_test_message TEXT NOT NULL DEFAULT '',
		last_tested_at TIMESTAMPTZ,
		last_discovered_at TIMESTAMPTZ,
		discovered_server_name TEXT NOT NULL DEFAULT '',
		discovered_version TEXT NOT NULL DEFAULT '',
		discovered_protocol TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE IF NOT EXISTS mcp_tools (
		server_name TEXT NOT NULL REFERENCES mcp_servers(name) ON DELETE CASCADE,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		input_schema TEXT NOT NULL DEFAULT '{}',
		schema_hash TEXT NOT NULL DEFAULT '',
		last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		PRIMARY KEY (server_name, name)
	)`,
	`CREATE TABLE IF NOT EXISTS mcp_profiles (
		name TEXT PRIMARY KEY,
		description TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		server_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
		allowed_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
}

type mcpServerView struct {
	models.MCPServer
	Tools []models.MCPTool `json:"tools,omitempty"`
}

type mcpServersResponse struct {
	Servers []mcpServerView `json:"servers"`
}

type mcpProfilesResponse struct {
	Profiles []models.MCPProfile `json:"profiles"`
}

type runtimeMCPServer struct {
	models.MCPServer
	AuthValue string `json:"auth_value,omitempty"`
}

type runtimeMCPRegistry struct {
	Servers  map[string]runtimeMCPServer  `json:"servers"`
	Profiles map[string]models.MCPProfile `json:"profiles"`
	Tools    map[string][]models.MCPTool  `json:"tools"`
}

type mcpProfileTestResponse struct {
	Status   string   `json:"status"`
	Message  string   `json:"message,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type mcpRegistryRequest struct {
	Servers     []models.MCPServer           `json:"servers" yaml:"servers"`
	MCPServers  map[string]models.MCPServer  `json:"mcp_servers" yaml:"mcp_servers"`
	Profiles    []models.MCPProfile          `json:"profiles" yaml:"profiles"`
	MCPProfiles map[string]models.MCPProfile `json:"mcp_profiles" yaml:"mcp_profiles"`
}

type gitOpsMCPDirectory struct {
	root  string
	files map[string]string
}

type gitOpsMCPPlan struct {
	servers    map[string]models.MCPServer
	profiles   map[string]models.MCPProfile
	sourcePath string
}

type gitOpsMCPFileCandidate struct {
	sourcePath string
	content    string
}

func ensureMCPSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin MCP schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	for idx, stmt := range mcpSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply MCP schema statement %d: %w", idx+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit MCP schema transaction: %w", err)
	}
	return nil
}

func (a *App) setMCPRegistry(servers map[string]models.MCPServer, profiles map[string]models.MCPProfile) config.Config {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	a.cfg.MCPServers = models.NormalizeMCPServers(servers)
	a.cfg.MCPProfiles = models.NormalizeMCPProfiles(profiles)
	return *a.cfg
}

func (a *App) mcpCatalogForValidation(cfg config.Config) map[string]validation.MCPProfileDefinition {
	profiles := cfg.EffectiveMCPProfiles()
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
	if !models.PipelineLLMEnabled(pipeline) {
		return nil
	}
	cfg := a.getConfigSnapshot()
	if err := validation.ValidatePipelineMCPProfiles(pipeline, validation.MCPProfileValidationOptions{
		Profiles: a.mcpCatalogForValidation(cfg),
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
		SELECT name, display_name, enabled, provider, transport, url, auth_type, auth_secret,
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
			&server.AuthSecret,
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
	if err := validateMCPRegistryDefinition(servers, profiles); err != nil {
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
	if err := validateMCPServerDefinition(server); err != nil {
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
			name, display_name, enabled, provider, transport, url, auth_type, auth_secret,
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
			auth_secret = EXCLUDED.auth_secret,
			headers = EXCLUDED.headers,
			timeout = EXCLUDED.timeout,
			allowed_scopes = EXCLUDED.allowed_scopes,
			updated_at = NOW()
	`, server.Name, server.DisplayName, server.Enabled, server.Provider, server.Transport, server.URL, server.AuthType, server.AuthSecret, string(headersJSON), server.Timeout, string(allowedJSON))
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

func parseGitOpsMCPRegistryPlan(binding models.ConfigRepository, directories ...gitOpsMCPDirectory) (*gitOpsMCPPlan, error) {
	candidates := []gitOpsMCPFileCandidate{}
	for _, directory := range directories {
		root := filepath.ToSlash(strings.Trim(directory.root, "/"))
		for path, content := range directory.files {
			normalized := filepath.ToSlash(path)
			rel, ok := relativeConfigPath(normalized, root)
			if !ok || !isGitOpsMCPRegistryRelativePath(rel) {
				continue
			}
			candidates = append(candidates, gitOpsMCPFileCandidate{
				sourcePath: normalized,
				content:    content,
			})
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}
	if binding.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil, fmt.Errorf("MCP registry can only be configured from a system config repository")
	}
	if len(candidates) > 1 {
		paths := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			paths = append(paths, candidate.sourcePath)
		}
		sort.Strings(paths)
		return nil, fmt.Errorf("multiple MCP registry GitOps files found: %s", strings.Join(paths, ", "))
	}

	return parseGitOpsMCPRegistryFile(candidates[0].content, candidates[0].sourcePath)
}

func isGitOpsMCPRegistryRelativePath(rel string) bool {
	switch strings.Trim(filepath.ToSlash(rel), "/") {
	case "system/mcp.yaml", "system/mcp.yml",
		"system/mcp_registry.yaml", "system/mcp_registry.yml",
		"system/mcp_profiles.yaml", "system/mcp_profiles.yml":
		return true
	default:
		return false
	}
}

func parseGitOpsMCPRegistryFile(content, sourcePath string) (*gitOpsMCPPlan, error) {
	var file mcpRegistryRequest
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		return nil, fmt.Errorf("failed to parse MCP registry GitOps file '%s': %w", sourcePath, err)
	}

	servers := map[string]models.MCPServer{}
	for name, server := range file.MCPServers {
		serverName := models.NormalizeMCPServerName(name)
		if serverName == "" {
			return nil, fmt.Errorf("MCP registry GitOps file '%s' contains an empty server name", sourcePath)
		}
		if declaredName := models.NormalizeMCPServerName(server.Name); declaredName != "" && declaredName != serverName {
			return nil, fmt.Errorf("MCP registry GitOps file '%s' server map key %q does not match name %q", sourcePath, serverName, declaredName)
		}
		if _, exists := servers[serverName]; exists {
			return nil, fmt.Errorf("MCP registry GitOps file '%s' defines server %q more than once", sourcePath, serverName)
		}
		server.Name = serverName
		servers[serverName] = models.NormalizeMCPServer(server)
	}
	for _, server := range file.Servers {
		serverName := models.NormalizeMCPServerName(server.Name)
		if serverName == "" {
			return nil, fmt.Errorf("MCP registry GitOps file '%s' contains a list server without a name", sourcePath)
		}
		if _, exists := servers[serverName]; exists {
			return nil, fmt.Errorf("MCP registry GitOps file '%s' defines server %q more than once", sourcePath, serverName)
		}
		server.Name = serverName
		servers[serverName] = models.NormalizeMCPServer(server)
	}

	profiles := map[string]models.MCPProfile{}
	for name, profile := range file.MCPProfiles {
		profileName := models.NormalizeMCPProfileName(name)
		if profileName == "" {
			return nil, fmt.Errorf("MCP registry GitOps file '%s' contains an empty profile name", sourcePath)
		}
		if declaredName := models.NormalizeMCPProfileName(profile.Name); declaredName != "" && declaredName != profileName {
			return nil, fmt.Errorf("MCP registry GitOps file '%s' profile map key %q does not match name %q", sourcePath, profileName, declaredName)
		}
		if _, exists := profiles[profileName]; exists {
			return nil, fmt.Errorf("MCP registry GitOps file '%s' defines profile %q more than once", sourcePath, profileName)
		}
		profile.Name = profileName
		profiles[profileName] = models.NormalizeMCPProfile(profile)
	}
	for _, profile := range file.Profiles {
		profileName := models.NormalizeMCPProfileName(profile.Name)
		if profileName == "" {
			return nil, fmt.Errorf("MCP registry GitOps file '%s' contains a list profile without a name", sourcePath)
		}
		if _, exists := profiles[profileName]; exists {
			return nil, fmt.Errorf("MCP registry GitOps file '%s' defines profile %q more than once", sourcePath, profileName)
		}
		profile.Name = profileName
		profiles[profileName] = models.NormalizeMCPProfile(profile)
	}

	if len(servers) == 0 && len(profiles) == 0 {
		return nil, fmt.Errorf("MCP registry GitOps file '%s' must define at least one server or profile", sourcePath)
	}
	if err := validateMCPRegistryDefinition(servers, profiles); err != nil {
		return nil, fmt.Errorf("invalid MCP registry in GitOps file '%s': %w", sourcePath, err)
	}

	return &gitOpsMCPPlan{
		servers:    servers,
		profiles:   profiles,
		sourcePath: sourcePath,
	}, nil
}

func validateMCPServerDefinition(server models.MCPServer) error {
	server = models.NormalizeMCPServer(server)
	if server.Name == "" {
		return fmt.Errorf("MCP server name is required")
	}
	if !mcpNamePattern.MatchString(server.Name) {
		return fmt.Errorf("MCP server name can only contain alphanumeric characters, underscores, dots, and hyphens")
	}
	if server.Transport != models.MCPTransportStreamableHTTP && server.Transport != models.MCPTransportHTTP {
		return fmt.Errorf("MCP server %q uses unsupported transport %q", server.Name, server.Transport)
	}
	if strings.TrimSpace(server.URL) == "" {
		return fmt.Errorf("MCP server %q is missing url", server.Name)
	}
	if server.AuthType != models.MCPAuthNone && server.AuthType != models.MCPAuthBearerToken {
		return fmt.Errorf("MCP server %q uses unsupported auth_type %q", server.Name, server.AuthType)
	}
	if server.AuthType == models.MCPAuthBearerToken && strings.TrimSpace(server.AuthSecret) == "" {
		return fmt.Errorf("MCP server %q bearer_token auth requires auth_secret", server.Name)
	}
	if _, err := time.ParseDuration(server.Timeout); err != nil {
		return fmt.Errorf("MCP server %q has invalid timeout %q", server.Name, server.Timeout)
	}
	if err := validateMCPAllowedScopes("MCP server", server.Name, server.AllowedScopes); err != nil {
		return err
	}
	return nil
}

func validateMCPProfileDefinition(profile models.MCPProfile, servers map[string]models.MCPServer, _ map[string][]models.MCPTool) error {
	profile = models.NormalizeMCPProfile(profile)
	if profile.Name == "" {
		return fmt.Errorf("MCP profile name is required")
	}
	if !mcpNamePattern.MatchString(profile.Name) {
		return fmt.Errorf("MCP profile name can only contain alphanumeric characters, underscores, dots, and hyphens")
	}
	if len(profile.ServerRefs) == 0 {
		return fmt.Errorf("MCP profile %q must select at least one server", profile.Name)
	}
	if err := validateMCPAllowedScopes("MCP profile", profile.Name, profile.AllowedScopes); err != nil {
		return err
	}
	for _, ref := range profile.ServerRefs {
		server, ok := servers[ref.ServerName]
		if !ok {
			return fmt.Errorf("MCP profile %q references unknown server %q", profile.Name, ref.ServerName)
		}
		if !server.Enabled {
			return fmt.Errorf("MCP profile %q references disabled server %q", profile.Name, ref.ServerName)
		}
		if len(ref.Tools) == 0 {
			return fmt.Errorf("MCP profile %q must select tools for server %q", profile.Name, ref.ServerName)
		}
		if mcpProfileRefSelectsAllTools(ref) && !mcpServerConfiguredReadonly(server) {
			return fmt.Errorf("MCP profile %q can use wildcard tools only with a read-only MCP server", profile.Name)
		}
		for _, tool := range ref.Tools {
			if isMCPAllToolsSelector(tool) {
				continue
			}
			if !isReadOnlyMCPToolName(tool) {
				return fmt.Errorf("MCP profile %q references write-like tool %q; only read-only MCP tools are allowed", profile.Name, tool)
			}
		}
	}
	return nil
}

func validateMCPRegistryDefinition(servers map[string]models.MCPServer, profiles map[string]models.MCPProfile) error {
	for name, server := range servers {
		server.Name = firstNonEmptyString(server.Name, name)
		if err := validateMCPServerDefinition(server); err != nil {
			return err
		}
	}
	for name, profile := range profiles {
		profile.Name = firstNonEmptyString(profile.Name, name)
		if err := validateMCPProfileDefinitionWithoutDiscovery(profile, servers); err != nil {
			return err
		}
	}
	return nil
}

func validateMCPProfileDefinitionWithoutDiscovery(profile models.MCPProfile, servers map[string]models.MCPServer) error {
	profile = models.NormalizeMCPProfile(profile)
	if profile.Name == "" {
		return fmt.Errorf("MCP profile name is required")
	}
	if !mcpNamePattern.MatchString(profile.Name) {
		return fmt.Errorf("MCP profile name can only contain alphanumeric characters, underscores, dots, and hyphens")
	}
	if len(profile.ServerRefs) == 0 {
		return fmt.Errorf("MCP profile %q must select at least one server", profile.Name)
	}
	if err := validateMCPAllowedScopes("MCP profile", profile.Name, profile.AllowedScopes); err != nil {
		return err
	}
	for _, ref := range profile.ServerRefs {
		server, ok := servers[ref.ServerName]
		if !ok {
			return fmt.Errorf("MCP profile %q references unknown server %q", profile.Name, ref.ServerName)
		}
		if !server.Enabled {
			return fmt.Errorf("MCP profile %q references disabled server %q", profile.Name, ref.ServerName)
		}
		if len(ref.Tools) == 0 {
			return fmt.Errorf("MCP profile %q must select tools for server %q", profile.Name, ref.ServerName)
		}
		if mcpProfileRefSelectsAllTools(ref) && !mcpServerConfiguredReadonly(server) {
			return fmt.Errorf("MCP profile %q can use wildcard tools only with a read-only MCP server", profile.Name)
		}
		for _, tool := range ref.Tools {
			if isMCPAllToolsSelector(tool) {
				continue
			}
			if !isReadOnlyMCPToolName(tool) {
				return fmt.Errorf("MCP profile %q references write-like tool %q; only read-only MCP tools are allowed", profile.Name, tool)
			}
		}
	}
	return nil
}

func validateMCPAllowedScopes(resourceType, name string, scopes []string) error {
	for _, scope := range scopes {
		if strings.TrimSpace(scope) == "" {
			continue
		}
		if _, err := cleanConfigPathSegments(scope, false); err != nil {
			return fmt.Errorf("%s %q has invalid allowed scope %q: %w", resourceType, name, scope, err)
		}
	}
	return nil
}

func isReadOnlyMCPToolName(toolName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(toolName))
	if normalized == "" {
		return false
	}
	writePrefixes := []string{
		"add", "approve", "assign", "close", "comment", "create", "delete", "deploy", "disable", "enable",
		"insert", "merge", "modify", "patch", "post", "publish", "remove", "request_changes", "send", "set",
		"submit", "transition", "update", "write",
	}
	firstToken := normalized
	for _, sep := range []string{"_", "-", ".", ":"} {
		if idx := strings.Index(firstToken, sep); idx >= 0 {
			firstToken = firstToken[:idx]
		}
	}
	for _, prefix := range writePrefixes {
		if firstToken == prefix || strings.HasPrefix(normalized, prefix+"_") || strings.HasPrefix(normalized, prefix+"-") {
			return false
		}
	}
	return true
}

func isMCPAllToolsSelector(toolName string) bool {
	return strings.TrimSpace(toolName) == "*"
}

func mcpProfileRefSelectsAllTools(ref models.MCPProfileServerRef) bool {
	for _, toolName := range ref.Tools {
		if isMCPAllToolsSelector(toolName) {
			return true
		}
	}
	return false
}

func mcpServerConfiguredReadonly(server models.MCPServer) bool {
	server = models.NormalizeMCPServer(server)
	for key, value := range server.Headers {
		if strings.EqualFold(strings.TrimSpace(key), "X-MCP-Readonly") && mcpTruthyHeader(value) {
			return true
		}
	}
	urlPath := strings.ToLower(strings.TrimRight(server.URL, "/"))
	return strings.Contains(urlPath, "/readonly")
}

func mcpTruthyHeader(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "false", "f", "no", "n", "0", "off":
		return false
	default:
		return true
	}
}

func (a *App) handleListMCPServers(w http.ResponseWriter, r *http.Request) {
	servers, err := a.loadMCPServersFromDB(r.Context())
	if err != nil {
		http.Error(w, "failed to load MCP servers", http.StatusInternalServerError)
		return
	}
	toolsByServer, err := a.loadMCPToolsByServer(r.Context())
	if err != nil {
		http.Error(w, "failed to load MCP tools", http.StatusInternalServerError)
		return
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	views := make([]mcpServerView, 0, len(names))
	for _, name := range names {
		views = append(views, mcpServerView{MCPServer: servers[name], Tools: toolsByServer[name]})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(mcpServersResponse{Servers: views})
}

func (a *App) handleCreateMCPServer(w http.ResponseWriter, r *http.Request) {
	a.handleUpsertMCPServer(w, r)
}

func (a *App) handleGetMCPServer(w http.ResponseWriter, r *http.Request) {
	name := models.NormalizeMCPServerName(r.PathValue("serverName"))
	if name == "" {
		http.Error(w, "MCP server name is required", http.StatusBadRequest)
		return
	}
	servers, err := a.loadMCPServersFromDB(r.Context())
	if err != nil {
		http.Error(w, "failed to load MCP servers", http.StatusInternalServerError)
		return
	}
	server, ok := servers[name]
	if !ok {
		http.Error(w, "MCP server not found", http.StatusNotFound)
		return
	}
	toolsByServer, err := a.loadMCPToolsByServer(r.Context())
	if err != nil {
		http.Error(w, "failed to load MCP tools", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(mcpServerView{MCPServer: server, Tools: toolsByServer[name]})
}

func (a *App) handleUpsertMCPServer(w http.ResponseWriter, r *http.Request) {
	var server models.MCPServer
	if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
		http.Error(w, "invalid MCP server payload", http.StatusBadRequest)
		return
	}
	pathName := models.NormalizeMCPServerName(r.PathValue("serverName"))
	if pathName != "" {
		if server.Name != "" && !strings.EqualFold(models.NormalizeMCPServerName(server.Name), pathName) {
			http.Error(w, "MCP server name in path and payload must match", http.StatusBadRequest)
			return
		}
		server.Name = pathName
	}
	server = models.NormalizeMCPServer(server)
	if err := validateMCPServerDefinition(server); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to save MCP server", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())
	if err := upsertMCPServerTx(r.Context(), tx, server); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to save MCP server", http.StatusInternalServerError)
		return
	}
	if err := a.refreshMCPRegistryFromDB(r.Context()); err != nil {
		http.Error(w, "failed to refresh MCP registry", http.StatusInternalServerError)
		return
	}
	a.handleListMCPServers(w, r)
}

func (a *App) handleDeleteMCPServer(w http.ResponseWriter, r *http.Request) {
	name := models.NormalizeMCPServerName(r.PathValue("serverName"))
	if name == "" {
		http.Error(w, "MCP server name is required", http.StatusBadRequest)
		return
	}
	profiles, err := a.loadMCPProfilesFromDB(r.Context())
	if err != nil {
		http.Error(w, "failed to inspect MCP profiles", http.StatusInternalServerError)
		return
	}
	var refs []string
	for profileName, profile := range profiles {
		for _, ref := range profile.ServerRefs {
			if strings.EqualFold(ref.ServerName, name) {
				refs = append(refs, profileName)
			}
		}
	}
	if len(refs) > 0 {
		sort.Strings(refs)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":      "MCP server is still referenced by profiles",
			"references": refs,
		})
		return
	}
	tag, err := a.db.Exec(r.Context(), `DELETE FROM mcp_servers WHERE name = $1`, name)
	if err != nil {
		http.Error(w, "failed to delete MCP server", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "MCP server not found", http.StatusNotFound)
		return
	}
	if err := a.refreshMCPRegistryFromDB(r.Context()); err != nil {
		http.Error(w, "failed to refresh MCP registry", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleTestMCPServer(w http.ResponseWriter, r *http.Request) {
	a.handleDiscoverMCPServerTools(w, r)
}

func (a *App) handleDiscoverMCPServerTools(w http.ResponseWriter, r *http.Request) {
	name := models.NormalizeMCPServerName(r.PathValue("serverName"))
	if name == "" {
		http.Error(w, "MCP server name is required", http.StatusBadRequest)
		return
	}
	servers, err := a.loadMCPServersFromDB(r.Context())
	if err != nil {
		http.Error(w, "failed to load MCP servers", http.StatusInternalServerError)
		return
	}
	server, ok := servers[name]
	if !ok {
		http.Error(w, "MCP server not found", http.StatusNotFound)
		return
	}
	result, err := a.discoverAndStoreMCPServerTools(r.Context(), server)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := a.refreshMCPRegistryFromDB(r.Context()); err != nil {
		http.Error(w, "failed to refresh MCP registry", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (a *App) discoverAndStoreMCPServerTools(ctx context.Context, server models.MCPServer) (mcpServerView, error) {
	client, err := mcpclient.New(server)
	if err != nil {
		return mcpServerView{}, err
	}
	timeout := 30 * time.Second
	if parsed, err := time.ParseDuration(server.Timeout); err == nil && parsed > 0 {
		timeout = parsed
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	testResult, err := client.Test(ctx)
	now := time.Now().UTC()
	status := "connected"
	message := "Connected and discovered tools"
	if err != nil {
		status = "failed"
		message = err.Error()
		_, _ = a.db.Exec(context.Background(), `
			UPDATE mcp_servers
			SET last_test_status = $1, last_test_message = $2, last_tested_at = $3, updated_at = NOW()
			WHERE name = $4
		`, status, message, now, server.Name)
		return mcpServerView{}, err
	}

	tx, err := a.db.Begin(context.Background())
	if err != nil {
		return mcpServerView{}, err
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(context.Background(), `
		UPDATE mcp_servers
		SET last_test_status = $1,
			last_test_message = $2,
			last_tested_at = $3,
			last_discovered_at = $3,
			discovered_server_name = $4,
			discovered_version = $5,
			discovered_protocol = $6,
			updated_at = NOW()
		WHERE name = $7
	`, status, message, now, testResult.Initialize.ServerInfo.Name, testResult.Initialize.ServerInfo.Version, testResult.Initialize.ProtocolVersion, server.Name)
	if err != nil {
		return mcpServerView{}, fmt.Errorf("store MCP server test status: %w", err)
	}
	if _, err := tx.Exec(context.Background(), `DELETE FROM mcp_tools WHERE server_name = $1`, server.Name); err != nil {
		return mcpServerView{}, fmt.Errorf("clear MCP tools: %w", err)
	}
	for _, tool := range testResult.Tools {
		if strings.TrimSpace(tool.Name) == "" {
			continue
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO mcp_tools (server_name, name, description, input_schema, schema_hash, last_seen_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (server_name, name) DO UPDATE SET
				description = EXCLUDED.description,
				input_schema = EXCLUDED.input_schema,
				schema_hash = EXCLUDED.schema_hash,
				last_seen_at = EXCLUDED.last_seen_at
		`, server.Name, tool.Name, tool.Description, tool.InputSchema, tool.SchemaHash, now)
		if err != nil {
			return mcpServerView{}, fmt.Errorf("store MCP tool %q: %w", tool.Name, err)
		}
	}
	if err := tx.Commit(context.Background()); err != nil {
		return mcpServerView{}, fmt.Errorf("commit MCP discovery: %w", err)
	}

	servers, _ := a.loadMCPServersFromDB(context.Background())
	updated := servers[server.Name]
	return mcpServerView{MCPServer: updated, Tools: testResult.Tools}, nil
}

func (a *App) handleListMCPProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := a.loadMCPProfilesFromDB(r.Context())
	if err != nil {
		http.Error(w, "failed to load MCP profiles", http.StatusInternalServerError)
		return
	}
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	response := mcpProfilesResponse{Profiles: make([]models.MCPProfile, 0, len(names))}
	for _, name := range names {
		response.Profiles = append(response.Profiles, profiles[name])
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (a *App) handleCreateMCPProfile(w http.ResponseWriter, r *http.Request) {
	a.handleUpsertMCPProfile(w, r)
}

func (a *App) handleGetMCPProfile(w http.ResponseWriter, r *http.Request) {
	name := models.NormalizeMCPProfileName(r.PathValue("profileName"))
	if name == "" {
		http.Error(w, "MCP profile name is required", http.StatusBadRequest)
		return
	}
	profiles, err := a.loadMCPProfilesFromDB(r.Context())
	if err != nil {
		http.Error(w, "failed to load MCP profiles", http.StatusInternalServerError)
		return
	}
	profile, ok := profiles[name]
	if !ok {
		http.Error(w, "MCP profile not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(profile)
}

func (a *App) handleUpsertMCPProfile(w http.ResponseWriter, r *http.Request) {
	var profile models.MCPProfile
	if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
		http.Error(w, "invalid MCP profile payload", http.StatusBadRequest)
		return
	}
	pathName := models.NormalizeMCPProfileName(r.PathValue("profileName"))
	if pathName != "" {
		if profile.Name != "" && !strings.EqualFold(models.NormalizeMCPProfileName(profile.Name), pathName) {
			http.Error(w, "MCP profile name in path and payload must match", http.StatusBadRequest)
			return
		}
		profile.Name = pathName
	}
	profile = models.NormalizeMCPProfile(profile)
	servers, err := a.loadMCPServersFromDB(r.Context())
	if err != nil {
		http.Error(w, "failed to load MCP servers", http.StatusInternalServerError)
		return
	}
	toolsByServer, err := a.loadMCPToolsByServer(r.Context())
	if err != nil {
		http.Error(w, "failed to load MCP tools", http.StatusInternalServerError)
		return
	}
	if err := validateMCPProfileDefinition(profile, servers, toolsByServer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		http.Error(w, "failed to save MCP profile", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())
	if err := upsertMCPProfileTx(r.Context(), tx, profile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "failed to save MCP profile", http.StatusInternalServerError)
		return
	}
	if err := a.refreshMCPRegistryFromDB(r.Context()); err != nil {
		http.Error(w, "failed to refresh MCP registry", http.StatusInternalServerError)
		return
	}
	a.handleListMCPProfiles(w, r)
}

func (a *App) handleDeleteMCPProfile(w http.ResponseWriter, r *http.Request) {
	name := models.NormalizeMCPProfileName(r.PathValue("profileName"))
	if name == "" {
		http.Error(w, "MCP profile name is required", http.StatusBadRequest)
		return
	}
	refs, err := a.findMCPProfileReferences(name)
	if err != nil {
		http.Error(w, "failed to inspect MCP profile references", http.StatusInternalServerError)
		return
	}
	if len(refs) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":      "MCP profile is still referenced",
			"references": refs,
		})
		return
	}
	tag, err := a.db.Exec(r.Context(), `DELETE FROM mcp_profiles WHERE name = $1`, name)
	if err != nil {
		http.Error(w, "failed to delete MCP profile", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "MCP profile not found", http.StatusNotFound)
		return
	}
	if err := a.refreshMCPRegistryFromDB(r.Context()); err != nil {
		http.Error(w, "failed to refresh MCP registry", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleTestMCPProfile(w http.ResponseWriter, r *http.Request) {
	name := models.NormalizeMCPProfileName(r.PathValue("profileName"))
	if name == "" {
		http.Error(w, "MCP profile name is required", http.StatusBadRequest)
		return
	}
	profiles, err := a.loadMCPProfilesFromDB(r.Context())
	if err != nil {
		http.Error(w, "failed to load MCP profiles", http.StatusInternalServerError)
		return
	}
	profile, ok := profiles[name]
	if !ok {
		http.Error(w, "MCP profile not found", http.StatusNotFound)
		return
	}
	servers, err := a.loadMCPServersFromDB(r.Context())
	if err != nil {
		http.Error(w, "failed to load MCP servers", http.StatusInternalServerError)
		return
	}
	toolsByServer, err := a.loadMCPToolsByServer(r.Context())
	if err != nil {
		http.Error(w, "failed to load MCP tools", http.StatusInternalServerError)
		return
	}
	if err := validateMCPProfileDefinition(profile, servers, toolsByServer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var warnings []string
	for _, ref := range profile.ServerRefs {
		server := servers[ref.ServerName]
		client, err := mcpclient.New(server)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		_, err = client.Initialize(ctx)
		if err == nil {
			var liveTools []models.MCPTool
			liveTools, err = client.ListTools(ctx)
			if err == nil {
				warnings = append(warnings, compareMCPToolSchemas(ref, toolsByServer[ref.ServerName], liveTools)...)
			}
		}
		cancel()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	status := "ok"
	if len(warnings) > 0 {
		status = "warning"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(mcpProfileTestResponse{Status: status, Message: "MCP profile test completed", Warnings: warnings})
}

func compareMCPToolSchemas(ref models.MCPProfileServerRef, cachedTools, liveTools []models.MCPTool) []string {
	if mcpProfileRefSelectsAllTools(ref) {
		if len(liveTools) == 0 {
			return []string{fmt.Sprintf("Server %s did not return any live tools", ref.ServerName)}
		}
		return nil
	}
	cached := map[string]models.MCPTool{}
	for _, tool := range cachedTools {
		cached[tool.Name] = tool
	}
	live := map[string]models.MCPTool{}
	for _, tool := range liveTools {
		live[tool.Name] = tool
	}
	var warnings []string
	for _, toolName := range ref.Tools {
		liveTool, ok := live[toolName]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("Tool %s no longer exists on server %s", toolName, ref.ServerName))
			continue
		}
		if cachedTool, ok := cached[toolName]; ok && cachedTool.SchemaHash != "" && liveTool.SchemaHash != cachedTool.SchemaHash {
			warnings = append(warnings, fmt.Sprintf("Tool %s on server %s has a changed schema", toolName, ref.ServerName))
		}
	}
	return warnings
}

func (a *App) findMCPProfileReferences(profileName string) ([]string, error) {
	profileName = models.NormalizeMCPProfileName(profileName)
	if profileName == "" || a == nil || a.db == nil {
		return nil, nil
	}
	var refs []string
	rows, err := a.db.Query(context.Background(), `SELECT path, name, definition FROM pipelines ORDER BY path ASC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pathPart, namePart, definition string
		if err := rows.Scan(&pathPart, &namePart, &definition); err != nil {
			return nil, err
		}
		var pipeline models.Pipeline
		if err := yaml.Unmarshal([]byte(definition), &pipeline); err != nil {
			continue
		}
		refs = append(refs, collectExplicitMCPProfileReferences(&pipeline, profileName, "pipeline "+buildPipelineIdentifier(pathPart, namePart))...)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return refs, nil
}

func collectExplicitMCPProfileReferences(pipeline *models.Pipeline, profileName, prefix string) []string {
	var refs []string
	if pipeline == nil {
		return refs
	}
	for _, name := range pipeline.MCPProfiles {
		if strings.EqualFold(strings.TrimSpace(name), profileName) {
			refs = append(refs, prefix)
		}
	}
	for _, step := range pipeline.Steps {
		stepName := strings.TrimSpace(step.GetName())
		if stepName == "" {
			stepName = "unknown"
		}
		stepRef := fmt.Sprintf("%s step %q", prefix, stepName)
		for _, name := range step.GetMCPProfiles() {
			if strings.EqualFold(strings.TrimSpace(name), profileName) {
				refs = append(refs, stepRef)
			}
		}
		for _, task := range step.GetTasks() {
			taskName := strings.TrimSpace(task.Name)
			if taskName == "" {
				taskName = "unknown"
			}
			for _, name := range task.MCPProfiles {
				if strings.EqualFold(strings.TrimSpace(name), profileName) {
					refs = append(refs, fmt.Sprintf("%s task %q", stepRef, taskName))
				}
			}
		}
	}
	return refs
}

func collectResolvedMCPProfiles(pipeline *models.Pipeline) map[string]bool {
	used := map[string]bool{}
	if pipeline == nil {
		return used
	}
	add := func(values []string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				used[value] = true
			}
		}
	}
	add(pipeline.MCPProfiles)
	for _, step := range pipeline.Steps {
		add(step.GetMCPProfiles())
		for _, task := range step.GetTasks() {
			add(task.MCPProfiles)
		}
	}
	return used
}

func (a *App) buildRuntimeMCPRegistry(pipeline *models.Pipeline, scope string) (runtimeMCPRegistry, error) {
	if !models.PipelineLLMEnabled(pipeline) {
		return runtimeMCPRegistry{}, nil
	}
	cfg := a.getConfigSnapshot()
	allServers := cfg.EffectiveMCPServers()
	allProfiles := cfg.EffectiveMCPProfiles()
	usedProfiles := collectResolvedMCPProfiles(pipeline)
	if len(usedProfiles) == 0 {
		return runtimeMCPRegistry{}, nil
	}
	toolsByServer, err := a.loadMCPToolsByServer(context.Background())
	if err != nil {
		return runtimeMCPRegistry{}, err
	}

	registry := runtimeMCPRegistry{
		Servers:  map[string]runtimeMCPServer{},
		Profiles: map[string]models.MCPProfile{},
		Tools:    map[string][]models.MCPTool{},
	}
	for profileName := range usedProfiles {
		profile, ok := allProfiles[profileName]
		if !ok {
			return runtimeMCPRegistry{}, fmt.Errorf("MCP profile %q is not configured", profileName)
		}
		if !profile.Enabled {
			return runtimeMCPRegistry{}, fmt.Errorf("MCP profile %q is disabled", profileName)
		}
		registry.Profiles[profileName] = profile
		for _, ref := range profile.ServerRefs {
			server, ok := allServers[ref.ServerName]
			if !ok {
				return runtimeMCPRegistry{}, fmt.Errorf("MCP profile %q references unknown server %q", profileName, ref.ServerName)
			}
			if !server.Enabled {
				return runtimeMCPRegistry{}, fmt.Errorf("MCP profile %q references disabled server %q", profileName, ref.ServerName)
			}
			if !models.MCPAllowedInScope(server.AllowedScopes, scope) {
				return runtimeMCPRegistry{}, fmt.Errorf("MCP server %q is not allowed in scope %q", ref.ServerName, strings.TrimSpace(scope))
			}
			registry.Servers[ref.ServerName] = runtimeMCPServer{MCPServer: server, AuthValue: resolveMCPAuthSecret(server)}
			if mcpProfileRefSelectsAllTools(ref) && len(toolsByServer[ref.ServerName]) == 0 {
				discovered, err := a.discoverAndStoreMCPServerTools(context.Background(), server)
				if err != nil {
					return runtimeMCPRegistry{}, fmt.Errorf("discover MCP tools for wildcard profile %q on server %q: %w", profileName, ref.ServerName, err)
				}
				toolsByServer[ref.ServerName] = discovered.Tools
			}
			selectedTools := selectMCPTools(ref.ServerName, toolsByServer[ref.ServerName], ref.Tools)
			if len(selectedTools) == 0 {
				return runtimeMCPRegistry{}, fmt.Errorf("MCP profile %q has no selected tools for server %q", profileName, ref.ServerName)
			}
			registry.Tools[ref.ServerName] = mergeMCPTools(registry.Tools[ref.ServerName], selectedTools)
		}
	}
	return registry, nil
}

func resolveMCPAuthSecret(server models.MCPServer) string {
	if strings.TrimSpace(server.AuthSecret) == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(server.AuthSecret))
}

func selectMCPTools(serverName string, discovered []models.MCPTool, names []string) []models.MCPTool {
	byName := map[string]models.MCPTool{}
	for _, tool := range discovered {
		tool.Name = strings.TrimSpace(tool.Name)
		if tool.Name == "" {
			continue
		}
		if strings.TrimSpace(tool.ServerName) == "" {
			tool.ServerName = serverName
		}
		if strings.TrimSpace(tool.InputSchema) == "" {
			tool.InputSchema = "{}"
		}
		byName[tool.Name] = tool
	}
	if mcpToolNamesSelectAll(names) {
		orderedNames := make([]string, 0, len(byName))
		for name := range byName {
			orderedNames = append(orderedNames, name)
		}
		sort.Strings(orderedNames)
		selected := make([]models.MCPTool, 0, len(orderedNames))
		for _, name := range orderedNames {
			selected = append(selected, byName[name])
		}
		return selected
	}
	selected := map[string]models.MCPTool{}
	for _, rawName := range names {
		name := strings.TrimSpace(rawName)
		if name == "" || isMCPAllToolsSelector(name) {
			continue
		}
		if tool, ok := byName[name]; ok {
			selected[name] = tool
			continue
		}
		selected[name] = models.MCPTool{
			ServerName:  serverName,
			Name:        name,
			InputSchema: "{}",
		}
	}
	orderedNames := make([]string, 0, len(selected))
	for name := range selected {
		orderedNames = append(orderedNames, name)
	}
	sort.Strings(orderedNames)
	filtered := make([]models.MCPTool, 0, len(orderedNames))
	for _, name := range orderedNames {
		filtered = append(filtered, selected[name])
	}
	return filtered
}

func mcpToolNamesSelectAll(names []string) bool {
	for _, name := range names {
		if isMCPAllToolsSelector(name) {
			return true
		}
	}
	return false
}

func mergeMCPTools(existing, next []models.MCPTool) []models.MCPTool {
	byName := map[string]models.MCPTool{}
	for _, tool := range existing {
		byName[tool.Name] = tool
	}
	for _, tool := range next {
		byName[tool.Name] = tool
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	merged := make([]models.MCPTool, 0, len(names))
	for _, name := range names {
		merged = append(merged, byName[name])
	}
	return merged
}

func writeMCPStoreError(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, pgx.ErrNoRows):
		http.Error(w, "MCP resource not found", http.StatusNotFound)
	default:
		http.Error(w, "MCP request failed", http.StatusInternalServerError)
	}
}
