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

// parseGitOpsMCPRegistry reads the MCP registry from mcp/servers/<name>.yaml and
// mcp/profiles/<name>.yaml, where the file path is the resource name so a
// team-scoped name such as team-1/platform/github lives at
// mcp/servers/team-1/platform/github.yaml.
func parseGitOpsMCPRegistry(
	files map[string]string,
	serverRoot, profileRoot string,
	binding models.ConfigRepository,
) (*gitOpsMCPRegistryPlan, error) {
	plan := &gitOpsMCPRegistryPlan{servers: map[string]storedMCPServer{}, profiles: map[string]storedMCPProfile{}}

	systemRepo := binding.ScopeType == models.ConfigRepositoryScopeSystem
	if !systemRepo {
		if err := requireNoSystemRegistryFiles(files, serverRoot, mcpServersGitOpsDirectory); err != nil {
			return nil, err
		}
		if err := requireNoSystemRegistryFiles(files, profileRoot, mcpProfilesGitOpsDirectory); err != nil {
			return nil, err
		}
	}
	serverVisit := func(resource registryGitOpsResource, content string) error {
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
	}
	if systemRepo {
		if err := registryGitOpsFiles(files, serverRoot, "MCP server", serverVisit); err != nil {
			return nil, err
		}
	}

	profileVisit := func(resource registryGitOpsResource, content string) error {
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
		if existing, exists := plan.profiles[name]; exists {
			return fmt.Errorf("duplicate MCP profile '%s' defined by '%s' and '%s'", name, existing.sourcePath, resource.sourcePath)
		}
		plan.profiles[name] = storedMCPProfile{name: name, profile: profile, sourcePath: resource.sourcePath}
		return nil
	}
	if systemRepo {
		if err := registryGitOpsFiles(files, profileRoot, "MCP profile", profileVisit); err != nil {
			return nil, err
		}
	}
	if plan.empty() {
		return nil, nil
	}

	if err := mcpregistry.ValidateRegistryDefinition(plan.registryServers(), plan.registryProfiles()); err != nil {
		return nil, err
	}
	return plan, nil
}

func (p *gitOpsMCPRegistryPlan) registryServers() map[string]models.MCPServer {
	servers := map[string]models.MCPServer{}
	for name, stored := range p.servers {
		servers[name] = stored.server
	}
	return servers
}

func (p *gitOpsMCPRegistryPlan) registryProfiles() map[string]models.MCPProfile {
	profiles := map[string]models.MCPProfile{}
	for _, stored := range p.profiles {
		profiles[stored.name] = stored.profile
	}
	return profiles
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
