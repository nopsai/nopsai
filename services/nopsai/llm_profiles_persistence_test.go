package nopsai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

// A client that does not know about rate cards — the models form is one — must
// not delete pricing by saving an unrelated field.
func TestHandleUpsertLLMProfileKeepsPricingWhenTheSaveOmitsIt(t *testing.T) {
	input, output := 1.25, 10.0
	app := newLLMProfilePricingTestApp(t, config.LLMProfile{
		Provider: config.LLMProviderOpenAI,
		Model:    "gpt-5",
		Pricing:  &config.LLMPricing{InputPerMillionUSD: input, OutputPerMillionUSD: output},
	})

	upsertLLMProfileForTest(t, app, "hosted", `{"name":"hosted","provider":"openai","model":"gpt-5-mini"}`)

	stored := app.getConfigSnapshot().LLMProfiles["hosted"]
	if stored.Model != "gpt-5-mini" {
		t.Fatalf("model = %q, want the saved value", stored.Model)
	}
	if stored.Pricing == nil {
		t.Fatal("rate card was deleted by a save that never mentioned pricing")
	}
	if stored.Pricing.InputPerMillionUSD != input || stored.Pricing.OutputPerMillionUSD != output {
		t.Fatalf("pricing = %#v, want the declared rates preserved", stored.Pricing)
	}
}

func TestHandleUpsertLLMProfileAppliesAndClearsStatedPricing(t *testing.T) {
	app := newLLMProfilePricingTestApp(t, config.LLMProfile{Provider: config.LLMProviderOpenAI, Model: "gpt-5"})

	upsertLLMProfileForTest(t, app, "hosted", `{"name":"hosted","provider":"openai","model":"gpt-5","pricing":{"input_per_million_usd":1.25,"output_per_million_usd":10,"cached_input_per_million_usd":0.125}}`)
	stored := app.getConfigSnapshot().LLMProfiles["hosted"]
	if stored.Pricing == nil || stored.Pricing.InputPerMillionUSD != 1.25 || stored.Pricing.CachedInputRate() != 0.125 {
		t.Fatalf("pricing = %#v, want the stated rates", stored.Pricing)
	}

	upsertLLMProfileForTest(t, app, "hosted", `{"name":"hosted","provider":"openai","model":"gpt-5","pricing":null}`)
	if stored := app.getConfigSnapshot().LLMProfiles["hosted"]; stored.Pricing != nil {
		t.Fatalf("pricing = %#v, want nil after an explicit null cleared it", stored.Pricing)
	}
}

func TestHandleUpsertLLMProfileDefaultsSelfHostedPricingToExplicitZeroes(t *testing.T) {
	app := newLLMProfilePricingTestApp(t, config.LLMProfile{Provider: config.LLMProviderOpenAI, Model: "gpt-5"})

	upsertLLMProfileForTest(t, app, "local", `{"name":"local","provider":"lmstudio","model":"qwen3-coder","base_url":"http://lmstudio:1234"}`)

	stored := app.getConfigSnapshot().LLMProfiles["local"]
	if stored.Pricing == nil {
		t.Fatal("a self-hosted model should state that it costs nothing rather than record no rate card")
	}
	if stored.Pricing.InputPerMillionUSD != 0 || stored.Pricing.OutputPerMillionUSD != 0 {
		t.Fatalf("pricing = %#v, want explicit zeroes", stored.Pricing)
	}
}

func TestHandleReplaceLLMProfilesKeepsPricingWhenTheSaveOmitsIt(t *testing.T) {
	app := newLLMProfilePricingTestApp(t, config.LLMProfile{
		Provider: config.LLMProviderOpenAI,
		Model:    "gpt-5",
		Pricing:  &config.LLMPricing{InputPerMillionUSD: 1.25, OutputPerMillionUSD: 10},
	})

	body := `{"default_profile":"hosted","profiles":[{"name":"hosted","provider":"openai","model":"gpt-5"}]}`
	req := httptest.NewRequest(http.MethodPut, "/v1/system/models", strings.NewReader(body))
	req = req.WithContext(withAAASubject(req.Context(), model.Subject{Type: model.SubjectTypeUser, ID: "alice"}))
	rec := httptest.NewRecorder()
	app.handleReplaceLLMProfiles(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	if stored := app.getConfigSnapshot().LLMProfiles["hosted"]; stored.Pricing == nil || stored.Pricing.OutputPerMillionUSD != 10 {
		t.Fatalf("pricing = %#v, want the declared rates preserved by a default-profile save", stored.Pricing)
	}
}

func newLLMProfilePricingTestApp(t *testing.T, profile config.LLMProfile) *App {
	t.Helper()
	return &App{
		cfg: &config.Config{
			LLMDefaultProfile: "hosted",
			LLMProfiles:       map[string]config.LLMProfile{"hosted": profile},
		},
		configPath: filepath.Join(t.TempDir(), "config.yml"),
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ctx context.Context, _subject model.Subject, _action string, _resource model.ResourceRef, _requestContext map[string]any) (model.Decision, error) {
				return model.Decision{Allowed: true}, nil
			},
		},
	}
}

func upsertLLMProfileForTest(t *testing.T, app *App, name string, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/v1/system/models/"+name, strings.NewReader(body))
	req.SetPathValue("profileName", name)
	req = req.WithContext(withAAASubject(req.Context(), model.Subject{Type: model.SubjectTypeUser, ID: "alice"}))
	rec := httptest.NewRecorder()
	app.handleUpsertLLMProfile(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upsert %s status = %d body = %s", name, rec.Code, rec.Body.String())
	}
}
