package auth

import (
	"context"
	"strings"
	"testing"
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
