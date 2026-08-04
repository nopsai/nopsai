package runnerinstall

import (
	"net/url"
	"strings"
	"testing"
)

func TestRunnerIdentityUsesPinnedUIDForGitOps(t *testing.T) {
	identity, err := runnerIdentityForInstall(" Prod Runner ", url.Values{"runner_uid": []string{" GitOps UID "}})
	if err != nil {
		t.Fatalf("runnerIdentityForInstall() error = %v", err)
	}
	if identity.Name != "Prod Runner" || identity.UID != "gitops-uid" || identity.ID != "prod-runner-gitops-uid" {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestRunnerIdentityGeneratesUniqueRunnerID(t *testing.T) {
	first, err := runnerIdentityForInstall("shared-runner", nil)
	if err != nil {
		t.Fatalf("runnerIdentityForInstall(first) error = %v", err)
	}
	second, err := runnerIdentityForInstall("shared-runner", nil)
	if err != nil {
		t.Fatalf("runnerIdentityForInstall(second) error = %v", err)
	}
	if first.ID == "" || second.ID == "" || first.ID == second.ID {
		t.Fatalf("generated identities should be non-empty and unique: %#v %#v", first, second)
	}
	if !strings.HasPrefix(first.ID, "shared-runner-") || !strings.HasPrefix(second.ID, "shared-runner-") {
		t.Fatalf("generated identities should preserve the runner name prefix: %#v %#v", first, second)
	}
}
