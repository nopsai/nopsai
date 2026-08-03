package auth

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLogoutRejectsEmptyRefreshToken(t *testing.T) {
	service := &Service{}

	err := service.Logout(context.Background(), "   ")
	if err == nil {
		t.Fatal("Logout() error = nil, want refresh token validation error")
	}
}

func TestGeneratePersonalAccessTokenUsesOpaquePrefixedSecret(t *testing.T) {
	token, err := GeneratePersonalAccessToken()
	if err != nil {
		t.Fatalf("GeneratePersonalAccessToken() error = %v", err)
	}
	if !strings.HasPrefix(token, PersonalAccessTokenPrefix) {
		t.Fatalf("token prefix = %q, want %q", token[:len(PersonalAccessTokenPrefix)], PersonalAccessTokenPrefix)
	}
	if len(strings.TrimPrefix(token, PersonalAccessTokenPrefix)) < 40 {
		t.Fatalf("token secret length = %d, want at least 40", len(strings.TrimPrefix(token, PersonalAccessTokenPrefix)))
	}
	if PersonalAccessTokenSuffix(token) != token[len(token)-8:] {
		t.Fatalf("PersonalAccessTokenSuffix() = %q, want last 8 chars", PersonalAccessTokenSuffix(token))
	}
}

func TestGenerateServiceAccountTokenUsesDistinctOpaquePrefix(t *testing.T) {
	token, err := GenerateServiceAccountToken()
	if err != nil {
		t.Fatalf("GenerateServiceAccountToken() error = %v", err)
	}
	if !strings.HasPrefix(token, ServiceAccountTokenPrefix) {
		t.Fatalf("token prefix = %q, want %q", token[:len(ServiceAccountTokenPrefix)], ServiceAccountTokenPrefix)
	}
	if strings.HasPrefix(token, PersonalAccessTokenPrefix) {
		t.Fatalf("service account token should not use personal token prefix %q", PersonalAccessTokenPrefix)
	}
	if len(strings.TrimPrefix(token, ServiceAccountTokenPrefix)) < 40 {
		t.Fatalf("token secret length = %d, want at least 40", len(strings.TrimPrefix(token, ServiceAccountTokenPrefix)))
	}
	if PersonalAccessTokenSuffix(token) != token[len(token)-8:] {
		t.Fatalf("PersonalAccessTokenSuffix() = %q, want last 8 chars", PersonalAccessTokenSuffix(token))
	}
}

func TestLocalJWTRejectsWrongIssuerAudience(t *testing.T) {
	localJWT := NewLocalJWTService([]byte("shared-key"), "local-issuer", "local-audience", time.Minute)
	serviceJWT := NewLocalJWTService([]byte("shared-key"), "service-issuer", "service-audience", time.Minute)

	token, _, err := serviceJWT.MintAccessToken(context.Background(), Claims{
		Sub:      "agent",
		Provider: "internal-service",
	})
	if err != nil {
		t.Fatalf("MintAccessToken() error = %v", err)
	}

	if _, err := localJWT.ParseAndValidate(token); err == nil {
		t.Fatal("ParseAndValidate() error = nil, want issuer/audience validation failure")
	}
}

func TestSetLocalEnabledCanDisableLocalLogin(t *testing.T) {
	service := &Service{cfg: Config{LocalEnabled: true}}

	service.SetLocalEnabled(false)

	if service.localEnabled() {
		t.Fatal("localEnabled() = true, want disabled")
	}
}

func TestCanonicalLoginKeyNormalizesCaseAndWhitespace(t *testing.T) {
	if got := canonicalLoginKey("  Alice@Example.COM  "); got != "alice@example.com" {
		t.Fatalf("canonicalLoginKey() = %q, want lowercase trimmed identifier", got)
	}
}
