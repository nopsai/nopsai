package nopsai

import (
	"fmt"
	"strings"

	"nopsai/pkg/models"

	"gopkg.in/yaml.v3"
)

// agentRoleGitOpsFile is one agent role definition. The file name is the role
// id: agent-roles/<name>.yaml is global and agent-roles/<team>/<name>.yaml
// belongs to that team.
type agentRoleGitOpsFile struct {
	Name    string `json:"name,omitempty" yaml:"name,omitempty"`
	Default bool   `json:"default,omitempty" yaml:"default,omitempty"`

	agentProfileForm `yaml:",inline"`
}

type storedAgentRole struct {
	team       string
	name       string
	profile    models.AgentProfile
	sourcePath string
}

type gitOpsAgentRolePlan struct {
	roles    map[string]storedAgentRole
	defaults map[string]string
}

func (p *gitOpsAgentRolePlan) empty() bool {
	return p == nil || len(p.roles) == 0
}

func parseGitOpsAgentRoles(files map[string]string, root string, binding models.ConfigRepository, boundTeam string) (*gitOpsAgentRolePlan, error) {
	plan := &gitOpsAgentRolePlan{roles: map[string]storedAgentRole{}, defaults: map[string]string{}}
	defaultResources := map[string]registryGitOpsResource{}

	err := registryGitOpsFiles(files, root, binding, boundTeam, "agent role", func(resource registryGitOpsResource, content string) error {
		var file agentRoleGitOpsFile
		if err := yaml.Unmarshal([]byte(content), &file); err != nil {
			return fmt.Errorf("failed to parse agent role '%s': %w", resource.sourcePath, err)
		}
		name := models.NormalizeAgentProfileID(resource.name)
		if name == "" {
			return fmt.Errorf("agent role '%s' has an invalid file name", resource.sourcePath)
		}
		for _, declared := range []string{file.Name, file.agentProfileForm.ID} {
			if strings.TrimSpace(declared) == "" {
				continue
			}
			if models.NormalizeAgentProfileID(declared) != name {
				return fmt.Errorf("agent role '%s' declares name %q but the file name implies %q", resource.sourcePath, declared, name)
			}
		}
		form := file.agentProfileForm
		form.ID = name
		profile := agentProfileFromForm(form, "git")
		if err := validateAgentProfileDefinition(profile); err != nil {
			return fmt.Errorf("invalid agent role '%s': %w", resource.sourcePath, err)
		}
		resource.name = name
		key := resource.key()
		if existing, exists := plan.roles[key]; exists {
			return fmt.Errorf("duplicate agent role '%s' defined by '%s' and '%s'", key, existing.sourcePath, resource.sourcePath)
		}
		plan.roles[key] = storedAgentRole{
			team:       resource.team,
			name:       name,
			profile:    profile,
			sourcePath: resource.sourcePath,
		}
		if file.Default {
			winner, err := requireSingleRegistryDefault(defaultResources[resource.team], resource, "agent role")
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
		if _, ok := plan.roles[key]; !ok {
			return nil, fmt.Errorf("agent role default %q is not defined for scope %q", defaultName, team)
		}
	}
	return plan, nil
}

func (p *gitOpsAgentRolePlan) globalRoles() (string, map[string]models.AgentProfile) {
	profiles := map[string]models.AgentProfile{}
	for _, stored := range p.roles {
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
		defaultName = models.DefaultAgentProfileID
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

func (p *gitOpsAgentRolePlan) teamRoles() map[string]map[string]storedAgentRole {
	byTeam := map[string]map[string]storedAgentRole{}
	for _, stored := range p.roles {
		if stored.team == "" {
			continue
		}
		if byTeam[stored.team] == nil {
			byTeam[stored.team] = map[string]storedAgentRole{}
		}
		byTeam[stored.team][stored.name] = stored
	}
	return byTeam
}

func buildAgentRoleGitOpsFile(profile models.AgentProfile, isDefault bool) agentRoleGitOpsFile {
	form := agentProfileFormFromModel(models.NormalizeAgentProfile(profile))
	name := form.ID
	form.ID = ""
	return agentRoleGitOpsFile{
		Name:             name,
		Default:          isDefault,
		agentProfileForm: form,
	}
}
