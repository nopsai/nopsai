package nopsai

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
)

// GitOps directories that hold one file per registry resource, in the same
// shape as pipelines, steps, and knowledge documents. A file directly under the
// directory defines a global resource; a file under a team segment defines a
// team-owned resource.
const (
	modelsGitOpsDirectory      = "models"
	agentRolesGitOpsDirectory  = "agent-roles"
	mcpGitOpsDirectory         = "mcp"
	mcpServersGitOpsDirectory  = mcpGitOpsDirectory + "/servers"
	mcpProfilesGitOpsDirectory = mcpGitOpsDirectory + "/profiles"
)

// registryGitOpsResource is one parsed registry file location: which team owns
// it, what the resource is called, and where the file came from.
type registryGitOpsResource struct {
	team       string
	name       string
	sourcePath string
}

func (r registryGitOpsResource) key() string {
	if r.team == "" {
		return r.name
	}
	return r.team + "/" + r.name
}

// resolveRegistryGitOpsPath maps <directory>/<team...>/<name>.yaml onto a team
// and resource name. Team-scoped repositories normalize the team the same way
// pipelines and knowledge documents do, so a team repository can use either the
// bare name or its own team prefix.
func resolveRegistryGitOpsPath(rel string, binding models.ConfigRepository, boundTeam string) (registryGitOpsResource, bool, error) {
	rel = strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/")
	if rel == "" || strings.HasSuffix(rel, "/") {
		return registryGitOpsResource{}, false, nil
	}
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".yaml", ".yml":
	default:
		return registryGitOpsResource{}, false, nil
	}
	parts := strings.Split(rel, "/")
	fileName := parts[len(parts)-1]
	name := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	if strings.TrimSpace(name) == "" {
		return registryGitOpsResource{}, false, fmt.Errorf("registry file name is required")
	}
	team := strings.Trim(strings.Join(parts[:len(parts)-1], "/"), "/")
	if team != "" {
		if _, err := configsync.CleanPathSegments(team, false); err != nil {
			return registryGitOpsResource{}, false, err
		}
	}
	if binding.ScopeType == models.ConfigRepositoryScopeTeam {
		normalizedTeam, err := configsync.NormalizePathForTeam(boundTeam, team)
		if err != nil {
			return registryGitOpsResource{}, false, err
		}
		team = normalizedTeam
	}
	return registryGitOpsResource{team: team, name: name}, true, nil
}

// registryGitOpsFiles walks one registry directory and yields every resource
// file it owns.
func registryGitOpsFiles(
	files map[string]string,
	root string,
	binding models.ConfigRepository,
	boundTeam string,
	resourceLabel string,
	visit func(registryGitOpsResource, string) error,
) error {
	for path, content := range files {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, root)
		if !ok {
			continue
		}
		resource, ok, err := resolveRegistryGitOpsPath(rel, binding, boundTeam)
		if err != nil {
			return fmt.Errorf("invalid %s path '%s': %w", resourceLabel, normalized, err)
		}
		if !ok {
			continue
		}
		resource.sourcePath = normalized
		if err := visit(resource, content); err != nil {
			return err
		}
	}
	return nil
}

// registryGitOpsExportPath is the canonical file a registry resource is written
// back to, or false when the repository does not own that team.
func registryGitOpsExportPath(repo models.ConfigRepository, directory, team, name string) (string, bool) {
	name = strings.Trim(strings.TrimSpace(name), "/")
	if name == "" {
		return "", false
	}
	team = strings.Trim(strings.TrimSpace(team), "/")
	if team == "" {
		if repo.ScopeType != models.ConfigRepositoryScopeSystem {
			return "", false
		}
		return filepath.ToSlash(filepath.Join(directory, name+".yaml")), true
	}
	relTeam, ok := configRepositoryRelativeResourceIdentifier(repo, team)
	if !ok {
		return "", false
	}
	if repo.ScopeType == models.ConfigRepositoryScopeTeam {
		team = strings.Trim(relTeam, "/")
		if team == "" {
			return filepath.ToSlash(filepath.Join(directory, name+".yaml")), true
		}
	}
	return filepath.ToSlash(filepath.Join(directory, team, name+".yaml")), true
}

// requireSingleRegistryDefault keeps the default selection unambiguous: exactly
// one file per scope may declare itself the default.
func requireSingleRegistryDefault(current, candidate registryGitOpsResource, resourceLabel string) (registryGitOpsResource, error) {
	if current.name == "" {
		return candidate, nil
	}
	scope := "global"
	if candidate.team != "" {
		scope = "team " + candidate.team
	}
	paths := []string{current.sourcePath, candidate.sourcePath}
	sort.Strings(paths)
	return registryGitOpsResource{}, fmt.Errorf("multiple %s files set default: true for %s: %s", resourceLabel, scope, strings.Join(paths, ", "))
}
