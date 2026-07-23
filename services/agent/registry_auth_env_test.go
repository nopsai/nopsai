package agent

import (
	"strings"
	"testing"

	"nopsai/pkg/registryauth"
)

func TestFilteredAgentEnvironmentRemovesRegistryAuthConfig(t *testing.T) {
	filtered := filteredAgentEnvironment([]string{
		"GIT_REPO_NAME=platform",
		registryauth.DockerConfigBase64Env + "=secret",
		registryauth.DockerConfigPathEnv + "=/run/nopsai/config.json",
		"SCOPE=prod",
	})
	dump := strings.Join(filtered, "\n")
	if strings.Contains(dump, "NOPSAI_REGISTRY_DOCKER_CONFIG_") {
		t.Fatalf("filtered environment leaked registry auth keys:\n%s", dump)
	}
	for _, want := range []string{"GIT_REPO_NAME=platform", "SCOPE=prod"} {
		if !strings.Contains(dump, want) {
			t.Fatalf("filtered environment missing %q:\n%s", want, dump)
		}
	}
}
