package configsync

import (
	"fmt"
	"net/url"
	"strings"
)

func ParseGitHubRepoURL(raw string) (string, string, error) {
	return ParseRepositoryIdentifier(raw, "config repository URL")
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
