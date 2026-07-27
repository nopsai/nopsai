package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nopsai/services/nopsai/internal/credentials"
)

type oauth2TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

type githubUserResponse struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type githubEmailResponse struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

type githubTeamResponse struct {
	ID           int64  `json:"id"`
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	Organization struct {
		Login string `json:"login"`
	} `json:"organization"`
}

func (a *App) handleAuthOAuth2Start(w http.ResponseWriter, r *http.Request) {
	providerID := normalizeOIDCProviderID(r.PathValue("provider"))
	settings, err := getOIDCSettings(r.Context(), a.db, a.cfg)
	if err != nil {
		http.Error(w, "failed to load auth settings", http.StatusInternalServerError)
		return
	}
	if !settings.OIDCEnabled {
		http.Error(w, "external authentication is disabled", http.StatusNotFound)
		return
	}
	provider, err := getOIDCProvider(r.Context(), a.db, providerID)
	if err != nil {
		http.Error(w, "provider not found", http.StatusNotFound)
		return
	}
	if !provider.Enabled || !providerUsesOAuth2(provider) {
		http.Error(w, "provider is disabled", http.StatusNotFound)
		return
	}
	state, err := generateOIDCSecret()
	if err != nil {
		http.Error(w, "failed to create oauth2 state", http.StatusInternalServerError)
		return
	}
	nonce, err := generateOIDCSecret()
	if err != nil {
		http.Error(w, "failed to create oauth2 nonce", http.StatusInternalServerError)
		return
	}
	codeVerifier, err := generateOIDCSecret()
	if err != nil {
		http.Error(w, "failed to create pkce verifier", http.StatusInternalServerError)
		return
	}
	returnTo := safeReturnTo(r.URL.Query().Get("return_to"))
	if err := createOIDCState(r.Context(), a.db, provider.ID, state, nonce, codeVerifier, returnTo, time.Now().Add(oidcStateTTL)); err != nil {
		http.Error(w, "failed to persist oauth2 state", http.StatusInternalServerError)
		return
	}
	values := url.Values{}
	values.Set("client_id", provider.ClientID)
	values.Set("redirect_uri", a.oauth2CallbackURL(r, provider.ID))
	values.Set("scope", strings.Join(provider.Scopes, " "))
	values.Set("state", state)
	values.Set("code_challenge", codeChallenge(codeVerifier))
	values.Set("code_challenge_method", "S256")
	redirectURL := firstNonEmptyString(provider.AuthorizationEndpoint, "https://github.com/login/oauth/authorize")
	http.Redirect(w, r, appendQuery(redirectURL, values), http.StatusFound)
}

func (a *App) handleAuthOAuth2Callback(w http.ResponseWriter, r *http.Request) {
	providerID := normalizeOIDCProviderID(r.PathValue("provider"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		a.auditOIDC(r, providerID, "", "auth.oauth2.failure", "failure", map[string]any{"reason": "missing_code_or_state"})
		http.Error(w, "missing oauth2 code or state", http.StatusBadRequest)
		return
	}
	settings, err := getOIDCSettings(r.Context(), a.db, a.cfg)
	if err != nil {
		http.Error(w, "failed to load auth settings", http.StatusInternalServerError)
		return
	}
	if !settings.OIDCEnabled {
		http.Error(w, "external authentication is disabled", http.StatusNotFound)
		return
	}
	provider, err := getOIDCProvider(r.Context(), a.db, providerID)
	if err != nil {
		a.auditOIDC(r, providerID, "", "auth.oauth2.failure", "failure", map[string]any{"reason": "provider_not_found"})
		http.Error(w, "provider not found", http.StatusNotFound)
		return
	}
	if !providerUsesOAuth2(provider) {
		http.Error(w, "provider does not use oauth2", http.StatusBadRequest)
		return
	}
	stateRecord, err := consumeOIDCState(r.Context(), a.db, provider.ID, state)
	if err != nil {
		a.auditOIDC(r, provider.ID, "", "auth.oauth2.failure", "failure", map[string]any{"reason": "invalid_state"})
		http.Error(w, "invalid oauth2 state", http.StatusBadRequest)
		return
	}
	token, err := a.exchangeOAuth2Code(r.Context(), provider, code, stateRecord.CodeVerifier, a.oauth2CallbackURL(r, provider.ID))
	if err != nil {
		a.auditOIDC(r, provider.ID, "", "auth.oauth2.failure", "failure", map[string]any{"reason": "token_exchange_failed"})
		http.Error(w, "oauth2 token exchange failed", http.StatusBadGateway)
		return
	}
	identity, err := a.verifyOAuth2Identity(r.Context(), provider, token.AccessToken)
	if err != nil {
		a.auditOIDC(r, provider.ID, "", "auth.oauth2.failure", "failure", map[string]any{"reason": "identity_lookup_failed"})
		http.Error(w, "oauth2 identity lookup failed", http.StatusBadGateway)
		return
	}
	resolution, err := resolveOIDCUser(r.Context(), a.db, settings, provider, identity)
	if err != nil {
		a.auditOIDC(r, provider.ID, identity.Email, "auth.oauth2.failure", "failure", map[string]any{"reason": "user_resolution_failed"})
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
	a.auditOIDC(r, provider.ID, identity.Email, "auth.oauth2.login", "success", map[string]any{
		"created_user": resolution.Created,
		"linked_user":  resolution.Linked,
	})
	http.Redirect(w, r, callbackRedirectURL(stateRecord.ReturnTo, loginCode), http.StatusFound)
}

func (a *App) exchangeOAuth2Code(ctx context.Context, provider oidcProviderRecord, code, codeVerifier, redirectURI string) (oauth2TokenResponse, error) {
	var token oauth2TokenResponse
	tokenEndpoint := firstNonEmptyString(provider.TokenEndpoint, "https://github.com/login/oauth/access_token")
	clientSecret, err := a.resolveCredentialText(ctx, provider.ClientCredentialRef, credentials.Purpose{
		ConsumerService: "nopsai",
		Operation:       "oauth2.token_exchange",
		SubjectType:     "identity_provider",
		SubjectID:       provider.ID,
	})
	if err != nil {
		return token, fmt.Errorf("resolve OAuth2 client credential: %w", err)
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", provider.ClientID)
	form.Set("code_verifier", codeVerifier)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return token, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := a.oidcHTTPClient().Do(req)
	if err != nil {
		return token, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return token, fmt.Errorf("oauth2 token exchange failed with status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return token, err
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return token, fmt.Errorf("oauth2 token response did not include access_token")
	}
	return token, nil
}

func (a *App) verifyOAuth2Identity(ctx context.Context, provider oidcProviderRecord, accessToken string) (oidcVerifiedIdentity, error) {
	if normalizeOIDCProviderType(provider.Type) != "github" {
		return oidcVerifiedIdentity{}, fmt.Errorf("unsupported oauth2 provider %q", provider.Type)
	}
	return a.githubOAuth2Identity(ctx, provider, accessToken)
}

func (a *App) githubOAuth2Identity(ctx context.Context, provider oidcProviderRecord, accessToken string) (oidcVerifiedIdentity, error) {
	userEndpoint := firstNonEmptyString(provider.UserInfoEndpoint, "https://api.github.com/user")
	var user githubUserResponse
	if err := a.githubGET(ctx, accessToken, userEndpoint, &user); err != nil {
		return oidcVerifiedIdentity{}, err
	}
	subject := strconv.FormatInt(user.ID, 10)
	if subject == "0" {
		subject = strings.TrimSpace(user.Login)
	}
	if subject == "" {
		return oidcVerifiedIdentity{}, fmt.Errorf("github user response did not include id or login")
	}
	email := strings.TrimSpace(user.Email)
	emailVerified := email != ""
	if email == "" {
		resolved, verified, err := a.githubPrimaryEmail(ctx, accessToken)
		if err != nil {
			return oidcVerifiedIdentity{}, err
		}
		email = resolved
		emailVerified = verified
	}
	if email != "" && !emailDomainAllowed(emailDomain(email), provider.AllowedEmailDomains) {
		return oidcVerifiedIdentity{}, fmt.Errorf("email domain is not allowed for this provider")
	}
	teams, err := a.githubTeams(ctx, accessToken)
	if err != nil {
		teams = nil
	}
	return oidcVerifiedIdentity{
		ProviderID:    provider.ID,
		Issuer:        firstNonEmptyString(provider.Issuer, "https://github.com"),
		Subject:       subject,
		Email:         email,
		EmailVerified: emailVerified,
		Teams:         teams,
	}, nil
}

func (a *App) githubPrimaryEmail(ctx context.Context, accessToken string) (string, bool, error) {
	var emails []githubEmailResponse
	if err := a.githubGET(ctx, accessToken, "https://api.github.com/user/emails", &emails); err != nil {
		return "", false, err
	}
	for _, email := range emails {
		if email.Primary && email.Verified && strings.TrimSpace(email.Email) != "" {
			return strings.TrimSpace(email.Email), true, nil
		}
	}
	for _, email := range emails {
		if email.Verified && strings.TrimSpace(email.Email) != "" {
			return strings.TrimSpace(email.Email), true, nil
		}
	}
	return "", false, fmt.Errorf("github account has no verified email visible to NopsAI")
}

func (a *App) githubTeams(ctx context.Context, accessToken string) ([]string, error) {
	var teams []githubTeamResponse
	if err := a.githubGET(ctx, accessToken, "https://api.github.com/user/teams", &teams); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(teams))
	for _, team := range teams {
		org := strings.TrimSpace(team.Organization.Login)
		slug := strings.TrimSpace(team.Slug)
		if org == "" || slug == "" {
			continue
		}
		out = appendOIDCTeam(out, org+"/"+slug)
	}
	return out, nil
}

func (a *App) githubGET(ctx context.Context, accessToken, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := a.oidcHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("github api request failed with status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (a *App) oauth2CallbackURL(r *http.Request, providerID string) string {
	path := "/v1/auth/oauth2/" + url.PathEscape(providerID) + "/callback"
	if a != nil && a.cfg != nil && strings.TrimSpace(a.cfg.PublicURL) != "" {
		return strings.TrimRight(strings.TrimSpace(a.cfg.PublicURL), "/") + path
	}
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = "http"
	}
	return scheme + "://" + r.Host + path
}
