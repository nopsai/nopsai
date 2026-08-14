package nopsai

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"nopsai/pkg/models"

	"github.com/jackc/pgx/v5"
)

const knowledgeAccessDocument = `---
name: release-evidence
kind: policy
description: Mandatory evidence rules.
access:
  visibility: restricted
  use_access:
    grants:
      - repository: acme/test-app
      - team: data-team
content: |-
  # Release Evidence Policy
---
`

// Embedded access must be collected exactly once per resource. Parsing the
// knowledge directory twice inside one plan used to raise a duplicate grant
// error and fail the whole sync.
func TestParseConfigSyncPlanCollectsKnowledgeAccessOnce(t *testing.T) {
	binding := models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
		RepoURL:   "https://github.com/acme/config.git",
		Branch:    "main",
	}
	repoCtx, err := newConfigSyncRepositoryContext(binding)
	if err != nil {
		t.Fatal(err)
	}
	files := configSyncRepositoryFiles{
		knowledge:     map[string]string{"knowledge/policy/data-team/release-evidence.yaml": knowledgeAccessDocument},
		notifications: map[string]string{},
	}
	app := &App{}
	plan, err := app.parseConfigSyncPlan(binding, repoCtx, files)
	if err != nil {
		t.Fatalf("parseConfigSyncPlan() error = %v", err)
	}
	if _, ok := plan.knowledgeContexts["policy/data-team/release-evidence"]; !ok {
		t.Fatalf("knowledge contexts = %#v, want the policy document", plan.knowledgeContexts)
	}
	grants := 0
	for key := range plan.accessPlan.grants {
		if key.resourceType == grantResourceKnowledgeContext {
			grants++
			if key.resourceID != "policy/data-team/release-evidence" {
				t.Fatalf("grant resource id = %q", key.resourceID)
			}
		}
	}
	if grants != 2 {
		t.Fatalf("knowledge grants = %d, want the one repository and one team grant the document declares", grants)
	}
}

func TestSplitKnowledgeContextRouteIdentifierAcceptsDocumentIDs(t *testing.T) {
	kind, team, name, err := splitKnowledgeContextRouteIdentifier("team-2/hjhj")
	if err != nil {
		t.Fatalf("document id should resolve without a kind: %v", err)
	}
	if kind != "" || team != "team-2" || name != "hjhj" {
		t.Fatalf("split = %q/%q/%q, want an unresolved kind for team-2/hjhj", kind, team, name)
	}
	if _, _, _, err := splitKnowledgeContextIdentifier("team-2/hjhj"); err == nil || !strings.Contains(err.Error(), "unsupported knowledge context kind") {
		t.Fatalf("strict split error = %v, want the kind rejection that access resolution must not use", err)
	}
}

// The Knowledge page addresses documents by document id, so opening Access on a
// document must not be read as kind/name.
func TestResolveAccessGrantResourceAcceptsKnowledgeDocumentID(t *testing.T) {
	resource, err := resolveAccessGrantResource(
		t.Context(),
		&fixedKindQueryRunner{kinds: []string{"architecture"}},
		grantResourceKnowledgeContext,
		"team-2/hjhj",
		false,
	)
	if err != nil {
		t.Fatalf("resolveAccessGrantResource(document id) error = %v", err)
	}
	if resource.ID != "architecture/team-2/hjhj" {
		t.Fatalf("resource id = %q, want the canonical kind/team/name id", resource.ID)
	}
}

func TestResolveAccessGrantResourceReportsAmbiguousKnowledgeDocumentID(t *testing.T) {
	_, err := resolveAccessGrantResource(
		t.Context(),
		&fixedKindQueryRunner{kinds: []string{"architecture", "policy"}},
		grantResourceKnowledgeContext,
		"team-2/hjhj",
		false,
	)
	if err == nil || !strings.Contains(err.Error(), "use kind/team/name") {
		t.Fatalf("error = %v, want an ambiguous document id error", err)
	}
}

func TestResolveAccessGrantResourceKeepsQualifiedKnowledgeID(t *testing.T) {
	resource, err := resolveAccessGrantResource(t.Context(), &noopQueryRunner{}, grantResourceKnowledgeContext, "policy/data-team/release-evidence", false)
	if err != nil {
		t.Fatalf("resolveAccessGrantResource(qualified id) error = %v", err)
	}
	if resource.ID != "policy/data-team/release-evidence" {
		t.Fatalf("resource id = %q", resource.ID)
	}
}

type fixedKindQueryRunner struct {
	noopQueryRunner
	kinds []string
}

func (r *fixedKindQueryRunner) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return &kindRows{kinds: r.kinds}, nil
}

type kindRows struct {
	pgx.Rows
	kinds []string
	index int
}

func (r *kindRows) Next() bool {
	if r.index >= len(r.kinds) {
		return false
	}
	r.index++
	return true
}

func (r *kindRows) Scan(dest ...any) error {
	target, ok := dest[0].(*string)
	if !ok {
		return fmt.Errorf("unexpected scan target")
	}
	*target = r.kinds[r.index-1]
	return nil
}

func (r *kindRows) Err() error { return nil }
func (r *kindRows) Close()     {}
