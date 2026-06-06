package nopsai

import (
	"testing"
	"time"
)

func TestValidatePersonalAccessTokenName(t *testing.T) {
	name, err := validatePersonalAccessTokenName("  deploy bot  ")
	if err != nil {
		t.Fatalf("validatePersonalAccessTokenName() error = %v", err)
	}
	if name != "deploy bot" {
		t.Fatalf("name = %q, want trimmed value", name)
	}

	for _, raw := range []string{"", "   ", "bad\nname", "bad\tname"} {
		if _, err := validatePersonalAccessTokenName(raw); err == nil {
			t.Fatalf("validatePersonalAccessTokenName(%q) error = nil, want validation error", raw)
		}
	}
}

func TestResolvePersonalAccessTokenExpiry(t *testing.T) {
	now := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)

	t.Run("default days", func(t *testing.T) {
		expiresAt, err := resolvePersonalAccessTokenExpiry(authPersonalTokenRequest{}, now)
		if err != nil {
			t.Fatalf("resolvePersonalAccessTokenExpiry() error = %v", err)
		}
		if expiresAt == nil || !expiresAt.Equal(now.AddDate(0, 0, defaultPersonalAccessTokenDays)) {
			t.Fatalf("expiresAt = %v, want default expiry", expiresAt)
		}
	})

	t.Run("explicit date", func(t *testing.T) {
		expiresAt, err := resolvePersonalAccessTokenExpiry(authPersonalTokenRequest{ExpiresAt: "2026-05-25"}, now)
		if err != nil {
			t.Fatalf("resolvePersonalAccessTokenExpiry() error = %v", err)
		}
		want := time.Date(2026, 5, 25, 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC)
		if expiresAt == nil || !expiresAt.Equal(want) {
			t.Fatalf("expiresAt = %v, want %v", expiresAt, want)
		}
	})

	t.Run("never", func(t *testing.T) {
		expiresAt, err := resolvePersonalAccessTokenExpiry(authPersonalTokenRequest{NeverExpires: true}, now)
		if err != nil {
			t.Fatalf("resolvePersonalAccessTokenExpiry() error = %v", err)
		}
		if expiresAt != nil {
			t.Fatalf("expiresAt = %v, want nil", expiresAt)
		}
	})

	t.Run("reject past date", func(t *testing.T) {
		if _, err := resolvePersonalAccessTokenExpiry(authPersonalTokenRequest{ExpiresAt: "2026-05-18"}, now); err == nil {
			t.Fatal("resolvePersonalAccessTokenExpiry() error = nil, want past-date validation error")
		}
	})

	t.Run("reject beyond max", func(t *testing.T) {
		if _, err := resolvePersonalAccessTokenExpiry(authPersonalTokenRequest{ExpiresAt: "2027-06-01"}, now); err == nil {
			t.Fatal("resolvePersonalAccessTokenExpiry() error = nil, want max-date validation error")
		}
	})
}
