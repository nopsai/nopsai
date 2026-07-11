package nopsai

import (
	"strings"
	"testing"
)

func TestKnowledgeContextSchemaMigratesLegacyTeamColumnsBeforeUse(t *testing.T) {
	joined := strings.Join(knowledgeContextSchemaStatements, "\n")
	required := []string{
		"ALTER TABLE knowledge_contexts RENAME COLUMN group_path TO team_path",
		"ALTER TABLE knowledge_contexts ADD COLUMN IF NOT EXISTS team_path TEXT NOT NULL DEFAULT ''",
		"UPDATE resource_visibility SET visibility = 'team' WHERE visibility = 'group'",
		"CASE visibility WHEN ''group'' THEN ''team'' ELSE visibility END",
		"ALTER TABLE knowledge_contexts DROP CONSTRAINT IF EXISTS knowledge_contexts_kind_group_path_name_key",
		"ADD CONSTRAINT knowledge_contexts_kind_team_path_name_key UNIQUE(kind, team_path, name)",
		"DROP INDEX IF EXISTS idx_knowledge_contexts_kind_group",
		"CREATE INDEX IF NOT EXISTS idx_knowledge_contexts_kind_team ON knowledge_contexts(kind, team_path, name)",
		"ALTER TABLE pipeline_run_knowledge_contexts RENAME COLUMN group_path TO team_path",
		"ALTER TABLE knowledge_context_legacy_metadata_backup RENAME COLUMN group_path TO team_path",
		"UPDATE knowledge_context_legacy_metadata_backup",
	}
	for _, statement := range required {
		if !strings.Contains(joined, statement) {
			t.Fatalf("knowledge context schema missing legacy team migration %q", statement)
		}
	}

	renameIndex := strings.Index(joined, "ALTER TABLE knowledge_contexts RENAME COLUMN group_path TO team_path")
	visibilityBackfillIndex := strings.Index(joined, "kind || ''/'' || CASE WHEN team_path")
	if renameIndex == -1 || visibilityBackfillIndex == -1 || renameIndex > visibilityBackfillIndex {
		t.Fatalf("team_path rename must run before visibility backfill; rename index %d backfill index %d", renameIndex, visibilityBackfillIndex)
	}
}
