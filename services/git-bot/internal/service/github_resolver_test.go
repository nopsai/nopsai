package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/go-github/v53/github"
)

func TestGitHubClientResolverRoutesOwnersToDifferentInstallationClients(t *testing.T) {
	factoryCalls := map[int64]int{}
	resolver := NewGitHubClientResolver(
		StaticGitHubInstallationFetcher([]GitHubInstallation{
			{InstallationID: 101, AccountLogin: "acme", AccountType: "organization", Enabled: true},
			{InstallationID: 202, AccountLogin: "beta", AccountType: "organization", Enabled: true},
		}),
		func(installationID int64) *github.Client {
			factoryCalls[installationID]++
			return github.NewClient(nil)
		},
		0,
	)

	acmeClient, acmeInstallation, err := resolver.ClientForRepository(context.Background(), "Acme", "widgets")
	if err != nil {
		t.Fatalf("ClientForRepository(acme) error = %v", err)
	}
	betaClient, betaInstallation, err := resolver.ClientForRepository(context.Background(), "beta", "widgets")
	if err != nil {
		t.Fatalf("ClientForRepository(beta) error = %v", err)
	}

	if acmeInstallation.InstallationID != 101 || betaInstallation.InstallationID != 202 {
		t.Fatalf("installation IDs = (%d, %d), want (101, 202)", acmeInstallation.InstallationID, betaInstallation.InstallationID)
	}
	if acmeClient == betaClient {
		t.Fatal("different owners resolved to the same GitHub client")
	}
	if factoryCalls[101] != 1 || factoryCalls[202] != 1 {
		t.Fatalf("factory calls = %#v, want one per installation", factoryCalls)
	}
}

func TestGitHubClientResolverReusesClientForSameOwner(t *testing.T) {
	factoryCalls := 0
	resolver := NewGitHubClientResolver(
		StaticGitHubInstallationFetcher([]GitHubInstallation{
			{InstallationID: 101, AccountLogin: "acme", AccountType: "organization", Enabled: true},
		}),
		func(int64) *github.Client {
			factoryCalls++
			return github.NewClient(nil)
		},
		0,
	)

	firstClient, firstInstallation, err := resolver.ClientForRepository(context.Background(), "acme", "widgets")
	if err != nil {
		t.Fatalf("first ClientForRepository() error = %v", err)
	}
	secondClient, secondInstallation, err := resolver.ClientForRepository(context.Background(), "ACME", "api")
	if err != nil {
		t.Fatalf("second ClientForRepository() error = %v", err)
	}

	if firstInstallation.InstallationID != 101 || secondInstallation.InstallationID != 101 {
		t.Fatalf("installation IDs = (%d, %d), want both 101", firstInstallation.InstallationID, secondInstallation.InstallationID)
	}
	if firstClient != secondClient {
		t.Fatal("same owner did not reuse the cached GitHub client")
	}
	if factoryCalls != 1 {
		t.Fatalf("factory calls = %d, want 1", factoryCalls)
	}
}

func TestGitHubClientResolverRejectsUnknownOwner(t *testing.T) {
	resolver := NewGitHubClientResolver(
		StaticGitHubInstallationFetcher([]GitHubInstallation{
			{InstallationID: 101, AccountLogin: "acme", AccountType: "organization", Enabled: true},
		}),
		func(int64) *github.Client { return github.NewClient(nil) },
		0,
	)

	_, _, err := resolver.ClientForRepository(context.Background(), "unknown", "widgets")
	assertProviderStatus(t, err, http.StatusForbidden)
}

func TestGitHubClientResolverRejectsDisabledInstallations(t *testing.T) {
	resolver := NewGitHubClientResolver(
		StaticGitHubInstallationFetcher([]GitHubInstallation{
			{InstallationID: 101, AccountLogin: "acme", AccountType: "organization", Enabled: false},
		}),
		func(int64) *github.Client { return github.NewClient(nil) },
		0,
	)

	_, _, err := resolver.ClientForRepository(context.Background(), "acme", "widgets")
	assertProviderStatus(t, err, http.StatusForbidden)

	_, err = resolver.InstallationForID(context.Background(), 101)
	assertProviderStatus(t, err, http.StatusForbidden)
}

func assertProviderStatus(t *testing.T, err error, wantStatus int) {
	t.Helper()
	var providerErr providerError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %v, want providerError", err)
	}
	if providerErr.Status != wantStatus {
		t.Fatalf("provider status = %d, want %d", providerErr.Status, wantStatus)
	}
}
