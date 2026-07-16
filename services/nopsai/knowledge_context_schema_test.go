package nopsai

import (
	"strings"
	"testing"
)

func TestKnowledgeContextSchemaUsesTeamPath(t *testing.T) {
	joined := strings.Join(knowledgeContextSchemaStatements, "\n")
	required := []string{
		"ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS team_path TEXT NOT NULL DEFAULT ''",
		"ADD CONSTRAINT knowledge_contexts_kind_team_path_name_key UNIQUE(kind, team_path, name)",
		"CREATE INDEX IF NOT EXISTS idx_knowledge_contexts_kind_team ON knowledge_contexts(kind, team_path, name)",
		"ALTER TABLE pipeline_run_knowledge_contexts ADD COLUMN IF NOT EXISTS team_path TEXT NOT NULL DEFAULT ''",
		"CREATE TABLE IF NOT EXISTS knowledge_context_connections",
		"UNIQUE(team_path, name)",
		"ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS connection_id UUID REFERENCES knowledge_context_connections(id) ON DELETE SET NULL",
		"ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS external_page_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS last_sync_started_at TIMESTAMPTZ",
		"ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS next_sync_attempt_at TIMESTAMPTZ",
		"ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS sync_attempt_count INTEGER NOT NULL DEFAULT 0",
		"CREATE INDEX IF NOT EXISTS idx_knowledge_context_connections_team ON knowledge_context_connections(team_path, name)",
		"CREATE INDEX IF NOT EXISTS idx_knowledge_contexts_connection_id ON knowledge_contexts(connection_id)",
		"CREATE INDEX IF NOT EXISTS idx_knowledge_contexts_periodic_sync_due",
		"CREATE TABLE IF NOT EXISTS knowledge_context_assets",
		"REFERENCES knowledge_contexts(id) ON DELETE CASCADE",
		"CREATE INDEX IF NOT EXISTS idx_knowledge_context_assets_context",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("knowledge context schema missing team statement %q", statement)
		}
	}

	createIndex := strings.Index(joined, "CREATE TABLE IF NOT EXISTS knowledge_contexts")
	visibilityBackfillIndex := strings.Index(joined, "kind || ''/'' || CASE WHEN team_path")
	if createIndex == -1 || visibilityBackfillIndex == -1 || createIndex > visibilityBackfillIndex {
		t.Fatalf("knowledge context creation must run before visibility backfill; create index %d backfill index %d", createIndex, visibilityBackfillIndex)
	}
	assertTeamOnlySchemaVocabulary(t, joined)
}
