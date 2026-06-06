package mcpregistry

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"

	"gopkg.in/yaml.v3"
)

type RegistryRequest struct {
	Servers     []models.MCPServer           `json:"servers" yaml:"servers"`
	MCPServers  map[string]models.MCPServer  `json:"mcp_servers" yaml:"mcp_servers"`
	Profiles    []models.MCPProfile          `json:"profiles" yaml:"profiles"`
	MCPProfiles map[string]models.MCPProfile `json:"mcp_profiles" yaml:"mcp_profiles"`
}

type GitOpsDirectory struct {
	Root  string
	Files map[string]string
}

type GitOpsPlan struct {
	Servers    map[string]models.MCPServer
	Profiles   map[string]models.MCPProfile
	SourcePath string
}

type gitOpsFileCandidate struct {
	sourcePath string
	content    string
}

func ParseGitOpsPlan(binding models.ConfigRepository, directories ...GitOpsDirectory) (*GitOpsPlan, error) {
	candidates := []gitOpsFileCandidate{}
	for _, directory := range directories {
		root := filepath.ToSlash(strings.Trim(directory.Root, "/"))
		for path, content := range directory.Files {
			normalized := filepath.ToSlash(path)
			rel, ok := configsync.RelativePath(normalized, root)
			if !ok || !IsGitOpsRelativePath(rel) {
				continue
			}
			candidates = append(candidates, gitOpsFileCandidate{
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

	return ParseGitOpsFile(candidates[0].content, candidates[0].sourcePath)
}

func IsGitOpsRelativePath(rel string) bool {
	switch strings.Trim(filepath.ToSlash(rel), "/") {
	case "system/mcp.yaml", "system/mcp.yml",
		"system/mcp_registry.yaml", "system/mcp_registry.yml",
		"system/mcp_profiles.yaml", "system/mcp_profiles.yml":
		return true
	default:
		return false
	}
}

func ParseGitOpsFile(content, sourcePath string) (*GitOpsPlan, error) {
	var file RegistryRequest
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
	if err := ValidateRegistryDefinition(servers, profiles); err != nil {
		return nil, fmt.Errorf("invalid MCP registry in GitOps file '%s': %w", sourcePath, err)
	}

	return &GitOpsPlan{
		Servers:    servers,
		Profiles:   profiles,
		SourcePath: sourcePath,
	}, nil
}
