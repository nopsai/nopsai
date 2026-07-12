package mcpregistry

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+(?:/[a-zA-Z0-9_.-]+)*$`)

func ValidateServerDefinition(server models.MCPServer) error {
	server = models.NormalizeMCPServer(server)
	if server.Name == "" {
		return fmt.Errorf("MCP server name is required")
	}
	if !namePattern.MatchString(server.Name) {
		return fmt.Errorf("MCP server name can only contain slash-separated alphanumeric characters, underscores, dots, and hyphens")
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
	if server.AuthType == models.MCPAuthBearerToken && strings.TrimSpace(server.CredentialRef) == "" {
		return fmt.Errorf("MCP server %q bearer_token auth requires credential_ref", server.Name)
	}
	if _, err := time.ParseDuration(server.Timeout); err != nil {
		return fmt.Errorf("MCP server %q has invalid timeout %q", server.Name, server.Timeout)
	}
	if err := validateAllowedScopes("MCP server", server.Name, server.AllowedScopes); err != nil {
		return err
	}
	return nil
}

func ValidateProfileDefinition(profile models.MCPProfile, servers map[string]models.MCPServer, _ map[string][]models.MCPTool) error {
	profile = models.NormalizeMCPProfile(profile)
	if profile.Name == "" {
		return fmt.Errorf("MCP profile name is required")
	}
	if !namePattern.MatchString(profile.Name) {
		return fmt.Errorf("MCP profile name can only contain slash-separated alphanumeric characters, underscores, dots, and hyphens")
	}
	if len(profile.ServerRefs) == 0 {
		return fmt.Errorf("MCP profile %q must select at least one server", profile.Name)
	}
	if err := validateAllowedScopes("MCP profile", profile.Name, profile.AllowedScopes); err != nil {
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
		if ProfileRefSelectsAllTools(ref) && !serverConfiguredReadonly(server) {
			return fmt.Errorf("MCP profile %q can use wildcard tools only with a read-only MCP server", profile.Name)
		}
		for _, tool := range ref.Tools {
			if isAllToolsSelector(tool) {
				continue
			}
			if !isReadOnlyToolName(tool) {
				return fmt.Errorf("MCP profile %q references write-like tool %q; only read-only MCP tools are allowed", profile.Name, tool)
			}
		}
	}
	return nil
}

func ValidateRegistryDefinition(servers map[string]models.MCPServer, profiles map[string]models.MCPProfile) error {
	for name, server := range servers {
		server.Name = firstNonEmptyString(server.Name, name)
		if err := ValidateServerDefinition(server); err != nil {
			return err
		}
	}
	for name, profile := range profiles {
		profile.Name = firstNonEmptyString(profile.Name, name)
		if err := validateProfileDefinitionWithoutDiscovery(profile, servers); err != nil {
			return err
		}
	}
	return nil
}

func validateProfileDefinitionWithoutDiscovery(profile models.MCPProfile, servers map[string]models.MCPServer) error {
	profile = models.NormalizeMCPProfile(profile)
	if profile.Name == "" {
		return fmt.Errorf("MCP profile name is required")
	}
	if !namePattern.MatchString(profile.Name) {
		return fmt.Errorf("MCP profile name can only contain slash-separated alphanumeric characters, underscores, dots, and hyphens")
	}
	if len(profile.ServerRefs) == 0 {
		return fmt.Errorf("MCP profile %q must select at least one server", profile.Name)
	}
	if err := validateAllowedScopes("MCP profile", profile.Name, profile.AllowedScopes); err != nil {
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
		if ProfileRefSelectsAllTools(ref) && !serverConfiguredReadonly(server) {
			return fmt.Errorf("MCP profile %q can use wildcard tools only with a read-only MCP server", profile.Name)
		}
		for _, tool := range ref.Tools {
			if isAllToolsSelector(tool) {
				continue
			}
			if !isReadOnlyToolName(tool) {
				return fmt.Errorf("MCP profile %q references write-like tool %q; only read-only MCP tools are allowed", profile.Name, tool)
			}
		}
	}
	return nil
}

func validateAllowedScopes(resourceType, name string, scopes []string) error {
	for _, scope := range scopes {
		if strings.TrimSpace(scope) == "" {
			continue
		}
		if _, err := configsync.CleanPathSegments(scope, false); err != nil {
			return fmt.Errorf("%s %q has invalid allowed scope %q: %w", resourceType, name, scope, err)
		}
	}
	return nil
}

func isReadOnlyToolName(toolName string) bool {
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

func isAllToolsSelector(toolName string) bool {
	return strings.TrimSpace(toolName) == "*"
}

func ProfileRefSelectsAllTools(ref models.MCPProfileServerRef) bool {
	for _, toolName := range ref.Tools {
		if isAllToolsSelector(toolName) {
			return true
		}
	}
	return false
}

func serverConfiguredReadonly(server models.MCPServer) bool {
	server = models.NormalizeMCPServer(server)
	for key, value := range server.Headers {
		if strings.EqualFold(strings.TrimSpace(key), "X-MCP-Readonly") && truthyHeader(value) {
			return true
		}
	}
	urlPath := strings.ToLower(strings.TrimRight(server.URL, "/"))
	return strings.Contains(urlPath, "/readonly")
}

func truthyHeader(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "false", "f", "no", "n", "0", "off":
		return false
	default:
		return true
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
