package nopsai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidatePipelineEndpointReturnsStructuredErrors(t *testing.T) {
	result := postValidationJSON(t, (&App{}).handleValidatePipeline, map[string]any{
		"yaml": `
name: missing-output-dependency
container_image: alpine:3.20
steps:
  - name: prepare
    tasks:
      - name: generate
        script: echo ok
        outputs:
          - name: image_tag
  - name: build
    tasks:
      - name: image
        variables:
          image_tag: $steps.prepare.generate.outputs.image_tag
        script: echo build
`,
	})

	if result.Valid {
		t.Fatalf("valid = true, want false")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("errors = %#v, want one structured error", result.Errors)
	}
	if result.Errors[0].Code != "runtime_output_missing_dependency" {
		t.Fatalf("error code = %q, want runtime_output_missing_dependency; error = %#v", result.Errors[0].Code, result.Errors[0])
	}
	if result.Errors[0].Line == 0 {
		t.Fatalf("error line = 0, want a best-effort source line; error = %#v", result.Errors[0])
	}
}

func TestValidatePipelineEndpointAcceptsValidPipeline(t *testing.T) {
	result := postValidationJSON(t, (&App{}).handleValidatePipeline, map[string]any{
		"resource_id": "deploy",
		"yaml": `
name: deploy
container_image: alpine:3.20
steps:
  - name: run
    script: echo ok
`,
	})

	if !result.Valid || len(result.Errors) != 0 {
		t.Fatalf("result = %#v, want valid response without errors", result)
	}
}

func TestValidateReusableStepEndpointChecksTargetName(t *testing.T) {
	result := postValidationJSON(t, (&App{}).handleValidateReusableStep, map[string]any{
		"resource_id": "library/prepare",
		"yaml": `
name: actual
script: echo ok
`,
	})

	if result.Valid {
		t.Fatalf("valid = true, want false")
	}
	if len(result.Errors) != 1 || result.Errors[0].Code != "name_mismatch" {
		t.Fatalf("errors = %#v, want name_mismatch", result.Errors)
	}
}

func TestValidateTriggerOverrideEndpointRequiresRepository(t *testing.T) {
	result := postValidationJSON(t, (&App{}).handleValidateTriggerOverride, map[string]any{
		"yaml": `
triggers:
  - on: push
    pipelines:
      - deploy
`,
	})

	if result.Valid {
		t.Fatalf("valid = true, want false")
	}
	if len(result.Errors) != 1 || result.Errors[0].Code != "required" || result.Errors[0].Path != "repository" {
		t.Fatalf("errors = %#v, want repository required error", result.Errors)
	}
}

func TestValidateConfigRepositoryBundleDryRunUsesIncomingReusableSteps(t *testing.T) {
	result := postValidationJSON(t, (&App{}).handleValidateGlobalConfigRepository, map[string]any{
		"base_path": "config",
		"files": []map[string]any{
			{
				"path": "config/steps/prepare-runtime.yaml",
				"content": `
name: prepare-runtime
script: |
  printf manifest > /nopsai/outputs/release_manifest
outputs:
  - release_manifest
`,
			},
			{
				"path": "config/pipelines/deploy.yaml",
				"content": `
name: deploy
container_image: alpine:3.20
steps:
  - name: prepare
    include: step:prepare-runtime
  - name: consume
    depends_on:
      - prepare
    variables:
      RELEASE_MANIFEST: $steps.prepare.prepare.outputs.release_manifest
    script: echo consume
`,
			},
		},
	})

	if !result.Valid || len(result.Errors) != 0 {
		t.Fatalf("result = %#v, want valid dry-run response", result)
	}
}

func TestValidateConfigRepositoryBundleDryRunReportsFileErrors(t *testing.T) {
	result := postValidationJSON(t, (&App{}).handleValidateGlobalConfigRepository, map[string]any{
		"base_path": "config",
		"files": []map[string]any{
			{
				"path": "config/pipelines/deploy.yaml",
				"content": `
name: other
container_image: alpine:3.20
steps:
  - name: run
    script: echo ok
`,
			},
		},
	})

	if result.Valid {
		t.Fatalf("valid = true, want false")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("errors = %#v, want one dry-run error", result.Errors)
	}
	if !strings.Contains(result.Errors[0].Message, "deploy.yaml") || result.Errors[0].Code != "name_mismatch" {
		t.Fatalf("error = %#v, want file-name/name mismatch", result.Errors[0])
	}
}

func postValidationJSON(t *testing.T, handler http.HandlerFunc, payload any) validationResponse {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/validate", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result validationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; body = %s", err, rec.Body.String())
	}
	return result
}
