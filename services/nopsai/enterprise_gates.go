package nopsai

import (
	"fmt"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/buildinfo"
	"nopsai/pkg/license"
	"nopsai/pkg/startupgates"
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
	checks := []setupPreflightCheck{enterpriseGateModeCheck(strict), licenseEntitlementCheck(cfg, strict)}

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
		metricsAuthenticationCheck(cfg, strict),
	)

	if githubAppConfigured(cfg) {
		checks = append(checks,
			configuredValueCheck(
				"github_private_key_credential_ref",
				"GitHub private key credential",
				cfg.GitHubPrivateKeyCredentialRef,
				"GitHub App private key uses the credential registry.",
				"Configure GITHUB_PRIVATE_KEY_CREDENTIAL_REF.",
				strict,
			),
			configuredValueCheck(
				"github_webhook_credential_ref",
				"GitHub webhook credential",
				cfg.GitHubWebhookCredentialRef,
				"GitHub webhook verification uses the credential registry.",
				"Configure GITHUB_WEBHOOK_CREDENTIAL_REF.",
				strict,
			),
		)
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

func configuredValueCheck(id, label, value, successMessage, failureMessage string, strict bool) setupPreflightCheck {
	if strings.TrimSpace(value) != "" {
		return setupPreflightCheck{
			ID:       id,
			Label:    label,
			Status:   "success",
			Required: false,
			Message:  successMessage,
		}
	}
	status := "warning"
	if strict {
		status = "error"
	}
	return setupPreflightCheck{
		ID:       id,
		Label:    label,
		Status:   status,
		Required: strict,
		Message:  failureMessage,
	}
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

func metricsAuthenticationCheck(cfg *config.Config, strict bool) setupPreflightCheck {
	if cfg != nil && cfg.MetricsRequireAuth {
		return setupPreflightCheck{
			ID:       "metrics_authentication",
			Label:    "Metrics endpoint authentication",
			Status:   "success",
			Required: false,
			Message:  "Metrics endpoint authentication is enabled.",
		}
	}
	return enterpriseFailureCheckWithSuggestedValue(
		"metrics_authentication",
		"Metrics endpoint authentication",
		"METRICS_REQUIRE_AUTH",
		"true",
		"Require authentication for /metrics before production use.",
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
	return startupgates.ProductionSecretReady(value)
}

func githubAppConfigured(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return strings.TrimSpace(cfg.GitHubAppID) != "" ||
		strings.TrimSpace(cfg.GitHubInstallID) != "" ||
		strings.TrimSpace(cfg.GitHubPrivateKeyCredentialRef) != "" ||
		strings.TrimSpace(cfg.GitHubWebhookCredentialRef) != ""
}

// licenseEntitlementCheck blocks production startup on an installation that is
// not licensed. Outside production mode it is advisory: an evaluator must be
// able to run NopsAI without talking to anyone, and refusing to start would
// make the product unevaluable rather than protected.
//
// In production mode it is required, because an administrator declaring an
// installation production while running under evaluation limits is running
// unlicensed by accident, which is exactly what the gate exists to prevent.
func licenseEntitlementCheck(cfg *config.Config, strict bool) setupPreflightCheck {
	publicKey, buildCanVerify := license.ParsePublicKey(buildinfo.LicensePublicKey)
	entitlement := license.Resolve(cfg.LicenseKey, publicKey, time.Now().UTC())

	// A build compiled without a verification key cannot check any licence, so
	// blocking on it would punish the build configuration rather than the
	// operator, and would stop a build that predates licensing from starting at
	// all. Such a build reports the situation and never blocks.
	blocking := strict && buildCanVerify

	if entitlement.Licensed {
		message := fmt.Sprintf("Licensed to %s (%s tier).", entitlement.Claims.Licensee, entitlement.Claims.Tier)
		if !entitlement.Claims.ExpiresAt.IsZero() {
			message += fmt.Sprintf(" Expires %s.", entitlement.Claims.ExpiresAt.UTC().Format("2006-01-02"))
		}
		return setupPreflightCheck{
			ID:       "license_entitlement",
			Label:    "Licence entitlement",
			Status:   "success",
			Required: blocking,
			Message:  message,
		}
	}

	status := "warning"
	if blocking {
		status = "error"
	}
	return setupPreflightCheck{
		ID:           "license_entitlement",
		Label:        "Licence entitlement",
		Status:       status,
		Required:     blocking,
		Message:      entitlement.Reason + " Set license_key in setting/system/runner.yaml, or NOPSAI_LICENSE_KEY, to license this installation.",
		SuggestedEnv: map[string]string{"NOPSAI_LICENSE_KEY": ""},
	}
}
