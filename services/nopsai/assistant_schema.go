package nopsai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var assistantSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS assistant_conversations (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		user_id TEXT NOT NULL,
		title TEXT NOT NULL DEFAULT '',
		selected_llm_profile TEXT NOT NULL DEFAULT '',
		docs_version TEXT NOT NULL DEFAULT 'auto',
		scope TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_assistant_conversations_user_updated ON assistant_conversations(user_id, updated_at DESC)`,
	`CREATE TABLE IF NOT EXISTS assistant_messages (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		conversation_id UUID NOT NULL REFERENCES assistant_conversations(id) ON DELETE CASCADE,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		tool_calls JSONB NOT NULL DEFAULT '[]'::jsonb,
		content_tokens BIGINT NOT NULL DEFAULT 0,
		prompt_tokens BIGINT NOT NULL DEFAULT 0,
		completion_tokens BIGINT NOT NULL DEFAULT 0,
		total_tokens BIGINT NOT NULL DEFAULT 0,
		usage_estimated BOOLEAN NOT NULL DEFAULT FALSE,
		duration_ms BIGINT NOT NULL DEFAULT 0,
		llm_calls INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`ALTER TABLE assistant_messages ADD COLUMN IF NOT EXISTS content_tokens BIGINT NOT NULL DEFAULT 0`,
	`ALTER TABLE assistant_messages ADD COLUMN IF NOT EXISTS prompt_tokens BIGINT NOT NULL DEFAULT 0`,
	`ALTER TABLE assistant_messages ADD COLUMN IF NOT EXISTS completion_tokens BIGINT NOT NULL DEFAULT 0`,
	`ALTER TABLE assistant_messages ADD COLUMN IF NOT EXISTS total_tokens BIGINT NOT NULL DEFAULT 0`,
	`ALTER TABLE assistant_messages ADD COLUMN IF NOT EXISTS usage_estimated BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE assistant_messages ADD COLUMN IF NOT EXISTS duration_ms BIGINT NOT NULL DEFAULT 0`,
	`ALTER TABLE assistant_messages ADD COLUMN IF NOT EXISTS llm_calls INTEGER NOT NULL DEFAULT 0`,
	`CREATE INDEX IF NOT EXISTS idx_assistant_messages_conversation_created ON assistant_messages(conversation_id, created_at ASC)`,
	`CREATE TABLE IF NOT EXISTS assistant_conversation_memory (
		conversation_id UUID PRIMARY KEY REFERENCES assistant_conversations(id) ON DELETE CASCADE,
		summary TEXT NOT NULL DEFAULT '',
		entities JSONB NOT NULL DEFAULT '{}'::jsonb,
		open_tasks JSONB NOT NULL DEFAULT '[]'::jsonb,
		previous_proposed_fixes JSONB NOT NULL DEFAULT '[]'::jsonb,
		selected_run TEXT NOT NULL DEFAULT '',
		selected_pipeline TEXT NOT NULL DEFAULT '',
		selected_scope TEXT NOT NULL DEFAULT '',
		selected_docs_version TEXT NOT NULL DEFAULT '',
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`ALTER TABLE assistant_conversation_memory ADD COLUMN IF NOT EXISTS previous_proposed_fixes JSONB NOT NULL DEFAULT '[]'::jsonb`,
	`ALTER TABLE assistant_conversation_memory ADD COLUMN IF NOT EXISTS selected_run TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE assistant_conversation_memory ADD COLUMN IF NOT EXISTS selected_pipeline TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE assistant_conversation_memory ADD COLUMN IF NOT EXISTS selected_scope TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE assistant_conversation_memory ADD COLUMN IF NOT EXISTS selected_docs_version TEXT NOT NULL DEFAULT ''`,
	`CREATE TABLE IF NOT EXISTS hosted_mcp_audit_logs (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		user_id TEXT NOT NULL,
		conversation_id UUID,
		tool_name TEXT NOT NULL,
		input_summary TEXT NOT NULL DEFAULT '',
		output_summary TEXT NOT NULL DEFAULT '',
		resource_scope TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_hosted_mcp_audit_logs_user_created ON hosted_mcp_audit_logs(user_id, created_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_hosted_mcp_audit_logs_conversation ON hosted_mcp_audit_logs(conversation_id)`,
}

func ensureAssistantSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin assistant schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range assistantSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply assistant schema statement %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit assistant schema transaction: %w", err)
	}
	return nil
}
