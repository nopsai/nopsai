package configsync

import (
	"fmt"
	"path/filepath"
	"strings"

	aaamodel "nopsai/services/aaa/pkg/model"

	"gopkg.in/yaml.v3"
)

type PipelineRunStructureNode struct {
	Description string
	Repos       []string
	Apps        []PipelineRunStructureApp
	Children    map[string]*PipelineRunStructureNode
	Config      *BindingFile
}

type PipelineRunStructureApp struct {
	Name               string
	RepoURL            string
	RepositoryFullName string
}

func ParsePipelineRunStructure(content string) (map[string]*PipelineRunStructureNode, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return map[string]*PipelineRunStructureNode{}, nil
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		return nil, err
	}
	result := make(map[string]*PipelineRunStructureNode, len(raw))
	for name, value := range raw {
		normalized, err := NormalizeStructureName(name)
		if err != nil {
			return nil, err
		}
		if _, exists := result[normalized]; exists {
			return nil, fmt.Errorf("duplicate folder '%s' in pipelinerun structure", normalized)
		}
		node, err := decodePipelineRunStructureNode(value)
		if err != nil {
			return nil, fmt.Errorf("folder '%s': %w", normalized, err)
		}
		result[normalized] = node
	}
	return result, nil
}

func ParseConfigRepositoryGroupPipelineRunStructure(rel, content string) (map[string]*PipelineRunStructureNode, bool, error) {
	scope, ok, err := configRepositoryGroupStructureFileScope(rel)
	if err != nil || !ok {
		return nil, ok, err
	}
	if scope == "" {
		structure, err := ParsePipelineRunStructure(content)
		return structure, true, err
	}

	node, err := ParsePipelineRunStructureNode(content)
	if err != nil {
		return nil, true, err
	}
	segments, err := CleanPathSegments(scope, false)
	if err != nil {
		return nil, true, err
	}
	structure := map[string]*PipelineRunStructureNode{}
	target := EnsurePipelineRunStructurePath(structure, segments)
	MergePipelineRunStructureNode(target, node)
	return structure, true, nil
}

func ParsePipelineRunStructureNode(content string) (*PipelineRunStructureNode, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return &PipelineRunStructureNode{Children: map[string]*PipelineRunStructureNode{}}, nil
	}

	var raw interface{}
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		return nil, err
	}
	return decodePipelineRunStructureNode(raw)
}

func NormalizePipelineRunStructureForFolder(boundFolder string, structure map[string]*PipelineRunStructureNode) (map[string]*PipelineRunStructureNode, error) {
	if len(structure) == 0 {
		return structure, nil
	}
	boundSegments, err := CleanPathSegments(boundFolder, false)
	if err != nil {
		return nil, err
	}
	result := map[string]*PipelineRunStructureNode{}

	var ensurePath func(path []string) *PipelineRunStructureNode
	ensurePath = func(path []string) *PipelineRunStructureNode {
		children := result
		var current *PipelineRunStructureNode
		for _, segment := range path {
			if node, ok := children[segment]; ok {
				current = node
			} else {
				current = &PipelineRunStructureNode{Children: map[string]*PipelineRunStructureNode{}}
				children[segment] = current
			}
			if current.Children == nil {
				current.Children = map[string]*PipelineRunStructureNode{}
			}
			children = current.Children
		}
		return current
	}

	var mergeNode func(path []string, node *PipelineRunStructureNode) error
	mergeNode = func(path []string, node *PipelineRunStructureNode) error {
		normalizedPath, err := NormalizePathForFolder(boundFolder, strings.Join(path, "/"))
		if err != nil {
			return err
		}
		targetSegments, err := CleanPathSegments(normalizedPath, false)
		if err != nil {
			return err
		}
		target := ensurePath(targetSegments)
		if node != nil {
			if description := strings.TrimSpace(node.Description); description != "" {
				target.Description = description
			}
			if node.Config != nil {
				target.Config = CopyBindingFile(node.Config)
			}
			target.Repos = append(target.Repos, node.Repos...)
			target.Apps = append(target.Apps, node.Apps...)
			for childName, childNode := range node.Children {
				childSegments, err := CleanPathSegments(childName, false)
				if err != nil {
					return err
				}
				if err := mergeNode(append(append([]string{}, path...), childSegments...), childNode); err != nil {
					return err
				}
			}
		}
		return nil
	}

	for name, node := range structure {
		segments, err := CleanPathSegments(name, false)
		if err != nil {
			return nil, err
		}
		if !HasPathSegmentPrefix(segments, boundSegments) {
			segments = append(append([]string{}, boundSegments...), segments...)
		}
		if err := mergeNode(segments, node); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func EnsurePipelineRunStructurePath(structure map[string]*PipelineRunStructureNode, segments []string) *PipelineRunStructureNode {
	children := structure
	var current *PipelineRunStructureNode
	for _, segment := range segments {
		if node, ok := children[segment]; ok {
			current = node
		} else {
			current = &PipelineRunStructureNode{Children: map[string]*PipelineRunStructureNode{}}
			children[segment] = current
		}
		if current.Children == nil {
			current.Children = map[string]*PipelineRunStructureNode{}
		}
		children = current.Children
	}
	return current
}

func MergePipelineRunStructureNode(target *PipelineRunStructureNode, source *PipelineRunStructureNode) {
	if target == nil || source == nil {
		return
	}
	if target.Children == nil {
		target.Children = map[string]*PipelineRunStructureNode{}
	}
	if description := strings.TrimSpace(source.Description); description != "" {
		target.Description = description
	}
	if source.Config != nil {
		target.Config = CopyBindingFile(source.Config)
	}
	if len(source.Repos) > 0 {
		target.Repos = append([]string{}, source.Repos...)
	}
	if len(source.Apps) > 0 {
		target.Apps = append([]PipelineRunStructureApp{}, source.Apps...)
	}
	for childName, childSource := range source.Children {
		childTarget, ok := target.Children[childName]
		if !ok {
			childTarget = &PipelineRunStructureNode{Children: map[string]*PipelineRunStructureNode{}}
			target.Children[childName] = childTarget
		}
		MergePipelineRunStructureNode(childTarget, childSource)
	}
}

func MergePipelineRunStructure(dst map[string]*PipelineRunStructureNode, src map[string]*PipelineRunStructureNode) {
	if len(src) == 0 {
		return
	}

	for name, source := range src {
		target, ok := dst[name]
		if !ok {
			target = &PipelineRunStructureNode{Children: map[string]*PipelineRunStructureNode{}}
			dst[name] = target
		}
		MergePipelineRunStructureNode(target, source)
	}
}

func FilterPipelineRunStructureByScopes(structure map[string]*PipelineRunStructureNode, scopes []string) map[string]*PipelineRunStructureNode {
	if len(structure) == 0 || len(scopes) == 0 {
		return structure
	}

	var filterNode func(path []string, node *PipelineRunStructureNode) *PipelineRunStructureNode
	filterNode = func(path []string, node *PipelineRunStructureNode) *PipelineRunStructureNode {
		if ResourceUnderAnyScope(strings.Join(path, "/"), scopes) {
			return nil
		}
		filtered := &PipelineRunStructureNode{Children: map[string]*PipelineRunStructureNode{}}
		if node != nil {
			filtered.Description = node.Description
			filtered.Repos = append([]string{}, node.Repos...)
			filtered.Apps = append([]PipelineRunStructureApp{}, node.Apps...)
			filtered.Config = CopyBindingFile(node.Config)
			for childName, childNode := range node.Children {
				child := filterNode(append(append([]string{}, path...), childName), childNode)
				if child != nil {
					filtered.Children[childName] = child
				}
			}
		}
		return filtered
	}

	filtered := map[string]*PipelineRunStructureNode{}
	for name, node := range structure {
		child := filterNode([]string{name}, node)
		if child != nil {
			filtered[name] = child
		}
	}
	return filtered
}

func NormalizeStructureName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("pipelinerun structure contains an empty folder or repository name")
	}
	if IsReservedRootGroupName(trimmed) {
		return "", fmt.Errorf("root is reserved and cannot be used as a group name")
	}
	return trimmed, nil
}

func IsReservedRootGroupName(name string) bool {
	normalized := strings.ToLower(strings.Trim(strings.TrimSpace(name), "/"))
	return normalized == "root" || normalized == strings.ToLower(aaamodel.FolderGeneralID)
}

func configRepositoryGroupStructureFileScope(rel string) (string, bool, error) {
	path := strings.Trim(strings.ReplaceAll(filepath.ToSlash(rel), "\\", "/"), "/")
	if path == "" || !isYAMLFile(path) {
		return "", false, nil
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] != "groups" {
		return "", false, nil
	}
	fileName := strings.ToLower(parts[len(parts)-1])
	if fileName != "structure.yaml" && fileName != "structure.yml" {
		return "", false, nil
	}
	if len(parts) == 2 {
		return "", true, fmt.Errorf("aggregate group structure file is not supported; use groups/<group>/structure.yaml")
	}
	scope := strings.Trim(strings.Join(parts[1:len(parts)-1], "/"), "/")
	if _, err := CleanPathSegments(scope, false); err != nil {
		return "", true, err
	}
	return scope, true, nil
}

func decodePipelineRunStructureNode(value interface{}) (*PipelineRunStructureNode, error) {
	node := &PipelineRunStructureNode{Children: map[string]*PipelineRunStructureNode{}}
	if value == nil {
		return node, nil
	}

	switch typed := value.(type) {
	case string:
		node.Description = strings.TrimSpace(typed)
		return node, nil
	case map[string]interface{}:
		return decodePipelineRunStructureMap(node, typed)
	default:
		return nil, fmt.Errorf("expected mapping or description for folder, got %T", value)
	}
}

func decodePipelineRunStructureMap(node *PipelineRunStructureNode, childMap map[string]interface{}) (*PipelineRunStructureNode, error) {
	for key, raw := range childMap {
		switch key {
		case "repos":
			return nil, fmt.Errorf("repos is not supported in group structure; use apps with name and repo_url")
		case "apps":
			apps, err := parseStructureAppList(raw)
			if err != nil {
				return nil, err
			}
			node.Repos = append(node.Repos, structureRepoNames(apps)...)
			node.Apps = append(node.Apps, apps...)
		case "description":
			if raw == nil {
				node.Description = ""
				continue
			}
			text, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("description must be a string, got %T", raw)
			}
			node.Description = strings.TrimSpace(text)
		case "config":
			config, err := parseStructureConfigRepositoryBinding(raw)
			if err != nil {
				return nil, err
			}
			node.Config = config
		default:
			childName, err := NormalizeStructureName(key)
			if err != nil {
				return nil, err
			}
			if _, exists := node.Children[childName]; exists {
				return nil, fmt.Errorf("duplicate folder '%s' detected", childName)
			}
			childNode, err := decodePipelineRunStructureNode(raw)
			if err != nil {
				return nil, fmt.Errorf("folder '%s': %w", childName, err)
			}
			node.Children[childName] = childNode
		}
	}

	return node, nil
}

func parseStructureConfigRepositoryBinding(raw interface{}) (*BindingFile, error) {
	if raw == nil {
		return nil, nil
	}
	encoded, err := yaml.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("config must be a mapping: %w", err)
	}
	var file BindingFile
	if err := yaml.Unmarshal(encoded, &file); err != nil {
		return nil, fmt.Errorf("config must match config repository binding schema: %w", err)
	}
	return &file, nil
}

func parseStructureAppList(value interface{}) ([]PipelineRunStructureApp, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("apps must be defined as a list, got %T", value)
	}
	var apps []PipelineRunStructureApp
	for idx, raw := range items {
		if raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case string:
			fullName, err := RepositoryFullNameFromURL(typed)
			if err != nil {
				return nil, fmt.Errorf("apps entry %d: %w", idx, err)
			}
			apps = append(apps, PipelineRunStructureApp{
				Name:               RepositoryDisplayNameFromFullName(fullName),
				RepoURL:            CanonicalRepositoryURL(fullName),
				RepositoryFullName: fullName,
			})
		case map[string]interface{}:
			name := strings.TrimSpace(stringMapValue(typed, "name"))
			repoURL := strings.TrimSpace(firstStringMapValue(typed, "repo_url", "repository_url", "url", "repo"))
			if repoURL == "" {
				return nil, fmt.Errorf("apps entry %d is missing repo_url", idx)
			}
			fullName, err := RepositoryFullNameFromURL(repoURL)
			if err != nil {
				return nil, fmt.Errorf("apps entry %d: %w", idx, err)
			}
			if name == "" {
				name = RepositoryDisplayNameFromFullName(fullName)
			}
			apps = append(apps, PipelineRunStructureApp{
				Name:               name,
				RepoURL:            repoURL,
				RepositoryFullName: fullName,
			})
		default:
			return nil, fmt.Errorf("apps entry %d must be a string or mapping, got %T", idx, raw)
		}
	}
	return apps, nil
}

func stringMapValue(values map[string]interface{}, key string) string {
	if raw, ok := values[key]; ok {
		if text, ok := raw.(string); ok {
			return text
		}
	}
	return ""
}

func firstStringMapValue(values map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := stringMapValue(values, key); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func structureRepoNames(apps []PipelineRunStructureApp) []string {
	repos := make([]string, 0, len(apps))
	for _, app := range apps {
		if app.RepositoryFullName != "" {
			repos = append(repos, app.RepositoryFullName)
		}
	}
	return repos
}

func isYAMLFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}
