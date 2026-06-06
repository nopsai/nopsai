package nopsai

import "time"

type authLoginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type authRefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type authLoginResponse struct {
	AccessToken        string                    `json:"access_token"`
	RefreshToken       string                    `json:"refresh_token,omitempty"`
	ExpiresAt          time.Time                 `json:"expires_at"`
	Roles              []string                  `json:"roles,omitempty"`
	Provider           string                    `json:"provider,omitempty"`
	Email              string                    `json:"email,omitempty"`
	Sub                string                    `json:"sub,omitempty"`
	MustChangePassword bool                      `json:"must_change_password,omitempty"`
	Capabilities       *authCapabilitiesResponse `json:"capabilities,omitempty"`
}

type authCapabilitiesResponse struct {
	Pipelines        authResourceCapabilities `json:"pipelines"`
	Steps            authResourceCapabilities `json:"steps"`
	Schedules        authReadCapabilities     `json:"schedules"`
	Triggers         authReadCapabilities     `json:"triggers"`
	ExternalTriggers authReadCapabilities     `json:"external_triggers"`
	Scopes           authReadCapabilities     `json:"scopes"`
	Knowledge        authReadCapabilities     `json:"knowledge_contexts"`
	System           authSystemCapabilities   `json:"system"`
}

type authResourceCapabilities struct {
	Write  bool `json:"write"`
	Delete bool `json:"delete"`
}

type authReadCapabilities struct {
	Read   bool `json:"read"`
	Write  bool `json:"write"`
	Delete bool `json:"delete"`
}

type authSystemCapabilities struct {
	ConfigRead       bool `json:"config_read"`
	ConfigWrite      bool `json:"config_write"`
	LLMProfilesRead  bool `json:"llm_profiles_read"`
	LLMProfilesWrite bool `json:"llm_profiles_write"`
	MCPRead          bool `json:"mcp_read"`
	MCPWrite         bool `json:"mcp_write"`
	ConfigReposRead  bool `json:"config_repos_read"`
	ConfigReposWrite bool `json:"config_repos_write"`
	DispatcherRead   bool `json:"dispatcher_read"`
	DispatcherWrite  bool `json:"dispatcher_write"`
	Access           bool `json:"access"`
}

type authChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type authUpdateEmailRequest struct {
	Email string `json:"email"`
}

type authPersonalTokenRequest struct {
	Name          string `json:"name"`
	ExpiresInDays int    `json:"expires_in_days"`
	ExpiresAt     string `json:"expires_at"`
	NeverExpires  bool   `json:"never_expires"`
}

type authPersonalTokenResponse struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Token       string     `json:"token,omitempty"`
	TokenSuffix string     `json:"token_suffix"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

type serviceAccountTokenRequest struct {
	Name          string `json:"name"`
	ExpiresInDays int    `json:"expires_in_days"`
	ExpiresAt     string `json:"expires_at"`
	NeverExpires  bool   `json:"never_expires"`
}

type serviceAccountTokenResponse struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Token       string     `json:"token,omitempty"`
	TokenSuffix string     `json:"token_suffix"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

type userRoleBinding struct {
	Role string `json:"role"`
}

type userSummary struct {
	ID        string            `json:"id"`
	Sub       string            `json:"sub"`
	Email     string            `json:"email"`
	Provider  string            `json:"provider"`
	Status    string            `json:"status"`
	LastLogin *time.Time        `json:"last_login,omitempty"`
	Roles     []userRoleBinding `json:"roles,omitempty"`
}

type serviceAccountSummary struct {
	ID         string            `json:"id"`
	Sub        string            `json:"sub"`
	Email      string            `json:"email"`
	Provider   string            `json:"provider"`
	Status     string            `json:"status"`
	TokenCount int               `json:"token_count"`
	LastUsedAt *time.Time        `json:"last_used_at,omitempty"`
	Roles      []userRoleBinding `json:"roles,omitempty"`
}

type createUserRequest struct {
	Sub      string `json:"sub"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type createServiceAccountRequest struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	TokenName     string `json:"token_name"`
	ExpiresInDays int    `json:"expires_in_days"`
	ExpiresAt     string `json:"expires_at"`
	NeverExpires  bool   `json:"never_expires"`
}

type createServiceAccountResponse struct {
	ServiceAccount serviceAccountSummary       `json:"service_account"`
	Token          serviceAccountTokenResponse `json:"token"`
}

type updateUserRequest struct {
	Email    string `json:"email"`
	Status   string `json:"status"`
	Password string `json:"password"`
}

type updateServiceAccountRequest struct {
	Email  string `json:"email"`
	Status string `json:"status"`
}

type userRoleRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

type serviceAccountRoleRequest struct {
	ServiceAccountID string `json:"service_account_id"`
	Role             string `json:"role"`
}

type createRoleRequest struct {
	Role         string `json:"role"`
	Name         string `json:"name"`
	Object       string `json:"obj"`
	Action       string `json:"act"`
	Effect       string `json:"effect"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
}
