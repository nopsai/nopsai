package mcpregistry

import "nopsai/pkg/models"

type RegistryRequest struct {
	Servers     []models.MCPServer           `json:"servers" yaml:"servers"`
	MCPServers  map[string]models.MCPServer  `json:"mcp_servers" yaml:"mcp_servers"`
	Profiles    []models.MCPProfile          `json:"profiles" yaml:"profiles"`
	MCPProfiles map[string]models.MCPProfile `json:"mcp_profiles" yaml:"mcp_profiles"`
}
