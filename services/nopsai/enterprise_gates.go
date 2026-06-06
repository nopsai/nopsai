package nopsai

import (
	"strings"

	"nopsai/config"
)

const (
	minProductionSecretLength = 32
	devDefaultAAAToken        = "dev-default-for-local-only"
)

func enterpriseStartupGateChecks(cfg *config.Config) []setupPreflightCheck {
	if cfg == nil {
		cfg = &config.Config{}
	}

	strict := cfg.RequiresProductionGates()
	checks := []setupPreflightCheck{enterpriseGateModeCheck(strict)}

	if masterKey := strings.TrimSpace(cfg.MasterKey); masterKey != "" {
		checks = append(checks, secretStrengthCheck(
			"master_key_strength",
			"Master encryption key strength",
			"NOPSAI_MASTER_KEY",
			masterKey,
			"Master encryption key has production-grade length and is not a known placeholder.",
			"Set a unique random NOPSAI_MASTER_KEY of at least 32 characters before production use.",
			strict,
		))
	}

	if jwtKey := strings.TrimSpace(cfg.JWTSigningKey); jwtKey != "" {
		checks = append(checks, secretStrengthCheck(
			"jwt_signing_key_strength",
			"JWT signing key strength",
			"JWT_SIGNING_KEY",
			jwtKey,
			"JWT signing key has production-grade length and is not a known placeholder.",
			"Set a unique random JWT_SIGNING_KEY of at least 32 characters before production use.",
			strict,
		))
	}

	checks = append(checks,
		serviceJWTIsolationCheck(cfg, strict),
		secretStrengthCheck(
			"aaa_shared_token_strength",
			"AAA shared internal token strength",
			"AAA_SHARED_INTERNAL_TOKEN",
			cfg.AAASharedToken,
			"AAA shared internal token has production-grade length and is not the development default.",
			"Set a unique random AAA_SHARED_INTERNAL_TOKEN of at least 32 characters before production use.",
			strict,
		),
		dispatcherTLSCheck(cfg, strict),
	)

	if githubAppConfigured(cfg) {
		checks = append(checks, secretStrengthCheck(
			"github_webhook_secret_strength",
			"GitHub webhook secret strength",
			"GITHUB_WEBHOOK_SECRET",
			cfg.GitHubWebhookSecret,
			"GitHub webhook secret has production-grade length and is not a known placeholder.",
			"Set a unique random GITHUB_WEBHOOK_SECRET before accepting GitHub webhooks in production.",
			strict,
		))
	}

	return checks
}

func hasBlockingEnterpriseStartupGates(cfg *config.Config) bool {
	for _, check := range enterpriseStartupGateChecks(cfg) {
		if check.Required && check.Status == "error" {
			return true
		}
	}
	return false
}

func HasBlockingEnterpriseStartupGates(cfg *config.Config) bool {
	return hasBlockingEnterpriseStartupGates(cfg)
}

func enterpriseGateModeCheck(strict bool) setupPreflightCheck {
	if strict {
		return setupPreflightCheck{
			ID:       "enterprise_startup_gates",
			Label:    "Enterprise startup gates",
			Status:   "success",
			Required: false,
			Message:  "Production startup gates are enforced for this deployment.",
		}
	}
	return setupPreflightCheck{
		ID:       "enterprise_startup_gates",
		Label:    "Enterprise startup gates",
		Status:   "warning",
		Required: false,
		Message:  "Set NOPSAI_ENVIRONMENT=production or NOPSAI_REQUIRE_PRODUCTION_GATES=true before production use.",
		SuggestedEnv: map[string]string{
			"NOPSAI_ENVIRONMENT":              "production",
			"NOPSAI_REQUIRE_PRODUCTION_GATES": "true",
		},
	}
}

func secretStrengthCheck(id, label, envName, value, successMessage, failureMessage string, strict bool) setupPreflightCheck {
	if productionSecretReady(value) {
		return setupPreflightCheck{
			ID:       id,
			Label:    label,
			Status:   "success",
			Required: false,
			Message:  successMessage,
		}
	}
	return enterpriseFailureCheck(id, label, envName, failureMessage, strict)
}

func serviceJWTIsolationCheck(cfg *config.Config, strict bool) setupPreflightCheck {
	serviceKey := strings.TrimSpace(cfg.ServiceJWTSigningKey)
	userKey := strings.TrimSpace(cfg.JWTSigningKey)
	if productionSecretReady(serviceKey) && serviceKey != userKey {
		return setupPreflightCheck{
			ID:       "service_jwt_isolated",
			Label:    "Service JWT isolation",
			Status:   "success",
			Required: false,
			Message:  "Service JWT signing uses a separate production-grade key.",
		}
	}
	return enterpriseFailureCheck(
		"service_jwt_isolated",
		"Service JWT isolation",
		"SERVICE_JWT_SIGNING_KEY",
		"Set a separate production-grade SERVICE_JWT_SIGNING_KEY so browser/API tokens and internal service tokens do not share one signing secret.",
		strict,
	)
}

func dispatcherTLSCheck(cfg *config.Config, strict bool) setupPreflightCheck {
	if cfg.EffectiveDispatcherTLSMode() != "disabled" {
		return setupPreflightCheck{
			ID:       "dispatcher_transport_security",
			Label:    "Dispatcher transport security",
			Status:   "success",
			Required: false,
			Message:  "Dispatcher transport security is enabled.",
		}
	}
	return enterpriseFailureCheckWithSuggestedValue(
		"dispatcher_transport_security",
		"Dispatcher transport security",
		"DISPATCHER_TLS_MODE",
		"mtls",
		"Enable dispatcher TLS or mTLS before production use.",
		strict,
	)
}

func enterpriseFailureCheck(id, label, envName, message string, strict bool) setupPreflightCheck {
	return enterpriseFailureCheckWithSuggestedValue(id, label, envName, "$(openssl rand -base64 32)", message, strict)
}

func enterpriseFailureCheckWithSuggestedValue(id, label, envName, suggestedValue, message string, strict bool) setupPreflightCheck {
	status := "warning"
	required := false
	if strict {
		status = "error"
		required = true
	}
	return setupPreflightCheck{
		ID:       id,
		Label:    label,
		Status:   status,
		Required: required,
		Message:  message,
		SuggestedEnv: map[string]string{
			envName: suggestedValue,
		},
	}
}

func productionSecretReady(value string) bool {
	secret := strings.TrimSpace(value)
	if len(secret) < minProductionSecretLength {
		return false
	}
	lower := strings.ToLower(secret)
	placeholders := []string{
		"replace-me",
		"changeme",
		"change-me",
		"dev-default",
		"yoursecurepassword",
		"password",
	}
	for _, placeholder := range placeholders {
		if strings.Contains(lower, placeholder) {
			return false
		}
	}
	return secret != devDefaultAAAToken
}

func githubAppConfigured(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return strings.TrimSpace(cfg.GitHubAppID) != "" ||
		strings.TrimSpace(cfg.GitHubInstallID) != "" ||
		strings.TrimSpace(cfg.GitHubPrivateKeyPath) != "" ||
		strings.TrimSpace(cfg.GitHubPrivateKey) != ""
}
