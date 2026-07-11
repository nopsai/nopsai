package configsync

import (
	"sort"
	"strings"

	"nopsai/pkg/models"
)

type TeamStructureExportNode struct {
	Description string
	Config      *TeamStructureBindingExport
	Apps        []TeamStructureAppExport
	Children    map[string]*TeamStructureExportNode
}

type TeamStructureAppExport struct {
	Name    string `yaml:"name"`
	RepoURL string `yaml:"repo_url"`
}

type TeamStructureBindingExport struct {
	RepoURL      string `yaml:"repo_url"`
	Branch       string `yaml:"branch,omitempty"`
	BasePath     string `yaml:"base_path,omitempty"`
	Enabled      *bool  `yaml:"enabled,omitempty"`
	WriteEnabled *bool  `yaml:"write_enabled,omitempty"`
	WriteBranch  string `yaml:"write_branch,omitempty"`
}

func TeamStructureIncludesPath(repo models.ConfigRepository, path string) bool {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return false
	}
	if repo.ScopeType == models.ConfigRepositoryScopeSystem {
		return true
	}
	return ResourceUnderScope(path, repo.ScopeID)
}

func EnsureTeamStructureExportPath(structure map[string]*TeamStructureExportNode, path string) *TeamStructureExportNode {
	parts := strings.Split(strings.Trim(strings.TrimSpace(path), "/"), "/")
	children := structure
	var current *TeamStructureExportNode
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		current = children[part]
		if current == nil {
			current = &TeamStructureExportNode{Children: map[string]*TeamStructureExportNode{}}
			children[part] = current
		}
		if current.Children == nil {
			current.Children = map[string]*TeamStructureExportNode{}
		}
		children = current.Children
	}
	return current
}

func BuildTeamStructureAppExport(name, repoURL, repositoryFullName string) (TeamStructureAppExport, bool) {
	app := TeamStructureAppExport{
		Name:    strings.TrimSpace(name),
		RepoURL: strings.TrimSpace(repoURL),
	}
	repositoryFullName = strings.Trim(strings.TrimSpace(repositoryFullName), "/")
	if app.Name == "" {
		app.Name = RepositoryDisplayNameFromFullName(repositoryFullName)
	}
	if app.RepoURL == "" && repositoryFullName != "" {
		app.RepoURL = CanonicalRepositoryURL(repositoryFullName)
	}
	return app, app.Name != "" && app.RepoURL != ""
}

func TeamStructureExportMap(structure map[string]*TeamStructureExportNode) map[string]any {
	out := map[string]any{}
	names := make([]string, 0, len(structure))
	for name := range structure {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out[name] = teamStructureNodeExportMap(structure[name])
	}
	return out
}

func TeamStructureExportNodeMap(node *TeamStructureExportNode) map[string]any {
	return teamStructureNodeExportMap(node)
}

func teamStructureNodeExportMap(node *TeamStructureExportNode) map[string]any {
	out := map[string]any{}
	if node == nil {
		return out
	}
	if strings.TrimSpace(node.Description) != "" {
		out["description"] = strings.TrimSpace(node.Description)
	}
	if node.Config != nil {
		out["config"] = node.Config
	}
	if len(node.Apps) > 0 {
		sort.Slice(node.Apps, func(i, j int) bool {
			return node.Apps[i].Name < node.Apps[j].Name
		})
		out["apps"] = node.Apps
	}
	for name, child := range TeamStructureExportMap(node.Children) {
		out[name] = child
	}
	return out
}
