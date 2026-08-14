package nopsai

import (
	"database/sql"
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestParseGitOpsKnowledgeConnections(t *testing.T) {
	connections, err := parseGitOpsKnowledgeConnections(
		map[string]string{
			"knowledge/connections/platform/engineering-wiki.yaml": `
name: engineering-wiki
display_name: Engineering Wiki
provider: notion
base_url: https://www.notion.so/acme
credential_ref: credential://team/platform/knowledge/notion-token
config:
  workspace: acme
`,
			"knowledge/architecture/platform/service.md": "---\nname: service\nkind: architecture\ncontent: body\n---\n",
		},
		"knowledge",
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		"",
	)
	if err != nil {
		t.Fatalf("parseGitOpsKnowledgeConnections() error = %v", err)
	}
	if len(connections) != 1 {
		t.Fatalf("connections = %#v, want only the connection file", connections)
	}
	connection, ok := connections["platform/engineering-wiki"]
	if !ok {
		t.Fatalf("connections = %#v, want platform/engineering-wiki", connections)
	}
	if connection.provider != knowledgeConnectionProviderNotion {
		t.Fatalf("provider = %q, want notion", connection.provider)
	}
	if connection.credentialRef != "credential://team/platform/knowledge/notion-token" {
		t.Fatalf("credential ref = %q", connection.credentialRef)
	}
	if connection.config["workspace"] != "acme" {
		t.Fatalf("config = %#v, want the provider workspace", connection.config)
	}
	if connection.disabled {
		t.Fatal("connection should be enabled when disabled is omitted")
	}
}

func TestParseGitOpsKnowledgeConnectionsRejectsInvalidDefinitions(t *testing.T) {
	for name, tc := range map[string]struct {
		path    string
		content string
		want    string
	}{
		"unknown provider": {
			path:    "knowledge/connections/platform/wiki.yaml",
			content: "provider: sharepoint\n",
			want:    "provider must be notion, confluence, or wiki",
		},
		"name mismatch": {
			path:    "knowledge/connections/platform/wiki.yaml",
			content: "name: other\nprovider: notion\n",
			want:    "declares name",
		},
		"invalid credential reference": {
			path:    "knowledge/connections/platform/wiki.yaml",
			content: "provider: notion\ncredential_ref: not-a-reference\n",
			want:    "credential reference",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseGitOpsKnowledgeConnections(
				map[string]string{tc.path: tc.content},
				"knowledge",
				models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
				"",
			)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseGitOpsKnowledgeConnectionsNormalizesTeamRepositoryPaths(t *testing.T) {
	connections, err := parseGitOpsKnowledgeConnections(
		map[string]string{"knowledge/connections/team-wiki.yaml": "provider: confluence\n"},
		"knowledge",
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "platform"},
		"platform",
	)
	if err != nil {
		t.Fatalf("parseGitOpsKnowledgeConnections() error = %v", err)
	}
	if _, ok := connections["platform/team-wiki"]; !ok {
		t.Fatalf("connections = %#v, want the bound team prefix", connections)
	}
}

func TestKnowledgeConnectionGitOpsPathsStayOutOfKnowledgeDocuments(t *testing.T) {
	contexts, err := parseGitOpsKnowledgeContexts(
		map[string]string{
			"knowledge/connections/platform/engineering-wiki.yaml": "provider: notion\n",
			"knowledge/architecture/platform/service.md":           "---\nname: service\nkind: architecture\ncontent: body\n---\n",
		},
		"knowledge",
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		"",
		newAccessSyncPlan(),
	)
	if err != nil {
		t.Fatalf("parseGitOpsKnowledgeContexts() error = %v", err)
	}
	if len(contexts) != 1 {
		t.Fatalf("contexts = %#v, want only the architecture document", contexts)
	}
	if _, ok := contexts["architecture/platform/service"]; !ok {
		t.Fatalf("contexts = %#v, want the architecture document", contexts)
	}
}

func TestKnowledgeConnectionExportPathIsDriftTracked(t *testing.T) {
	repo := models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	path, ok := configRepositoryKnowledgeConnectionExportPath(repo, "platform", "engineering-wiki", "", false, sql.NullInt64{})
	if !ok {
		t.Fatal("expected an export path for a system repository")
	}
	if path != "knowledge/connections/platform/engineering-wiki.yaml" {
		t.Fatalf("export path = %q", path)
	}
	if !isConfigRepositoryDriftPath(path) {
		t.Fatalf("export path %q is not tracked by drift", path)
	}
}

func TestParseGitOpsKnowledgeContextsAcceptsExternalPageDefinitions(t *testing.T) {
	contexts, err := parseGitOpsKnowledgeContexts(
		map[string]string{
			"knowledge/runbook/platform/onboarding.md": `---
name: onboarding
kind: runbook
description: Team onboarding runbook mirrored from Notion.
source:
  type: external_page
  connection: engineering-wiki
  provider: notion
  page_id: 8a7f0c11
  page_url: https://www.notion.so/acme/onboarding-8a7f0c11
  page_title: Onboarding
  sync:
    mode: periodic
    interval_minutes: 120
    failure_mode: use_cached
---
`,
		},
		"knowledge",
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		"",
		newAccessSyncPlan(),
	)
	if err != nil {
		t.Fatalf("parseGitOpsKnowledgeContexts() error = %v", err)
	}
	stored, ok := contexts["runbook/platform/onboarding"]
	if !ok {
		t.Fatalf("contexts = %#v, want the external page document", contexts)
	}
	if stored.external == nil {
		t.Fatal("external page document should carry a source definition")
	}
	if stored.external.connection != "engineering-wiki" || stored.external.provider != knowledgeConnectionProviderNotion {
		t.Fatalf("source = %#v", stored.external)
	}
	if stored.external.pageID != "8a7f0c11" || stored.external.pageTitle != "Onboarding" {
		t.Fatalf("source page = %#v", stored.external)
	}
	if stored.external.syncMode != knowledgeSyncModePeriodic || stored.external.intervalMinutes != 120 {
		t.Fatalf("sync settings = %#v", stored.external)
	}
	if stored.external.failureMode != knowledgeFailureModeUseCached {
		t.Fatalf("failure mode = %q, want use_cached", stored.external.failureMode)
	}
	if stored.content != "" {
		t.Fatalf("content = %q, want the mirrored body to stay out of Git", stored.content)
	}
}

func TestParseGitOpsKnowledgeContextsRejectsIncompleteExternalSources(t *testing.T) {
	for name, tc := range map[string]struct {
		frontMatter string
		want        string
	}{
		"missing connection": {
			frontMatter: "source:\n  type: external_page\n  page_id: abc\n",
			want:        "source.connection is required",
		},
		"missing page": {
			frontMatter: "source:\n  type: external_page\n  connection: engineering-wiki\n",
			want:        "source.page_id or source.page_url is required",
		},
		"unknown type": {
			frontMatter: "source:\n  type: database\n  connection: engineering-wiki\n  page_id: abc\n",
			want:        "source.type must be inline or external_page",
		},
		"invalid sync mode": {
			frontMatter: "source:\n  type: external_page\n  connection: engineering-wiki\n  page_id: abc\n  sync:\n    mode: hourly\n",
			want:        "source.sync.mode",
		},
		"interval below minimum": {
			frontMatter: "source:\n  type: external_page\n  connection: engineering-wiki\n  page_id: abc\n  sync:\n    mode: periodic\n    interval_minutes: 1\n",
			want:        "source.sync.interval_minutes",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseGitOpsKnowledgeContexts(
				map[string]string{
					"knowledge/runbook/platform/onboarding.md": "---\nname: onboarding\nkind: runbook\n" + tc.frontMatter + "---\n",
				},
				"knowledge",
				models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
				"",
				newAccessSyncPlan(),
			)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseGitOpsKnowledgeContextsRejectsExternalPageWithInlineBody(t *testing.T) {
	_, err := parseGitOpsKnowledgeContexts(
		map[string]string{
			"knowledge/runbook/platform/onboarding.md": "---\nname: onboarding\nkind: runbook\nsource:\n  type: external_page\n  connection: engineering-wiki\n  page_id: abc\ncontent: |\n  Inline body that would fight the mirror.\n---\n",
		},
		"knowledge",
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		"",
		newAccessSyncPlan(),
	)
	if err == nil || !strings.Contains(err.Error(), "must not carry inline content") {
		t.Fatalf("error = %v, want an inline content rejection", err)
	}
}

func TestRenderKnowledgeContextGitOpsDocumentExportsExternalSource(t *testing.T) {
	document := renderKnowledgeContextGitOpsDocument(
		"runbook", "onboarding", "Team onboarding runbook.", "", "runbook/platform/onboarding", nil,
		&knowledgeSourceFrontMatter{
			Type:       knowledgeContentSourceExternalPage,
			Connection: "engineering-wiki",
			Provider:   knowledgeConnectionProviderNotion,
			PageID:     "8a7f0c11",
			PageURL:    "https://www.notion.so/acme/onboarding-8a7f0c11",
			PageTitle:  "Onboarding",
			Sync:       &knowledgeSyncFrontMatter{Mode: knowledgeSyncModePeriodic, IntervalMinutes: 120, FailureMode: knowledgeFailureModeUseCached},
		},
	)
	for _, want := range []string{
		"kind: runbook",
		"source:",
		"type: external_page",
		"connection: engineering-wiki",
		"page_id: 8a7f0c11",
		"mode: periodic",
		"interval_minutes: 120",
		"failure_mode: use_cached",
	} {
		if !strings.Contains(document, want) {
			t.Fatalf("exported document missing %q:\n%s", want, document)
		}
	}
	if strings.Contains(document, "content:") {
		t.Fatalf("exported external page document should not carry mirrored content:\n%s", document)
	}

	contexts, err := parseGitOpsKnowledgeContexts(
		map[string]string{"knowledge/runbook/platform/onboarding.md": document},
		"knowledge",
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		"",
		newAccessSyncPlan(),
	)
	if err != nil {
		t.Fatalf("exported document does not round-trip: %v", err)
	}
	stored, ok := contexts["runbook/platform/onboarding"]
	if !ok || stored.external == nil || stored.external.pageID != "8a7f0c11" {
		t.Fatalf("round-tripped document = %#v", contexts)
	}
}
