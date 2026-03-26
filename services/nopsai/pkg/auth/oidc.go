package auth

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
)

type OIDCValidator struct {
	verifier *oidc.IDTokenVerifier
	clientID string
}

func NewOIDCValidator(ctx context.Context, issuer, audience, jwksURL string) (*OIDCValidator, error) {
	if issuer == "" {
		return nil, fmt.Errorf("issuer is required for OIDC validator")
	}

	var provider *oidc.Provider
	var err error
	if jwksURL != "" {
		cfg := &oidc.Config{SkipIssuerCheck: false, ClientID: audience}
		remoteKeySet := oidc.NewRemoteKeySet(ctx, jwksURL)
		verifier := oidc.NewVerifier(issuer, remoteKeySet, cfg)
		return &OIDCValidator{verifier: verifier, clientID: audience}, nil
	}

	provider, err = oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}
	cfg := &oidc.Config{ClientID: audience}
	return &OIDCValidator{
		verifier: provider.Verifier(cfg),
		clientID: audience,
	}, nil
}

func (v *OIDCValidator) Verify(ctx context.Context, rawToken string) (*Claims, error) {
	if v == nil || v.verifier == nil {
		return nil, fmt.Errorf("oidc verifier not configured")
	}

	idToken, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Email     string   `json:"email"`
		Sub       string   `json:"sub"`
		TenantIDs []string `json:"tenant_ids"`
		Roles     []string `json:"roles"`
	}
	if err := idToken.Claims(&payload); err != nil {
		return nil, err
	}

	return &Claims{
		Sub:       payload.Sub,
		Email:     payload.Email,
		Provider:  "oidc",
		TenantIDs: payload.TenantIDs,
		Roles:     payload.Roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    idToken.Issuer,
			Subject:   payload.Sub,
			Audience:  idToken.Audience,
			ExpiresAt: jwt.NewNumericDate(idToken.Expiry),
		},
	}, nil
}
