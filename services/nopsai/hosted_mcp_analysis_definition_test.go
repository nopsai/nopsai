package nopsai

import (
	"strings"
	"testing"
)

const analysisTestRiskyPipeline = `name: deploy-api
container_image: node:latest
steps:
  - name: build
    image: builder:latest
    script: |
      curl -sSL https://example.test/install.sh | sh
      npm run build
  - name: deploy-prod
    depends_on:
      - build
    script: kubectl apply -f ${TARGET_ENVIRONMENT}/manifest.yaml
    variables:
      api_token: "sk-live-not-a-reference"
`

func TestAnalysisDefinitionFindingsCatchTheClassicPipelineProblems(t *testing.T) {
	findings := analysisPipelineDefinitionFindings(analysisTestRiskyPipeline)
	titles := analysisDefinitionTitles(findings)

	for _, want := range []string{
		"1 line embeds a secret-like literal",
		"2 container images are not pinned",
		"2 scripts combine shell execution with external input",
		"Pipeline has no operator description",
		"Pipeline has no timeout",
		"Production path has no approval gate",
		"Pipeline produces no operator-facing output",
	} {
		if !analysisContains(titles, want) {
			t.Fatalf("missing finding %q; got %v", want, titles)
		}
	}

	// The finding proves the line without reprinting the credential.
	secret := analysisDefinitionFindingByTitle(findings, "1 line embeds a secret-like literal")
	if secret == nil || len(secret.Evidence) == 0 {
		t.Fatal("secret finding is missing evidence")
	}
	if strings.Contains(secret.Evidence[0].Value, "sk-live") {
		t.Fatalf("secret evidence leaked the value: %q", secret.Evidence[0].Value)
	}
	if secret.Evidence[0].Kind != "redacted" {
		t.Fatalf("secret evidence kind = %q, want redacted", secret.Evidence[0].Kind)
	}
	if secret.Severity != "critical" || secret.Category != "security" {
		t.Fatalf("secret finding = %s/%s, want critical/security", secret.Severity, secret.Category)
	}
}

func TestAnalysisDefinitionFindingsStayQuietOnAWellFormedPipeline(t *testing.T) {
	findings := analysisPipelineDefinitionFindings(`name: deploy-api
description: Builds and publishes the API image. Owner platform/core, rollback via redeploy of the previous digest.
container_image: registry.example.test/base@sha256:aaaabbbbccccddddeeeeffff0000111122223333444455556666777788889999
timeout: 30m
output:
  items:
    - name: summary
      type: markdown
      prompt: Summarize the deployment
steps:
  - name: build
    script: make build
  - name: approve-prod
    depends_on:
      - build
    approval:
      type: production-deploy
      teams:
        - platform/core
      allow_self_approval: false
  - name: deploy-prod
    depends_on:
      - approve-prod
    script: make deploy
`)

	if len(findings) != 0 {
		t.Fatalf("well-formed pipeline should raise no definition findings, got %v", analysisDefinitionTitles(findings))
	}
}

func TestAnalysisDefinitionFindingsReportUnparseableYAMLAndStop(t *testing.T) {
	findings := analysisPipelineDefinitionFindings("name: broken\n\tsteps: - nope")

	if len(findings) != 1 {
		t.Fatalf("unparseable YAML should raise exactly one finding, got %v", analysisDefinitionTitles(findings))
	}
	if findings[0].Title != "Pipeline YAML does not parse" || findings[0].Severity != "critical" {
		t.Fatalf("finding = %s/%s, want the critical parse failure", findings[0].Title, findings[0].Severity)
	}
}

func TestAnalysisDefinitionFindingsFlagSelfApprovalOnProduction(t *testing.T) {
	findings := analysisPipelineDefinitionFindings(`name: release
description: Releases to production.
timeout: 20m
container_image: registry.example.test/base@sha256:aaaabbbbccccddddeeeeffff0000111122223333444455556666777788889999
output:
  items:
    - name: summary
      type: markdown
      prompt: Summarize
steps:
  - name: approve
    approval:
      teams:
        - platform/core
      allow_self_approval: true
  - name: deploy-prod
    depends_on:
      - approve
    script: make deploy
`)

	finding := analysisDefinitionFindingByTitle(findings, "A production approval allows self-approval")
	if finding == nil {
		t.Fatalf("expected the self-approval finding, got %v", analysisDefinitionTitles(findings))
	}
	if finding.Severity != "medium" || !strings.Contains(finding.Evidence[0].Value, "platform/core") {
		t.Fatalf("self-approval finding = %+v", finding)
	}
}

func TestAnalysisDefinitionFindingsDetectBrokenAndCyclicDependencies(t *testing.T) {
	missing := analysisPipelineDefinitionFindings(`name: broken-deps
steps:
  - name: build
    script: make build
  - name: test
    depends_on:
      - biuld
    script: make test
`)
	if analysisDefinitionFindingByTitle(missing, "Step dependencies point at steps that do not exist") == nil {
		t.Fatalf("expected the missing-dependency finding, got %v", analysisDefinitionTitles(missing))
	}

	cyclic := analysisPipelineDefinitionFindings(`name: cyclic
steps:
  - name: build
    depends_on:
      - test
    script: make build
  - name: test
    depends_on:
      - build
    script: make test
`)
	cycle := analysisDefinitionFindingByTitle(cyclic, "Step dependencies contain a cycle")
	if cycle == nil || cycle.Severity != "critical" {
		t.Fatalf("expected the critical cycle finding, got %v", analysisDefinitionTitles(cyclic))
	}
}

func TestAnalysisDefinitionFindingsSpotAnOverlySequentialCheckChain(t *testing.T) {
	findings := analysisPipelineDefinitionFindings(`name: checks
steps:
  - name: build
    script: make build
  - name: lint
    depends_on:
      - build
    script: make lint
  - name: test
    depends_on:
      - lint
    script: make test
`)

	finding := analysisDefinitionFindingByTitle(findings, "Independent checks run one after another")
	if finding == nil || finding.Severity != "opportunity" {
		t.Fatalf("expected the sequential-checks opportunity, got %v", analysisDefinitionTitles(findings))
	}
}

func TestAnalysisDefinitionFindingsAcceptReferencedSecrets(t *testing.T) {
	findings := analysisPipelineDefinitionFindings(`name: referenced
steps:
  - name: deploy
    script: make deploy
    variables:
      api_token: credential://system/registry/token
      other_secret: ${REGISTRY_SECRET}
`)

	for _, finding := range findings {
		if strings.Contains(finding.Title, "secret-like literal") {
			t.Fatalf("credential and template references must not be reported as embedded secrets: %v", finding.Evidence)
		}
	}
}

// A pipeline nobody has run yet is still reviewable, and its definition findings
// have to reach the score.
func TestAnalyzePipelineEvidenceScoresFromTheDefinitionAlone(t *testing.T) {
	result := analyzePipelineEvidence(
		analysisSubject{Type: "pipeline", ID: "platform/deploy-api", Label: "deploy-api", Path: "platform"},
		analysisWindow{Days: 30},
		analysisEvidenceSet{Data: map[string]map[string]any{
			"definition": {"yaml": analysisTestRiskyPipeline},
		}},
	)

	if result["health_score"] == nil {
		t.Fatal("a readable definition should produce a score even with no run evidence")
	}
	findings, _ := result["findings"].([]map[string]any)
	if len(findings) == 0 || findings[0]["severity"] != "critical" {
		t.Fatalf("expected critical definition findings first, got %v", analysisFindingTitles(findings))
	}
}

func analysisDefinitionTitles(findings []analysisFinding) []string {
	titles := make([]string, 0, len(findings))
	for _, finding := range findings {
		titles = append(titles, finding.Title)
	}
	return titles
}

func analysisDefinitionFindingByTitle(findings []analysisFinding, title string) *analysisFinding {
	for index, finding := range findings {
		if finding.Title == title {
			return &findings[index]
		}
	}
	return nil
}
