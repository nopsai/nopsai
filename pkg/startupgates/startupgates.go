package startupgates

import (
	"fmt"
	"strconv"
	"strings"

	"nopsai/config"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicetls"
)

const minProductionSecretLength = 32

type Check struct {
	ID       string
	Passed   bool
	Required bool
	Message  string
}

type Error struct {
	Service string
	Checks  []Check
}

func (e Error) Error() string {
	parts := make([]string, 0, len(e.Checks))
	for _, check := range e.Checks {
		parts = append(parts, check.Message)
	}
	return fmt.Sprintf("%s startup gates failed: %s", e.Service, strings.Join(parts, "; "))
}

func ProductionSecretReady(value string) bool {
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
	return secret != "dev-default-for-local-only"
}

func ValidateDispatcher(cfg *config.Config) error {
	return enforce("dispatcher", dispatcherChecks(cfg))
}

func ValidateGitBot(cfg *config.Config) error {
	return enforce("git-bot", gitBotChecks(cfg))
}

func ValidateAAA(cfg *config.Config) error {
	return enforce("aaa", aaaChecks(cfg))
}

func ValidateRunner(cfg *config.Config) error {
	return enforce("runner", runnerChecks(cfg))
}

func ValidateKubernetesRunner(cfg *config.Config) error {
	return enforce("k8s-runner", runnerChecks(cfg))
}

func ValidateAgentEnv(lookup func(string) string) error {
	return enforce("agent", agentEnvChecks(lookup))
}

func dispatcherChecks(cfg *config.Config) []Check {
	strict := requiresProductionGates(cfg)
	checks := commonServiceChecks(cfg, strict)
	checks = append(checks, Check{
		ID:       "dispatcher_nopsai_api_url",
		Passed:   strings.TrimSpace(valueOrEmpty(cfg, func(c *config.Config) string { return c.EffectiveNopsaiAPIURL() })) != "",
		Required: strict,
		Message:  "NOPSAI_API_URL must be configured so dispatcher callbacks reach NopsAI",
	})
	return checks
}

func gitBotChecks(cfg *config.Config) []Check {
	strict := requiresProductionGates(cfg)
	checks := commonServiceChecks(cfg, strict)
	return append(checks,
		Check{
			ID:       "git_bot_service_id",
			Passed:   strings.TrimSpace(valueOrEmpty(cfg, func(c *config.Config) string { return c.EffectiveGitBotServiceID() })) != "",
			Required: strict,
			Message:  "GIT_BOT_SERVICE_ID must identify git-bot for service authentication",
		},
		Check{
			ID:       "nopsai_api_url",
			Passed:   strings.TrimSpace(valueOrEmpty(cfg, func(c *config.Config) string { return c.EffectiveNopsaiAPIURL() })) != "",
			Required: strict,
			Message:  "NOPSAI_API_URL must be configured so git-bot can forward webhook events",
		},
	)
}

func aaaChecks(cfg *config.Config) []Check {
	strict := requiresProductionGates(cfg)
	return []Check{
		{
			ID:       "aaa_database_url",
			Passed:   strings.TrimSpace(valueOrEmpty(cfg, func(c *config.Config) string { return c.DatabaseURL })) != "",
			Required: strict,
			Message:  "DATABASE_URL must be configured for AAA",
		},
		{
			ID:       "aaa_shared_token",
			Passed:   ProductionSecretReady(valueOrEmpty(cfg, func(c *config.Config) string { return c.AAASharedToken })),
			Required: strict,
			Message:  "AAA_SHARED_INTERNAL_TOKEN must be a production-grade internal token",
		},
	}
}

func runnerChecks(cfg *config.Config) []Check {
	strict := requiresProductionGates(cfg)
	checks := commonServiceChecks(cfg, strict)
	checks = append(checks, Check{
		ID:       "runner_dispatcher_grpc_address",
		Passed:   strings.TrimSpace(valueOrEmpty(cfg, func(c *config.Config) string { return c.DispatcherAddress })) != "",
		Required: strict,
		Message:  "DISPATCHER_GRPC_ADDRESS must be configured explicitly for production runners",
	})
	return checks
}

func agentEnvChecks(lookup func(string) string) []Check {
	if lookup == nil {
		lookup = func(string) string { return "" }
	}
	strict := requiresProductionGatesEnv(lookup)
	signingKey := lookup(serviceauth.EnvSigningKey)
	tlsSecret := strings.TrimSpace(lookup(servicetls.EnvSecret))
	if tlsSecret == "" {
		tlsSecret = signingKey
	}
	tlsMode := servicetls.NormalizeMode(lookup(servicetls.EnvMode))
	serviceID := strings.TrimSpace(lookup(serviceauth.EnvServiceID))
	if serviceID == "" {
		serviceID = strings.TrimSpace(lookup("AGENT_SERVICE_ID"))
	}
	return []Check{
		{
			ID:       "agent_dispatcher_grpc_address",
			Passed:   strings.TrimSpace(lookup("DISPATCHER_GRPC_ADDRESS")) != "",
			Required: strict,
			Message:  "DISPATCHER_GRPC_ADDRESS must be configured for agent",
		},
		{
			ID:       "agent_service_id",
			Passed:   serviceID != "",
			Required: strict,
			Message:  "SERVICE_ID or AGENT_SERVICE_ID must be configured for agent service authentication",
		},
		{
			ID:       "agent_service_jwt_signing_key",
			Passed:   ProductionSecretReady(signingKey),
			Required: strict,
			Message:  "SERVICE_JWT_SIGNING_KEY must be a production-grade secret for agent callbacks",
		},
		{
			ID:       "agent_dispatcher_tls_enabled",
			Passed:   tlsMode != servicetls.ModeDisabled,
			Required: strict,
			Message:  "DISPATCHER_TLS_MODE must enable TLS or mTLS for agent dispatcher traffic",
		},
		{
			ID:       "agent_dispatcher_tls_secret",
			Passed:   tlsMode == servicetls.ModeDisabled || ProductionSecretReady(tlsSecret),
			Required: strict,
			Message:  "DISPATCHER_TLS_SECRET must be production-grade when dispatcher TLS is enabled",
		},
	}
}

func commonServiceChecks(cfg *config.Config, strict bool) []Check {
	serviceKey := strings.TrimSpace(valueOrEmpty(cfg, func(c *config.Config) string { return c.ServiceJWTSigningKey }))
	userKey := strings.TrimSpace(valueOrEmpty(cfg, func(c *config.Config) string { return c.JWTSigningKey }))
	tlsMode := servicetls.NormalizeMode(valueOrEmpty(cfg, func(c *config.Config) string { return c.EffectiveDispatcherTLSMode() }))
	tlsSecret := valueOrEmpty(cfg, func(c *config.Config) string { return c.EffectiveDispatcherTLSSecret() })
	return []Check{
		{
			ID:       "service_jwt_signing_key",
			Passed:   ProductionSecretReady(serviceKey),
			Required: strict,
			Message:  "SERVICE_JWT_SIGNING_KEY must be a production-grade secret",
		},
		{
			ID:       "service_jwt_isolation",
			Passed:   ProductionSecretReady(serviceKey) && serviceKey != userKey,
			Required: strict,
			Message:  "SERVICE_JWT_SIGNING_KEY must be separate from JWT_SIGNING_KEY",
		},
		{
			ID:       "dispatcher_tls_enabled",
			Passed:   tlsMode != servicetls.ModeDisabled,
			Required: strict,
			Message:  "DISPATCHER_TLS_MODE must enable TLS or mTLS",
		},
		{
			ID:       "dispatcher_tls_secret",
			Passed:   tlsMode == servicetls.ModeDisabled || ProductionSecretReady(tlsSecret),
			Required: strict,
			Message:  "DISPATCHER_TLS_SECRET must be production-grade when dispatcher TLS is enabled",
		},
	}
}

func enforce(service string, checks []Check) error {
	var blocking []Check
	for _, check := range checks {
		if check.Required && !check.Passed {
			blocking = append(blocking, check)
		}
	}
	if len(blocking) == 0 {
		return nil
	}
	return Error{Service: service, Checks: blocking}
}

func requiresProductionGates(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return cfg.RequiresProductionGates()
}

func requiresProductionGatesEnv(lookup func(string) string) bool {
	if raw := strings.TrimSpace(lookup("NOPSAI_REQUIRE_PRODUCTION_GATES")); raw != "" {
		if enabled, err := strconv.ParseBool(raw); err == nil && enabled {
			return true
		}
	}
	env := strings.ToLower(strings.TrimSpace(lookup("NOPSAI_ENVIRONMENT")))
	return env == "production" || env == "prod"
}

func valueOrEmpty(cfg *config.Config, getter func(*config.Config) string) string {
	if cfg == nil || getter == nil {
		return ""
	}
	return getter(cfg)
}
