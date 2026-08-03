package nopsai

import (
	"strings"
	"testing"
)

func TestGitWebhookSourceSchemaIncludesTeamPathAndVisibility(t *testing.T) {
	joined := strings.Join(gitWebhookSourceSchemaStatements, "\n")
	for _, want := range []string{
		"team_path TEXT NOT NULL DEFAULT 'global'",
		"ALTER TABLE git_webhook_sources ADD COLUMN IF NOT EXISTS team_path",
		"UPDATE git_webhook_sources",
		"visibility TEXT NOT NULL DEFAULT 'team'",
		"ALTER TABLE git_webhook_sources ADD COLUMN IF NOT EXISTS visibility",
		"CREATE INDEX IF NOT EXISTS idx_git_webhook_sources_team",
		"CREATE INDEX IF NOT EXISTS idx_git_webhook_sources_visibility",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("git webhook source schema missing %q in:\n%s", want, joined)
		}
	}
}
