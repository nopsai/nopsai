package nopsai

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"

	"gopkg.in/yaml.v3"
)

const authGitOpsPath = "system/auth.yaml"

type authSettingsGitOpsFile struct {
	Auth         *config.AuthConfig     `json:"auth,omitempty" yaml:"auth,omitempty"`
	LocalEnabled *bool                  `json:"local_enabled,omitempty" yaml:"local_enabled,omitempty"`
	OIDC         *config.OIDCAuthConfig `json:"oidc,omitempty" yaml:"oidc,omitempty"`
}

type gitOpsAuthSettingsPlan struct {
	auth           config.AuthConfig
	settings       oidcSettings
	providers      map[string]oidcProviderRecord
	domainMappings map[string]string
	sourcePath     string
}

func parseGitOpsAuthSettingsPlan(binding models.ConfigRepository, directories ...gitOpsRuntimeSettingsDirectory) (*gitOpsAuthSettingsPlan, error) {
	var candidates []gitOpsRuntimeSettingsFileCandidate
	for _, directory := range directories {
		root := filepath.ToSlash(strings.Trim(directory.root, "/"))
		for path, content := range directory.files {
			normalized := filepath.ToSlash(path)
			rel, ok := configsync.RelativePath(normalized, root)
			if !ok || !isGitOpsAuthSettingsRelativePath(rel) {
				continue
			}
			candidates = append(candidates, gitOpsRuntimeSettingsFileCandidate{sourcePath: normalized, content: content})
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	if binding.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil, fmt.Errorf("auth settings can only be configured from a system config repository")
	}
	if len(candidates) > 1 {
		paths := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			paths = append(paths, candidate.sourcePath)
		}
		sort.Strings(paths)
		return nil, fmt.Errorf("multiple auth settings GitOps files found: %s", strings.Join(paths, ", "))
	}
	return parseGitOpsAuthSettingsFile(candidates[0].content, candidates[0].sourcePath)
}

func isGitOpsAuthSettingsRelativePath(rel string) bool {
	return strings.Trim(filepath.ToSlash(rel), "/") == authGitOpsPath
}

func parseGitOpsAuthSettingsFile(content, sourcePath string) (*gitOpsAuthSettingsPlan, error) {
	var file authSettingsGitOpsFile
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		return nil, fmt.Errorf("failed to parse auth settings GitOps file '%s': %w", sourcePath, err)
	}

	hasWrappedAuth := file.Auth != nil
	hasTopLevelAuth := file.LocalEnabled != nil || file.OIDC != nil
	if hasWrappedAuth && hasTopLevelAuth {
		return nil, fmt.Errorf("auth settings GitOps file '%s' must use either auth: or top-level auth fields, not both", sourcePath)
	}

	var authCfg config.AuthConfig
	switch {
	case hasWrappedAuth:
		authCfg = *file.Auth
	case hasTopLevelAuth:
		authCfg.LocalEnabled = file.LocalEnabled
		if file.OIDC != nil {
			authCfg.OIDC = *file.OIDC
		}
	default:
		return nil, fmt.Errorf("auth settings GitOps file '%s' must define auth settings", sourcePath)
	}
	if authCfg.LocalEnabled != nil && !*authCfg.LocalEnabled {
		return nil, fmt.Errorf("auth settings GitOps file '%s' cannot disable local authentication", sourcePath)
	}

	authCfg = config.NormalizeAuthConfig(authCfg)
	settings, providers, domainMappings, err := oidcGitOpsStateFromAuthConfig(authCfg)
	if err != nil {
		return nil, fmt.Errorf("auth settings GitOps file '%s' is invalid: %w", sourcePath, err)
	}

	return &gitOpsAuthSettingsPlan{
		auth:           authCfg,
		settings:       settings,
		providers:      providers,
		domainMappings: domainMappings,
		sourcePath:     sourcePath,
	}, nil
}

func oidcGitOpsStateFromAuthConfig(authCfg config.AuthConfig) (oidcSettings, map[string]oidcProviderRecord, map[string]string, error) {
	localEnabled := true

	settings := oidcSettings{
		LocalEnabled:      localEnabled,
		OIDCEnabled:       authCfg.OIDC.Enabled,
		AutoCreateUsers:   authCfg.OIDC.AutoCreateUsers,
		DefaultRole:       strings.TrimSpace(authCfg.OIDC.DefaultRole),
		AllowEmailLinking: authCfg.OIDC.AllowEmailLinking,
	}

	providers := make(map[string]oidcProviderRecord, len(authCfg.OIDC.Providers))
	providerIDs := make(map[string]struct{}, len(authCfg.OIDC.Providers))
	for id, providerCfg := range authCfg.OIDC.Providers {
		provider := oidcProviderRecordFromConfig(id, providerCfg, authProviderSourceGitOps)
		if provider.ID == "" {
			continue
		}
		if providerUsesOAuth2(provider) {
			if provider.ClientID == "" {
				return oidcSettings{}, nil, nil, fmt.Errorf("provider %q requires client_id", provider.ID)
			}
		} else if provider.Issuer == "" || provider.ClientID == "" {
			return oidcSettings{}, nil, nil, fmt.Errorf("provider %q requires issuer and client_id", provider.ID)
		}
		providers[provider.ID] = provider
		providerIDs[provider.ID] = struct{}{}
	}

	domainMappings := normalizeOIDCDomainMappings(authCfg.OIDC.DomainMapping)
	for domain, providerID := range domainMappings {
		if _, ok := providerIDs[providerID]; !ok {
			return oidcSettings{}, nil, nil, fmt.Errorf("domain mapping %q references unknown provider %q", domain, providerID)
		}
	}

	return settings, providers, domainMappings, nil
}

func oidcProviderRecordFromConfig(id string, providerCfg config.OIDCProviderConfig, source string) oidcProviderRecord {
	enabled := true
	if providerCfg.Enabled != nil {
		enabled = *providerCfg.Enabled
	}
	return oidcProviderRecord{
		ID:                    normalizeOIDCProviderID(id),
		Type:                  normalizeOIDCProviderType(providerCfg.Type),
		DisplayName:           firstNonEmpty(strings.TrimSpace(providerCfg.DisplayName), normalizeOIDCProviderID(id)),
		Issuer:                strings.TrimRight(strings.TrimSpace(providerCfg.Issuer), "/"),
		AuthorizationEndpoint: strings.TrimSpace(providerCfg.AuthorizationEndpoint),
		TokenEndpoint:         strings.TrimSpace(providerCfg.TokenEndpoint),
		JWKSURI:               strings.TrimSpace(providerCfg.JWKSURI),
		UserInfoEndpoint:      strings.TrimSpace(providerCfg.UserInfoEndpoint),
		ClientID:              strings.TrimSpace(providerCfg.ClientID),
		ClientCredentialRef:   strings.TrimSpace(providerCfg.ClientCredentialRef),
		Scopes:                normalizeExternalProviderScopes(providerCfg.Type, providerCfg.Scopes),
		AllowedEmailDomains:   normalizeOIDCEmailDomains(providerCfg.AllowedEmailDomains),
		TeamClaim:             strings.TrimSpace(providerCfg.TeamClaim),
		RoleMapping:           normalizeOIDCRoleMapping(providerCfg.RoleMapping),
		TeamMapping:           normalizeOIDCTeamMapping(providerCfg.TeamMapping),
		BasicRoleMapping:      normalizeOIDCBasicRoleMapping(basicRoleMappingFromConfig(providerCfg.BasicRoleMapping)),
		EntitlementSync:       normalizeOIDCEntitlementSync(entitlementSyncFromConfig(providerCfg.EntitlementSync)),
		AutoCreateUsers:       providerCfg.AutoCreateUsers,
		DefaultRole:           strings.TrimSpace(providerCfg.DefaultRole),
		AllowEmailLinking:     providerCfg.AllowEmailLinking,
		Enabled:               enabled,
		ConfigSource:          source,
	}
}

func (a *App) applyAuthSettingsGitOpsPlan(ctx context.Context, plan *gitOpsAuthSettingsPlan) error {
	if plan == nil {
		return nil
	}
	if a != nil && a.cfg != nil {
		a.cfgMu.Lock()
		a.cfg.Auth = config.NormalizeAuthConfig(plan.auth)
		a.cfgMu.Unlock()
	}
	if a == nil || a.db == nil {
		return nil
	}

	if err := upsertOIDCSettings(ctx, a.db, plan.settings); err != nil {
		return err
	}

	providerIDs := make([]string, 0, len(plan.providers))
	for id := range plan.providers {
		providerIDs = append(providerIDs, id)
	}
	sort.Strings(providerIDs)
	for _, id := range providerIDs {
		provider := plan.providers[id]
		provider.ConfigSource = authProviderSourceGitOps
		if err := upsertOIDCProvider(ctx, a.db, provider, false); err != nil {
			return err
		}
	}

	if err := replaceOIDCDomainMappings(ctx, a.db, plan.domainMappings, authProviderSourceGitOps); err != nil {
		return err
	}

	existing, err := listOIDCProviders(ctx, a.db, false)
	if err != nil {
		return err
	}
	managedProviderIDs := map[string]struct{}{}
	for _, id := range providerIDs {
		managedProviderIDs[id] = struct{}{}
	}
	for _, provider := range existing {
		if provider.ConfigSource != authProviderSourceGitOps {
			continue
		}
		if _, ok := managedProviderIDs[provider.ID]; ok {
			continue
		}
		if err := deleteOIDCProvider(ctx, a.db, provider.ID); err != nil {
			return err
		}
	}

	if err := reconcileOIDCAuthTeamMappings(ctx, a.db); err != nil {
		return err
	}
	return reconcileOIDCBasicRoleMappings(ctx, a.db)
}

func buildAuthSettingsGitOpsFile(settings oidcSettings, providers []oidcProviderRecord, mappings map[string]string) authSettingsGitOpsFile {
	doc := authSettingsGitOpsFile{
		LocalEnabled: boolPtr(true),
		OIDC: &config.OIDCAuthConfig{
			Enabled:           settings.OIDCEnabled,
			AutoCreateUsers:   settings.AutoCreateUsers,
			DefaultRole:       strings.TrimSpace(settings.DefaultRole),
			AllowEmailLinking: settings.AllowEmailLinking,
			DomainMapping:     normalizeOIDCDomainMappings(mappings),
			Providers:         map[string]config.OIDCProviderConfig{},
		},
	}

	sort.Slice(providers, func(i, j int) bool {
		return providers[i].ID < providers[j].ID
	})
	for _, provider := range providers {
		provider.ID = normalizeOIDCProviderID(provider.ID)
		if provider.ID == "" {
			continue
		}
		doc.OIDC.Providers[provider.ID] = oidcProviderConfigFromRecord(provider)
	}
	if len(doc.OIDC.Providers) == 0 {
		doc.OIDC.Providers = nil
	}
	return doc
}

func oidcProviderConfigFromRecord(provider oidcProviderRecord) config.OIDCProviderConfig {
	entitlementSync := provider.EntitlementSync

	cfg := config.OIDCProviderConfig{
		Type:                  normalizeOIDCProviderType(provider.Type),
		DisplayName:           strings.TrimSpace(provider.DisplayName),
		Issuer:                strings.TrimRight(strings.TrimSpace(provider.Issuer), "/"),
		AuthorizationEndpoint: strings.TrimSpace(provider.AuthorizationEndpoint),
		TokenEndpoint:         strings.TrimSpace(provider.TokenEndpoint),
		JWKSURI:               strings.TrimSpace(provider.JWKSURI),
		UserInfoEndpoint:      strings.TrimSpace(provider.UserInfoEndpoint),
		ClientID:              strings.TrimSpace(provider.ClientID),
		ClientCredentialRef:   strings.TrimSpace(provider.ClientCredentialRef),
		Scopes:                normalizeExternalProviderScopes(provider.Type, provider.Scopes),
		AllowedEmailDomains:   normalizeOIDCEmailDomains(provider.AllowedEmailDomains),
		TeamClaim:             strings.TrimSpace(provider.TeamClaim),
		RoleMapping:           normalizeOIDCRoleMapping(provider.RoleMapping),
		TeamMapping:           normalizeOIDCTeamMapping(provider.TeamMapping),
		BasicRoleMapping:      basicRoleMappingToConfig(provider.BasicRoleMapping),
		EntitlementSync:       entitlementSyncToConfig(entitlementSync),
		AutoCreateUsers:       provider.AutoCreateUsers,
		DefaultRole:           strings.TrimSpace(provider.DefaultRole),
		AllowEmailLinking:     provider.AllowEmailLinking,
	}
	if !provider.Enabled {
		cfg.Enabled = boolPtr(false)
	}
	return config.NormalizeAuthConfig(config.AuthConfig{
		OIDC: config.OIDCAuthConfig{Providers: map[string]config.OIDCProviderConfig{provider.ID: cfg}},
	}).OIDC.Providers[normalizeOIDCProviderID(provider.ID)]
}

func basicRoleMappingToConfig(mapping map[string]oidcBasicRoleGrantMapping) map[string]config.OIDCBasicRoleGrantConfig {
	if len(mapping) == 0 {
		return nil
	}
	out := make(map[string]config.OIDCBasicRoleGrantConfig, len(mapping))
	for team, grant := range mapping {
		out[team] = config.OIDCBasicRoleGrantConfig{
			Role:         grant.Role,
			Resource:     grant.Resource,
			ResourceType: grant.ResourceType,
			ResourceID:   grant.ResourceID,
		}
	}
	return out
}

func entitlementSyncToConfig(sync oidcEntitlementSyncConfig) config.OIDCEntitlementSyncConfig {
	return config.OIDCEntitlementSyncConfig{
		Mode:                       sync.Mode,
		AdminBaseURL:               sync.AdminBaseURL,
		Realm:                      sync.Realm,
		AdminRealm:                 sync.AdminRealm,
		AdminClientID:              sync.AdminClientID,
		AdminClientCredentialRef:   sync.AdminClientCredentialRef,
		AdminUsername:              sync.AdminUsername,
		AdminPasswordCredentialRef: sync.AdminPasswordCredentialRef,
		ClientID:                   sync.ClientID,
		TargetResourceType:         sync.TargetResourceType,
		TeamPathPrefix:             sync.TeamPathPrefix,
	}
}

func authSettingsGitOpsFileHasState(doc authSettingsGitOpsFile) bool {
	if doc.LocalEnabled != nil && !*doc.LocalEnabled {
		return true
	}
	if doc.OIDC == nil {
		return false
	}
	return doc.OIDC.Enabled ||
		doc.OIDC.AutoCreateUsers ||
		doc.OIDC.AllowEmailLinking ||
		strings.TrimSpace(doc.OIDC.DefaultRole) != "" ||
		len(doc.OIDC.DomainMapping) > 0 ||
		len(doc.OIDC.Providers) > 0
}
