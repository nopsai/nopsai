package nopsai

import (
	"fmt"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/mcpregistry"

	"gopkg.in/yaml.v3"
)

// mcpServerGitOpsFile is one MCP server definition at mcp/servers/<name>.yaml.
// Servers are workspace-wide connection definitions, so they are only owned by a
// system config repository; teams select servers through profiles.
type mcpServerGitOpsFile struct {
	models.MCPServer `yaml:",inline"`
}

// mcpProfileGitOpsFile is one MCP profile at mcp/profiles/<name>.yaml or
// mcp/profiles/<team>/<name>.yaml.
type mcpProfileGitOpsFile struct {
	models.MCPProfile `yaml:",inline"`
}

type storedMCPServer struct {
	name       string
	server     models.MCPServer
	sourcePath string
}

type storedMCPProfile struct {
	team       string
	name       string
	profile    models.MCPProfile
	sourcePath string
}

type gitOpsMCPRegistryPlan struct {
	servers  map[string]storedMCPServer
	profiles map[string]storedMCPProfile
}

func (p *gitOpsMCPRegistryPlan) empty() bool {
	return p == nil || (len(p.servers) == 0 && len(p.profiles) == 0)
}

func parseGitOpsMCPRegistry(
	files map[string]string,
	serverRoot, profileRoot string,
	binding models.ConfigRepository,
	boundTeam string,
) (*gitOpsMCPRegistryPlan, error) {
	plan := &gitOpsMCPRegistryPlan{servers: map[string]storedMCPServer{}, profiles: map[string]storedMCPProfile{}}

	err := registryGitOpsFiles(files, serverRoot, binding, boundTeam, "MCP server", func(resource registryGitOpsResource, content string) error {
		if resource.team != "" {
			return fmt.Errorf("MCP server '%s' must live directly under %s; servers are workspace-wide and teams select them through profiles", resource.sourcePath, mcpServersGitOpsDirectory)
		}
		if binding.ScopeType != models.ConfigRepositoryScopeSystem {
			return fmt.Errorf("MCP server '%s' can only be defined by a system config repository", resource.sourcePath)
		}
		var file mcpServerGitOpsFile
		if err := yaml.Unmarshal([]byte(content), &file); err != nil {
			return fmt.Errorf("failed to parse MCP server '%s': %w", resource.sourcePath, err)
		}
		name := models.NormalizeMCPServerName(resource.name)
		if declared := strings.TrimSpace(file.Name); declared != "" && models.NormalizeMCPServerName(declared) != name {
			return fmt.Errorf("MCP server '%s' declares name %q but the file name implies %q", resource.sourcePath, declared, name)
		}
		server := models.NormalizeMCPServer(file.MCPServer)
		server.Name = name
		if existing, exists := plan.servers[name]; exists {
			return fmt.Errorf("duplicate MCP server '%s' defined by '%s' and '%s'", name, existing.sourcePath, resource.sourcePath)
		}
		plan.servers[name] = storedMCPServer{name: name, server: server, sourcePath: resource.sourcePath}
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = registryGitOpsFiles(files, profileRoot, binding, boundTeam, "MCP profile", func(resource registryGitOpsResource, content string) error {
		var file mcpProfileGitOpsFile
		if err := yaml.Unmarshal([]byte(content), &file); err != nil {
			return fmt.Errorf("failed to parse MCP profile '%s': %w", resource.sourcePath, err)
		}
		name := models.NormalizeMCPProfileName(resource.name)
		if declared := strings.TrimSpace(file.Name); declared != "" && models.NormalizeMCPProfileName(declared) != name {
			return fmt.Errorf("MCP profile '%s' declares name %q but the file name implies %q", resource.sourcePath, declared, name)
		}
		profile := models.NormalizeMCPProfile(file.MCPProfile)
		profile.Name = name
		resource.name = name
		key := resource.key()
		if existing, exists := plan.profiles[key]; exists {
			return fmt.Errorf("duplicate MCP profile '%s' defined by '%s' and '%s'", key, existing.sourcePath, resource.sourcePath)
		}
		plan.profiles[key] = storedMCPProfile{
			team:       resource.team,
			name:       name,
			profile:    profile,
			sourcePath: resource.sourcePath,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if plan.empty() {
		return nil, nil
	}

	servers := plan.globalServers()
	globalProfiles := map[string]models.MCPProfile{}
	for _, stored := range plan.profiles {
		if stored.team == "" {
			globalProfiles[stored.name] = stored.profile
		}
	}
	if err := mcpregistry.ValidateRegistryDefinition(servers, globalProfiles); err != nil {
		return nil, err
	}
	// Team profiles may only reference servers the workspace already approves.
	for key, stored := range plan.profiles {
		if stored.team == "" {
			continue
		}
		for _, ref := range stored.profile.ServerRefs {
			if _, ok := servers[models.NormalizeMCPServerName(ref.ServerName)]; !ok {
				return nil, fmt.Errorf("MCP profile '%s' references unknown MCP server %q; define it under %s/", key, ref.ServerName, mcpServersGitOpsDirectory)
			}
		}
	}
	return plan, nil
}

func (p *gitOpsMCPRegistryPlan) globalServers() map[string]models.MCPServer {
	servers := map[string]models.MCPServer{}
	for name, stored := range p.servers {
		servers[name] = stored.server
	}
	return servers
}

func (p *gitOpsMCPRegistryPlan) globalProfiles() map[string]models.MCPProfile {
	profiles := map[string]models.MCPProfile{}
	for _, stored := range p.profiles {
		if stored.team == "" {
			profiles[stored.name] = stored.profile
		}
	}
	return profiles
}

func (p *gitOpsMCPRegistryPlan) teamProfiles() map[string]map[string]storedMCPProfile {
	byTeam := map[string]map[string]storedMCPProfile{}
	for _, stored := range p.profiles {
		if stored.team == "" {
			continue
		}
		if byTeam[stored.team] == nil {
			byTeam[stored.team] = map[string]storedMCPProfile{}
		}
		byTeam[stored.team][stored.name] = stored
	}
	return byTeam
}

func buildMCPServerGitOpsFile(name string, server models.MCPServer) mcpServerGitOpsFile {
	server = models.NormalizeMCPServer(server)
	server.Name = name
	return mcpServerGitOpsFile{MCPServer: server}
}

func buildMCPProfileGitOpsFile(name string, profile models.MCPProfile) mcpProfileGitOpsFile {
	profile = models.NormalizeMCPProfile(profile)
	profile.Name = name
	return mcpProfileGitOpsFile{MCPProfile: profile}
}
