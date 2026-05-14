package serviceauth

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestCredentialsAndAuthenticatorRoundTrip(t *testing.T) {
	creds, err := NewCredentials(Config{
		SigningKey: "test-key",
		Role:       RoleAgent,
		ServiceID:  "agent-1",
	})
	if err != nil {
		t.Fatalf("NewCredentials() error = %v", err)
	}
	md, err := creds.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("GetRequestMetadata() error = %v", err)
	}

	authenticator, err := NewAuthenticator(Config{SigningKey: "test-key"})
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}
	claims, err := authenticator.AuthenticateContext(metadata.NewIncomingContext(context.Background(), metadata.New(md)))
	if err != nil {
		t.Fatalf("AuthenticateContext() error = %v", err)
	}
	if claims.ServiceRole() != RoleAgent {
		t.Fatalf("ServiceRole() = %q, want %q", claims.ServiceRole(), RoleAgent)
	}
	if claims.ServiceID() != "agent-1" {
		t.Fatalf("ServiceID() = %q, want agent-1", claims.ServiceID())
	}
}

func TestAuthenticatorRejectsMissingBearerToken(t *testing.T) {
	authenticator, err := NewAuthenticator(Config{SigningKey: "test-key"})
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}
	if _, err := authenticator.AuthenticateContext(context.Background()); err == nil {
		t.Fatal("AuthenticateContext() succeeded without metadata")
	}
}
