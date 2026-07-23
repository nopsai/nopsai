package nopsai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nopsai/config"
)

func TestResolveBootstrapAdminCredentialsUsesDevelopmentDefault(t *testing.T) {
	credentials, err := resolveBootstrapAdminCredentials(&config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Email != defaultAdminEmail ||
		credentials.Password != "admin" ||
		!credentials.PasswordConfigured ||
		!credentials.AllowDefaultPassword ||
		!credentials.MustChangePassword {
		t.Fatalf("credentials = %#v", credentials)
	}
}

func TestResolveBootstrapAdminCredentialsReadsPasswordFile(t *testing.T) {
	passwordPath := filepath.Join(t.TempDir(), "admin-password")
	if err := os.WriteFile(passwordPath, []byte("first-install-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	credentials, err := resolveBootstrapAdminCredentials(&config.Config{
		Environment: "production",
		BootstrapAdmin: config.BootstrapAdminConfig{
			Email:        " platform-admin@example.com ",
			PasswordFile: passwordPath,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Email != "platform-admin@example.com" ||
		credentials.Password != "first-install-secret" ||
		!credentials.PasswordConfigured ||
		credentials.MustChangePassword {
		t.Fatalf("credentials = %#v", credentials)
	}
}

func TestResolveBootstrapAdminCredentialsRejectsDefaultInProduction(t *testing.T) {
	_, err := resolveBootstrapAdminCredentials(&config.Config{
		Environment: "production",
		BootstrapAdmin: config.BootstrapAdminConfig{
			Password:             "admin",
			AllowDefaultPassword: true,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "reject") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveBootstrapAdminCredentialsRejectsAmbiguousPasswordSources(t *testing.T) {
	_, err := resolveBootstrapAdminCredentials(&config.Config{
		BootstrapAdmin: config.BootstrapAdminConfig{
			Password:     "first-install-secret",
			PasswordFile: "/run/secrets/bootstrap-admin-password",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("error = %v", err)
	}
}
