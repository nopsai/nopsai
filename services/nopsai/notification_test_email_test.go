package nopsai

import (
	"strings"
	"testing"

	"nopsai/config"
)

func TestRenderNotificationMailTestUsesBrandedMultipartContent(t *testing.T) {
	app := App{cfg: &config.Config{
		Environment:                   "staging",
		PublicURL:                     "https://ci.example.com",
		NotificationMailLogoURL:       "https://cdn.example.com/nopsai-logo.png",
		NotificationMailWebsiteURL:    "https://www.example.com",
		NotificationMailSupportURL:    "https://support.example.com",
		NotificationMailFooterAddress: "Example Corp, Amsterdam",
	}}
	settings := notificationMailSettingsFile{
		Enabled: true,
		From:    "alerts@example.com",
		SMTP: notificationMailSMTPSettings{
			Host:              "smtp.example.com",
			Port:              587,
			StartTLS:          true,
			Username:          "alerts@example.com",
			PasswordSecretRef: "SMTP_PASSWORD",
		},
	}

	message, err := app.renderNotificationMailTest(settings, "operator@example.com", "", "Hello <team>")
	if err != nil {
		t.Fatalf("renderNotificationMailTest() error = %v", err)
	}
	if message.Subject != "[NopsAI] Mail delivery test successful" {
		t.Fatalf("subject = %q, want default branded subject", message.Subject)
	}
	for _, want := range []string{
		"CONFIGURATION VERIFIED",
		"smtp.example.com:587",
		"STARTTLS required",
		"Username and secret reference configured",
		"operator@example.com",
		"staging",
		"https://cdn.example.com/nopsai-logo.png",
		"Example Corp, Amsterdam",
		"Hello &lt;team&gt;",
	} {
		if !strings.Contains(message.HTMLBody, want) {
			t.Fatalf("HTML body missing %q", want)
		}
	}
	if strings.Contains(message.HTMLBody, "Hello <team>") {
		t.Fatal("custom note was not HTML-escaped")
	}
	for _, want := range []string{
		"MAIL DELIVERY VERIFIED",
		"SMTP server: smtp.example.com:587",
		"Security: STARTTLS required",
		"This was a configuration test, not a pipeline notification.",
	} {
		if !strings.Contains(message.TextBody, want) {
			t.Fatalf("text body missing %q", want)
		}
	}
}

func TestRenderNotificationMailTestPreservesCustomSubject(t *testing.T) {
	app := App{cfg: &config.Config{}}
	message, err := app.renderNotificationMailTest(notificationMailSettingsFile{
		Enabled: true,
		From:    "alerts@example.com",
		SMTP: notificationMailSMTPSettings{
			Host: "smtp.example.com",
			Port: 465,
		},
	}, "operator@example.com", "Custom delivery check", "")
	if err != nil {
		t.Fatalf("renderNotificationMailTest() error = %v", err)
	}
	if message.Subject != "Custom delivery check" {
		t.Fatalf("subject = %q, want custom subject", message.Subject)
	}
	if !strings.Contains(message.HTMLBody, "Implicit TLS") {
		t.Fatal("HTML body should identify implicit TLS on port 465")
	}
	if !strings.Contains(message.HTMLBody, "Not configured") {
		t.Fatal("HTML body should identify unauthenticated SMTP")
	}
}
