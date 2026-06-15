package model

import "testing"

func TestGitWebhookSourceMutationsAreSensitive(t *testing.T) {
	for _, action := range []string{
		"git_webhook_source.create",
		"git_webhook_source.update",
		"git_webhook_source.delete",
		"git_webhook_source.manage_acl",
	} {
		if !IsSensitiveAction(action) {
			t.Fatalf("IsSensitiveAction(%q) = false", action)
		}
	}
	if IsSensitiveAction("git_webhook_source.read") {
		t.Fatal("git_webhook_source.read should not be sensitive")
	}
}
