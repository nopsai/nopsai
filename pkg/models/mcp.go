package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

const (
	MCPTransportStreamableHTTP = "streamable_http"
	MCPTransportHTTP           = "http"

	MCPAuthNone        = "none"
	MCPAuthBearerToken = "bearer_token"

	DefaultMCPTimeout = "30s"
)

type MCPServer struct {
	Name             string            `yaml:"name,omitempty" json:"name"`
	DisplayName      string            `yaml:"display_name,omitempty" json:"display_name,omitempty"`
	Enabled          bool              `yaml:"enabled" json:"enabled"`
	Provider         string            `yaml:"provider,omitempty" json:"provider,omitempty"`
	Transport        string            `yaml:"transport,omitempty" json:"transport,omitempty"`
	URL              string            `yaml:"url,omitempty" json:"url,omitempty"`
	AuthType         string            `yaml:"auth_type,omitempty" json:"auth_type,omitempty"`
	CredentialRef    string            `yaml:"credential_ref,omitempty" json:"credential_ref,omitempty"`
	LegacyAuthSecret string            `yaml:"auth_secret,omitempty" json:"-"`
	Headers          map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	Timeout          string            `yaml:"timeout,omitempty" json:"timeout,omitempty"`

	AllowedScopes []string `yaml:"allowed_scopes,omitempty" json:"allowed_scopes,omitempty"`

	LastTestStatus       string     `yaml:"-" json:"last_test_status,omitempty"`
	LastTestMessage      string     `yaml:"-" json:"last_test_message,omitempty"`
	LastTestedAt         *time.Time `yaml:"-" json:"last_tested_at,omitempty"`
	LastDiscoveredAt     *time.Time `yaml:"-" json:"last_discovered_at,omitempty"`
	DiscoveredServerName string     `yaml:"-" json:"discovered_server_name,omitempty"`
	DiscoveredVersion    string     `yaml:"-" json:"discovered_version,omitempty"`
	DiscoveredProtocol   string     `yaml:"-" json:"discovered_protocol,omitempty"`
}

type MCPTool struct {
	ServerName  string    `json:"server_name"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	InputSchema string    `json:"input_schema,omitempty"`
	SchemaHash  string    `json:"schema_hash,omitempty"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

type MCPProfile struct {
	Name          string                `yaml:"name,omitempty" json:"name"`
	Description   string                `yaml:"description,omitempty" json:"description,omitempty"`
	Enabled       bool                  `yaml:"enabled" json:"enabled"`
	ServerRefs    []MCPProfileServerRef `yaml:"servers,omitempty" json:"servers,omitempty"`
	AllowedScopes []string              `yaml:"allowed_scopes,omitempty" json:"allowed_scopes,omitempty"`
}

type MCPProfileServerRef struct {
	ServerName string   `yaml:"server" json:"server"`
	Tools      []string `yaml:"tools,omitempty" json:"tools,omitempty"`
}

func NormalizeMCPServerName(raw string) string {
	return strings.TrimSpace(raw)
}

func NormalizeMCPProfileName(raw string) string {
	return strings.TrimSpace(raw)
}

func NormalizeMCPServer(server MCPServer) MCPServer {
	server.Name = NormalizeMCPServerName(server.Name)
	server.DisplayName = strings.TrimSpace(server.DisplayName)
	server.Provider = strings.TrimSpace(server.Provider)
	server.Transport = normalizeMCPTransport(server.Transport)
	server.URL = strings.TrimSpace(server.URL)
	server.AuthType = normalizeMCPAuthType(server.AuthType)
	server.CredentialRef = strings.TrimSpace(server.CredentialRef)
	server.LegacyAuthSecret = strings.TrimSpace(server.LegacyAuthSecret)
	if strings.TrimSpace(server.Timeout) == "" {
		server.Timeout = DefaultMCPTimeout
	} else {
		server.Timeout = strings.TrimSpace(server.Timeout)
	}

	headers := make(map[string]string, len(server.Headers))
	for key, value := range server.Headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		headers[key] = strings.TrimSpace(value)
	}
	if len(headers) == 0 {
		server.Headers = nil
	} else {
		server.Headers = headers
	}
	server.AllowedScopes = NormalizeScopeList(server.AllowedScopes)
	return server
}

func NormalizeMCPServers(raw map[string]MCPServer) map[string]MCPServer {
	if len(raw) == 0 {
		return nil
	}
	servers := make(map[string]MCPServer, len(raw))
	for name, server := range raw {
		serverName := NormalizeMCPServerName(firstNonEmptyString(server.Name, name))
		if serverName == "" {
			continue
		}
		server.Name = serverName
		servers[serverName] = NormalizeMCPServer(server)
	}
	return servers
}

func NormalizeMCPProfile(profile MCPProfile) MCPProfile {
	profile.Name = NormalizeMCPProfileName(profile.Name)
	profile.Description = strings.TrimSpace(profile.Description)
	profile.AllowedScopes = NormalizeScopeList(profile.AllowedScopes)
	refs := make([]MCPProfileServerRef, 0, len(profile.ServerRefs))
	seenServerTools := map[string]map[string]bool{}
	for _, ref := range profile.ServerRefs {
		serverName := NormalizeMCPServerName(ref.ServerName)
		if serverName == "" {
			continue
		}
		if seenServerTools[serverName] == nil {
			seenServerTools[serverName] = map[string]bool{}
		}
		tools := make([]string, 0, len(ref.Tools))
		for _, tool := range ref.Tools {
			tool = strings.TrimSpace(tool)
			if tool == "" || seenServerTools[serverName][tool] {
				continue
			}
			seenServerTools[serverName][tool] = true
			tools = append(tools, tool)
		}
		sort.Strings(tools)
		refs = append(refs, MCPProfileServerRef{ServerName: serverName, Tools: tools})
	}
	sort.SliceStable(refs, func(i, j int) bool {
		return refs[i].ServerName < refs[j].ServerName
	})
	profile.ServerRefs = refs
	return profile
}

func NormalizeMCPProfiles(raw map[string]MCPProfile) map[string]MCPProfile {
	if len(raw) == 0 {
		return nil
	}
	profiles := make(map[string]MCPProfile, len(raw))
	for name, profile := range raw {
		profileName := NormalizeMCPProfileName(firstNonEmptyString(profile.Name, name))
		if profileName == "" {
			continue
		}
		profile.Name = profileName
		profiles[profileName] = NormalizeMCPProfile(profile)
	}
	return profiles
}

func NormalizeScopeList(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	seen := map[string]bool{}
	scopes := make([]string, 0, len(raw))
	for _, scope := range raw {
		normalized := strings.Trim(strings.TrimSpace(scope), "/")
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		scopes = append(scopes, normalized)
	}
	sort.Strings(scopes)
	return scopes
}

func MCPAllowedInScope(allowedScopes []string, scope string) bool {
	if len(allowedScopes) == 0 {
		return true
	}
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	for _, allowed := range allowedScopes {
		if strings.EqualFold(strings.Trim(strings.TrimSpace(allowed), "/"), scope) {
			return true
		}
	}
	return false
}

func MCPToolSchemaHash(schema string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(schema)))
	return hex.EncodeToString(sum[:])
}

func CanonicalJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return strings.TrimSpace(string(raw))
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return strings.TrimSpace(string(raw))
	}
	return string(encoded)
}

func normalizeMCPTransport(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "", "streamable-http", "streamable_http":
		return MCPTransportStreamableHTTP
	case "http", "https":
		return MCPTransportHTTP
	default:
		return normalized
	}
}

func normalizeMCPAuthType(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "", "none", "no_auth":
		return MCPAuthNone
	case "bearer", "bearer_token", "token":
		return MCPAuthBearerToken
	default:
		return normalized
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
