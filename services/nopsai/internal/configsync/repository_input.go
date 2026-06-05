package configsync

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"nopsai/pkg/models"
)

type RepositoryInputRequest struct {
	RepoURL      string `json:"repo_url"`
	Branch       string `json:"branch"`
	BasePath     string `json:"base_path"`
	Enabled      *bool  `json:"enabled"`
	WriteEnabled *bool  `json:"write_enabled"`
	WriteBranch  string `json:"write_branch"`
}

func BuildRepositoryInput(req RepositoryInputRequest, scopeType, scopeID, actor string) (models.ConfigRepositoryInput, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.Trim(strings.TrimSpace(scopeID), "/")
	if scopeType != models.ConfigRepositoryScopeFolder && scopeType != models.ConfigRepositoryScopeSystem {
		return models.ConfigRepositoryInput{}, fmt.Errorf("scope_type must be folder or system")
	}
	if scopeID == "" {
		return models.ConfigRepositoryInput{}, fmt.Errorf("scope_id is required")
	}

	repoURL := strings.TrimSpace(req.RepoURL)
	if repoURL == "" {
		return models.ConfigRepositoryInput{}, fmt.Errorf("repo_url is required")
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = "main"
	}
	basePath, err := NormalizeRepositoryBasePathForRequest(req.BasePath)
	if err != nil {
		return models.ConfigRepositoryInput{}, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	writeEnabled := false
	if req.WriteEnabled != nil {
		writeEnabled = *req.WriteEnabled
	}
	writeBranch := strings.TrimSpace(req.WriteBranch)
	if writeEnabled && writeBranch == "" {
		writeBranch = "nopsai/ui-changes"
	}
	if writeBranch != "" {
		if err := ValidateBranchName(writeBranch, "write_branch"); err != nil {
			return models.ConfigRepositoryInput{}, err
		}
	}

	return models.ConfigRepositoryInput{
		ScopeType:    scopeType,
		ScopeID:      scopeID,
		RepoURL:      repoURL,
		Branch:       branch,
		BasePath:     basePath,
		Enabled:      enabled,
		WriteEnabled: writeEnabled,
		WriteBranch:  writeBranch,
		Actor:        actor,
	}, nil
}

func NormalizeRepositoryBasePathForRequest(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	normalized = strings.ReplaceAll(normalized, "\\", "/")
	if filepath.IsAbs(normalized) {
		return "", fmt.Errorf("base_path must be relative")
	}
	normalized = strings.Trim(normalized, "/")
	if normalized == "." {
		return "", nil
	}
	if normalized == "" {
		return "", nil
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("base_path contains invalid path segments")
		}
	}
	return normalized, nil
}

func CleanRepositoryWritePath(basePath, rawPath string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(rawPath), "\\", "/")
	normalized = strings.TrimPrefix(normalized, "/")
	cleaned := path.Clean(normalized)
	if cleaned == "." || cleaned == "" || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("invalid file path: %s", rawPath)
	}
	base, err := NormalizeRepositoryBasePathForRequest(basePath)
	if err != nil {
		return "", err
	}
	if base == "" {
		return cleaned, nil
	}
	return base + "/" + cleaned, nil
}
