package nopsai

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type localOIDCTestIdentity struct {
	Subject           string
	Email             string
	EmailVerified     *bool
	OmitEmail         bool
	OmitEmailVerified bool
	Nonce             string
	Teams             []string
}

type localOIDCTestProvider struct {
	t      *testing.T
	server *httptest.Server

	mu           sync.Mutex
	key          *rsa.PrivateKey
	kid          string
	codes        map[string]localOIDCTestIdentity
	jwksRequests int
	tokenForms   []url.Values
}

func newLocalOIDCTestProvider(t *testing.T) *localOIDCTestProvider {
	t.Helper()

	provider := &localOIDCTestProvider{
		t:     t,
		codes: map[string]localOIDCTestIdentity{},
	}
	provider.rotateKey("oidc-key-1")

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", provider.handleDiscovery)
	mux.HandleFunc("/token", provider.handleToken)
	mux.HandleFunc("/certs", provider.handleJWKS)

	provider.server = httptest.NewServer(mux)
	t.Cleanup(provider.server.Close)
	return provider
}

func (p *localOIDCTestProvider) issuer() string {
	return p.server.URL
}

func (p *localOIDCTestProvider) addCode(code string, identity localOIDCTestIdentity) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.codes[code] = identity
}

func (p *localOIDCTestProvider) rotateKey(kid string) {
	p.t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		p.t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.key = key
	p.kid = kid
}

func (p *localOIDCTestProvider) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(oidcMetadata{
		Issuer:                p.issuer(),
		AuthorizationEndpoint: p.issuer() + "/auth",
		TokenEndpoint:         p.issuer() + "/token",
		JWKSURI:               p.issuer() + "/certs",
		UserInfoEndpoint:      p.issuer() + "/userinfo",
	})
}

func (p *localOIDCTestProvider) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	code := r.Form.Get("code")
	clientID := r.Form.Get("client_id")
	if r.Form.Get("grant_type") != "authorization_code" || code == "" || clientID == "" || r.Form.Get("code_verifier") == "" {
		http.Error(w, "invalid token request", http.StatusBadRequest)
		return
	}

	p.mu.Lock()
	identity, ok := p.codes[code]
	if ok {
		delete(p.codes, code)
	}
	p.tokenForms = append(p.tokenForms, cloneURLValues(r.Form))
	p.mu.Unlock()
	if !ok {
		http.Error(w, "invalid code", http.StatusBadRequest)
		return
	}

	rawIDToken, err := p.signIDToken(clientID, identity)
	if err != nil {
		http.Error(w, "failed to sign token", http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(oidcTokenResponse{
		AccessToken: "provider-access-token",
		TokenType:   "Bearer",
		IDToken:     rawIDToken,
		ExpiresIn:   300,
	})
}

func (p *localOIDCTestProvider) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.jwksRequests++
	_ = json.NewEncoder(w).Encode(oidcJWKS{
		Keys: []oidcJWK{{
			Kty: "RSA",
			Kid: p.kid,
			Use: "sig",
			Alg: "RS256",
			N:   base64.RawURLEncoding.EncodeToString(p.key.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(p.key.PublicKey.E)).Bytes()),
		}},
	})
}

func (p *localOIDCTestProvider) signIDToken(audience string, identity localOIDCTestIdentity) (string, error) {
	p.mu.Lock()
	key := p.key
	kid := p.kid
	p.mu.Unlock()

	claims := jwt.MapClaims{
		"iss":   p.issuer(),
		"sub":   identity.Subject,
		"aud":   audience,
		"exp":   time.Now().Add(5 * time.Minute).Unix(),
		"iat":   time.Now().Add(-time.Minute).Unix(),
		"nonce": identity.Nonce,
		"teams": identity.Teams,
	}
	if !identity.OmitEmail {
		claims["email"] = identity.Email
	}
	if identity.EmailVerified != nil {
		claims["email_verified"] = *identity.EmailVerified
	} else if !identity.OmitEmailVerified {
		claims["email_verified"] = true
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	return token.SignedString(key)
}

func (p *localOIDCTestProvider) requestSnapshot() (int, []url.Values) {
	p.mu.Lock()
	defer p.mu.Unlock()
	forms := make([]url.Values, 0, len(p.tokenForms))
	for _, form := range p.tokenForms {
		forms = append(forms, cloneURLValues(form))
	}
	return p.jwksRequests, forms
}

func cloneURLValues(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for key, items := range values {
		out[key] = append([]string(nil), items...)
	}
	return out
}

func TestOIDCFlowWithLocalProviderAndRotatingJWKS(t *testing.T) {
	ctx := context.Background()
	idp := newLocalOIDCTestProvider(t)
	app := &App{
		httpClient: idp.server.Client(),
		credentialResolver: staticCredentialResolver{
			"credential://system/oidc/keycloak/client-secret": "dev-nopsai-secret",
		},
	}
	provider := oidcProviderRecord{
		ID:                  "keycloak",
		Type:                "oidc",
		DisplayName:         "Local Keycloak",
		Issuer:              idp.issuer(),
		ClientID:            "nopsai",
		ClientCredentialRef: "credential://system/oidc/keycloak/client-secret",
		Scopes:              []string{"openid", "email", "profile", "teams"},
		AllowedEmailDomains: []string{"example.com"},
		TeamClaim:           "teams",
	}

	metadata, err := app.discoverOIDCMetadata(ctx, provider)
	if err != nil {
		t.Fatalf("discoverOIDCMetadata() error = %v", err)
	}
	if metadata.TokenEndpoint != idp.issuer()+"/token" || metadata.JWKSURI != idp.issuer()+"/certs" {
		t.Fatalf("metadata = %#v, want local token and JWKS endpoints", metadata)
	}

	idp.addCode("admin-code", localOIDCTestIdentity{
		Subject: "keycloak-admin",
		Email:   "sso-admin@example.com",
		Nonce:   "nonce-admin",
		Teams:   []string{"nopsai-admins", "nopsai-viewers"},
	})
	firstToken, err := app.exchangeOIDCCode(ctx, provider, metadata, "admin-code", "verifier-admin", "http://nopsai.test/callback")
	if err != nil {
		t.Fatalf("exchangeOIDCCode(first) error = %v", err)
	}
	adminIdentity, err := app.verifyOIDCIDToken(ctx, provider, metadata, firstToken.IDToken, authHash("nonce-admin"))
	if err != nil {
		t.Fatalf("verifyOIDCIDToken(first) error = %v", err)
	}
	if adminIdentity.Email != "sso-admin@example.com" || adminIdentity.Subject != "keycloak-admin" {
		t.Fatalf("admin identity = %#v, want Keycloak admin subject/email", adminIdentity)
	}
	if !reflect.DeepEqual(adminIdentity.Teams, []string{"nopsai-admins", "nopsai-viewers"}) {
		t.Fatalf("admin teams = %#v, want mapped teams", adminIdentity.Teams)
	}

	idp.rotateKey("oidc-key-2")
	idp.addCode("operator-code", localOIDCTestIdentity{
		Subject: "keycloak-operator",
		Email:   "sso-operator@example.com",
		Nonce:   "nonce-operator",
		Teams:   []string{"nopsai-operators"},
	})
	secondToken, err := app.exchangeOIDCCode(ctx, provider, metadata, "operator-code", "verifier-operator", "http://nopsai.test/callback")
	if err != nil {
		t.Fatalf("exchangeOIDCCode(second) error = %v", err)
	}
	operatorIdentity, err := app.verifyOIDCIDToken(ctx, provider, metadata, secondToken.IDToken, authHash("nonce-operator"))
	if err != nil {
		t.Fatalf("verifyOIDCIDToken(second) error = %v", err)
	}
	if operatorIdentity.Subject != "keycloak-operator" || !reflect.DeepEqual(operatorIdentity.Teams, []string{"nopsai-operators"}) {
		t.Fatalf("operator identity = %#v, want identity from rotated signing key", operatorIdentity)
	}

	jwksRequests, tokenForms := idp.requestSnapshot()
	if jwksRequests < 2 {
		t.Fatalf("JWKS requests = %d, want one fetch per token validation across rotation", jwksRequests)
	}
	if len(tokenForms) != 2 {
		t.Fatalf("token requests = %d, want 2", len(tokenForms))
	}
	if tokenForms[0].Get("code_verifier") != "verifier-admin" || tokenForms[1].Get("code_verifier") != "verifier-operator" {
		t.Fatalf("token code_verifier forms = %#v, want PKCE verifier forwarded", tokenForms)
	}
}

func TestOIDCIDTokenEmailVerificationStatusDoesNotBlockLogin(t *testing.T) {
	ctx := context.Background()
	idp := newLocalOIDCTestProvider(t)
	app := &App{httpClient: idp.server.Client()}
	provider := oidcProviderRecord{
		ID:                  "keycloak",
		Issuer:              idp.issuer(),
		ClientID:            "nopsai",
		AllowedEmailDomains: []string{"example.com"},
	}
	metadata := oidcMetadata{JWKSURI: idp.issuer() + "/certs"}
	emailNotVerified := false

	tests := []struct {
		name                   string
		identity               localOIDCTestIdentity
		wantEmail              string
		wantEmailVerified      bool
		wantVerificationStatus string
	}{
		{
			name: "explicit false claim",
			identity: localOIDCTestIdentity{
				Subject:       "unverified-user",
				Email:         "unverified@example.com",
				EmailVerified: &emailNotVerified,
				Nonce:         "nonce-unverified",
			},
			wantEmail:              "unverified@example.com",
			wantVerificationStatus: oidcEmailVerificationUnverified,
		},
		{
			name: "missing claim",
			identity: localOIDCTestIdentity{
				Subject:           "unknown-user",
				Email:             "unknown@example.com",
				OmitEmailVerified: true,
				Nonce:             "nonce-unknown",
			},
			wantEmail:              "unknown@example.com",
			wantVerificationStatus: oidcEmailVerificationUnknown,
		},
		{
			name: "missing email",
			identity: localOIDCTestIdentity{
				Subject:           "no-email-user",
				OmitEmail:         true,
				OmitEmailVerified: true,
				Nonce:             "nonce-no-email",
			},
			wantVerificationStatus: oidcEmailVerificationNotProvided,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawIDToken, err := idp.signIDToken(provider.ClientID, tt.identity)
			if err != nil {
				t.Fatalf("signIDToken() error = %v", err)
			}
			identity, err := app.verifyOIDCIDToken(ctx, provider, metadata, rawIDToken, authHash(tt.identity.Nonce))
			if err != nil {
				t.Fatalf("verifyOIDCIDToken() error = %v", err)
			}
			if identity.Email != tt.wantEmail {
				t.Fatalf("email = %q, want %q", identity.Email, tt.wantEmail)
			}
			if identity.EmailVerified != tt.wantEmailVerified {
				t.Fatalf("email verified = %v, want %v", identity.EmailVerified, tt.wantEmailVerified)
			}
			if identity.EmailVerificationStatus != tt.wantVerificationStatus {
				t.Fatalf("email verification status = %q, want %q", identity.EmailVerificationStatus, tt.wantVerificationStatus)
			}
		})
	}
}

func TestOIDCIDTokenRejectsTokenWhenJWKSNoLongerContainsKid(t *testing.T) {
	ctx := context.Background()
	idp := newLocalOIDCTestProvider(t)
	app := &App{httpClient: idp.server.Client()}
	provider := oidcProviderRecord{
		ID:                  "keycloak",
		Issuer:              idp.issuer(),
		ClientID:            "nopsai",
		AllowedEmailDomains: []string{"example.com"},
	}
	metadata := oidcMetadata{JWKSURI: idp.issuer() + "/certs"}

	rawIDToken, err := idp.signIDToken(provider.ClientID, localOIDCTestIdentity{
		Subject: "stale-user",
		Email:   "stale@example.com",
		Nonce:   "nonce-stale",
	})
	if err != nil {
		t.Fatalf("signIDToken() error = %v", err)
	}
	idp.rotateKey("oidc-key-after-token-issued")

	if _, err := app.verifyOIDCIDToken(ctx, provider, metadata, rawIDToken, authHash("nonce-stale")); err == nil {
		t.Fatal("verifyOIDCIDToken() error = nil, want stale kid rejection")
	}
}
