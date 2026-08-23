package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/pkg/validation"
)

// Example pipelines are copied into the product wiki and pasted by readers, so
// they have to pass the same validator the API runs. A broken example is a
// broken first experience.
func TestExamplePipelinesValidate(t *testing.T) {
	root := filepath.Join("examples", "gitops-quickstart", "team-repo", "pipelines")
	var found int
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		found++
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		var pipeline models.Pipeline
		if unmarshalErr := yaml.Unmarshal(raw, &pipeline); unmarshalErr != nil {
			t.Fatalf("unmarshal %s: %v", path, unmarshalErr)
		}
		if validateErr := validation.ValidatePipeline(&pipeline); validateErr != nil {
			t.Fatalf("validate %s: %v", path, validateErr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if found == 0 {
		t.Fatalf("expected example pipelines under %s", root)
	}
}

// The wiki's pipeline chapter grows this manifest one directive at a time, so
// the example and the chapter's final page must stay the same document.
func TestChapterExamplePipelineIsComplete(t *testing.T) {
	raw := readTextFile(t, filepath.Join("examples", "gitops-quickstart", "team-repo", "pipelines", "platform", "release-service.yaml"))
	for _, directive := range []string{
		"display_option:",
		"llm_enabled:",
		"agent_role:",
		"knowledge_context:",
		"mcp_profiles:",
		"governance_level:",
		"variables:",
		"secrets:",
		"outputs:",
		"depends_on:",
		"tasks:",
		"condition:",
		"ignore_failure:",
		"approval:",
		"goal:",
		"include:",
		"sync:",
		"output:",
		"dashboard:",
	} {
		if !strings.Contains(raw, directive) {
			t.Fatalf("chapter example pipeline is missing %q", directive)
		}
	}
}
