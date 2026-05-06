package auth

import (
	"context"
	"testing"
)

func TestLogoutRejectsEmptyRefreshToken(t *testing.T) {
	service := &Service{}

	err := service.Logout(context.Background(), "   ")
	if err == nil {
		t.Fatal("Logout() error = nil, want refresh token validation error")
	}
}
