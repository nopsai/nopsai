package nopsai

import (
	"testing"

	"nopsai/config"
)

func TestEnterpriseStartupGatesWarnByDefault(t *testing.T) {
	cfg := &config.Config{
		MasterKey:            "short",
		JWTSigningKey:        "replace-me",
		AAASharedToken:       devDefaultAAAToken,
		DispatcherTLSMode:    "disabled",
		ServiceJWTSigningKey: "",
		GitHubAppID:          "123",
		GitHubWebhookSecret:  "",
	}

	if HasBlockingEnterpriseStartupGates(cfg) {
		t.Fatal("default environment should warn without blocking startup")
	}

	checks := checksByID(enterpriseStartupGateChecks(cfg))
	if checks["service_jwt_isolated"].Status != "warning" || checks["service_jwt_isolated"].Required {
		t.Fatalf("service JWT check = %#v, want non-blocking warning", checks["service_jwt_isolated"])
	}
	if checks["dispatcher_transport_security"].Status != "warning" || checks["dispatcher_transport_security"].Required {
		t.Fatalf("dispatcher TLS check = %#v, want non-blocking warning", checks["dispatcher_transport_security"])
	}
}

func TestEnterpriseStartupGatesBlockProductionDefaults(t *testing.T) {
	cfg := &config.Config{
		Environment:          "production",
		MasterKey:            "01234567890123456789012345678901",
		JWTSigningKey:        "abcdefghijklmnopqrstuvwxyz123456",
		AAASharedToken:       devDefaultAAAToken,
		DispatcherTLSMode:    "disabled",
		ServiceJWTSigningKey: "",
		GitHubAppID:          "123",
		GitHubWebhookSecret:  "",
	}

	if !HasBlockingEnterpriseStartupGates(cfg) {
		t.Fatal("production defaults should block startup")
	}

	checks := checksByID(enterpriseStartupGateChecks(cfg))
	for _, id := range []string{
		"service_jwt_isolated",
		"aaa_shared_token_strength",
		"dispatcher_transport_security",
		"github_webhook_secret_strength",
	} {
		if checks[id].Status != "error" || !checks[id].Required {
			t.Fatalf("%s check = %#v, want blocking error", id, checks[id])
		}
	}
}

func TestEnterpriseStartupGatesAcceptProductionSecrets(t *testing.T) {
	cfg := &config.Config{
		Environment:          "production",
		MasterKey:            "01234567890123456789012345678901",
		JWTSigningKey:        "abcdefghijklmnopqrstuvwxyz123456",
		ServiceJWTSigningKey: "service-token-key-012345678901234",
		AAASharedToken:       "aaa-shared-token-01234567890123456",
		DispatcherTLSMode:    "mtls",
		GitHubAppID:          "123",
		GitHubWebhookSecret:  "github-webhook-secret-012345678901",
	}

	if HasBlockingEnterpriseStartupGates(cfg) {
		t.Fatal("production-ready secrets should not block startup")
	}

	checks := checksByID(enterpriseStartupGateChecks(cfg))
	for _, id := range []string{
		"master_key_strength",
		"jwt_signing_key_strength",
		"service_jwt_isolated",
		"aaa_shared_token_strength",
		"dispatcher_transport_security",
		"github_webhook_secret_strength",
	} {
		if checks[id].Status != "success" {
			t.Fatalf("%s check = %#v, want success", id, checks[id])
		}
	}
}

func TestEnterpriseDatabaseGatesSkipNonProductionAndNilDatabase(t *testing.T) {
	if HasBlockingEnterpriseDatabaseGates(t.Context(), &config.Config{}, nil) {
		t.Fatal("database gates should not block non-production startup")
	}
	if HasBlockingEnterpriseDatabaseGates(t.Context(), &config.Config{Environment: "production"}, nil) {
		t.Fatal("database gates should not block without a database handle")
	}
}

func checksByID(checks []setupPreflightCheck) map[string]setupPreflightCheck {
	byID := make(map[string]setupPreflightCheck, len(checks))
	for _, check := range checks {
		byID[check.ID] = check
	}
	return byID
}
