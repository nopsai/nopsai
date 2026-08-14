package nopsai

import (
	"fmt"
	"strings"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"

	"gopkg.in/yaml.v3"
)

// modelGitOpsFile is one model definition. The file name is the model name and
// the directory decides ownership: models/<name>.yaml is global, and
// models/<team>/<name>.yaml belongs to that team.
type modelGitOpsFile struct {
	Default bool `json:"default,omitempty" yaml:"default,omitempty"`

	// llmProfileForm supplies the optional name plus every provider setting.
	llmProfileForm `yaml:",inline"`
}

type storedModel struct {
	name       string
	profile    config.LLMProfile
	sourcePath string
}

type gitOpsModelPlan struct {
	// models holds every model file keyed by model name. A team-scoped model
	// carries its team in the name, exactly like a team-scoped pipeline.
	models map[string]storedModel
	// defaultModel is the model that declared default: true, if any.
	defaultModel string
}

func (p *gitOpsModelPlan) empty() bool {
	return p == nil || len(p.models) == 0
}

// parseGitOpsModels reads the model registry from models/<name>.yaml, where a
// team-scoped model lives at models/<team>/<name>.yaml just like a team-scoped
// pipeline lives at pipelines/<team>/<name>.yaml.
func parseGitOpsModels(files map[string]string, root string, binding models.ConfigRepository) (*gitOpsModelPlan, error) {
	plan := &gitOpsModelPlan{models: map[string]storedModel{}}
	var defaultResource registryGitOpsResource

	visit := func(resource registryGitOpsResource, content string) error {
		var file modelGitOpsFile
		if err := yaml.Unmarshal([]byte(content), &file); err != nil {
			return fmt.Errorf("failed to parse model '%s': %w", resource.sourcePath, err)
		}
		name := config.NormalizeLLMProfileName(resource.name)
		if name == "" {
			return fmt.Errorf("model '%s' has an invalid file name", resource.sourcePath)
		}
		if declared := strings.TrimSpace(file.llmProfileForm.Name); declared != "" {
			if config.NormalizeLLMProfileName(declared) != name {
				return fmt.Errorf("model '%s' declares name %q but the file name implies %q", resource.sourcePath, declared, name)
			}
		}
		form := file.llmProfileForm
		form.Name = name
		profile := config.NormalizeLLMProfile(profileConfigFromForm(form))
		if status, message := validateLLMProfileDefinition(name, profile); status != "valid" {
			return fmt.Errorf("invalid model '%s': %s", resource.sourcePath, message)
		}
		for _, scope := range profile.AllowedScopes {
			if strings.TrimSpace(scope) == "" {
				continue
			}
			if _, err := configsync.CleanPathSegments(scope, false); err != nil {
				return fmt.Errorf("invalid allowed scope %q in model '%s': %w", scope, resource.sourcePath, err)
			}
		}
		resource.name = name
		if existing, exists := plan.models[name]; exists {
			return fmt.Errorf("duplicate model '%s' defined by '%s' and '%s'", name, existing.sourcePath, resource.sourcePath)
		}
		plan.models[name] = storedModel{name: name, profile: profile, sourcePath: resource.sourcePath}
		if file.Default {
			winner, err := requireSingleRegistryDefault(defaultResource, resource, "model")
			if err != nil {
				return err
			}
			defaultResource = winner
			plan.defaultModel = name
		}
		return nil
	}
	if binding.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil, requireNoSystemRegistryFiles(files, root, modelsGitOpsDirectory)
	}
	if err := registryGitOpsFiles(files, root, "model", visit); err != nil {
		return nil, err
	}
	if plan.empty() {
		return nil, nil
	}
	return plan, nil
}

// registryModels returns the model registry from the plan.
func (p *gitOpsModelPlan) registryModels() (string, map[string]config.LLMProfile) {
	profiles := map[string]config.LLMProfile{}
	for _, stored := range p.models {
		profiles[stored.name] = stored.profile
	}
	if len(profiles) == 0 {
		return "", nil
	}
	defaultName := p.defaultModel
	if defaultName == "" {
		defaultName = config.DefaultLLMProfileName
	}
	if _, ok := profiles[defaultName]; !ok {
		for name := range profiles {
			if defaultName == "" || name < defaultName {
				defaultName = name
			}
		}
	}
	return defaultName, profiles
}

func buildModelGitOpsFile(name string, profile config.LLMProfile, isDefault bool) modelGitOpsFile {
	return modelGitOpsFile{
		Default:        isDefault,
		llmProfileForm: profileFormFromConfig(name, config.NormalizeLLMProfile(profile)),
	}
}
