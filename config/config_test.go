package config

import "testing"

func TestNormalizeLLMProvider(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty preserved", raw: "", want: ""},
		{name: "gemini alias", raw: "google-gemini", want: LLMProviderGemini},
		{name: "lmstudio canonical", raw: "lmstudio", want: LLMProviderLMStudio},
		{name: "openai compatible alias", raw: "openai-compatible", want: LLMProviderLMStudio},
		{name: "openai alias", raw: "ChatGPT", want: LLMProviderOpenAI},
		{name: "anthropic alias", raw: "Claude", want: LLMProviderAnthropic},
		{name: "openrouter alias", raw: "open-router", want: LLMProviderOpenRouter},
		{name: "azure alias", raw: "azure_openai", want: LLMProviderAzureOpenAI},
		{name: "unknown passes through normalized", raw: "CustomProvider", want: "customprovider"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeLLMProvider(tt.raw); got != tt.want {
				t.Fatalf("NormalizeLLMProvider(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestLLMProviderDefaultsAndAPIKeyRequirements(t *testing.T) {
	if got := DefaultLLMProviderBaseURL(LLMProviderGroq); got != "https://api.groq.com/openai/v1" {
		t.Fatalf("DefaultLLMProviderBaseURL(groq) = %q", got)
	}
	if got := EffectiveLLMProfileBaseURL(LLMProfile{Provider: LLMProviderOpenAI}); got != "https://api.openai.com/v1" {
		t.Fatalf("EffectiveLLMProfileBaseURL(openai) = %q", got)
	}
	if got := EffectiveLLMProfileBaseURL(LLMProfile{Provider: LLMProviderOpenAI, BaseURL: " http://proxy/v1 "}); got != "http://proxy/v1" {
		t.Fatalf("EffectiveLLMProfileBaseURL(custom) = %q", got)
	}
	if !LLMProviderRequiresAPIKey(LLMProviderAnthropic) {
		t.Fatal("Anthropic should require an API key")
	}
	if LLMProviderRequiresAPIKey(LLMProviderOllama) {
		t.Fatal("Ollama should not require an API key")
	}
	if got := DefaultLLMProviderModel(LLMProviderAnthropic); got != "claude-sonnet-4-6" {
		t.Fatalf("DefaultLLMProviderModel(anthropic) = %q", got)
	}
	if got := DefaultLLMProviderAPIKeySecret(LLMProviderOllama); got != "OLLAMA_API_KEY" {
		t.Fatalf("DefaultLLMProviderAPIKeySecret(ollama) = %q", got)
	}
	if got := DefaultLLMProviderAPIKeySecret(LLMProviderAzureOpenAI); got != "AZURE_OPENAI_API_KEY" {
		t.Fatalf("DefaultLLMProviderAPIKeySecret(azure-openai) = %q", got)
	}
}

func TestRepositoryConfigEnablesAssistantBootstrapDefault(t *testing.T) {
	cfg, err := LoadConfig("../config.yml")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if !cfg.EffectiveAssistantConfig().Enabled {
		t.Fatal("checked-in config.yml should enable the assistant as a local bootstrap default")
	}
	if !cfg.EffectiveAssistantConfig().Memory.Enabled {
		t.Fatal("checked-in config.yml should enable assistant memory")
	}
	if !AssistantMCPEnabled(cfg.EffectiveAssistantConfig().MCP) {
		t.Fatal("checked-in config.yml should enable hosted MCP for assistant bootstrap")
	}
}

func TestLLMProviderGenerationCapabilities(t *testing.T) {
	providers := []string{
		LLMProviderGemini,
		LLMProviderLMStudio,
		LLMProviderOpenAI,
		LLMProviderAnthropic,
		LLMProviderGroq,
		LLMProviderMistral,
		LLMProviderOllama,
		LLMProviderOpenRouter,
		LLMProviderAzureOpenAI,
	}
	for _, provider := range providers {
		if !LLMProviderSupportsMaxTokens(provider) {
			t.Errorf("LLMProviderSupportsMaxTokens(%q) = false", provider)
		}
		if _, _, ok := LLMProviderTemperatureRange(provider); !ok {
			t.Errorf("LLMProviderTemperatureRange(%q) is unsupported", provider)
		}
	}

	if !LLMProviderSupportsGenericReasoning(LLMProviderLMStudio) {
		t.Fatal("LM Studio should support generic reasoning")
	}
	if LLMProviderSupportsGenericReasoning(LLMProviderOpenAI) {
		t.Fatal("OpenAI reasoning requires model-specific configuration")
	}
	if min, max, ok := LLMProviderTemperatureRange(LLMProviderAnthropic); !ok || min != 0 || max != 1 {
		t.Fatalf("Anthropic temperature range = (%g, %g, %v), want (0, 1, true)", min, max, ok)
	}
	if !LLMProviderUsesMaxCompletionTokens(LLMProviderOpenAI) ||
		!LLMProviderUsesMaxCompletionTokens(LLMProviderGroq) ||
		LLMProviderUsesMaxCompletionTokens(LLMProviderMistral) {
		t.Fatal("max completion token field selection is incorrect")
	}
	if LLMProviderSupportsMaxTokens("custom") {
		t.Fatal("custom provider should not advertise max token support")
	}
	if _, _, ok := LLMProviderTemperatureRange("custom"); ok {
		t.Fatal("custom provider should not advertise temperature support")
	}
}

func TestNormalizeLLMProfileNormalizesExtra(t *testing.T) {
	profile := NormalizeLLMProfile(LLMProfile{
		Provider: "openrouter",
		Extra: map[string]string{
			" http_referer ": " https://nopsai.example.com ",
			"":               "ignored",
		},
	})
	if got := profile.Extra["http_referer"]; got != "https://nopsai.example.com" {
		t.Fatalf("normalized extra = %#v", profile.Extra)
	}
	if _, ok := profile.Extra[""]; ok {
		t.Fatalf("empty extra key preserved: %#v", profile.Extra)
	}
}

func TestNormalizeLMStudioReasoning(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty preserved", raw: "", want: ""},
		{name: "off preserved", raw: "off", want: "off"},
		{name: "bool false alias", raw: "false", want: "off"},
		{name: "bool true alias", raw: "true", want: "on"},
		{name: "mixed case normalized", raw: "Medium", want: "medium"},
		{name: "unknown passes through normalized", raw: "Custom", want: "custom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeLMStudioReasoning(tt.raw); got != tt.want {
				t.Fatalf("NormalizeLMStudioReasoning(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestEffectiveLLMProfileReasoningUsesThinking(t *testing.T) {
	enabled := true
	profile := NormalizeLLMProfile(LLMProfile{
		Provider: LLMProviderLMStudio,
		Thinking: &enabled,
	})

	if profile.Thinking == nil || !*profile.Thinking {
		t.Fatalf("NormalizeLLMProfile() did not preserve thinking")
	}
	if got := EffectiveLLMProfileReasoning(profile); got != "on" {
		t.Fatalf("EffectiveLLMProfileReasoning() = %q, want on", got)
	}
}

func TestEffectiveLLMProfileReasoningDefaultsLMStudioOff(t *testing.T) {
	profile := NormalizeLLMProfile(LLMProfile{Provider: LLMProviderLMStudio})

	if got := EffectiveLLMProfileReasoning(profile); got != "off" {
		t.Fatalf("EffectiveLLMProfileReasoning() = %q, want off", got)
	}
}

func TestEffectiveLLMProfileReasoningPrefersExplicitReasoning(t *testing.T) {
	enabled := false
	profile := LLMProfile{Reasoning: "high", Thinking: &enabled}

	if got := EffectiveLLMProfileReasoning(profile); got != "high" {
		t.Fatalf("EffectiveLLMProfileReasoning() = %q, want high", got)
	}
}

func TestEffectiveServiceJWTConfig(t *testing.T) {
	cfg := Config{
		JWTSigningKey: " user-jwt-key ",
		JWTIssuer:     " user-issuer ",
		JWTAudience:   " user-audience ",
	}

	if got := cfg.EffectiveServiceJWTSigningKey(); got != "user-jwt-key" {
		t.Fatalf("EffectiveServiceJWTSigningKey() = %q, want fallback key", got)
	}
	if got := cfg.EffectiveServiceJWTIssuer(); got != "user-issuer" {
		t.Fatalf("EffectiveServiceJWTIssuer() = %q, want fallback issuer", got)
	}
	if got := cfg.EffectiveServiceJWTAudience(); got != "nopsai-dispatcher" {
		t.Fatalf("EffectiveServiceJWTAudience() = %q, want dispatcher audience", got)
	}

	cfg.ServiceJWTSigningKey = " service-key "
	cfg.ServiceJWTIssuer = " service-issuer "
	cfg.ServiceJWTAudience = " service-audience "
	cfg.NopsaiServiceID = " control-plane "
	cfg.RunnerServiceID = " runner-node "
	cfg.AgentServiceID = " agent-worker "
	cfg.DispatcherTLSServerName = " dispatcher.internal "

	if got := cfg.EffectiveServiceJWTSigningKey(); got != "service-key" {
		t.Fatalf("EffectiveServiceJWTSigningKey() = %q, want service key", got)
	}
	if got := cfg.EffectiveServiceJWTIssuer(); got != "service-issuer" {
		t.Fatalf("EffectiveServiceJWTIssuer() = %q, want service issuer", got)
	}
	if got := cfg.EffectiveServiceJWTAudience(); got != "service-audience" {
		t.Fatalf("EffectiveServiceJWTAudience() = %q, want service audience", got)
	}
	if got := cfg.EffectiveNopsaiServiceID(); got != "control-plane" {
		t.Fatalf("EffectiveNopsaiServiceID() = %q, want configured ID", got)
	}
	if got := cfg.EffectiveRunnerServiceID(); got != "runner-node" {
		t.Fatalf("EffectiveRunnerServiceID() = %q, want configured ID", got)
	}
	if got := cfg.EffectiveAgentServiceID(); got != "agent-worker" {
		t.Fatalf("EffectiveAgentServiceID() = %q, want configured ID", got)
	}
	if got := cfg.EffectiveDispatcherTLSMode(); got != "mtls" {
		t.Fatalf("EffectiveDispatcherTLSMode() = %q, want mtls default", got)
	}
	if got := cfg.EffectiveDispatcherTLSSecret(); got != "service-key" {
		t.Fatalf("EffectiveDispatcherTLSSecret() = %q, want service key fallback", got)
	}
	if got := cfg.EffectiveDispatcherTLSServerName(); got != "dispatcher.internal" {
		t.Fatalf("EffectiveDispatcherTLSServerName() = %q, want configured server name", got)
	}

	cfg.DispatcherTLSMode = "server-tls"
	cfg.DispatcherTLSSecret = " tls-key "
	if got := cfg.EffectiveDispatcherTLSMode(); got != "tls" {
		t.Fatalf("EffectiveDispatcherTLSMode() = %q, want tls", got)
	}
	if got := cfg.EffectiveDispatcherTLSSecret(); got != "tls-key" {
		t.Fatalf("EffectiveDispatcherTLSSecret() = %q, want explicit tls key", got)
	}
}

func TestEffectiveAuthProviderLocalEnabledPrefersNestedAuthConfig(t *testing.T) {
	disabled := false
	cfg := Config{
		AuthProviderLocalEnabled: true,
		Auth: AuthConfig{
			LocalEnabled: &disabled,
		},
	}

	if cfg.EffectiveAuthProviderLocalEnabled() {
		t.Fatal("EffectiveAuthProviderLocalEnabled() = true, want nested false to win")
	}
}

func TestEffectiveOIDCAuthDoesNotInventDefaultRole(t *testing.T) {
	cfg := Config{
		Auth: AuthConfig{
			OIDC: OIDCAuthConfig{
				Enabled:         true,
				AutoCreateUsers: true,
			},
		},
	}

	if got := cfg.EffectiveOIDCAuth().DefaultRole; got != "" {
		t.Fatalf("EffectiveOIDCAuth().DefaultRole = %q, want empty", got)
	}

	cfg.Auth.OIDC.DefaultRole = " viewer "
	if got := cfg.EffectiveOIDCAuth().DefaultRole; got != "viewer" {
		t.Fatalf("EffectiveOIDCAuth().DefaultRole = %q, want trimmed configured role", got)
	}
}

func TestNormalizeAssistantConfigUsesMinimalDefaults(t *testing.T) {
	cfg := NormalizeAssistantConfig(AssistantConfig{
		Enabled:                   true,
		Provider:                  " Google-Gemini ",
		Model:                     " ",
		BaseURL:                   " https://proxy.example/v1 ",
		CredentialRef:             " credential://system/assistant/api-key ",
		LegacyAPIKeySecret:        " NOPSAI_ASSISTANT_API_KEY ",
		Timeout:                   "invalid",
		MaxInputLogsBytes:         -1,
		MaxConversationTurns:      0,
		DefaultDocsVersion:        " ",
		ConversationRetentionDays: -1,
		Memory: AssistantMemoryConfig{
			Enabled: true,
			Scope:   "global",
		},
	})

	if !cfg.Enabled {
		t.Fatal("assistant enabled flag should be preserved")
	}
	if cfg.DefaultDocsVersion != "auto" {
		t.Fatalf("default docs version = %q, want auto", cfg.DefaultDocsVersion)
	}
	if cfg.Provider != LLMProviderGemini {
		t.Fatalf("provider = %q, want gemini", cfg.Provider)
	}
	if cfg.Model != DefaultLLMProviderModel(LLMProviderGemini) {
		t.Fatalf("model = %q, want provider default", cfg.Model)
	}
	if cfg.BaseURL != "https://proxy.example/v1" {
		t.Fatalf("base URL = %q", cfg.BaseURL)
	}
	if cfg.CredentialRef != "credential://system/assistant/api-key" || cfg.LegacyAPIKeySecret != "NOPSAI_ASSISTANT_API_KEY" {
		t.Fatalf("credential fields = (%q, %q)", cfg.CredentialRef, cfg.LegacyAPIKeySecret)
	}
	if cfg.Timeout != "60s" {
		t.Fatalf("timeout = %q, want 60s", cfg.Timeout)
	}
	if cfg.MaxInputLogsBytes != 120000 || cfg.MaxConversationTurns != 30 {
		t.Fatalf("limits = (%d, %d)", cfg.MaxInputLogsBytes, cfg.MaxConversationTurns)
	}
	if cfg.ConversationRetentionDays != 30 {
		t.Fatalf("retention = %d, want 30", cfg.ConversationRetentionDays)
	}
	if !cfg.Memory.Enabled || cfg.Memory.Scope != "conversation" {
		t.Fatalf("memory = %#v, want enabled conversation scope", cfg.Memory)
	}
	if !AssistantMCPEnabled(cfg.MCP) {
		t.Fatalf("hosted MCP should default on for assistant tool orchestration: %#v", cfg.MCP)
	}
	if !AssistantFeatureFlagEnabled(cfg.DocsEnabled) || !AssistantFeatureFlagEnabled(cfg.DocsVersionAware) {
		t.Fatalf("docs flags = (%v, %v), want enabled", cfg.DocsEnabled, cfg.DocsVersionAware)
	}
	if !AssistantFeatureFlagEnabled(cfg.Features.Docs) ||
		!AssistantFeatureFlagEnabled(cfg.Features.PipelineDebugging) ||
		!AssistantFeatureFlagEnabled(cfg.Features.ConfigGeneration) ||
		!AssistantFeatureFlagEnabled(cfg.Features.StatisticsInsights) ||
		!AssistantFeatureFlagEnabled(cfg.Features.MaintenanceRecommendations) ||
		!AssistantFeatureFlagEnabled(cfg.Features.CostRecommendations) {
		t.Fatalf("default assistant feature flags should be enabled: %#v", cfg.Features)
	}
	if AssistantFeatureFlagEnabled(cfg.Features.ActionExecution) {
		t.Fatalf("action execution should default off: %#v", cfg.Features)
	}
	if !AssistantRequireConfirmation(cfg.Actions) {
		t.Fatalf("actions should require confirmation by default: %#v", cfg.Actions)
	}
}

func TestNormalizeAssistantConfigPreservesExplicitFeatureDisables(t *testing.T) {
	disabled := false
	enabled := true
	cfg := NormalizeAssistantConfig(AssistantConfig{
		DocsEnabled: &disabled,
		Features: AssistantFeaturesConfig{
			Docs:                       &disabled,
			PipelineDebugging:          &disabled,
			ConfigGeneration:           &enabled,
			StatisticsInsights:         &disabled,
			MaintenanceRecommendations: &disabled,
			CostRecommendations:        &disabled,
			ActionExecution:            &enabled,
		},
		MCP:     AssistantMCPConfig{Enabled: &disabled},
		Actions: AssistantActionsConfig{RequireConfirmation: &disabled},
	})

	if AssistantFeatureFlagEnabled(cfg.DocsEnabled) ||
		AssistantFeatureFlagEnabled(cfg.Features.Docs) ||
		AssistantFeatureFlagEnabled(cfg.Features.PipelineDebugging) ||
		AssistantFeatureFlagEnabled(cfg.Features.StatisticsInsights) ||
		AssistantFeatureFlagEnabled(cfg.Features.MaintenanceRecommendations) ||
		AssistantFeatureFlagEnabled(cfg.Features.CostRecommendations) {
		t.Fatalf("explicit feature disables were not preserved: %#v", cfg)
	}
	if !AssistantFeatureFlagEnabled(cfg.Features.ConfigGeneration) || !AssistantFeatureFlagEnabled(cfg.Features.ActionExecution) {
		t.Fatalf("explicit enabled flags were not preserved: %#v", cfg.Features)
	}
	if AssistantRequireConfirmation(cfg.Actions) {
		t.Fatalf("explicit relaxed confirmation policy was not preserved: %#v", cfg.Actions)
	}
	if AssistantMCPEnabled(cfg.MCP) {
		t.Fatalf("explicit hosted MCP disable was not preserved: %#v", cfg.MCP)
	}
}

func TestNormalizeAuthConfigNormalizesOIDCProviders(t *testing.T) {
	enabled := true
	allowEmailLinking := true
	cfg := NormalizeAuthConfig(AuthConfig{
		OIDC: OIDCAuthConfig{
			DefaultRole: " viewer ",
			DomainMapping: map[string]string{
				"@Company.COM": " Corporate ",
			},
			Providers: map[string]OIDCProviderConfig{
				" Corporate ": {
					Type:                "generic",
					DisplayName:         " Company SSO ",
					Issuer:              "https://idp.company.com/",
					ClientID:            " client-id ",
					Scopes:              []string{"email", "profile", "email"},
					AllowedEmailDomains: []string{"Company.COM", "@company.com"},
					RoleMapping: map[string]string{
						" nopsai-admins ": " admin ",
					},
					TeamMapping: map[string]string{
						" team-platform ": " Platform Engineers ",
					},
					BasicRoleMapping: map[string]OIDCBasicRoleGrantConfig{
						" team-1-owner ": {
							Role:     " Owner ",
							Resource: " team:team-1 ",
						},
					},
					EntitlementSync: OIDCEntitlementSyncConfig{
						Mode:                       "keycloak_team_roles",
						AdminBaseURL:               " http://keycloak:8080/ ",
						Realm:                      " nopsai ",
						AdminUsername:              " admin ",
						AdminPasswordCredentialRef: " credential://system/oidc/corporate/admin-password ",
						TargetResourceType:         "",
						TeamPathPrefix:             " /teams ",
					},
					AllowEmailLinking: &allowEmailLinking,
					Enabled:           &enabled,
				},
			},
		},
	})

	provider := cfg.OIDC.Providers["corporate"]
	if provider.Type != "oidc" {
		t.Fatalf("provider type = %q, want oidc", provider.Type)
	}
	if provider.Issuer != "https://idp.company.com" {
		t.Fatalf("issuer = %q, want trimmed issuer", provider.Issuer)
	}
	if got := cfg.OIDC.DomainMapping["company.com"]; got != "corporate" {
		t.Fatalf("domain mapping = %q, want corporate", got)
	}
	if len(provider.Scopes) != 3 || provider.Scopes[0] != "openid" {
		t.Fatalf("scopes = %#v, want openid plus normalized configured scopes", provider.Scopes)
	}
	if len(provider.AllowedEmailDomains) != 1 || provider.AllowedEmailDomains[0] != "company.com" {
		t.Fatalf("allowed domains = %#v, want company.com", provider.AllowedEmailDomains)
	}
	if provider.RoleMapping["nopsai-admins"] != "admin" {
		t.Fatalf("role mapping = %#v, want trimmed mapping", provider.RoleMapping)
	}
	if provider.TeamMapping["team-platform"] != "Platform Engineers" {
		t.Fatalf("team mapping = %#v, want trimmed mapping", provider.TeamMapping)
	}
	if provider.BasicRoleMapping["team-1-owner"].Role != "owner" || provider.BasicRoleMapping["team-1-owner"].Resource != "team:team-1" {
		t.Fatalf("basic role mapping = %#v, want trimmed scoped grant", provider.BasicRoleMapping)
	}
	if provider.EntitlementSync.Mode != "keycloak_team_roles" || provider.EntitlementSync.AdminBaseURL != "http://keycloak:8080" || provider.EntitlementSync.TargetResourceType != "team" || provider.EntitlementSync.TeamPathPrefix != "teams" {
		t.Fatalf("entitlement sync = %#v, want normalized Keycloak sync config", provider.EntitlementSync)
	}
	if provider.AllowEmailLinking == nil || !*provider.AllowEmailLinking {
		t.Fatalf("allow email linking = %#v, want true provider override", provider.AllowEmailLinking)
	}
}

func TestEffectiveEnvironmentAndProductionGates(t *testing.T) {
	cfg := Config{}
	if got := cfg.EffectiveEnvironment(); got != "development" {
		t.Fatalf("EffectiveEnvironment() = %q, want development", got)
	}
	if cfg.RequiresProductionGates() {
		t.Fatal("RequiresProductionGates() = true, want false for default environment")
	}

	cfg.Environment = " prod "
	if got := cfg.EffectiveEnvironment(); got != "production" {
		t.Fatalf("EffectiveEnvironment() = %q, want production", got)
	}
	if !cfg.RequiresProductionGates() {
		t.Fatal("RequiresProductionGates() = false, want true for production")
	}

	cfg.Environment = "staging"
	cfg.RequireProductionGates = true
	if got := cfg.EffectiveEnvironment(); got != "staging" {
		t.Fatalf("EffectiveEnvironment() = %q, want staging", got)
	}
	if !cfg.RequiresProductionGates() {
		t.Fatal("RequiresProductionGates() = false, want true when explicitly required")
	}
}

func TestNormalizeKubernetesRuntimeConfig(t *testing.T) {
	affinity := false
	cfg := NormalizeKubernetesConfig(KubernetesConfig{
		Namespace:                  " nopsai-runs ",
		ServiceAccount:             " nopsai-runner ",
		DefaultImagePullPolicy:     "if-not-present",
		DefaultWorkspaceSize:       " 5Gi ",
		DefaultWorkspaceAccessMode: "rwo",
		WorkspaceVolumeMode:        "existing-pvc",
		ExistingWorkspacePVC:       " shared-workspace ",
		StorageClass:               " fast-rwo ",
		AffinityEnabled:            &affinity,
		PodLabels:                  map[string]string{" team ": " platform "},
	})

	if cfg.Namespace != "nopsai-runs" || cfg.ServiceAccount != "nopsai-runner" {
		t.Fatalf("namespace/service account not normalized: %#v", cfg)
	}
	if cfg.DefaultImagePullPolicy != "IfNotPresent" || cfg.DefaultWorkspaceAccessMode != "ReadWriteOnce" {
		t.Fatalf("policy/access mode not normalized: %#v", cfg)
	}
	if cfg.WorkspaceVolumeMode != "existing" || cfg.ExistingWorkspacePVC != "shared-workspace" || cfg.StorageClass != "fast-rwo" {
		t.Fatalf("workspace settings not normalized: %#v", cfg)
	}
	if cfg.AffinityEnabled == nil || *cfg.AffinityEnabled {
		t.Fatalf("affinity pointer not preserved: %#v", cfg.AffinityEnabled)
	}
	if cfg.PodLabels["team"] != "platform" {
		t.Fatalf("pod labels not normalized: %#v", cfg.PodLabels)
	}
}

func TestNormalizeRuntimePools(t *testing.T) {
	pools := NormalizeRuntimePools(map[string]RuntimePool{
		" high-memory ": {
			NodeSelector: map[string]string{" workload ": " nopsai "},
			Resources: RuntimePoolResources{
				Requests: map[string]string{" memory ": " 4Gi "},
				Limits:   map[string]string{" memory ": " 16Gi "},
			},
		},
		"": {},
	})

	pool, ok := pools["high-memory"]
	if !ok {
		t.Fatalf("normalized pool missing: %#v", pools)
	}
	if pool.NodeSelector["workload"] != "nopsai" ||
		pool.Resources.Requests["memory"] != "4Gi" ||
		pool.Resources.Limits["memory"] != "16Gi" {
		t.Fatalf("pool not normalized: %#v", pool)
	}
}

func TestSystemLogsConfigurationRequiresAnExplicitProvider(t *testing.T) {
	cfg := Config{}
	if cfg.SystemLogsEnabled() {
		t.Fatal("SystemLogsEnabled() = true without a provider")
	}
	cfg.SystemLogs.DockerHost = "tcp://gitops-proxy:2375"
	if !cfg.SystemLogsEnabled() || cfg.EffectiveSystemLogsDockerHost() != "tcp://gitops-proxy:2375" {
		t.Fatalf("nested system log config not applied: %#v", cfg.SystemLogs)
	}
	cfg.SystemLogsDockerHost = "tcp://environment-proxy:2375"
	if cfg.EffectiveSystemLogsDockerHost() != "tcp://environment-proxy:2375" {
		t.Fatalf("environment Docker host did not override GitOps config")
	}
	disabled := false
	cfg.SystemLogs.Enabled = &disabled
	if cfg.SystemLogsEnabled() {
		t.Fatal("explicit disabled flag should override configured provider")
	}
}

func TestSystemLogsConfigurationSupportsKubernetesProvider(t *testing.T) {
	cfg := Config{SystemLogs: NormalizeSystemLogsConfig(SystemLogsConfig{
		Provider: " k8s ",
		Kubernetes: SystemLogsKubernetesConfig{
			Namespace:     " nopsai ",
			LabelSelector: " app.kubernetes.io/name=nopsai ",
			Container:     " api ",
		},
	})}

	if got := cfg.EffectiveSystemLogsProvider(); got != "kubernetes" {
		t.Fatalf("EffectiveSystemLogsProvider() = %q, want kubernetes", got)
	}
	if !cfg.SystemLogsEnabled() {
		t.Fatal("SystemLogsEnabled() = false, want true for Kubernetes provider")
	}
	if cfg.SystemLogs.Kubernetes.Namespace != "nopsai" ||
		cfg.SystemLogs.Kubernetes.LabelSelector != "app.kubernetes.io/name=nopsai" ||
		cfg.SystemLogs.Kubernetes.Container != "api" {
		t.Fatalf("Kubernetes system log config not normalized: %#v", cfg.SystemLogs.Kubernetes)
	}
}

func TestLoadConfigAppliesSystemLogsDockerHostEnvironment(t *testing.T) {
	t.Setenv("SYSTEM_LOGS_DOCKER_HOST", "tcp://proxy.internal:2375")
	cfg, err := LoadConfig("../config.yml")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if !cfg.SystemLogsEnabled() || cfg.EffectiveSystemLogsDockerHost() != "tcp://proxy.internal:2375" {
		t.Fatalf("system log environment not applied: %#v", cfg.SystemLogs)
	}
}

func TestLoadConfigAppliesCanonicalServiceEndpoints(t *testing.T) {
	t.Setenv("NOPSAI_API_URL", " http://nopsai-api.pre-nopsai:8080 ")
	t.Setenv("DISPATCHER_GRPC_ADDRESS", " dispatcher.pre-nopsai:9090 ")
	t.Setenv("AAA_LISTEN_ADDRESS", " 0.0.0.0:8082 ")
	t.Setenv("GIT_BOT_API_URL", " http://git-bot.pre-nopsai:8081 ")
	t.Setenv("SYSTEM_LOGS_PROVIDER", " kubernetes ")
	t.Setenv("SYSTEM_LOGS_KUBERNETES_NAMESPACE", " nopsai ")
	t.Setenv("SYSTEM_LOGS_KUBERNETES_LABEL_SELECTOR", " app.kubernetes.io/name=nopsai ")

	cfg, err := LoadConfig("../config.yml")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.NopsaiAPIURL != "http://nopsai-api.pre-nopsai:8080" || cfg.AgentNopsaiAPIURL != "http://nopsai-api.pre-nopsai:8080" || cfg.GitBotNopsaiAPIURL != "http://nopsai-api.pre-nopsai:8080" {
		t.Fatalf("NopsAI API endpoint = (%q, %q, %q)", cfg.NopsaiAPIURL, cfg.AgentNopsaiAPIURL, cfg.GitBotNopsaiAPIURL)
	}
	if cfg.DispatcherAddress != "dispatcher.pre-nopsai:9090" || cfg.AAAAddr != "0.0.0.0:8082" || cfg.NopsaiGitBotAPIURL != "http://git-bot.pre-nopsai:8081" {
		t.Fatalf("service endpoints not applied: dispatcher=%q aaa=%q git-bot=%q", cfg.DispatcherAddress, cfg.AAAAddr, cfg.NopsaiGitBotAPIURL)
	}
	if cfg.EffectiveSystemLogsProvider() != "kubernetes" || cfg.SystemLogs.Kubernetes.Namespace != "nopsai" {
		t.Fatalf("system log Kubernetes env aliases not applied: %#v", cfg.SystemLogs)
	}
}
