package nopsai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/mcpregistry"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"
)

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
		refs = append(refs, collectExplicitMCPProfileReferences(&pipeline, profileName, "pipeline "+configsync.BuildPipelineIdentifier(pathPart, namePart))...)
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
			if mcpregistry.ProfileRefSelectsAllTools(ref) && len(toolsByServer[ref.ServerName]) == 0 {
				discovered, err := a.discoverAndStoreMCPServerTools(context.Background(), server)
				if err != nil {
					return runtimeMCPRegistry{}, fmt.Errorf("discover MCP tools for wildcard profile %q on server %q: %w", profileName, ref.ServerName, err)
				}
				toolsByServer[ref.ServerName] = discovered.Tools
			}
			selectedTools := mcpregistry.SelectTools(ref.ServerName, toolsByServer[ref.ServerName], ref.Tools)
			if len(selectedTools) == 0 {
				return runtimeMCPRegistry{}, fmt.Errorf("MCP profile %q has no selected tools for server %q", profileName, ref.ServerName)
			}
			registry.Tools[ref.ServerName] = mcpregistry.MergeTools(registry.Tools[ref.ServerName], selectedTools)
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
