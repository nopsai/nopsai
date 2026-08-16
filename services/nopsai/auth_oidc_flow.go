package nopsai

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/pkg/auth"

	"github.com/golang-jwt/jwt/v5"
)

type oidcMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

type oidcTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
}

type oidcJWKS struct {
	Keys []oidcJWK `json:"keys"`
}

type oidcJWK struct {
	Kty string   `json:"kty"`
	Kid string   `json:"kid"`
	Use string   `json:"use"`
	Alg string   `json:"alg"`
	N   string   `json:"n"`
	E   string   `json:"e"`
	X5C []string `json:"x5c"`
}

func (a *App) discoverOIDCMetadata(ctx context.Context, provider oidcProviderRecord) (oidcMetadata, error) {
	metadata := oidcMetadata{
		Issuer:                provider.Issuer,
		AuthorizationEndpoint: provider.AuthorizationEndpoint,
		TokenEndpoint:         provider.TokenEndpoint,
		JWKSURI:               provider.JWKSURI,
		UserInfoEndpoint:      provider.UserInfoEndpoint,
	}
	if metadata.AuthorizationEndpoint != "" && metadata.TokenEndpoint != "" && metadata.JWKSURI != "" {
		return metadata, nil
	}
	if provider.Issuer == "" {
		return metadata, fmt.Errorf("provider issuer is required")
	}
	discoveryURL := strings.TrimRight(provider.Issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return metadata, err
	}
	resp, err := a.oidcHTTPClient().Do(req)
	if err != nil {
		return metadata, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return metadata, fmt.Errorf("oidc discovery failed with status %d", resp.StatusCode)
	}
	var discovered oidcMetadata
	if err := json.NewDecoder(resp.Body).Decode(&discovered); err != nil {
		return metadata, err
	}
	if metadata.AuthorizationEndpoint == "" {
		metadata.AuthorizationEndpoint = discovered.AuthorizationEndpoint
	}
	if metadata.TokenEndpoint == "" {
		metadata.TokenEndpoint = discovered.TokenEndpoint
	}
	if metadata.JWKSURI == "" {
		metadata.JWKSURI = discovered.JWKSURI
	}
	if metadata.UserInfoEndpoint == "" {
		metadata.UserInfoEndpoint = discovered.UserInfoEndpoint
	}
	if metadata.EndSessionEndpoint == "" {
		metadata.EndSessionEndpoint = discovered.EndSessionEndpoint
	}
	return metadata, nil
}

func (a *App) discoverOIDCEndSessionEndpoint(ctx context.Context, provider oidcProviderRecord) (string, error) {
	issuer := strings.TrimRight(strings.TrimSpace(provider.Issuer), "/")
	if issuer == "" {
		return "", fmt.Errorf("provider issuer is required")
	}
	discoveryURL := issuer + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := a.oidcHTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("oidc discovery failed with status %d", resp.StatusCode)
	}
	var discovered oidcMetadata
	if err := json.NewDecoder(resp.Body).Decode(&discovered); err != nil {
		return "", err
	}
	return strings.TrimSpace(discovered.EndSessionEndpoint), nil
}

func (a *App) exchangeOIDCCode(ctx context.Context, provider oidcProviderRecord, metadata oidcMetadata, code, codeVerifier, redirectURI string) (oidcTokenResponse, error) {
	var tokenResponse oidcTokenResponse
	if metadata.TokenEndpoint == "" {
		return tokenResponse, fmt.Errorf("provider token endpoint is not configured")
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", provider.ClientID)
	form.Set("code_verifier", codeVerifier)
	clientSecret, err := a.resolveCredentialText(ctx, provider.ClientCredentialRef, credentials.Purpose{
		ConsumerService: "nopsai",
		Operation:       "oidc.token_exchange",
		SubjectType:     "identity_provider",
		SubjectID:       provider.ID,
	})
	if err != nil {
		return tokenResponse, fmt.Errorf("resolve OIDC client credential: %w", err)
	}
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, metadata.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := a.oidcHTTPClient().Do(req)
	if err != nil {
		return tokenResponse, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tokenResponse, fmt.Errorf("oidc token exchange failed with status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return tokenResponse, err
	}
	if strings.TrimSpace(tokenResponse.IDToken) == "" {
		return tokenResponse, fmt.Errorf("oidc token response did not include an id_token")
	}
	return tokenResponse, nil
}

func (a *App) verifyOIDCIDToken(ctx context.Context, provider oidcProviderRecord, metadata oidcMetadata, rawIDToken, nonceHash string) (oidcVerifiedIdentity, error) {
	var identity oidcVerifiedIdentity
	if metadata.JWKSURI == "" {
		return identity, fmt.Errorf("provider jwks_uri is not configured")
	}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{
			jwt.SigningMethodRS256.Alg(),
			jwt.SigningMethodRS384.Alg(),
			jwt.SigningMethodRS512.Alg(),
		}),
		jwt.WithIssuer(provider.Issuer),
		jwt.WithAudience(provider.ClientID),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30*time.Second),
	)
	claims := jwt.MapClaims{}
	_, err := parser.ParseWithClaims(rawIDToken, claims, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("id_token missing key id")
		}
		return a.oidcSigningKey(ctx, metadata.JWKSURI, kid)
	})
	if err != nil {
		return identity, err
	}
	sub, err := claims.GetSubject()
	if err != nil || strings.TrimSpace(sub) == "" {
		return identity, fmt.Errorf("id_token subject is required")
	}
	issuer, err := claims.GetIssuer()
	if err != nil || strings.TrimSpace(issuer) == "" {
		issuer = provider.Issuer
	}
	nonce, _ := claims["nonce"].(string)
	if nonceHash == "" || authHash(nonce) != nonceHash {
		return identity, fmt.Errorf("invalid oidc nonce")
	}
	email, _ := claims["email"].(string)
	email = strings.TrimSpace(email)
	emailVerificationStatus, emailVerified, emailVerificationClaimMalformed := oidcEmailVerificationStatusFromClaim(email, claims["email_verified"])
	if email != "" && !emailDomainAllowed(emailDomain(email), provider.AllowedEmailDomains) {
		return identity, fmt.Errorf("email domain is not allowed for this provider")
	}
	teams := stringSliceClaim(claims[provider.TeamClaim])
	if provider.TeamClaim == "" {
		teams = stringSliceClaim(claims["teams"])
	}
	return oidcVerifiedIdentity{
		ProviderID:                      provider.ID,
		Issuer:                          issuer,
		Subject:                         sub,
		Email:                           email,
		EmailVerified:                   emailVerified,
		EmailVerificationStatus:         emailVerificationStatus,
		EmailVerificationClaimMalformed: emailVerificationClaimMalformed,
		Teams:                           teams,
	}, nil
}

func (a *App) oidcSigningKey(ctx context.Context, jwksURI, kid string) (*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.oidcHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jwks fetch failed with status %d", resp.StatusCode)
	}
	var jwks oidcJWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, err
	}
	for _, key := range jwks.Keys {
		if key.Kid != kid {
			continue
		}
		return rsaPublicKeyFromJWK(key)
	}
	return nil, fmt.Errorf("jwks key %q not found", kid)
}

func rsaPublicKeyFromJWK(key oidcJWK) (*rsa.PublicKey, error) {
	if len(key.X5C) > 0 {
		certDER, err := base64.StdEncoding.DecodeString(key.X5C[0])
		if err != nil {
			return nil, err
		}
		cert, err := x509.ParseCertificate(certDER)
		if err != nil {
			return nil, err
		}
		if pub, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			return pub, nil
		}
		return nil, fmt.Errorf("x5c certificate does not contain an rsa public key")
	}
	if key.Kty != "" && !strings.EqualFold(key.Kty, "RSA") {
		return nil, fmt.Errorf("unsupported jwk key type %q", key.Kty)
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, err
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	if e == 0 {
		return nil, fmt.Errorf("invalid jwk exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

func (a *App) oidcHTTPClient() *http.Client {
	if a != nil && a.httpClient != nil {
		return a.httpClient
	}
	return http.DefaultClient
}

func generateOIDCSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func authHash(value string) string {
	return auth.HashToken(value)
}

func boolClaim(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	}
	return false, false
}

func oidcEmailVerificationStatusFromClaim(email string, claim any) (string, bool, bool) {
	emailVerified, claimParsed := boolClaim(claim)
	claimMalformed := claim != nil && !claimParsed
	if strings.TrimSpace(email) == "" {
		return oidcEmailVerificationNotProvided, false, claimMalformed
	}
	if !claimParsed {
		return oidcEmailVerificationUnknown, false, claimMalformed
	}
	if emailVerified {
		return oidcEmailVerificationVerified, true, false
	}
	return oidcEmailVerificationUnverified, false, false
}

func stringSliceClaim(value any) []string {
	var raw []string
	switch typed := value.(type) {
	case []string:
		raw = typed
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				raw = append(raw, text)
			}
		}
	case string:
		raw = append(raw, strings.Split(typed, ",")...)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func safeReturnTo(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/pipelineruns/main"
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/pipelineruns/main"
	}
	// Browsers normalise backslashes to forward slashes while parsing the
	// authority, so "/\evil.example" navigates cross-origin exactly like
	// "//evil.example". An internal path never needs a backslash.
	if strings.Contains(raw, `\`) {
		return "/pipelineruns/main"
	}
	if strings.HasPrefix(raw, "/login") {
		return "/pipelineruns/main"
	}
	if strings.ContainsAny(raw, "\r\n\t") {
		return "/pipelineruns/main"
	}
	return raw
}

func callbackRedirectURL(returnTo, code string) string {
	values := url.Values{}
	values.Set("session_code", code)
	values.Set("return_to", safeReturnTo(returnTo))
	return "/#/login?" + values.Encode()
}

func appendQuery(rawURL string, values url.Values) string {
	if len(values) == 0 {
		return rawURL
	}
	separator := "?"
	if strings.Contains(rawURL, "?") {
		separator = "&"
	}
	return rawURL + separator + values.Encode()
}

func (a *App) oidcCallbackURL(r *http.Request, providerID string) (string, error) {
	path := "/v1/auth/oidc/" + url.PathEscape(providerID) + "/callback"
	baseURL, err := a.externalAuthCallbackBaseURL(r)
	if err != nil {
		return "", err
	}
	return baseURL + path, nil
}

func (a *App) externalAuthCallbackBaseURL(r *http.Request) (string, error) {
	if a != nil && a.cfg != nil && strings.TrimSpace(a.cfg.PublicURL) != "" {
		rawPublicURL := strings.TrimSpace(a.cfg.PublicURL)
		parsed, err := url.Parse(rawPublicURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", fmt.Errorf("public_url must be an absolute URL without query or fragment")
		}
		return strings.TrimRight(rawPublicURL, "/"), nil
	}
	if a != nil && a.cfg != nil && a.cfg.RequiresProductionGates() {
		return "", fmt.Errorf("public_url is required for external authentication callbacks in production")
	}
	if r == nil || strings.TrimSpace(r.Host) == "" {
		return "", fmt.Errorf("request host is required for development callback URL")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host, nil
}
