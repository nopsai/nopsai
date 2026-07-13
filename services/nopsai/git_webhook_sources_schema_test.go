package nopsai

import (
	"strings"
	"testing"
)

func TestGitWebhookSourceSchemaIncludesTeamPath(t *testing.T) {
	joined := strings.Join(gitWebhookSourceSchemaStatements, "\n")
	for _, want := range []string{
		"team_path TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE git_webhook_sources ADD COLUMN IF NOT EXISTS team_path",
		"CREATE INDEX IF NOT EXISTS idx_git_webhook_sources_team",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("git webhook source schema missing %q in:\n%s", want, joined)
		}
	}
}
