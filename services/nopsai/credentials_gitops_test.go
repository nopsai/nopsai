package nopsai

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/google/uuid"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/credentials"
)

func TestParseGitOpsCredentialPlanMapsEncryptedCredentials(t *testing.T) {
	plan, err := parseGitOpsCredentialPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		gitOpsCredentialDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/credentials.yaml": `
credentials:
  - reference: credential://system/llm/openai
    kind: api_key
    description: OpenAI key
    active_version: 1
    versions:
      - version: 1
        encryption_format_version: 1
        encryption_key_id: key-1
        ciphertext: Y2lwaGVydGV4dA==
        wrapped_data_key: d3JhcHBlZA==
`,
			},
		},
	)
	if err != nil {
		t.Fatalf("parseGitOpsCredentialPlan() error = %v", err)
	}
	if plan == nil || len(plan.credentials) != 1 {
		t.Fatalf("plan credentials = %#v, want one credential", plan)
	}
	item := plan.credentials[0]
	if item.Reference != "credential://system/llm/openai" || item.Kind != "api_key" || item.Status != credentials.StatusActive {
		t.Fatalf("credential = %#v, want normalized active OpenAI key", item)
	}
	if item.Versions[0].Ciphertext != "Y2lwaGVydGV4dA==" || item.Versions[0].WrappedDataKey != "d3JhcHBlZA==" {
		t.Fatalf("encrypted version = %#v, want preserved base64 envelope", item.Versions[0])
	}
}

func TestParseGitOpsCredentialPlanRejectsInvalidEncryptedEnvelope(t *testing.T) {
	_, err := parseGitOpsCredentialFile(`
credentials:
  - reference: credential://system/llm/openai
    kind: api_key
    active_version: 1
    versions:
      - version: 1
        encryption_format_version: 1
        encryption_key_id: key-1
        ciphertext: "not base64"
        wrapped_data_key: d3JhcHBlZA==
`, "setting/system/credentials.yaml")
	if err == nil || !strings.Contains(err.Error(), "invalid ciphertext") {
		t.Fatalf("expected invalid ciphertext error, got %v", err)
	}
}

func TestBuildGitOpsCredentialDocumentExportsEncryptedVersions(t *testing.T) {
	credentialID := uuid.MustParse("00000000-0000-0000-0000-000000000123")
	doc := buildGitOpsCredentialDocument(
		[]credentials.Credential{
			{
				ID:            credentialID,
				Reference:     credentials.Reference{Namespace: "system", Name: "github/app-private-key"},
				Kind:          "private_key",
				Description:   "GitHub App private key",
				Status:        credentials.StatusActive,
				ActiveVersion: 2,
			},
		},
		map[uuid.UUID][]credentials.Version{
			credentialID: {
				{
					CredentialID: credentialID,
					Version:      2,
					Envelope: credentials.Envelope{
						Ciphertext:              []byte("ciphertext"),
						WrappedDataKey:          []byte("wrapped"),
						EncryptionKeyID:         "key-1",
						EncryptionFormatVersion: 1,
					},
				},
			},
		},
	)
	if len(doc.Credentials) != 1 || len(doc.Credentials[0].Versions) != 1 {
		t.Fatalf("doc = %#v, want one exported credential version", doc)
	}
	got := doc.Credentials[0].Versions[0]
	if got.Ciphertext != base64.StdEncoding.EncodeToString([]byte("ciphertext")) {
		t.Fatalf("ciphertext = %q, want base64 ciphertext", got.Ciphertext)
	}
	if got.WrappedDataKey != base64.StdEncoding.EncodeToString([]byte("wrapped")) {
		t.Fatalf("wrapped_data_key = %q, want base64 wrapped data key", got.WrappedDataKey)
	}
}

func TestParseGitOpsCredentialPlanRequiresSystemRepository(t *testing.T) {
	_, err := parseGitOpsCredentialPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"},
		gitOpsCredentialDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/credentials.yaml": "credentials: []",
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "system config repository") {
		t.Fatalf("expected system repository error, got %v", err)
	}
}
