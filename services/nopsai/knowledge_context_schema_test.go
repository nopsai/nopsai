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
