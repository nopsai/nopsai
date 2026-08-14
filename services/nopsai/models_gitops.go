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
	team       string
	name       string
	profile    config.LLMProfile
	sourcePath string
}

type gitOpsModelPlan struct {
	// models holds every model file, keyed by team/name for team models and by
	// name for global models.
	models map[string]storedModel
	// defaults maps a team ("" for global) to the model that declared itself
	// the default for that scope.
	defaults map[string]string
}

func (p *gitOpsModelPlan) empty() bool {
	return p == nil || len(p.models) == 0
}

func parseGitOpsModels(files map[string]string, root string, binding models.ConfigRepository, boundTeam string) (*gitOpsModelPlan, error) {
	plan := &gitOpsModelPlan{models: map[string]storedModel{}, defaults: map[string]string{}}
	defaultResources := map[string]registryGitOpsResource{}

	err := registryGitOpsFiles(files, root, binding, boundTeam, "model", func(resource registryGitOpsResource, content string) error {
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
		key := resource.key()
		if existing, exists := plan.models[key]; exists {
			return fmt.Errorf("duplicate model '%s' defined by '%s' and '%s'", key, existing.sourcePath, resource.sourcePath)
		}
		plan.models[key] = storedModel{
			team:       resource.team,
			name:       name,
			profile:    profile,
			sourcePath: resource.sourcePath,
		}
		if file.Default {
			winner, err := requireSingleRegistryDefault(defaultResources[resource.team], resource, "model")
			if err != nil {
				return err
			}
			defaultResources[resource.team] = winner
			plan.defaults[resource.team] = name
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if plan.empty() {
		return nil, nil
	}
	for team, defaultName := range plan.defaults {
		key := defaultName
		if team != "" {
			key = team + "/" + defaultName
		}
		if _, ok := plan.models[key]; !ok {
			return nil, fmt.Errorf("model default %q is not defined for scope %q", defaultName, team)
		}
	}
	return plan, nil
}

// globalModels returns the system-wide model registry from the plan.
func (p *gitOpsModelPlan) globalModels() (string, map[string]config.LLMProfile) {
	profiles := map[string]config.LLMProfile{}
	for _, stored := range p.models {
		if stored.team != "" {
			continue
		}
		profiles[stored.name] = stored.profile
	}
	if len(profiles) == 0 {
		return "", nil
	}
	defaultName := p.defaults[""]
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

// teamModels groups team-owned models by team path.
func (p *gitOpsModelPlan) teamModels() map[string]map[string]storedModel {
	byTeam := map[string]map[string]storedModel{}
	for _, stored := range p.models {
		if stored.team == "" {
			continue
		}
		if byTeam[stored.team] == nil {
			byTeam[stored.team] = map[string]storedModel{}
		}
		byTeam[stored.team][stored.name] = stored
	}
	return byTeam
}

func buildModelGitOpsFile(name string, profile config.LLMProfile, isDefault bool) modelGitOpsFile {
	return modelGitOpsFile{
		Default:        isDefault,
		llmProfileForm: profileFormFromConfig(name, config.NormalizeLLMProfile(profile)),
	}
}
