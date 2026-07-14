package configsync

import (
	"testing"

	"nopsai/pkg/models"
)

func boolPtr(value bool) *bool {
	return &value
}

func TestBuildRepositoryInputDefaultsAndValidation(t *testing.T) {
	input, err := BuildRepositoryInput(RepositoryInputRequest{
		RepoURL: " https://github.com/acme/configs.git ",
	}, models.ConfigRepositoryScopeTeam, " team-1/ ", "user-1")
	if err != nil {
		t.Fatalf("BuildRepositoryInput() error = %v", err)
	}
	if input.RepoURL != "https://github.com/acme/configs.git" {
		t.Fatalf("RepoURL = %q", input.RepoURL)
	}
	if input.Branch != "main" {
		t.Fatalf("Branch = %q, want main", input.Branch)
	}
	if input.ScopeID != "team-1" {
		t.Fatalf("ScopeID = %q, want team-1", input.ScopeID)
	}
	if input.Provider != models.ConfigRepositoryProviderGitHub {
		t.Fatalf("Provider = %q, want github", input.Provider)
	}
	if !input.Enabled {
		t.Fatal("Enabled = false, want true")
	}
	if input.WriteEnabled {
		t.Fatal("WriteEnabled = true, want false")
	}

	if _, err := BuildRepositoryInput(RepositoryInputRequest{
		RepoURL:  "https://github.com/acme/configs.git",
		BasePath: "../outside",
	}, models.ConfigRepositoryScopeTeam, "team-1", "user-1"); err == nil {
		t.Fatal("BuildRepositoryInput() accepted escaping base_path")
	}

	input, err = BuildRepositoryInput(RepositoryInputRequest{
		RepoURL:      "https://github.com/acme/configs.git",
		WriteEnabled: boolPtr(true),
	}, models.ConfigRepositoryScopeTeam, "team-1", "user-1")
	if err != nil {
		t.Fatalf("BuildRepositoryInput() with write enabled error = %v", err)
	}
	if !input.WriteEnabled || input.WriteBranch != "nopsai/ui-changes" {
		t.Fatalf("write settings = (%v, %q), want (true, nopsai/ui-changes)", input.WriteEnabled, input.WriteBranch)
	}

	if _, err := BuildRepositoryInput(RepositoryInputRequest{
		RepoURL:     "https://github.com/acme/configs.git",
		WriteBranch: "bad branch",
	}, models.ConfigRepositoryScopeTeam, "team-1", "user-1"); err == nil {
		t.Fatal("BuildRepositoryInput() accepted invalid write_branch")
	}
}

func TestBuildRepositoryInputSupportsCredentialBackedProviders(t *testing.T) {
	input, err := BuildRepositoryInput(RepositoryInputRequest{
		RepoURL:       "https://gitlab.com/acme/platform/configs.git",
		Provider:      "gitlab",
		CredentialRef: "credential://system/gitops/gitlab",
	}, models.ConfigRepositoryScopeSystem, models.ConfigRepositorySystemGlobalID, "user-1")
	if err != nil {
		t.Fatalf("BuildRepositoryInput() error = %v", err)
	}
	if input.Provider != models.ConfigRepositoryProviderGitLab {
		t.Fatalf("Provider = %q, want gitlab", input.Provider)
	}
	if input.CredentialRef != "credential://system/gitops/gitlab" {
		t.Fatalf("CredentialRef = %q", input.CredentialRef)
	}

	if _, err := BuildRepositoryInput(RepositoryInputRequest{
		RepoURL:  "https://gitlab.com/acme/configs.git",
		Provider: "gitlab",
	}, models.ConfigRepositoryScopeTeam, "team-1", "user-1"); err == nil {
		t.Fatal("BuildRepositoryInput() accepted gitlab without credential_ref")
	}
	if _, err := BuildRepositoryInput(RepositoryInputRequest{
		RepoURL:       "https://bitbucket.org/acme/configs",
		Provider:      "bitbucket",
		CredentialRef: "not-a-reference",
	}, models.ConfigRepositoryScopeTeam, "team-1", "user-1"); err == nil {
		t.Fatal("BuildRepositoryInput() accepted invalid credential_ref")
	}
}

func TestCleanRepositoryWritePath(t *testing.T) {
	got, err := CleanRepositoryWritePath("nopsai", "/pipelines/build.yaml")
	if err != nil {
		t.Fatalf("CleanRepositoryWritePath() error = %v", err)
	}
	if got != "nopsai/pipelines/build.yaml" {
		t.Fatalf("path = %q, want nopsai/pipelines/build.yaml", got)
	}
	if _, err := CleanRepositoryWritePath("", "../outside.yaml"); err == nil {
		t.Fatal("CleanRepositoryWritePath() accepted escaping path")
	}
	if _, err := CleanRepositoryWritePath("../base", "pipelines/build.yaml"); err == nil {
		t.Fatal("CleanRepositoryWritePath() accepted escaping base path")
	}
}
