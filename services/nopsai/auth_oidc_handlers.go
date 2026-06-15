package nopsai

import (
	"database/sql"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"nopsai/pkg/httpapi"
	"nopsai/services/nopsai/pkg/audit"

	"github.com/jackc/pgx/v5"
)

func (a *App) handleAuthProviders(w http.ResponseWriter, r *http.Request) {
	settings, err := getOIDCSettings(r.Context(), a.db, a.cfg)
	if err != nil {
		http.Error(w, "failed to load auth settings", http.StatusInternalServerError)
		return
	}
	providers, err := listOIDCProviders(r.Context(), a.db, true)
	if err != nil {
		http.Error(w, "failed to load auth providers", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, oidcProvidersResponse{
		LocalEnabled: settings.LocalEnabled,
		OIDCEnabled:  settings.OIDCEnabled,
		Providers:    publicProvidersFromRecords(providers),
	})
}

func (a *App) handleAuthDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req oidcDiscoverRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	email, err := normalizeDiscoveryEmail(req.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	settings, err := getOIDCSettings(r.Context(), a.db, a.cfg)
	if err != nil {
		http.Error(w, "failed to load auth settings", http.StatusInternalServerError)
		return
	}
	if !settings.OIDCEnabled {
		_ = httpapi.WriteJSON(w, http.StatusOK, oidcDiscoverResponse{Found: false})
		return
	}
	provider, found, err := findOIDCProviderForEmail(r.Context(), a.db, email)
	if err != nil {
		http.Error(w, "failed to discover provider", http.StatusInternalServerError)
		return
	}
	if !found {
		_ = httpapi.WriteJSON(w, http.StatusOK, oidcDiscoverResponse{Found: false})
		return
	}
	public := publicProviderFromRecord(provider)
	_ = httpapi.WriteJSON(w, http.StatusOK, oidcDiscoverResponse{
		Found:    true,
		Provider: &public,
	})
}

func (a *App) handleAuthOIDCStart(w http.ResponseWriter, r *http.Request) {
	providerID := normalizeOIDCProviderID(r.PathValue("provider"))
	settings, err := getOIDCSettings(r.Context(), a.db, a.cfg)
	if err != nil {
		http.Error(w, "failed to load auth settings", http.StatusInternalServerError)
		return
	}
	if !settings.OIDCEnabled {
		http.Error(w, "oidc authentication is disabled", http.StatusNotFound)
		return
	}
	provider, err := getOIDCProvider(r.Context(), a.db, providerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "provider not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load provider", http.StatusInternalServerError)
		return
	}
	if !provider.Enabled {
		http.Error(w, "provider is disabled", http.StatusNotFound)
		return
	}
	metadata, err := a.discoverOIDCMetadata(r.Context(), provider)
	if err != nil {
		http.Error(w, "failed to discover provider metadata", http.StatusBadGateway)
		return
	}
	if metadata.AuthorizationEndpoint == "" {
		http.Error(w, "provider authorization endpoint is not configured", http.StatusBadGateway)
		return
	}
	state, err := generateOIDCSecret()
	if err != nil {
		http.Error(w, "failed to create oidc state", http.StatusInternalServerError)
		return
	}
	nonce, err := generateOIDCSecret()
	if err != nil {
		http.Error(w, "failed to create oidc nonce", http.StatusInternalServerError)
		return
	}
	codeVerifier, err := generateOIDCSecret()
	if err != nil {
		http.Error(w, "failed to create pkce verifier", http.StatusInternalServerError)
		return
	}
	returnTo := safeReturnTo(r.URL.Query().Get("return_to"))
	if err := createOIDCState(r.Context(), a.db, provider.ID, state, nonce, codeVerifier, returnTo, time.Now().Add(oidcStateTTL)); err != nil {
		http.Error(w, "failed to persist oidc state", http.StatusInternalServerError)
		return
	}
	values := url.Values{}
	values.Set("client_id", provider.ClientID)
	values.Set("redirect_uri", a.oidcCallbackURL(r, provider.ID))
	values.Set("response_type", "code")
	values.Set("scope", strings.Join(provider.Scopes, " "))
	values.Set("state", state)
	values.Set("nonce", nonce)
	values.Set("code_challenge", codeChallenge(codeVerifier))
	values.Set("code_challenge_method", "S256")
	if prompt := strings.TrimSpace(r.URL.Query().Get("prompt")); prompt != "" {
		values.Set("prompt", prompt)
	}
	redirectURL := metadata.AuthorizationEndpoint
	if strings.Contains(redirectURL, "?") {
		redirectURL += "&" + values.Encode()
	} else {
		redirectURL += "?" + values.Encode()
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (a *App) handleAuthOIDCCallback(w http.ResponseWriter, r *http.Request) {
	providerID := normalizeOIDCProviderID(r.PathValue("provider"))
	providerError := strings.TrimSpace(r.URL.Query().Get("error"))
	if providerError != "" {
		errorDescription := strings.TrimSpace(r.URL.Query().Get("error_description"))
		a.auditOIDC(r, providerID, "", "auth.oidc.failure", "failure", map[string]any{
			"reason":           "authorization_error",
			"provider_error":   providerError,
			"provider_message": errorDescription,
		})
		http.Error(w, oidcAuthorizationErrorMessage(providerError, errorDescription), http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		a.auditOIDC(r, providerID, "", "auth.oidc.failure", "failure", map[string]any{"reason": "missing_code_or_state"})
		http.Error(w, "missing oidc code or state", http.StatusBadRequest)
		return
	}
	settings, err := getOIDCSettings(r.Context(), a.db, a.cfg)
	if err != nil {
		http.Error(w, "failed to load auth settings", http.StatusInternalServerError)
		return
	}
	if !settings.OIDCEnabled {
		http.Error(w, "oidc authentication is disabled", http.StatusNotFound)
		return
	}
	provider, err := getOIDCProvider(r.Context(), a.db, providerID)
	if err != nil {
		a.auditOIDC(r, providerID, "", "auth.oidc.failure", "failure", map[string]any{"reason": "provider_not_found"})
		http.Error(w, "provider not found", http.StatusNotFound)
		return
	}
	stateRecord, err := consumeOIDCState(r.Context(), a.db, provider.ID, state)
	if err != nil {
		a.auditOIDC(r, provider.ID, "", "auth.oidc.failure", "failure", map[string]any{"reason": "invalid_state"})
		http.Error(w, "invalid oidc state", http.StatusBadRequest)
		return
	}
	metadata, err := a.discoverOIDCMetadata(r.Context(), provider)
	if err != nil {
		a.auditOIDC(r, provider.ID, "", "auth.oidc.failure", "failure", map[string]any{"reason": "discovery_failed"})
		http.Error(w, "failed to discover provider metadata", http.StatusBadGateway)
		return
	}
	tokenResponse, err := a.exchangeOIDCCode(r.Context(), provider, metadata, code, stateRecord.CodeVerifier, a.oidcCallbackURL(r, provider.ID))
	if err != nil {
		a.auditOIDC(r, provider.ID, "", "auth.oidc.failure", "failure", map[string]any{"reason": "token_exchange_failed"})
		http.Error(w, "oidc token exchange failed", http.StatusBadGateway)
		return
	}
	identity, err := a.verifyOIDCIDToken(r.Context(), provider, metadata, tokenResponse.IDToken, stateRecord.NonceHash)
	if err != nil {
		a.auditOIDC(r, provider.ID, "", "auth.oidc.failure", "failure", map[string]any{"reason": "id_token_validation_failed"})
		http.Error(w, "oidc id token validation failed", http.StatusUnauthorized)
		return
	}
	identity, err = a.enrichOIDCIdentityEntitlements(r.Context(), provider, identity)
	if err != nil {
		a.auditOIDC(r, provider.ID, identity.Email, "auth.oidc.failure", "failure", map[string]any{"reason": "entitlement_sync_failed"})
		http.Error(w, "oidc entitlement sync failed", http.StatusBadGateway)
		return
	}
	resolution, err := resolveOIDCUser(r.Context(), a.db, settings, provider, identity)
	if err != nil {
		a.auditOIDC(r, provider.ID, identity.Email, "auth.oidc.failure", "failure", map[string]any{"reason": "user_resolution_failed"})
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	loginCode, err := generateOIDCSecret()
	if err != nil {
		http.Error(w, "failed to create session exchange code", http.StatusInternalServerError)
		return
	}
	if err := createOIDCLoginCode(r.Context(), a.db, resolution.UserID, provider.ID, loginCode, stateRecord.ReturnTo, time.Now().Add(oidcLoginCodeTTL)); err != nil {
		http.Error(w, "failed to persist session exchange code", http.StatusInternalServerError)
		return
	}
	a.auditOIDC(r, provider.ID, identity.Email, "auth.oidc.login", "success", map[string]any{
		"created_user": resolution.Created,
		"linked_user":  resolution.Linked,
	})
	http.Redirect(w, r, callbackRedirectURL(stateRecord.ReturnTo, loginCode), http.StatusFound)
}

func oidcAuthorizationErrorMessage(providerError, description string) string {
	if description == "" {
		return "oidc authorization failed: " + providerError
	}
	return "oidc authorization failed: " + providerError + " (" + description + ")"
}

func (a *App) handleAuthSessionExchange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req oidcExchangeRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		http.Error(w, "code is required", http.StatusBadRequest)
		return
	}
	userID, _, err := consumeOIDCLoginCode(r.Context(), a.db, code)
	if err != nil {
		http.Error(w, "invalid or expired session exchange code", http.StatusUnauthorized)
		return
	}
	result, err := a.authService.IssueSessionForUser(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to issue session", http.StatusUnauthorized)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, authLoginResponse{
		AccessToken:        result.AccessToken,
		RefreshToken:       result.RefreshToken,
		ExpiresAt:          result.ExpiresAt,
		Roles:              a.resolveAAARoles(r.Context(), result.Claims),
		Provider:           result.Claims.Provider,
		Email:              result.Claims.Email,
		Sub:                result.Claims.Sub,
		MustChangePassword: result.MustChangePassword,
	})
}

func (a *App) handleAdminIdentityProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.handleListAdminIdentityProviders(w, r)
	case http.MethodPut:
		a.handleUpdateAdminIdentityProviderSettings(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleAdminIdentityProvider(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		a.handleUpsertAdminIdentityProvider(w, r)
	case http.MethodDelete:
		a.handleDeleteAdminIdentityProvider(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleListAdminIdentityProviders(w http.ResponseWriter, r *http.Request) {
	settings, err := getOIDCSettings(r.Context(), a.db, a.cfg)
	if err != nil {
		http.Error(w, "failed to load auth settings", http.StatusInternalServerError)
		return
	}
	providers, err := listOIDCProviders(r.Context(), a.db, false)
	if err != nil {
		http.Error(w, "failed to load identity providers", http.StatusInternalServerError)
		return
	}
	mappings, err := listOIDCDomainMappings(r.Context(), a.db)
	if err != nil {
		http.Error(w, "failed to load domain mappings", http.StatusInternalServerError)
		return
	}
	out := make([]oidcAdminProviderResponse, 0, len(providers))
	for _, provider := range providers {
		out = append(out, oidcAdminProviderResponse{
			oidcProviderRecord:  provider,
			HasClientCredential: strings.TrimSpace(provider.ClientCredentialRef) != "",
		})
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, oidcAdminResponse{
		Settings:       settings,
		Providers:      out,
		DomainMappings: mappings,
	})
}

func (a *App) handleUpdateAdminIdentityProviderSettings(w http.ResponseWriter, r *http.Request) {
	var req oidcSettingsRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	current, err := getOIDCSettings(r.Context(), a.db, a.cfg)
	if err != nil {
		http.Error(w, "failed to load auth settings", http.StatusInternalServerError)
		return
	}
	if req.LocalEnabled != nil {
		current.LocalEnabled = *req.LocalEnabled
	}
	if req.OIDCEnabled != nil {
		current.OIDCEnabled = *req.OIDCEnabled
	}
	if req.AutoCreateUsers != nil {
		current.AutoCreateUsers = *req.AutoCreateUsers
	}
	if req.AllowEmailLinking != nil {
		current.AllowEmailLinking = *req.AllowEmailLinking
	}
	if req.DefaultRole != nil {
		current.DefaultRole = strings.TrimSpace(*req.DefaultRole)
	}
	if err := upsertOIDCSettings(r.Context(), a.db, current); err != nil {
		http.Error(w, "failed to save auth settings", http.StatusInternalServerError)
		return
	}
	if req.DomainMappings != nil {
		if err := replaceOIDCDomainMappings(r.Context(), a.db, req.DomainMappings, authProviderSourceDatabase); err != nil {
			http.Error(w, "failed to save domain mappings", http.StatusBadRequest)
			return
		}
	}
	a.handleListAdminIdentityProviders(w, r)
}

func (a *App) handleUpsertAdminIdentityProvider(w http.ResponseWriter, r *http.Request) {
	providerID := normalizeOIDCProviderID(r.PathValue("provider"))
	var req oidcProviderRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		req.ID = providerID
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	entitlementSync := normalizeOIDCEntitlementSync(req.EntitlementSync)
	if entitlementSync.Mode == "" && entitlementSync.AdminBaseURL == "" {
		if existing, err := getOIDCProvider(r.Context(), a.db, providerID); err == nil {
			entitlementSync = existing.EntitlementSync
		}
	}
	provider := oidcProviderRecord{
		ID:                    normalizeOIDCProviderID(req.ID),
		Type:                  normalizeOIDCProviderType(req.Type),
		DisplayName:           strings.TrimSpace(req.DisplayName),
		Issuer:                strings.TrimRight(strings.TrimSpace(req.Issuer), "/"),
		AuthorizationEndpoint: strings.TrimSpace(req.AuthorizationEndpoint),
		TokenEndpoint:         strings.TrimSpace(req.TokenEndpoint),
		JWKSURI:               strings.TrimSpace(req.JWKSURI),
		UserInfoEndpoint:      strings.TrimSpace(req.UserInfoEndpoint),
		ClientID:              strings.TrimSpace(req.ClientID),
		ClientCredentialRef:   strings.TrimSpace(req.ClientCredentialRef),
		Scopes:                normalizeOIDCScopes(req.Scopes),
		AllowedEmailDomains:   normalizeOIDCEmailDomains(req.AllowedEmailDomains),
		GroupClaim:            strings.TrimSpace(req.GroupClaim),
		RoleMapping:           normalizeOIDCRoleMapping(req.RoleMapping),
		GroupMapping:          normalizeOIDCGroupMapping(req.GroupMapping),
		BasicRoleMapping:      normalizeOIDCBasicRoleMapping(req.BasicRoleMapping),
		EntitlementSync:       entitlementSync,
		AutoCreateUsers:       req.AutoCreateUsers,
		DefaultRole:           strings.TrimSpace(req.DefaultRole),
		AllowEmailLinking:     req.AllowEmailLinking,
		Enabled:               enabled,
		ConfigSource:          authProviderSourceDatabase,
	}
	if provider.ID == "" || provider.ID != providerID {
		http.Error(w, "provider id is invalid", http.StatusBadRequest)
		return
	}
	if err := a.ensureOIDCProviderCredentialReferences(r.Context(), provider, credentialActor(r)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := upsertOIDCProvider(r.Context(), a.db, provider, false); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := reconcileOIDCAuthGroupMappings(r.Context(), a.db); err != nil {
		http.Error(w, "failed to reconcile identity provider group mappings", http.StatusInternalServerError)
		return
	}
	if err := reconcileOIDCBasicRoleMappings(r.Context(), a.db); err != nil {
		http.Error(w, "failed to reconcile identity provider basic role mappings", http.StatusInternalServerError)
		return
	}
	a.handleListAdminIdentityProviders(w, r)
}

func (a *App) handleDeleteAdminIdentityProvider(w http.ResponseWriter, r *http.Request) {
	providerID := normalizeOIDCProviderID(r.PathValue("provider"))
	if err := deleteOIDCProvider(r.Context(), a.db, providerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "provider not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to delete provider", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func normalizeDiscoveryEmail(raw string) (string, error) {
	email := strings.TrimSpace(raw)
	if email == "" {
		return "", errors.New("email is required")
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil {
		return "", errors.New("invalid email")
	}
	if emailDomain(parsed.Address) == "" {
		return "", errors.New("invalid email")
	}
	return parsed.Address, nil
}

func (a *App) auditOIDC(r *http.Request, providerID, email, action, result string, metadata map[string]any) {
	if a == nil || a.auditLogger == nil || r == nil {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["provider"] = providerID
	_ = a.auditLogger.Write(r.Context(), audit.Entry{
		ActorEmail: email,
		Provider:   "oidc:" + providerID,
		Action:     action,
		Resource:   "auth",
		Result:     result,
		Metadata:   metadata,
	})
}
