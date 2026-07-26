package systemconfig

import (
	"testing"

	"nopsai/config"
)

func TestNormalizeRunnerScopesTrimsDeduplicatesAndDropsEmpty(t *testing.T) {
	got := NormalizeRunnerScopes(" /prod/, prod, team/a, TEAM/a, , /staging ")
	want := "prod,team/a,staging"
	if got != want {
		t.Fatalf("NormalizeRunnerScopes() = %q, want %q", got, want)
	}
}

func TestCloneDispatcherRoutingNormalizesScopesAndRunners(t *testing.T) {
	got := CloneDispatcherRouting(map[string][]string{
		"":      {" runner-1 ", "", "runner-2"},
		" prod": {" runner-prod "},
	})

	if runners := got["*"]; len(runners) != 2 || runners[0] != "runner-1" || runners[1] != "runner-2" {
		t.Fatalf("root runners = %#v, want trimmed non-empty runners", runners)
	}
	if runners := got["prod"]; len(runners) != 1 || runners[0] != "runner-prod" {
		t.Fatalf("prod runners = %#v, want trimmed runner", runners)
	}
}

func TestRemoveRunnerFromDispatcherRoutingDropsRunnerAndEmptyScopes(t *testing.T) {
	got, changed := RemoveRunnerFromDispatcherRouting(map[string][]string{
		"*":    {"runner-general", " runner-prod-5 "},
		"prod": {"runner-prod-5"},
		"dev":  {"runner-dev"},
	}, " runner-prod-5 ")

	if !changed {
		t.Fatal("changed = false, want true")
	}
	if runners := got["*"]; len(runners) != 1 || runners[0] != "runner-general" {
		t.Fatalf("default runners = %#v, want runner-general only", runners)
	}
	if _, exists := got["prod"]; exists {
		t.Fatalf("prod route remained after removing its only runner: %#v", got)
	}
	if runners := got["dev"]; len(runners) != 1 || runners[0] != "runner-dev" {
		t.Fatalf("dev runners = %#v, want runner-dev unchanged", runners)
	}
}

func TestRemoveRunnerFromDispatcherRoutingReportsUnchanged(t *testing.T) {
	got, changed := RemoveRunnerFromDispatcherRouting(map[string][]string{
		"prod": {"runner-prod-1"},
	}, "runner-prod-5")

	if changed {
		t.Fatal("changed = true, want false")
	}
	if runners := got["prod"]; len(runners) != 1 || runners[0] != "runner-prod-1" {
		t.Fatalf("prod runners = %#v, want original runner", runners)
	}
}

func TestRemoveRunnersFromDispatcherRoutingDropsMultipleIDs(t *testing.T) {
	got, changed := RemoveRunnersFromDispatcherRouting(map[string][]string{
		"prod": {"runner-prod-1", "runner-prod-2", "runner-prod-3"},
	}, []string{" runner-prod-1 ", "runner-prod-3"})

	if !changed {
		t.Fatal("changed = false, want true")
	}
	if runners := got["prod"]; len(runners) != 1 || runners[0] != "runner-prod-2" {
		t.Fatalf("prod runners = %#v, want runner-prod-2 only", runners)
	}
}

func TestBuildResponseFiltersEjectedRunnerIDsFromDispatcherRouting(t *testing.T) {
	resp := BuildResponse(config.Config{
		DispatcherRouting: map[string][]string{
			"prod": {"runner-prod-1", "runner-prod-5"},
		},
		EjectedRunnerIDs: []string{"runner-prod-5"},
	}, "")

	routing, ok := resp["dispatcher_routing"].(map[string][]string)
	if !ok {
		t.Fatalf("dispatcher_routing = %T, want map[string][]string", resp["dispatcher_routing"])
	}
	if got := routing["prod"]; len(got) != 1 || got[0] != "runner-prod-1" {
		t.Fatalf("prod runners = %#v, want runner-prod-1 only", got)
	}
}

func TestBuildResponseIncludesGitHubAppCompatibilityFields(t *testing.T) {
	resp := BuildResponse(config.Config{
		GitHubAppID:                   "123456",
		GitHubPrivateKeyCredentialRef: "credential://system/github/app-private-key",
		GitHubWebhookCredentialRef:    "credential://system/github/webhook-secret",
		GitHubInstallations: []config.GitHubInstallationConfig{{
			InstallationID: "987654",
			AccountLogin:   "nopsai",
			AccountType:    "organization",
		}},
	}, "")

	if resp["github_app_id"] != "123456" ||
		resp["github_installation_id"] != "987654" ||
		resp["github_private_key_credential_ref"] != "credential://system/github/app-private-key" ||
		resp["github_webhook_credential_ref"] != "credential://system/github/webhook-secret" {
		t.Fatalf("GitHub compatibility fields = %#v", resp)
	}
	installations, ok := resp["github_installations"].([]config.GitHubInstallationConfig)
	if !ok || len(installations) != 1 || installations[0].InstallationID != "987654" {
		t.Fatalf("github_installations = %#v", resp["github_installations"])
	}
}
