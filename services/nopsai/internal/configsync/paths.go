package configsync

import (
	"fmt"
	"path/filepath"
	"strings"

	"nopsai/pkg/models"
)

func NormalizeRepositoryBasePathValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.Trim(value, "/")
	if value == "." {
		return ""
	}
	return value
}

func RepoJoinPath(basePath, child string) string {
	basePath = NormalizeRepositoryBasePathValue(basePath)
	child = strings.Trim(strings.ReplaceAll(strings.TrimSpace(child), "\\", "/"), "/")
	if basePath == "" {
		return child
	}
	if child == "" {
		return basePath
	}
	return basePath + "/" + child
}

func RelativePath(path, dir string) (string, bool) {
	path = strings.Trim(strings.ReplaceAll(filepath.ToSlash(path), "\\", "/"), "/")
	dir = strings.Trim(strings.ReplaceAll(filepath.ToSlash(dir), "\\", "/"), "/")
	if dir == "" {
		return path, true
	}
	if path == dir {
		return "", true
	}
	prefix := dir + "/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	return strings.TrimPrefix(path, prefix), true
}

func NormalizePathForTeam(boundTeam string, repoRelativePath string) (string, error) {
	boundSegments, err := CleanPathSegments(boundTeam, false)
	if err != nil {
		return "", fmt.Errorf("invalid bound team: %w", err)
	}
	if len(boundSegments) == 0 {
		return "", fmt.Errorf("bound team is required")
	}

	relative := strings.Trim(strings.ReplaceAll(filepath.ToSlash(repoRelativePath), "\\", "/"), "/")
	if relative == "" {
		return strings.Join(boundSegments, "/"), nil
	}
	relative = StripResourcePrefix(relative)
	relative = strings.TrimSuffix(relative, filepath.Ext(relative))
	globalQualified := false
	if normalized, globalOnly := stripGlobalPathPrefix(relative); globalOnly {
		return "", nil
	} else if normalized != relative {
		relative = normalized
		globalQualified = true
	}
	relSegments, err := CleanPathSegments(relative, true)
	if err != nil {
		return "", err
	}
	if globalQualified {
		return strings.Join(relSegments, "/"), nil
	}
	if HasPathSegmentPrefix(relSegments, boundSegments) {
		relSegments = relSegments[len(boundSegments):]
	}

	finalSegments := append(append([]string{}, boundSegments...), relSegments...)
	return strings.Join(finalSegments, "/"), nil
}

func StripResourcePrefix(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) <= 1 {
		return path
	}
	switch strings.ToLower(strings.TrimSpace(parts[0])) {
	case "pipelines", "steps", "triggers", "scopes":
		return strings.Join(parts[1:], "/")
	default:
		return path
	}
}

func CleanPathSegments(path string, allowEmpty bool) ([]string, error) {
	path = strings.Trim(strings.ReplaceAll(filepath.ToSlash(path), "\\", "/"), "/")
	if path == "" {
		if allowEmpty {
			return nil, nil
		}
		return nil, fmt.Errorf("path is required")
	}
	if filepath.IsAbs(path) {
		return nil, fmt.Errorf("path must be relative")
	}
	parts := strings.Split(path, "/")
	segments := make([]string, 0, len(parts))
	for _, segment := range parts {
		segment = strings.TrimSpace(segment)
		if segment == "" || segment == "." || segment == ".." {
			return nil, fmt.Errorf("path contains invalid segment %q", segment)
		}
		segments = append(segments, segment)
	}
	return segments, nil
}

func HasPathSegmentPrefix(path, prefix []string) bool {
	if len(prefix) == 0 || len(path) < len(prefix) {
		return false
	}
	for idx := range prefix {
		if path[idx] != prefix[idx] {
			return false
		}
	}
	return true
}

func ResourceUnderAnyScope(resource string, scopes []string) bool {
	for _, scope := range scopes {
		if ResourceUnderScope(resource, scope) {
			return true
		}
	}
	return false
}

func ResourceUnderScope(resource, scope string) bool {
	resource = strings.Trim(strings.TrimSpace(resource), "/")
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	if resource == "" || scope == "" {
		return false
	}
	if resource == scope {
		return true
	}
	return strings.HasPrefix(resource, scope+"/")
}

func CanRepositoryWriteOver(current, existing models.ConfigRepository, resourceScope string) bool {
	if existing.ID == 0 || existing.ID == current.ID {
		return true
	}
	if !ResourceUnderBindingScope(resourceScope, current) {
		return false
	}
	if current.ScopeType == models.ConfigRepositoryScopeTeam {
		return existing.ScopeType == models.ConfigRepositoryScopeSystem ||
			(existing.ScopeType == models.ConfigRepositoryScopeTeam &&
				ResourceUnderScope(current.ScopeID, existing.ScopeID))
	}
	return false
}

func CanRepositoryAdoptUnmanagedResource(binding models.ConfigRepository, resourceScope string) bool {
	return ResourceUnderBindingScope(resourceScope, binding)
}

func RepositoryShadowsCurrent(existing, current models.ConfigRepository, resourceScope string) bool {
	if existing.ID == 0 || existing.ID == current.ID {
		return false
	}
	if !ResourceUnderBindingScope(resourceScope, existing) {
		return false
	}
	if existing.ScopeType == models.ConfigRepositoryScopeTeam {
		return current.ScopeType == models.ConfigRepositoryScopeSystem ||
			(current.ScopeType == models.ConfigRepositoryScopeTeam &&
				ResourceUnderScope(existing.ScopeID, current.ScopeID))
	}
	return false
}

func ResourceUnderBindingScope(resourceScope string, binding models.ConfigRepository) bool {
	switch binding.ScopeType {
	case models.ConfigRepositoryScopeSystem:
		return true
	case models.ConfigRepositoryScopeTeam:
		return ResourceUnderScope(resourceScope, binding.ScopeID)
	default:
		return false
	}
}

func SplitYAMLIdentifier(identifier string) (string, string, string, error) {
	trimmed := strings.TrimSpace(identifier)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "", "", "", fmt.Errorf("identifier cannot be empty")
	}

	normalized := filepath.ToSlash(trimmed)
	lower := strings.ToLower(normalized)
	var ext string
	switch {
	case strings.HasSuffix(lower, ".yaml"):
		ext = normalized[len(normalized)-len(".yaml"):]
		normalized = normalized[:len(normalized)-len(".yaml")]
	case strings.HasSuffix(lower, ".yml"):
		ext = normalized[len(normalized)-len(".yml"):]
		normalized = normalized[:len(normalized)-len(".yml")]
	}

	parts := strings.Split(normalized, "/")
	name := parts[len(parts)-1]
	if name == "" {
		return "", "", "", fmt.Errorf("identifier missing name")
	}

	var path string
	if len(parts) > 1 {
		path = strings.Join(parts[:len(parts)-1], "/")
		if normalizedPath, globalOnly := stripGlobalPathPrefix(path); globalOnly {
			path = ""
		} else {
			path = normalizedPath
		}
	}
	if strings.Contains(path, "..") {
		return "", "", "", fmt.Errorf("identifier contains invalid path segments")
	}

	return path, name, ext, nil
}

func SplitPipelineIdentifier(identifier string) (string, string, string, error) {
	return SplitYAMLIdentifier(identifier)
}

func NormalizePipelineIdentifierReference(identifier string) (string, bool, error) {
	trimmed := strings.Trim(strings.TrimSpace(identifier), "/")
	if trimmed == "" {
		return "", false, fmt.Errorf("identifier cannot be empty")
	}
	globalQualified := YAMLIdentifierPathHasGlobalPrefix(trimmed)
	path, name, _, err := SplitPipelineIdentifier(trimmed)
	if err != nil {
		return "", globalQualified, err
	}
	return BuildPipelineIdentifier(path, name), globalQualified, nil
}

func YAMLIdentifierPathHasGlobalPrefix(identifier string) bool {
	normalized := filepath.ToSlash(strings.Trim(strings.TrimSpace(identifier), "/"))
	lower := strings.ToLower(normalized)
	switch {
	case strings.HasSuffix(lower, ".yaml"):
		normalized = normalized[:len(normalized)-len(".yaml")]
	case strings.HasSuffix(lower, ".yml"):
		normalized = normalized[:len(normalized)-len(".yml")]
	}
	parts := strings.Split(normalized, "/")
	if len(parts) <= 1 {
		return false
	}
	path := strings.Join(parts[:len(parts)-1], "/")
	stripped, globalOnly := stripGlobalPathPrefix(path)
	return globalOnly || stripped != path
}

func BuildPipelineIdentifier(path, name string) string {
	if path == "" {
		return name
	}
	return path + "/" + name
}

func BuildPipelineFilePath(path, name, ext string) string {
	if ext == "" {
		ext = ".yaml"
	}
	if path == "" {
		return name + ext
	}
	return path + "/" + name + ext
}

func SplitStepIdentifier(identifier string) (string, string, string, error) {
	return SplitYAMLIdentifier(identifier)
}

func BuildStepIdentifier(path, name string) string {
	return BuildPipelineIdentifier(path, name)
}

func ParseScopeFilePath(rel string) (string, bool, error) {
	lower := strings.ToLower(rel)
	if !strings.HasSuffix(lower, "scope.yaml") && !strings.HasSuffix(lower, "scope.yml") {
		return "", false, nil
	}

	base := filepath.Base(rel)
	if !strings.EqualFold(base, "scope.yaml") && !strings.EqualFold(base, "scope.yml") {
		return "", false, nil
	}

	scopePath := strings.TrimSuffix(rel[:len(rel)-len(base)], "/")
	scopePath = strings.Trim(scopePath, "/")
	if scopePath != "" {
		if normalizedPath, globalOnly := stripGlobalPathPrefix(scopePath); globalOnly {
			scopePath = ""
		} else {
			scopePath = normalizedPath
		}
	}
	if scopePath != "" {
		if strings.Contains(scopePath, "..") {
			return "", false, fmt.Errorf("scope path contains invalid segments")
		}
		segments := strings.Split(scopePath, "/")
		for _, segment := range segments {
			if segment == "" {
				return "", false, fmt.Errorf("scope path contains empty segments")
			}
		}
		scopePath = filepath.ToSlash(scopePath)
	}

	return scopePath, true, nil
}

func stripGlobalPathPrefix(raw string) (string, bool) {
	path := strings.Trim(strings.TrimSpace(raw), "/")
	if isGlobalPath(path) {
		return "", true
	}
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return "", true
	}
	if !isGlobalPath(parts[0]) {
		return path, false
	}
	parts = parts[1:]
	if len(parts) == 0 {
		return "", true
	}
	return strings.Join(parts, "/"), false
}

func isGlobalPath(raw string) bool {
	switch strings.ToLower(strings.Trim(strings.TrimSpace(raw), "/")) {
	case "global":
		return true
	default:
		return false
	}
}
