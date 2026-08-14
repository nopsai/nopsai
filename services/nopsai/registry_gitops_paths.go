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
	name       string
	sourcePath string
}

// resolveRegistryGitOpsPath maps <directory>/<name>.yaml onto a resource name.
// The whole relative path is the name, so a team-qualified registry name such as
// team-1/platform/github round-trips as mcp/servers/team-1/platform/github.yaml.
// Team-owned registries are a separate concept and live under
// config-repositories/teams/<team>/, next to the other team-owned settings.
func resolveRegistryGitOpsPath(rel string) (registryGitOpsResource, bool, error) {
	rel = strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/")
	if rel == "" || strings.HasSuffix(rel, "/") {
		return registryGitOpsResource{}, false, nil
	}
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".yaml", ".yml":
	default:
		return registryGitOpsResource{}, false, nil
	}
	name := strings.TrimSuffix(rel, filepath.Ext(rel))
	if strings.TrimSpace(name) == "" {
		return registryGitOpsResource{}, false, fmt.Errorf("registry file name is required")
	}
	if _, err := configsync.CleanPathSegments(name, false); err != nil {
		return registryGitOpsResource{}, false, err
	}
	return registryGitOpsResource{name: name}, true, nil
}

// registryGitOpsFiles walks one system registry directory and yields every
// resource file it owns.
func registryGitOpsFiles(
	files map[string]string,
	root string,
	resourceLabel string,
	visit func(registryGitOpsResource, string) error,
) error {
	for path, content := range files {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, root)
		if !ok {
			continue
		}
		resource, ok, err := resolveRegistryGitOpsPath(rel)
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
// back to. The resource name is the path under the registry directory, so a
// team-scoped name such as team-1/reviewer lands in agent-roles/team-1/, in the
// same shape as pipelines and reusable steps.
func registryGitOpsExportPath(repo models.ConfigRepository, directory, name string) (string, bool) {
	name = strings.Trim(strings.TrimSpace(name), "/")
	if name == "" {
		return "", false
	}
	if _, ok := configRepositoryRelativeResourceIdentifier(repo, name); !ok {
		return "", false
	}
	return filepath.ToSlash(filepath.Join(directory, name+".yaml")), true
}

// requireSingleRegistryDefault keeps the default selection unambiguous: exactly
// one file in a registry may declare itself the default.
func requireSingleRegistryDefault(current, candidate registryGitOpsResource, resourceLabel string) (registryGitOpsResource, error) {
	if current.name == "" {
		return candidate, nil
	}
	paths := []string{current.sourcePath, candidate.sourcePath}
	sort.Strings(paths)
	return registryGitOpsResource{}, fmt.Errorf("multiple %s files set default: true: %s", resourceLabel, strings.Join(paths, ", "))
}

// requireNoSystemRegistryFiles keeps workspace registry definitions out of team
// repositories, where only team-owned registries under config-repositories/ are
// allowed.
func requireNoSystemRegistryFiles(files map[string]string, root, directory string) error {
	for path := range files {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, root)
		if !ok {
			continue
		}
		if _, ok, err := resolveRegistryGitOpsPath(rel); err != nil || !ok {
			continue
		}
		return fmt.Errorf("%s can only be defined by a system config repository; move '%s' under config-repositories/teams/<team>/%s/ to keep it team-owned", directory, normalized, directory)
	}
	return nil
}
