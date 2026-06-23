package nopsai

import (
	"os"
	"strings"
	"testing"
)

func TestBaseImageBuildsSeparateCLIAndAPIBinaries(t *testing.T) {
	contents, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{
		"ARG VERSION=dev",
		"nopsai/pkg/buildinfo.Version=${VERSION}",
		"nopsai/pkg/buildinfo.ReleaseManifestDigest=${RELEASE_MANIFEST_DIGEST}",
		"-o /out/nopsai ./cmd/nopsai-cli",
		"-o /out/nopsai-api ./services/nopsai/cmd/nopsai",
		"-o /out/nopsai-aaa ./services/aaa",
		"COPY --from=builder /out/nopsai /nopsai",
		"COPY --from=builder /out/nopsai-api /nopsai-api",
		"COPY --from=builder /out/nopsai-aaa /nopsai-aaa",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("Dockerfile does not contain %q", required)
		}
	}
}

func TestServiceImagesCarrySharedOCIReleaseLabels(t *testing.T) {
	for _, path := range []string{
		"container/Dockerfile.nopsai",
		"container/Dockerfile.aaa",
		"container/Dockerfile.agent",
		"container/Dockerfile.dispatcher",
		"container/Dockerfile.git-bot",
		"container/Dockerfile.docker-runner",
		"container/Dockerfile.k8s-runner",
		"container/Dockerfile.pipeline",
		"container/Dockerfile.socket-proxy",
		"services/ui/Dockerfile",
	} {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(contents)
		for _, label := range []string{"org.opencontainers.image.version", "org.opencontainers.image.revision", "org.opencontainers.image.created"} {
			if !strings.Contains(text, label) {
				t.Errorf("%s is missing %s", path, label)
			}
		}
	}
}

func TestAPIImageRunsRenamedBinaryAsNonRoot(t *testing.T) {
	contents, err := os.ReadFile("container/Dockerfile.nopsai")
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{"/nopsai-api /usr/local/bin/nopsai-api", "USER nopsai", `ENTRYPOINT ["nopsai-api"]`} {
		if !strings.Contains(text, required) {
			t.Errorf("container/Dockerfile.nopsai does not contain %q", required)
		}
	}
	if strings.Contains(text, `ENTRYPOINT ["nopsai"]`) {
		t.Fatal("API image still starts the CLI binary name")
	}
}

func TestAAAAndAgentImagesConsumeBaseImageArtifacts(t *testing.T) {
	for _, tt := range []struct {
		path     string
		artifact string
	}{
		{path: "container/Dockerfile.aaa", artifact: "/nopsai-aaa /usr/local/bin/nopsai-aaa"},
		{path: "container/Dockerfile.agent", artifact: "/nopsai-agent /usr/local/bin/nopsai-agent"},
	} {
		contents, err := os.ReadFile(tt.path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(contents)
		for _, required := range []string{"ARG BASE_IMAGE=", "FROM ${BASE_IMAGE} AS base", "COPY --from=base " + tt.artifact} {
			if !strings.Contains(text, required) {
				t.Errorf("%s does not contain %q", tt.path, required)
			}
		}
		if strings.Contains(text, "go build") || strings.Contains(text, "go mod download") {
			t.Errorf("%s should copy from nopsai-base instead of rebuilding Go sources", tt.path)
		}
	}
}
