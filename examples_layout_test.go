package nopsai

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExampleArtifactsLiveUnderExamples(t *testing.T) {
	for _, path := range []string{
		"examples/README.md",
		"examples/sample-config-repo/README.md",
		"examples/sample-config-repo/global-repo/setting/system/auth.yaml",
		"examples/sample-config-repo/global-repo/triggers/nopsai/nopsai.yaml",
		"examples/sample-config-repo/team-1-repo/pipelines/team-1/variable-feature-exercise.yaml",
		"examples/sample-pipeline/README.md",
		"examples/sample-pipeline/5-pipeline.yaml",
		"examples/sample-pipeline/triggers.yaml",
		"examples/sso/README.md",
		"examples/sso/keycloak/docker-compose.yaml",
		"examples/sso/keycloak/nopsai-realm.json",
		"examples/sso/idp-test-pack/README.md",
		"examples/sso/idp-test-pack/docker-compose.yml",
		"examples/sso/idp-test-pack/mock-oauth2/config.json",
		"examples/sso/idp-test-pack/test-matrix.yaml",
		"examples/sso/idp-test-pack/scripts/validate-fixtures.py",
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected example artifact path %q: %v", path, err)
		}
	}

	for _, legacyPath := range []string{
		filepath.Join("dev", "keycloak"),
		filepath.Join("doc", "sample-config-repo"),
		filepath.Join("doc", "sample-pipeline"),
		filepath.Join("examples", "sample-config-repo", "global-repo", "triggers", "hosein-yousefii"),
	} {
		if _, err := os.Stat(legacyPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("example artifact should not live at legacy path %q: %v", legacyPath, err)
		}
	}
}

func TestExampleDocsPointToExamples(t *testing.T) {
	keycloakCompose := readTextFile(t, "examples/sso/keycloak/docker-compose.yaml")
	if !strings.Contains(keycloakCompose, "name: nopsai") {
		t.Fatal("Keycloak compose fixture should share the local nopsai compose project")
	}
	if !strings.Contains(keycloakCompose, "./nopsai-realm.json:/opt/keycloak/data/import/nopsai-realm.json:ro") {
		t.Fatal("Keycloak compose fixture should mount the colocated realm file")
	}
	if strings.Contains(keycloakCompose, legacyPathReference("dev", "keycloak")) {
		t.Fatal("Keycloak compose fixture should not reference the legacy dev path")
	}

	idpCompose := readTextFile(t, "examples/sso/idp-test-pack/docker-compose.yml")
	if !strings.Contains(idpCompose, "ghcr.io/navikt/mock-oauth2-server:${MOCK_OAUTH2_SERVER_VERSION:-5.0.2}") {
		t.Fatal("IdP test pack compose file should document the pinned default mock-oauth2 image version")
	}

	for path, required := range map[string][]string{
		"Readme.md": {
			"examples/sample-config-repo/README.md",
			"examples/sample-pipeline",
			"examples/sso",
		},
		"doc/README.md": {
			"../examples/README.md",
			"../examples/sample-config-repo/README.md",
			"../examples/sample-pipeline/README.md",
			"../examples/sso/README.md",
		},
		"doc/local-keycloak-sso.md": {
			"examples/sso/keycloak",
			"docker compose -f examples/sso/keycloak/docker-compose.yaml up -d keycloak",
			"examples/sso/idp-test-pack",
		},
		"examples/sso/idp-test-pack/README.md": {
			"examples/sso/idp-test-pack",
			"python3 examples/sso/idp-test-pack/scripts/validate-fixtures.py",
			"ghcr.io/navikt/mock-oauth2-server:5.0.2",
		},
		"doc/wiki": {
			"artifacts under `examples/`",
			"examples/sso/keycloak",
			"examples/sso/idp-test-pack",
			"setting/system/auth.yaml",
		},
	} {
		contents := readTextFile(t, path)
		for _, want := range required {
			if !strings.Contains(contents, want) {
				t.Fatalf("%s should mention %q", path, want)
			}
		}
	}
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

func legacyPathReference(parts ...string) string {
	return filepath.ToSlash(filepath.Join(parts...))
}
