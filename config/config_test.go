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
					GroupMapping: map[string]string{
						" team-platform ": " Platform Engineers ",
					},
					BasicRoleMapping: map[string]OIDCBasicRoleGrantConfig{
						" team-1-owner ": {
							Role:     " Owner ",
							Resource: " folder:team-1 ",
						},
					},
					EntitlementSync: OIDCEntitlementSyncConfig{
						Mode:               "keycloak",
						AdminBaseURL:       " http://keycloak:8080/ ",
						Realm:              " nopsai ",
						AdminUsername:      " admin ",
						AdminPassword:      " admin ",
						TargetResourceType: "",
						GroupPathPrefix:    " /teams ",
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
	if provider.GroupMapping["team-platform"] != "Platform Engineers" {
		t.Fatalf("group mapping = %#v, want trimmed mapping", provider.GroupMapping)
	}
	if provider.BasicRoleMapping["team-1-owner"].Role != "owner" || provider.BasicRoleMapping["team-1-owner"].Resource != "folder:team-1" {
		t.Fatalf("basic role mapping = %#v, want trimmed scoped grant", provider.BasicRoleMapping)
	}
	if provider.EntitlementSync.Mode != "keycloak_group_roles" || provider.EntitlementSync.AdminBaseURL != "http://keycloak:8080" || provider.EntitlementSync.TargetResourceType != "folder" || provider.EntitlementSync.GroupPathPrefix != "teams" {
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
