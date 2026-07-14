package configsync

import (
	"fmt"
	"net/url"
	"strings"

	"nopsai/pkg/models"
)

type RepositoryIdentity struct {
	Provider    string
	Scheme      string
	Host        string
	Owner       string
	Repo        string
	ProjectPath string
	WebURL      string
}

func ParseGitHubRepoURL(raw string) (string, string, error) {
	return ParseRepositoryIdentifier(raw, "config repository URL")
}

func NormalizeRepositoryProvider(raw, repoURL string) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(raw))
	if provider == "" {
		provider = InferRepositoryProvider(repoURL)
	}
	switch provider {
	case "", "git", models.ConfigRepositoryProviderGitHub:
		return models.ConfigRepositoryProviderGitHub, nil
	case models.ConfigRepositoryProviderGitLab,
		models.ConfigRepositoryProviderBitbucket,
		models.ConfigRepositoryProviderGitea:
		return provider, nil
	default:
		return "", fmt.Errorf("provider must be one of github, gitlab, bitbucket, or gitea")
	}
}

func InferRepositoryProvider(repoURL string) string {
	host, _, err := repositoryHostAndPath(repoURL)
	if err != nil {
		return models.ConfigRepositoryProviderGitHub
	}
	host = strings.ToLower(host)
	switch {
	case host == "gitlab.com" || strings.HasSuffix(host, ".gitlab.com"):
		return models.ConfigRepositoryProviderGitLab
	case host == "bitbucket.org" || strings.HasSuffix(host, ".bitbucket.org"):
		return models.ConfigRepositoryProviderBitbucket
	case strings.Contains(host, "gitea"):
		return models.ConfigRepositoryProviderGitea
	default:
		return models.ConfigRepositoryProviderGitHub
	}
}

func ParseRepositoryIdentity(raw, provider string) (RepositoryIdentity, error) {
	provider, err := NormalizeRepositoryProvider(provider, raw)
	if err != nil {
		return RepositoryIdentity{}, err
	}
	scheme, host, repoPath, err := repositorySchemeHostAndPath(raw)
	if err != nil {
		return RepositoryIdentity{}, err
	}
	repoPath = strings.Trim(strings.TrimSuffix(repoPath, ".git"), "/")
	if repoPath == "" {
		return RepositoryIdentity{}, fmt.Errorf("invalid repository URL: %s", raw)
	}
	parts := strings.Split(repoPath, "/")
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || part == "." || part == ".." {
			return RepositoryIdentity{}, fmt.Errorf("invalid repository URL: %s", raw)
		}
	}
	if len(parts) < 2 {
		return RepositoryIdentity{}, fmt.Errorf("invalid repository URL: %s", raw)
	}

	identity := RepositoryIdentity{
		Provider:    provider,
		Scheme:      scheme,
		Host:        strings.ToLower(strings.TrimSpace(host)),
		ProjectPath: repoPath,
		WebURL:      repositoryWebURL(host, repoPath),
	}
	switch provider {
	case models.ConfigRepositoryProviderGitLab:
		identity.Owner = strings.Join(parts[:len(parts)-1], "/")
		identity.Repo = parts[len(parts)-1]
	default:
		identity.Owner = parts[len(parts)-2]
		identity.Repo = parts[len(parts)-1]
		identity.ProjectPath = identity.Owner + "/" + identity.Repo
	}
	return identity, nil
}

func repositoryHostAndPath(raw string) (string, string, error) {
	_, host, repoPath, err := repositorySchemeHostAndPath(raw)
	return host, repoPath, err
}

func repositorySchemeHostAndPath(raw string) (string, string, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", "", fmt.Errorf("repository URL is empty")
	}
	trimmed = strings.TrimSuffix(trimmed, ".git")
	if strings.HasPrefix(trimmed, "git@") {
		withoutUser := strings.TrimPrefix(trimmed, "git@")
		parts := strings.SplitN(withoutUser, ":", 2)
		if len(parts) != 2 {
			return "", "", "", fmt.Errorf("invalid repository URL: %s", raw)
		}
		return "", parts[0], strings.Trim(parts[1], "/"), nil
	}
	if strings.Contains(trimmed, "://") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return "", "", "", fmt.Errorf("invalid repository URL: %w", err)
		}
		if u.Host == "" {
			return "", "", "", fmt.Errorf("invalid repository URL: %s", raw)
		}
		path := strings.Trim(u.Path, "/")
		if strings.HasPrefix(path, "scm/") {
			path = strings.TrimPrefix(path, "scm/")
		}
		return strings.ToLower(strings.TrimSpace(u.Scheme)), u.Host, path, nil
	}
	trimmed = strings.Trim(trimmed, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid repository URL: %s", raw)
	}
	return "", "", strings.Join(parts, "/"), nil
}

func repositoryWebURL(host, repoPath string) string {
	host = strings.TrimSpace(host)
	repoPath = strings.Trim(strings.TrimSpace(repoPath), "/")
	if repoPath == "" {
		return ""
	}
	if host == "" {
		return "https://github.com/" + repoPath
	}
	return "https://" + host + "/" + repoPath
}

func ParseRepositoryIdentifier(raw, label string) (string, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", fmt.Errorf("%s is empty", label)
	}

	trimmed = strings.TrimSuffix(trimmed, ".git")

	if strings.HasPrefix(trimmed, "git@") {
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid %s: %s", label, raw)
		}
		trimmed = parts[1]
	}

	var owner, repo string
	if strings.Contains(trimmed, "://") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return "", "", fmt.Errorf("invalid %s: %w", label, err)
		}
		path := strings.Trim(u.Path, "/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			return "", "", fmt.Errorf("invalid %s: %s", label, raw)
		}
		if strings.EqualFold(u.Host, "github.com") || strings.HasSuffix(strings.ToLower(u.Host), ".github.com") {
			owner, repo = parts[0], parts[1]
		} else {
			owner, repo = parts[len(parts)-2], parts[len(parts)-1]
		}
	} else {
		trimmed = strings.Trim(trimmed, "/")
		parts := strings.Split(trimmed, "/")
		if len(parts) < 2 {
			return "", "", fmt.Errorf("invalid %s: %s", label, raw)
		}
		owner, repo = parts[len(parts)-2], parts[len(parts)-1]
	}

	owner = strings.Trim(strings.TrimSpace(owner), "/")
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	repo = strings.TrimSuffix(repo, ".git")
	if owner == "" || repo == "" || strings.Contains(owner, "..") || strings.Contains(repo, "..") {
		return "", "", fmt.Errorf("invalid %s: %s", label, raw)
	}

	return owner, repo, nil
}

func RepositoryFullNameFromURL(raw string) (string, error) {
	owner, repo, err := ParseRepositoryIdentifier(raw, "repository URL")
	if err != nil {
		return "", err
	}
	fullName := repositoryFullName(owner, repo)
	if fullName == "" {
		return "", fmt.Errorf("invalid repository URL: %s", raw)
	}
	return fullName, nil
}

func CanonicalRepositoryURL(fullName string) string {
	fullName = strings.Trim(strings.TrimSpace(fullName), "/")
	if fullName == "" {
		return ""
	}
	return "https://github.com/" + fullName
}

func RepositoryDisplayNameFromFullName(fullName string) string {
	_, repo := splitRepositoryID(fullName)
	repo = strings.TrimSpace(repo)
	if repo != "" {
		return repo
	}
	return strings.Trim(strings.TrimSpace(fullName), "/")
}

func repositoryFullName(owner, repo string) string {
	owner = strings.Trim(strings.TrimSpace(owner), "/")
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	if owner == "" {
		return repo
	}
	if repo == "" {
		return ""
	}
	return owner + "/" + repo
}

func splitRepositoryID(repositoryID string) (string, string) {
	repositoryID = strings.Trim(strings.TrimSpace(repositoryID), "/")
	if repositoryID == "" {
		return "", ""
	}
	parts := strings.Split(repositoryID, "/")
	if len(parts) < 2 {
		return "", repositoryID
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}
