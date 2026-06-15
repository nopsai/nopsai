package nopsai

import (
	"context"
	"fmt"

	"nopsai/pkg/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

const mcpRegistryRuntimeEnv = "NOPSAI_MCP_REGISTRY"

var mcpSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS mcp_servers (
		name TEXT PRIMARY KEY,
		display_name TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT TRUE,
		provider TEXT NOT NULL DEFAULT '',
		transport TEXT NOT NULL DEFAULT 'streamable_http',
		url TEXT NOT NULL DEFAULT '',
		auth_type TEXT NOT NULL DEFAULT 'none',
		credential_ref TEXT NOT NULL DEFAULT '',
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
	`ALTER TABLE mcp_servers ADD COLUMN IF NOT EXISTS credential_ref TEXT NOT NULL DEFAULT ''`,
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
