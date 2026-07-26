package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"nopsai/pkg/mcpclient"
	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/mcpregistry"
)

func (a *App) newMCPClient(ctx context.Context, server models.MCPServer) (*mcpclient.Client, error) {
	authValue, err := a.resolveMCPAuthCredential(ctx, server)
	if err != nil {
		return nil, err
	}
	return mcpclient.New(server, mcpclient.WithAuthValue(authValue))
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
		visible, err := a.aiResourceVisible(r, mcpServerAccessSpec, name)
		if err != nil {
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return
		}
		if !visible {
			continue
		}
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
	if !a.requireAIResourceVisible(w, r, mcpServerAccessSpec, name) {
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
	if server.Name == "" {
		http.Error(w, "MCP server name is required", http.StatusBadRequest)
		return
	}
	if !a.requireAIResourceWrite(w, r, mcpServerAccessSpec, server.Name) {
		return
	}
	if err := mcpregistry.ValidateServerDefinition(server); err != nil {
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
	if !a.requireAIResourceWrite(w, r, mcpServerAccessSpec, name) {
		return
	}
	refs, err := a.findMCPServerReferences(r.Context(), name)
	if err != nil {
		http.Error(w, "failed to inspect MCP profiles", http.StatusInternalServerError)
		return
	}
	if len(refs) > 0 {
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

func (a *App) findMCPServerReferences(ctx context.Context, name string) ([]string, error) {
	name = models.NormalizeMCPServerName(name)
	if name == "" {
		return nil, nil
	}
	references := map[string]bool{}
	profiles, err := a.loadMCPProfilesFromDB(ctx)
	if err != nil {
		return nil, err
	}
	for profileName, profile := range profiles {
		for _, ref := range profile.ServerRefs {
			if strings.EqualFold(ref.ServerName, name) {
				references[profileName] = true
			}
		}
	}

	rows, err := a.db.Query(ctx, `
		SELECT COALESCE(NULLIF(t.path, ''), t.name, tm.team_id::text), tm.name, tm.server_refs
		FROM team_mcp_profiles tm
		JOIN teams t ON t.id = tm.team_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			teamPath    string
			profileName string
			refsRaw     []byte
			serverRefs  []models.MCPProfileServerRef
		)
		if err := rows.Scan(&teamPath, &profileName, &refsRaw); err != nil {
			return nil, err
		}
		if len(refsRaw) > 0 {
			_ = json.Unmarshal(refsRaw, &serverRefs)
		}
		for _, ref := range serverRefs {
			if strings.EqualFold(ref.ServerName, name) {
				referenceName := models.NormalizeMCPProfileName(profileName)
				if normalizedTeamPath := strings.Trim(strings.TrimSpace(teamPath), "/"); normalizedTeamPath != "" {
					referenceName = normalizedTeamPath + "/" + referenceName
				}
				references[referenceName] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]string, 0, len(references))
	for ref := range references {
		result = append(result, ref)
	}
	sort.Strings(result)
	return result, nil
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
	if !a.requireAIResourceUse(w, r, mcpServerAccessSpec, name) {
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
	client, err := a.newMCPClient(ctx, server)
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
		visible, err := a.aiResourceVisible(r, mcpProfileAccessSpec, name)
		if err != nil {
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return
		}
		if !visible {
			continue
		}
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
	if !a.requireAIResourceVisible(w, r, mcpProfileAccessSpec, name) {
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
	if profile.Name == "" {
		http.Error(w, "MCP profile name is required", http.StatusBadRequest)
		return
	}
	if !a.requireAIResourceWrite(w, r, mcpProfileAccessSpec, profile.Name) {
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
	if err := mcpregistry.ValidateProfileDefinition(profile, servers, toolsByServer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, ref := range profile.ServerRefs {
		if !a.requireAIResourceUse(w, r, mcpServerAccessSpec, ref.ServerName) {
			return
		}
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
	if !a.requireAIResourceWrite(w, r, mcpProfileAccessSpec, name) {
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
	if !a.requireAIResourceUse(w, r, mcpProfileAccessSpec, name) {
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
	if err := mcpregistry.ValidateProfileDefinition(profile, servers, toolsByServer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var warnings []string
	for _, ref := range profile.ServerRefs {
		if !a.requireAIResourceUse(w, r, mcpServerAccessSpec, ref.ServerName) {
			return
		}
		server := servers[ref.ServerName]
		client, err := a.newMCPClient(r.Context(), server)
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
				warnings = append(warnings, mcpregistry.CompareToolSchemas(ref, toolsByServer[ref.ServerName], liveTools)...)
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
