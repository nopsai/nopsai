package configsync

import (
	"fmt"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/credentials"
)

type BindingFile struct {
	ScopeType     string `yaml:"scope_type" json:"scope_type"`
	ScopeID       string `yaml:"scope_id" json:"scope_id"`
	Provider      string `yaml:"provider" json:"provider"`
	RepoURL       string `yaml:"repo_url" json:"repo_url"`
	Branch        string `yaml:"branch" json:"branch"`
	BasePath      string `yaml:"base_path" json:"base_path"`
	CredentialRef string `yaml:"credential_ref" json:"credential_ref"`
	Enabled       *bool  `yaml:"enabled" json:"enabled"`
	WriteEnabled  *bool  `yaml:"write_enabled" json:"write_enabled"`
	WriteBranch   string `yaml:"write_branch" json:"write_branch"`
}

func ParseBindingPath(rel string) (string, string, error) {
	path, name, _, err := SplitYAMLIdentifier(rel)
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", fmt.Errorf("binding path must start with teams/")
	}
	switch parts[0] {
	case "teams":
		scopeID := strings.Trim(strings.Join(append(parts[1:], name), "/"), "/")
		if scopeID == "" {
			return "", "", fmt.Errorf("team binding is missing a team path")
		}
		if _, err := CleanPathSegments(scopeID, false); err != nil {
			return "", "", err
		}
		return models.ConfigRepositoryScopeTeam, scopeID, nil
	default:
		return "", "", fmt.Errorf("unsupported config repository binding scope %q", parts[0])
	}
}

func ValidateBindingFile(file BindingFile, scopeType, scopeID, sourcePath string) error {
	if declaredScopeType := strings.TrimSpace(file.ScopeType); declaredScopeType != "" && declaredScopeType != scopeType {
		return fmt.Errorf("config repository binding '%s' declares scope_type %q but path implies %q", sourcePath, declaredScopeType, scopeType)
	}
	if declaredScopeID := strings.Trim(strings.TrimSpace(file.ScopeID), "/"); declaredScopeID != "" && declaredScopeID != scopeID {
		return fmt.Errorf("config repository binding '%s' declares scope_id %q but path implies %q", sourcePath, declaredScopeID, scopeID)
	}
	if strings.TrimSpace(file.RepoURL) == "" {
		return fmt.Errorf("config repository binding '%s' is missing repo_url", sourcePath)
	}
	provider, err := NormalizeRepositoryProvider(file.Provider, file.RepoURL)
	if err != nil {
		return fmt.Errorf("config repository binding '%s' has invalid provider: %w", sourcePath, err)
	}
	if strings.TrimSpace(file.CredentialRef) != "" {
		if _, err := credentials.ParseReference(file.CredentialRef); err != nil {
			return fmt.Errorf("config repository binding '%s' has invalid credential_ref: %w", sourcePath, err)
		}
	}
	if provider != models.ConfigRepositoryProviderGitHub && strings.TrimSpace(file.CredentialRef) == "" {
		return fmt.Errorf("config repository binding '%s' requires credential_ref for provider %s", sourcePath, provider)
	}
	if strings.TrimSpace(file.WriteBranch) != "" {
		if err := ValidateBranchName(file.WriteBranch, "write_branch"); err != nil {
			return fmt.Errorf("invalid config repository binding '%s': %w", sourcePath, err)
		}
	}
	return nil
}

func ValidateBranchName(value, field string) error {
	branch := strings.TrimSpace(value)
	if branch == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.HasPrefix(branch, "/") || strings.HasSuffix(branch, "/") || strings.HasPrefix(branch, "refs/") {
		return fmt.Errorf("%s must be a branch name, not a ref path", field)
	}
	if strings.HasSuffix(branch, ".") || branch == "@" {
		return fmt.Errorf("%s is not a valid branch name", field)
	}
	invalidFragments := []string{"..", "//", "@{", "\\", ":", "?", "*", "[", "^", "~", " "}
	for _, fragment := range invalidFragments {
		if strings.Contains(branch, fragment) {
			return fmt.Errorf("%s contains invalid branch characters", field)
		}
	}
	for _, segment := range strings.Split(branch, "/") {
		if segment == "" || strings.HasPrefix(segment, ".") || strings.HasSuffix(segment, ".lock") {
			return fmt.Errorf("%s contains invalid branch path segments", field)
		}
	}
	return nil
}

func BindingWriteSettings(file BindingFile) (bool, string) {
	writeEnabled := false
	if file.WriteEnabled != nil {
		writeEnabled = *file.WriteEnabled
	}
	writeBranch := strings.TrimSpace(file.WriteBranch)
	if writeEnabled && writeBranch == "" {
		writeBranch = "nopsai/ui-changes"
	}
	return writeEnabled, writeBranch
}

func CopyBindingFile(file *BindingFile) *BindingFile {
	if file == nil {
		return nil
	}
	copied := *file
	if file.Enabled != nil {
		enabled := *file.Enabled
		copied.Enabled = &enabled
	}
	if file.WriteEnabled != nil {
		writeEnabled := *file.WriteEnabled
		copied.WriteEnabled = &writeEnabled
	}
	return &copied
}
