package nopsai

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	authSettingsKeyOIDC        = "oidc"
	authProviderSourceConfig   = "config"
	authProviderSourceDatabase = "database"
	authProviderSourceGitOps   = "gitops"
	oidcStateTTL               = 10 * time.Minute
	oidcLoginCodeTTL           = 2 * time.Minute

	oidcEmailVerificationNotProvided = "not_provided"
	oidcEmailVerificationUnknown     = "unknown"
	oidcEmailVerificationUnverified  = "unverified"
	oidcEmailVerificationVerified    = "verified"
)

type oidcSettings struct {
	LocalEnabled      bool   `json:"local_enabled"`
	OIDCEnabled       bool   `json:"oidc_enabled"`
	AutoCreateUsers   bool   `json:"auto_create_users"`
	DefaultRole       string `json:"default_role"`
	AllowEmailLinking bool   `json:"allow_email_linking"`
}

type oidcProviderRecord struct {
	ID                    string                               `json:"id"`
	Type                  string                               `json:"type"`
	DisplayName           string                               `json:"display_name"`
	Issuer                string                               `json:"issuer"`
	AuthorizationEndpoint string                               `json:"authorization_endpoint,omitempty"`
	TokenEndpoint         string                               `json:"token_endpoint,omitempty"`
	JWKSURI               string                               `json:"jwks_uri,omitempty"`
	UserInfoEndpoint      string                               `json:"userinfo_endpoint,omitempty"`
	ClientID              string                               `json:"client_id,omitempty"`
	ClientCredentialRef   string                               `json:"client_credential_ref,omitempty"`
	Scopes                []string                             `json:"scopes"`
	AllowedEmailDomains   []string                             `json:"allowed_email_domains"`
	TeamClaim             string                               `json:"team_claim,omitempty"`
	RoleMapping           map[string]string                    `json:"role_mapping,omitempty"`
	TeamMapping           map[string]string                    `json:"team_mapping,omitempty"`
	BasicRoleMapping      map[string]oidcBasicRoleGrantMapping `json:"basic_role_mapping,omitempty"`
	EntitlementSync       oidcEntitlementSyncConfig            `json:"entitlement_sync,omitempty"`
	AutoCreateUsers       *bool                                `json:"auto_create_users,omitempty"`
	DefaultRole           string                               `json:"default_role,omitempty"`
	AllowEmailLinking     *bool                                `json:"allow_email_linking,omitempty"`
	Enabled               bool                                 `json:"enabled"`
	ConfigSource          string                               `json:"config_source"`
	Capabilities          identityProviderCapabilities         `json:"capabilities"`
	CreatedAt             time.Time                            `json:"created_at"`
	UpdatedAt             time.Time                            `json:"updated_at"`
}

type identityProviderCapabilities struct {
	Authentication bool `json:"authentication"`
	Provisioning   bool `json:"provisioning"`
	GroupSync      bool `json:"group_sync"`
	RoleSync       bool `json:"role_sync"`
	DirectorySync  bool `json:"directory_sync"`
}

type oidcPublicProvider struct {
	ID                  string   `json:"id"`
	Type                string   `json:"type"`
	DisplayName         string   `json:"display_name"`
	Scopes              []string `json:"scopes,omitempty"`
	AllowedEmailDomains []string `json:"allowed_email_domains,omitempty"`
	AuthURLKind         string   `json:"auth_url_kind"`
}

type oidcProvidersResponse struct {
	LocalEnabled bool                 `json:"local_enabled"`
	OIDCEnabled  bool                 `json:"oidc_enabled"`
	Providers    []oidcPublicProvider `json:"providers"`
}

type oidcDiscoverRequest struct {
	Email string `json:"email"`
}

type oidcDiscoverResponse struct {
	Found    bool                `json:"found"`
	Provider *oidcPublicProvider `json:"provider,omitempty"`
}

type oidcExchangeRequest struct {
	Code string `json:"code"`
}

type oidcAdminProviderResponse struct {
	oidcProviderRecord
	HasClientCredential bool `json:"has_client_credential"`
}

type oidcAdminResponse struct {
	Settings       oidcSettings                `json:"settings"`
	Providers      []oidcAdminProviderResponse `json:"providers"`
	DomainMappings map[string]string           `json:"domain_mappings"`
}

type oidcSettingsRequest struct {
	LocalEnabled      *bool             `json:"local_enabled"`
	OIDCEnabled       *bool             `json:"oidc_enabled"`
	AutoCreateUsers   *bool             `json:"auto_create_users"`
	DefaultRole       *string           `json:"default_role"`
	AllowEmailLinking *bool             `json:"allow_email_linking"`
	DomainMappings    map[string]string `json:"domain_mappings"`
}

type oidcProviderRequest struct {
	ID                    string                               `json:"id"`
	Type                  string                               `json:"type"`
	DisplayName           string                               `json:"display_name"`
	Issuer                string                               `json:"issuer"`
	AuthorizationEndpoint string                               `json:"authorization_endpoint"`
	TokenEndpoint         string                               `json:"token_endpoint"`
	JWKSURI               string                               `json:"jwks_uri"`
	UserInfoEndpoint      string                               `json:"userinfo_endpoint"`
	ClientID              string                               `json:"client_id"`
	ClientCredentialRef   string                               `json:"client_credential_ref"`
	Scopes                []string                             `json:"scopes"`
	AllowedEmailDomains   []string                             `json:"allowed_email_domains"`
	TeamClaim             string                               `json:"team_claim"`
	RoleMapping           map[string]string                    `json:"role_mapping"`
	TeamMapping           map[string]string                    `json:"team_mapping"`
	BasicRoleMapping      map[string]oidcBasicRoleGrantMapping `json:"basic_role_mapping"`
	EntitlementSync       oidcEntitlementSyncConfig            `json:"entitlement_sync"`
	AutoCreateUsers       *bool                                `json:"auto_create_users"`
	DefaultRole           string                               `json:"default_role"`
	AllowEmailLinking     *bool                                `json:"allow_email_linking"`
	Enabled               *bool                                `json:"enabled"`
}

type oidcStateRecord struct {
	ProviderID   string
	NonceHash    string
	CodeVerifier string
	ReturnTo     string
	ExpiresAt    time.Time
}

type oidcVerifiedIdentity struct {
	ProviderID                      string
	Issuer                          string
	Subject                         string
	Email                           string
	EmailVerified                   bool
	EmailVerificationStatus         string
	EmailVerificationClaimMalformed bool
	Teams                           []string
	AccessRoles                     []string
	BasicRoles                      []oidcDesiredBasicRoleGrant
}

type oidcBasicRoleGrantMapping struct {
	Role         string `json:"role"`
	Resource     string `json:"resource,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
}

type oidcEntitlementSyncConfig struct {
	Mode                       string `json:"mode,omitempty"`
	AdminBaseURL               string `json:"admin_base_url,omitempty"`
	Realm                      string `json:"realm,omitempty"`
	AdminRealm                 string `json:"admin_realm,omitempty"`
	AdminClientID              string `json:"admin_client_id,omitempty"`
	AdminClientCredentialRef   string `json:"admin_client_credential_ref,omitempty"`
	AdminUsername              string `json:"admin_username,omitempty"`
	AdminPasswordCredentialRef string `json:"admin_password_credential_ref,omitempty"`
	ClientID                   string `json:"client_id,omitempty"`
	TargetResourceType         string `json:"target_resource_type,omitempty"`
	TeamPathPrefix             string `json:"team_path_prefix,omitempty"`
}

type oidcUserResolution struct {
	UserID  uuid.UUID
	Linked  bool
	Created bool
}

func normalizeOIDCEmailVerificationStatus(status, email string, emailVerified bool) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case oidcEmailVerificationNotProvided:
		return oidcEmailVerificationNotProvided
	case oidcEmailVerificationUnknown:
		return oidcEmailVerificationUnknown
	case oidcEmailVerificationUnverified:
		return oidcEmailVerificationUnverified
	case oidcEmailVerificationVerified:
		return oidcEmailVerificationVerified
	}
	if strings.TrimSpace(email) == "" {
		return oidcEmailVerificationNotProvided
	}
	if emailVerified {
		return oidcEmailVerificationVerified
	}
	return oidcEmailVerificationUnknown
}

func (identity oidcVerifiedIdentity) normalizedEmailVerificationStatus() string {
	return normalizeOIDCEmailVerificationStatus(identity.EmailVerificationStatus, identity.Email, identity.EmailVerified)
}

func publicProviderFromRecord(provider oidcProviderRecord) oidcPublicProvider {
	return oidcPublicProvider{
		ID:                  provider.ID,
		Type:                provider.Type,
		DisplayName:         provider.DisplayName,
		Scopes:              append([]string(nil), provider.Scopes...),
		AllowedEmailDomains: append([]string(nil), provider.AllowedEmailDomains...),
		AuthURLKind:         authURLKindForProvider(provider),
	}
}

func publicProvidersFromRecords(records []oidcProviderRecord) []oidcPublicProvider {
	out := make([]oidcPublicProvider, 0, len(records))
	for _, record := range records {
		if !record.Enabled {
			continue
		}
		out = append(out, publicProviderFromRecord(record))
	}
	return out
}

func normalizeOIDCProviderID(raw string) string {
	id := strings.ToLower(strings.TrimSpace(raw))
	id = strings.ReplaceAll(id, " ", "-")
	return id
}

func normalizeOIDCProviderType(raw string) string {
	typ := strings.ToLower(strings.TrimSpace(raw))
	switch typ {
	case "", "generic":
		return "oidc"
	case "okta":
		return "okta"
	case "keycloak":
		return "keycloak"
	case "github", "github-oauth", "oauth2-github":
		return "github"
	case "entra", "entra-id", "azure", "azure-ad", "microsoft-entra":
		return "microsoft"
	case "google-workspace":
		return "google"
	default:
		return typ
	}
}

func providerUsesOAuth2(provider oidcProviderRecord) bool {
	return normalizeOIDCProviderType(provider.Type) == "github"
}

func authURLKindForProvider(provider oidcProviderRecord) string {
	if providerUsesOAuth2(provider) {
		return "oauth2"
	}
	return "oidc"
}

func normalizeOIDCEmailDomain(raw string) string {
	domain := strings.ToLower(strings.TrimSpace(raw))
	return strings.TrimPrefix(domain, "@")
}

func normalizeOIDCEmailDomains(domains []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(domains))
	for _, domain := range domains {
		domain = normalizeOIDCEmailDomain(domain)
		if domain == "" || seen[domain] {
			continue
		}
		seen[domain] = true
		out = append(out, domain)
	}
	return out
}

func normalizeOIDCScopes(scopes []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(scopes)+3)
	add := func(scope string) {
		scope = strings.TrimSpace(scope)
		if scope == "" || seen[scope] {
			return
		}
		seen[scope] = true
		out = append(out, scope)
	}
	add("openid")
	for _, scope := range scopes {
		add(scope)
	}
	if len(out) == 1 {
		add("email")
		add("profile")
	}
	return out
}

func normalizeExternalProviderScopes(providerType string, scopes []string) []string {
	if normalizeOIDCProviderType(providerType) == "github" {
		seen := map[string]bool{}
		out := make([]string, 0, len(scopes)+3)
		add := func(scope string) {
			scope = strings.TrimSpace(scope)
			if scope == "" || seen[scope] {
				return
			}
			seen[scope] = true
			out = append(out, scope)
		}
		for _, scope := range scopes {
			add(scope)
		}
		if len(out) == 0 {
			add("read:user")
			add("user:email")
			add("read:org")
		}
		return out
	}
	return normalizeOIDCScopes(scopes)
}

func normalizeOIDCRoleMapping(mapping map[string]string) map[string]string {
	out := make(map[string]string, len(mapping))
	for team, role := range mapping {
		team = strings.TrimSpace(team)
		role = strings.TrimSpace(role)
		if team == "" || role == "" {
			continue
		}
		out[team] = role
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeOIDCTeamMapping(mapping map[string]string) map[string]string {
	out := make(map[string]string, len(mapping))
	for externalTeam, authTeam := range mapping {
		externalTeam = strings.TrimSpace(externalTeam)
		authTeam = strings.TrimSpace(authTeam)
		if externalTeam == "" || authTeam == "" {
			continue
		}
		out[externalTeam] = authTeam
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeOIDCBasicRoleMapping(mapping map[string]oidcBasicRoleGrantMapping) map[string]oidcBasicRoleGrantMapping {
	out := make(map[string]oidcBasicRoleGrantMapping, len(mapping))
	for externalTeam, grant := range mapping {
		externalTeam = strings.TrimSpace(externalTeam)
		grant.Role = strings.ToLower(strings.TrimSpace(grant.Role))
		grant.Resource = strings.TrimSpace(grant.Resource)
		grant.ResourceType = strings.TrimSpace(grant.ResourceType)
		grant.ResourceID = strings.TrimSpace(grant.ResourceID)
		if externalTeam == "" || grant.Role == "" {
			continue
		}
		if grant.Resource == "" && (grant.ResourceType == "" || grant.ResourceID == "") {
			continue
		}
		if grant.Resource != "" {
			resourceType, resourceID, ok := strings.Cut(grant.Resource, ":")
			if ok {
				grant.ResourceType = strings.TrimSpace(resourceType)
				grant.ResourceID = strings.TrimSpace(resourceID)
			}
		}
		role, err := normalizeProductRoleName(grant.Role)
		if err != nil || role == productRoleAdmin {
			continue
		}
		grant.Role = role
		if strings.TrimSpace(grant.ResourceType) == "" || strings.TrimSpace(grant.ResourceID) == "" {
			continue
		}
		out[externalTeam] = grant
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeOIDCEntitlementSync(sync oidcEntitlementSyncConfig) oidcEntitlementSyncConfig {
	sync.Mode = strings.ToLower(strings.TrimSpace(sync.Mode))
	sync.AdminBaseURL = strings.TrimRight(strings.TrimSpace(sync.AdminBaseURL), "/")
	sync.Realm = strings.TrimSpace(sync.Realm)
	sync.AdminRealm = strings.TrimSpace(sync.AdminRealm)
	if sync.AdminRealm == "" {
		sync.AdminRealm = "master"
	}
	sync.AdminClientID = strings.TrimSpace(sync.AdminClientID)
	if sync.AdminClientID == "" {
		sync.AdminClientID = "admin-cli"
	}
	sync.AdminClientCredentialRef = strings.TrimSpace(sync.AdminClientCredentialRef)
	sync.AdminUsername = strings.TrimSpace(sync.AdminUsername)
	sync.AdminPasswordCredentialRef = strings.TrimSpace(sync.AdminPasswordCredentialRef)
	sync.ClientID = strings.TrimSpace(sync.ClientID)
	sync.TargetResourceType = strings.TrimSpace(sync.TargetResourceType)
	if sync.TargetResourceType == "" {
		sync.TargetResourceType = grantResourceTeam
	}
	sync.TeamPathPrefix = strings.Trim(strings.TrimSpace(sync.TeamPathPrefix), "/")
	if sync.Mode == "" && sync.AdminBaseURL == "" {
		return oidcEntitlementSyncConfig{}
	}
	return sync
}

func normalizeOIDCDomainMappings(mapping map[string]string) map[string]string {
	out := make(map[string]string, len(mapping))
	for domain, providerID := range mapping {
		domain = normalizeOIDCEmailDomain(domain)
		providerID = normalizeOIDCProviderID(providerID)
		if domain == "" || providerID == "" {
			continue
		}
		out[domain] = providerID
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
