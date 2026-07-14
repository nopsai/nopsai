package configsync

import (
	"testing"

	"nopsai/pkg/models"
)

func TestParseRepositoryIdentityProviders(t *testing.T) {
	tests := []struct {
		name         string
		raw          string
		provider     string
		wantProvider string
		wantHost     string
		wantOwner    string
		wantRepo     string
		wantProject  string
	}{
		{
			name:         "github default",
			raw:          "https://github.com/acme/configs.git",
			wantProvider: models.ConfigRepositoryProviderGitHub,
			wantHost:     "github.com",
			wantOwner:    "acme",
			wantRepo:     "configs",
			wantProject:  "acme/configs",
		},
		{
			name:         "gitlab subgroup",
			raw:          "https://gitlab.com/acme/platform/configs.git",
			provider:     models.ConfigRepositoryProviderGitLab,
			wantProvider: models.ConfigRepositoryProviderGitLab,
			wantHost:     "gitlab.com",
			wantOwner:    "acme/platform",
			wantRepo:     "configs",
			wantProject:  "acme/platform/configs",
		},
		{
			name:         "bitbucket cloud",
			raw:          "git@bitbucket.org:workspace/configs.git",
			provider:     models.ConfigRepositoryProviderBitbucket,
			wantProvider: models.ConfigRepositoryProviderBitbucket,
			wantHost:     "bitbucket.org",
			wantOwner:    "workspace",
			wantRepo:     "configs",
			wantProject:  "workspace/configs",
		},
		{
			name:         "gitea",
			raw:          "https://git.example.com/org/configs",
			provider:     models.ConfigRepositoryProviderGitea,
			wantProvider: models.ConfigRepositoryProviderGitea,
			wantHost:     "git.example.com",
			wantOwner:    "org",
			wantRepo:     "configs",
			wantProject:  "org/configs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRepositoryIdentity(tt.raw, tt.provider)
			if err != nil {
				t.Fatalf("ParseRepositoryIdentity() error = %v", err)
			}
			if got.Provider != tt.wantProvider || got.Host != tt.wantHost || got.Owner != tt.wantOwner || got.Repo != tt.wantRepo || got.ProjectPath != tt.wantProject {
				t.Fatalf("identity = %#v", got)
			}
		})
	}
}

func TestNormalizeRepositoryProviderInfersHostedProviders(t *testing.T) {
	tests := map[string]string{
		"https://github.com/acme/configs":     models.ConfigRepositoryProviderGitHub,
		"https://gitlab.com/acme/configs":     models.ConfigRepositoryProviderGitLab,
		"https://bitbucket.org/acme/configs":  models.ConfigRepositoryProviderBitbucket,
		"https://gitea.internal/acme/configs": models.ConfigRepositoryProviderGitea,
		"git@github.com:acme/configs.git":     models.ConfigRepositoryProviderGitHub,
	}
	for raw, want := range tests {
		got, err := NormalizeRepositoryProvider("", raw)
		if err != nil {
			t.Fatalf("NormalizeRepositoryProvider(%q) error = %v", raw, err)
		}
		if got != want {
			t.Fatalf("NormalizeRepositoryProvider(%q) = %q, want %q", raw, got, want)
		}
	}
}
