package nopsai

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"nopsai/pkg/models"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"
)

// agentRoleGitOpsFile is one agent role definition. The file path is the role
// name: agent-roles/<name>.yaml defines a workspace role, and
// config-repositories/teams/<team>/agent-roles/<name>.yaml defines a team-owned
// role.
type agentRoleGitOpsFile struct {
	Name    string `json:"name,omitempty" yaml:"name,omitempty"`
	Default bool   `json:"default,omitempty" yaml:"default,omitempty"`

	agentProfileForm `yaml:",inline"`
}

// agentRoleExportDocument keeps the exported file free of the empty id field the
// shared API form would otherwise emit.
type agentRoleExportDocument struct {
	Name         string `yaml:"name"`
	Default      bool   `yaml:"default,omitempty"`
	DisplayName  string `yaml:"display_name"`
	Role         string `yaml:"role,omitempty"`
	Description  string `yaml:"description,omitempty"`
	Enabled      bool   `yaml:"enabled"`
	Instructions string `yaml:"instructions"`
}

type storedAgentRole struct {
	name       string
	profile    models.AgentProfile
	sourcePath string
}

type gitOpsAgentRolePlan struct {
	roles       map[string]storedAgentRole
	defaultRole string
}

func (p *gitOpsAgentRolePlan) empty() bool {
	return p == nil || len(p.roles) == 0
}

// parseGitOpsAgentRoles reads the agent role registry from
// agent-roles/<name>.yaml, where a team-scoped role lives at
// agent-roles/<team>/<name>.yaml in the same shape as a team-scoped pipeline.
func parseGitOpsAgentRoles(files map[string]string, root string, binding models.ConfigRepository) (*gitOpsAgentRolePlan, error) {
	plan := &gitOpsAgentRolePlan{roles: map[string]storedAgentRole{}}
	var defaultResource registryGitOpsResource

	visit := func(resource registryGitOpsResource, content string) error {
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
		if existing, exists := plan.roles[name]; exists {
			return fmt.Errorf("duplicate agent role '%s' defined by '%s' and '%s'", name, existing.sourcePath, resource.sourcePath)
		}
		plan.roles[name] = storedAgentRole{name: name, profile: profile, sourcePath: resource.sourcePath}
		if file.Default {
			winner, err := requireSingleRegistryDefault(defaultResource, resource, "agent role")
			if err != nil {
				return err
			}
			defaultResource = winner
			plan.defaultRole = name
		}
		return nil
	}
	if binding.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil, requireNoSystemRegistryFiles(files, root, agentRolesGitOpsDirectory)
	}
	if err := registryGitOpsFiles(files, root, "agent role", visit); err != nil {
		return nil, err
	}
	if plan.empty() {
		return nil, nil
	}
	return plan, nil
}

func (p *gitOpsAgentRolePlan) registryRoles() (string, map[string]models.AgentProfile) {
	profiles := map[string]models.AgentProfile{}
	for _, stored := range p.roles {
		profiles[stored.name] = stored.profile
	}
	if len(profiles) == 0 {
		return "", nil
	}
	defaultName := p.defaultRole
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

// persistGitOpsAgentRolesToTx replaces the Git-owned agent role registry and its
// default with the roles this repository declares.
func persistGitOpsAgentRolesToTx(
	ctx context.Context,
	tx pgx.Tx,
	plan *gitOpsAgentRolePlan,
	defaultRole string,
	roles map[string]models.AgentProfile,
	configRepoID int64,
	commitSHA string,
) error {
	if plan == nil {
		return nil
	}
	ids := make([]string, 0, len(roles))
	for id := range roles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM agent_profiles WHERE source = 'gitops'`); err != nil {
			return fmt.Errorf("clear GitOps agent roles: %w", err)
		}
	} else if _, err := tx.Exec(ctx, `DELETE FROM agent_profiles WHERE source = 'gitops' AND id != ALL($1::text[])`, ids); err != nil {
		return fmt.Errorf("prune GitOps agent roles: %w", err)
	}
	for _, id := range ids {
		if err := persistAgentProfileToTx(ctx, tx, roles[id], "gitops", plan.roles[id].sourcePath, commitSHA, configRepoID, true); err != nil {
			return err
		}
	}
	return persistAgentProfileDefaultToTx(ctx, tx, defaultRole)
}

func buildAgentRoleGitOpsFile(profile models.AgentProfile, isDefault bool) agentRoleExportDocument {
	profile = models.NormalizeAgentProfile(profile)
	return agentRoleExportDocument{
		Name:         profile.ID,
		Default:      isDefault,
		DisplayName:  profile.DisplayName,
		Role:         profile.Role,
		Description:  profile.Description,
		Enabled:      profile.Enabled,
		Instructions: profile.Instructions,
	}
}
