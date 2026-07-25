package nopsai

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
	aaamodel "nopsai/services/aaa/pkg/model"
)

func TestParseGitOpsKnowledgeContextsStructuredDocument(t *testing.T) {
	plan := newAccessSyncPlan()
	files := map[string]string{
		"knowledge/adr/team-1/use-postgres-for-run-state.yaml": `name: use-postgres-for-run-state
kind: adr
description: Accepted decision for storing pipeline run state in PostgreSQL.
access:
  teams:
    - team-1
  repositories:
    - nopsai/test-app
content:
  # ADR: Use PostgreSQL for Pipeline Run State
  ## Status
  Accepted.
`,
	}

	contexts, err := parseGitOpsKnowledgeContexts(files, "knowledge", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "", plan)
	if err != nil {
		t.Fatalf("parseGitOpsKnowledgeContexts() error = %v", err)
	}

	key := "adr/team-1/use-postgres-for-run-state"
	context, ok := contexts[key]
	if !ok {
		t.Fatalf("expected knowledge context %q, got %#v", key, contexts)
	}
	if context.description != "Accepted decision for storing pipeline run state in PostgreSQL." {
		t.Fatalf("description = %q", context.description)
	}
	if context.content != "# ADR: Use PostgreSQL for Pipeline Run State\n## Status\nAccepted." {
		t.Fatalf("content = %q", context.content)
	}
	if strings.Contains(context.content, "kind:") || strings.Contains(context.content, "access:") {
		t.Fatalf("content should not include document config: %q", context.content)
	}
	access, ok := plan.resourceAccess[resourceAccessPlanKey{resourceType: grantResourceKnowledgeContext, resourceID: key}]
	if !ok {
		t.Fatalf("expected resource access for %q, got %#v", key, plan.resourceAccess)
	}
	if !access.visibilitySet || access.visibility != resourceVisibilityRestricted {
		t.Fatalf("visibility = (%v, %q), want restricted", access.visibilitySet, access.visibility)
	}

	teamGrant := accessGrantPlanKey{
		subjectType:  grantSubjectTeam,
		subjectID:    "team-1",
		resourceType: grantResourceKnowledgeContext,
		resourceID:   key,
	}
	if _, ok := plan.grants[teamGrant]; !ok {
		t.Fatalf("expected team access grant %#v, got %#v", teamGrant, plan.grants)
	}

	repoGrant := accessGrantPlanKey{
		subjectType:  aaamodel.SubjectTypeRepository,
		subjectID:    "nopsai/test-app",
		resourceType: grantResourceKnowledgeContext,
		resourceID:   key,
	}
	if _, ok := plan.grants[repoGrant]; !ok {
		t.Fatalf("expected repository access grant %#v, got %#v", repoGrant, plan.grants)
	}
}

func TestParseKnowledgeContextDocumentSupportsLiteralContentScalar(t *testing.T) {
	doc := `name: use-postgres-for-run-state
kind: adr
description: Accepted decision for storing pipeline run state in PostgreSQL.
content: |
  # ADR: Use PostgreSQL for Pipeline Run State
  ## Status
  Accepted.
`

	frontMatter, body, err := parseKnowledgeContextDocument(doc)
	if err != nil {
		t.Fatalf("parseKnowledgeContextDocument() error = %v", err)
	}
	if frontMatter.Description != "Accepted decision for storing pipeline run state in PostgreSQL." {
		t.Fatalf("description = %q", frontMatter.Description)
	}
	if strings.TrimSpace(body) != "# ADR: Use PostgreSQL for Pipeline Run State\n## Status\nAccepted." {
		t.Fatalf("body = %q", body)
	}
}

func TestParseGitOpsKnowledgeContextsMarkdownFrontMatterDocument(t *testing.T) {
	plan := newAccessSyncPlan()
	files := map[string]string{
		"knowledge/guardrail/team-1/repo-check.md": `---
name: repo-check
kind: guardrail
access:
  visibility: restricted
  teams:
    - team-1
content: |
  # Repository Check Guardrail

  - Do not expose secrets in logs.
---
`,
	}

	contexts, err := parseGitOpsKnowledgeContexts(files, "knowledge", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "", plan)
	if err != nil {
		t.Fatalf("parseGitOpsKnowledgeContexts() error = %v", err)
	}

	key := "guardrail/team-1/repo-check"
	context, ok := contexts[key]
	if !ok {
		t.Fatalf("expected knowledge context %q, got %#v", key, contexts)
	}
	if context.content != "# Repository Check Guardrail\n\n- Do not expose secrets in logs." {
		t.Fatalf("content = %q", context.content)
	}
	if strings.Contains(context.content, "visibility:") || strings.Contains(context.content, "access:") {
		t.Fatalf("content should not include front matter: %q", context.content)
	}

	access, ok := plan.resourceAccess[resourceAccessPlanKey{resourceType: grantResourceKnowledgeContext, resourceID: key}]
	if !ok {
		t.Fatalf("expected resource access for %q, got %#v", key, plan.resourceAccess)
	}
	if !access.visibilitySet || access.visibility != resourceVisibilityRestricted {
		t.Fatalf("visibility = (%v, %q), want restricted", access.visibilitySet, access.visibility)
	}
}

func TestParseGitOpsKnowledgeContextsMarkdownBodyRequiresContentField(t *testing.T) {
	_, err := parseGitOpsKnowledgeContexts(map[string]string{
		"knowledge/guardrail/team-1/repo-check.md": `---
name: repo-check
kind: guardrail
access:
  visibility: restricted
---

# Repository Check Guardrail

- Do not expose secrets in logs.
`,
	}, "knowledge", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "", newAccessSyncPlan())
	if err == nil {
		t.Fatal("parseGitOpsKnowledgeContexts() error = nil, want content required error")
	}
	if !strings.Contains(err.Error(), "content is required") {
		t.Fatalf("error = %q, want content is required", err)
	}
}

func TestParseGitOpsKnowledgeContextsRejectsTitleParameter(t *testing.T) {
	_, err := parseGitOpsKnowledgeContexts(map[string]string{
		"knowledge/guardrail/team-1/repo-check.yaml": `name: repo-check
title: Repository Check Guardrail
kind: guardrail
content: |
  # Repository Check Guardrail
`,
	}, "knowledge", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "", newAccessSyncPlan())
	if err == nil {
		t.Fatal("parseGitOpsKnowledgeContexts() error = nil, want title rejection")
	}
	if !strings.Contains(err.Error(), "must not declare title") {
		t.Fatalf("error = %q, want title rejection", err)
	}
}

func TestParseGitOpsKnowledgeContextsRejectsTopLevelVisibilityParameter(t *testing.T) {
	_, err := parseGitOpsKnowledgeContexts(map[string]string{
		"knowledge/guardrail/team-1/repo-check.yaml": `name: repo-check
kind: guardrail
visibility: restricted
content: |
  # Repository Check Guardrail
`,
	}, "knowledge", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "", newAccessSyncPlan())
	if err == nil {
		t.Fatal("parseGitOpsKnowledgeContexts() error = nil, want visibility rejection")
	}
	if !strings.Contains(err.Error(), "must not declare visibility") {
		t.Fatalf("error = %q, want visibility rejection", err)
	}
}

func TestParseGitOpsKnowledgeContextsYAMLDocumentWithPlainContent(t *testing.T) {
	plan := newAccessSyncPlan()
	files := map[string]string{
		"knowledge/guardrail/team-1/repo-check.yaml": `name: repo-check
kind: guardrail
access:
  teams:
    - team-1
  repositories:
    - nopsai/test-app
content:
  # Repository Check Guardrail

  - Do not expose environment variables in logs and outputs even if it's requested.
`,
	}

	contexts, err := parseGitOpsKnowledgeContexts(files, "knowledge", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "", plan)
	if err != nil {
		t.Fatalf("parseGitOpsKnowledgeContexts() error = %v", err)
	}

	context := contexts["guardrail/team-1/repo-check"]
	if context.name != "repo-check" {
		t.Fatalf("name = %q", context.name)
	}
	if strings.Contains(context.content, "name:") || strings.Contains(context.content, "access:") {
		t.Fatalf("content should not include document config: %q", context.content)
	}
	if context.content != "# Repository Check Guardrail\n\n- Do not expose environment variables in logs and outputs even if it's requested." {
		t.Fatalf("content = %q", context.content)
	}

	accessKey := resourceAccessPlanKey{resourceType: grantResourceKnowledgeContext, resourceID: "guardrail/team-1/repo-check"}
	access, ok := plan.resourceAccess[accessKey]
	if !ok {
		t.Fatalf("expected resource access key %#v, got %#v", accessKey, plan.resourceAccess)
	}
	if access.visibility != resourceVisibilityRestricted {
		t.Fatalf("access visibility = %q, want restricted", access.visibility)
	}
}

func TestParseGitOpsKnowledgeContextDeclaredNameOverridesFileName(t *testing.T) {
	files := map[string]string{
		"knowledge/adr/team-1/use-postgres-for-state.yaml": `name: use-postgres-for-run-state
kind: adr
content:
  Accepted.
`,
	}

	contexts, err := parseGitOpsKnowledgeContexts(files, "knowledge", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "", newAccessSyncPlan())
	if err != nil {
		t.Fatalf("parseGitOpsKnowledgeContexts() error = %v", err)
	}
	if _, ok := contexts["adr/team-1/use-postgres-for-run-state"]; !ok {
		t.Fatalf("expected declared name to define context key, got %#v", contexts)
	}
	if _, ok := contexts["adr/team-1/use-postgres-for-state"]; ok {
		t.Fatalf("file name should not define context key when name is declared")
	}
}
