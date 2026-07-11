package nopsai

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestParseGitOpsMailSettingsPlanSystemRepo(t *testing.T) {
	plan, err := parseGitOpsMailSettingsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		gitOpsRuntimeSettingsDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/mail.yaml": `
enabled: true
from: nopsai@example.com
smtp:
  host: smtp.example.com
  port: 587
  start_tls: true
  username: nopsai@example.com
  password_credential_ref: credential://system/mail/smtp-primary
`,
			},
		},
	)
	if err != nil {
		t.Fatalf("parseGitOpsMailSettingsPlan() error = %v", err)
	}
	if plan == nil {
		t.Fatal("parseGitOpsMailSettingsPlan() = nil, want plan")
	}
	if plan.settings.From != "nopsai@example.com" || plan.settings.SMTP.Host != "smtp.example.com" {
		t.Fatalf("settings = %#v, want normalized SMTP settings", plan.settings)
	}
}

func TestParseGitOpsMailSettingsPlanRejectsTeamRepo(t *testing.T) {
	_, err := parseGitOpsMailSettingsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"},
		gitOpsRuntimeSettingsDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/mail.yaml": "enabled: false",
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "system config repository") {
		t.Fatalf("error = %v, want system repo rejection", err)
	}
}

func TestNormalizeNotificationMailSettingsRequiresCredentialRefForUsername(t *testing.T) {
	_, err := normalizeNotificationMailSettings(notificationMailSettingsFile{
		Enabled: true,
		From:    "nopsai@example.com",
		SMTP: notificationMailSMTPSettings{
			Host:     "smtp.example.com",
			Port:     587,
			StartTLS: true,
			Username: "nopsai@example.com",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "password_credential_ref") {
		t.Fatalf("error = %v, want password credential ref validation", err)
	}
}
