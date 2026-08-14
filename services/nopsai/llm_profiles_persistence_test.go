package nopsai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/config"
	"nopsai/services/aaa/pkg/model"
)

func TestPersistLLMProfilesBootstrapConfigIsOptionalAfterDBPersistence(t *testing.T) {
	app := &App{configPath: t.TempDir()}
	cfg := config.Config{
		LLMDefaultProfile: "standard",
		LLMProfiles: map[string]config.LLMProfile{
			"standard": {
				Provider: config.LLMProviderLMStudio,
				Model:    "qwen",
				BaseURL:  "http://lmstudio:1234",
			},
		},
	}

	if err := app.persistLLMProfilesBootstrapConfig(cfg, false); err != nil {
		t.Fatalf("optional bootstrap persistence error = %v, want nil", err)
	}
	if err := app.persistLLMProfilesBootstrapConfig(cfg, true); err == nil {
		t.Fatal("required bootstrap persistence unexpectedly succeeded")
	}
}

func TestHandleUpsertLLMProfileDoesNotPublishMemoryWhenPersistenceFails(t *testing.T) {
	app := &App{
		cfg: &config.Config{
			LLMDefaultProfile: "standard",
			LLMProfiles: map[string]config.LLMProfile{
				"standard": {
					Provider: config.LLMProviderLMStudio,
					Model:    "old-model",
					BaseURL:  "http://lmstudio:1234",
				},
			},
		},
		configPath: t.TempDir(),
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ctx context.Context, _subject model.Subject, _action string, _resource model.ResourceRef, _requestContext map[string]any) (model.Decision, error) {
				return model.Decision{Allowed: true}, nil
			},
		},
	}

	body := `{"name":"standard","provider":"lmstudio","model":"new-model","base_url":"http://lmstudio:1234"}`
	req := httptest.NewRequest(http.MethodPut, "/v1/system/models/standard", strings.NewReader(body))
	req.SetPathValue("profileName", "standard")
	req = req.WithContext(withAAASubject(req.Context(), model.Subject{Type: model.SubjectTypeUser, ID: "alice"}))
	rec := httptest.NewRecorder()

	app.handleUpsertLLMProfile(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if got := app.getConfigSnapshot().LLMProfiles["standard"].Model; got != "old-model" {
		t.Fatalf("stored model = %q, want old-model after failed persistence", got)
	}
}
