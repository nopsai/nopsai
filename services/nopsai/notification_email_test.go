package nopsai

import (
	"strings"
	"testing"
	"time"

	"nopsai/config"
)

func TestPipelineNotificationSubjectIncludesFailureAndProgress(t *testing.T) {
	ctx := pipelineNotificationContext{
		PipelineName: "include-pipeline",
		PipelinePath: "general",
		Status:       "failure",
		FailureStep:  "preparation",
		FailureTask:  "branch-versioning",
		Steps: []pipelineNotificationStep{
			{Name: "preparation", Status: "failure"},
			{Name: "secret-validation", Status: "skipped"},
		},
	}
	got := pipelineNotificationSubject(ctx, "failure")
	want := "[FAILED] general/include-pipeline - preparation / branch-versioning (0/2 steps passed)"
	if got != want {
		t.Fatalf("pipelineNotificationSubject() = %q, want %q", got, want)
	}
}

func TestPipelineNotificationBrandingUsesBundledAppIcon(t *testing.T) {
	app := App{cfg: &config.Config{PublicURL: "https://ci.example.com/"}}

	branding := app.pipelineNotificationBranding()
	if branding.LogoURL != "https://ci.example.com/brand/nopsai-app-icon.png" {
		t.Fatalf("LogoURL = %q, want bundled app icon", branding.LogoURL)
	}
}

func TestParsePipelineNotificationLogEntryRedactsSecrets(t *testing.T) {
	raw := `2026-06-08T06:07:27Z {"level":"error","step":"deploy","task":"publish","error":"api_key=topsecret Bearer abc.def","message":"Publish failed"}`
	entry, ok := parsePipelineNotificationLogEntry(raw)
	if !ok {
		t.Fatal("parsePipelineNotificationLogEntry() rejected an error log")
	}
	if entry.Step != "deploy" || entry.Task != "publish" {
		t.Fatalf("location = %s/%s, want deploy/publish", entry.Step, entry.Task)
	}
	if strings.Contains(entry.Text, "topsecret") || strings.Contains(entry.Text, "abc.def") {
		t.Fatalf("entry.Text contains a secret: %q", entry.Text)
	}
	if !strings.Contains(entry.Text, "[REDACTED]") {
		t.Fatalf("entry.Text = %q, want redaction marker", entry.Text)
	}
}

func TestRenderPipelineNotificationMailIncludesLinksAndEscapesLogs(t *testing.T) {
	app := App{cfg: &config.Config{
		PublicURL:                     "https://ci.example.com",
		NotificationMailWebsiteURL:    "https://www.example.com",
		NotificationMailSupportURL:    "https://support.example.com",
		NotificationMailFooterAddress: "Example Corp, Amsterdam",
		NotificationMailLogoURL:       "https://cdn.example.com/nopsai-logo.png",
	}}
	ctx := pipelineNotificationContext{
		RunID:         "8569e07f-42f4-4e8d-b0a9-992907a56276",
		PipelineName:  "deploy",
		PipelinePath:  "production",
		Status:        "failure",
		TeamID:        3,
		TeamPath:      "team-1/test-app",
		RepoOwner:     "acme",
		RepoName:      "service-api",
		RepoURL:       "https://github.com/acme/service-api",
		GitRef:        "refs/heads/main",
		GitCommitSHA:  "d759d83ed6aacbe19b12997a618500caa6726b2b",
		GitCommitURL:  "https://github.com/acme/service-api/commit/d759d83ed6aacbe19b12997a618500caa6726b2b",
		TriggerSource: "manual_rerun",
		StartedAt:     time.Date(2026, 6, 8, 6, 5, 17, 0, time.UTC),
		Duration:      "2m10s",
		FailureStep:   "build",
		FailureTask:   "compile",
		Steps: []pipelineNotificationStep{
			{Name: "build", Status: "failure", TaskTotal: 2, TaskPassed: 1},
			{Name: "deploy", Status: "skipped", TaskTotal: 1},
		},
		LogExcerpt: []pipelineNotificationLogEntry{{
			Step: "build",
			Task: "compile",
			Text: `compiler failed: <unexpected>`,
		}},
	}

	message, err := app.renderPipelineNotificationMail(ctx, "failure")
	if err != nil {
		t.Fatalf("renderPipelineNotificationMail() error = %v", err)
	}
	for _, want := range []string{
		"https://ci.example.com/#/pipelineruns/main/team/team-1/test-app?run=8569e07f-42f4-4e8d-b0a9-992907a56276",
		"https://cdn.example.com/nopsai-logo.png",
		"Example Corp, Amsterdam",
		"1 of 2 tasks passed",
		"compiler failed: &lt;unexpected&gt;",
	} {
		if !strings.Contains(message.HTMLBody, want) {
			t.Fatalf("HTML body missing %q", want)
		}
	}
	if strings.Contains(message.HTMLBody, "ZgotmplZ") {
		t.Fatal("HTML body contains a template sanitization placeholder")
	}
	if strings.Contains(message.HTMLBody, "team=") || strings.Contains(message.TextBody, "team=") {
		t.Fatal("notification body uses a team URL query parameter")
	}
	if !strings.Contains(message.TextBody, "View run: https://ci.example.com/#/pipelineruns/main/team/team-1/test-app?run=") {
		t.Fatal("plain-text body is missing the run link")
	}
}

func TestBuildNotificationMailMessageMultipartAlternative(t *testing.T) {
	raw, err := buildNotificationMailMessage("alerts@example.com", []string{"operator@example.com"}, notificationMailMessage{
		FromName: "NopsAI",
		Subject:  "Pipeline failed",
		TextBody: "Plain fallback",
		HTMLBody: "<strong>Rich body</strong>",
	})
	if err != nil {
		t.Fatalf("buildNotificationMailMessage() error = %v", err)
	}
	message := string(raw)
	for _, want := range []string{
		"From: \"NopsAI\" <alerts@example.com>",
		"Content-Type: multipart/alternative;",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Type: text/html; charset=utf-8",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("mail message missing %q", want)
		}
	}
}
