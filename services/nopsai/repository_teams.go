package nopsai

import (
	"context"
	"sort"
	"strings"
)

type repositoryTeamMatch struct {
	ID                 int
	Path               string
	RepositoryFullName string
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

func (a *App) repositoryTeamMatches(ctx context.Context, owner, repo string) ([]repositoryTeamMatch, error) {
	fullName := repositoryFullName(owner, repo)
	if fullName == "" || a == nil || a.db == nil {
		return nil, nil
	}

	records, err := loadTeamPathRecords(ctx, a.db)
	if err != nil {
		return nil, err
	}

	suffix := "/" + fullName
	matches := make([]repositoryTeamMatch, 0, 1)
	for _, record := range records {
		path := strings.Trim(strings.TrimSpace(record.Path), "/")
		if path == "" {
			continue
		}
		recordRepo := strings.Trim(strings.TrimSpace(record.RepositoryFullName), "/")
		if strings.EqualFold(recordRepo, fullName) {
			matches = append(matches, repositoryTeamMatch{ID: record.ID, Path: path, RepositoryFullName: recordRepo})
			continue
		}
		if path == fullName || strings.HasSuffix(path, suffix) {
			matches = append(matches, repositoryTeamMatch{ID: record.ID, Path: path, RepositoryFullName: fullName})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if len(matches[i].Path) == len(matches[j].Path) {
			return matches[i].Path < matches[j].Path
		}
		return len(matches[i].Path) > len(matches[j].Path)
	})
	return matches, nil
}

func repositoryTriggerOverrideKeys(owner, repo string, teamPaths []string) ([]string, []string) {
	fullName := repositoryFullName(owner, repo)
	if fullName == "" {
		return nil, nil
	}

	var specific []string
	var ownerWide []string
	for _, path := range teamPaths {
		path = strings.Trim(strings.TrimSpace(path), "/")
		if path == fullName || strings.HasSuffix(path, "/"+fullName) {
			specific = appendUniqueString(specific, path)
			ownerWide = appendUniqueString(ownerWide, teamedOwnerAllTriggerKey(path, owner, repo))
			continue
		}
		parentPath := teamItemParentPath(path)
		if parentPath != "" {
			specific = appendUniqueString(specific, strings.Trim(parentPath+"/"+fullName, "/"))
			ownerWide = appendUniqueString(ownerWide, strings.Trim(parentPath+"/"+repositoryFullName(owner, "all"), "/"))
		}
	}

	specific = appendUniqueString(specific, fullName)
	ownerWide = appendUniqueString(ownerWide, repositoryFullName(owner, "all"))
	return specific, ownerWide
}

func teamItemParentPath(path string) string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts[:len(parts)-1], "/")
}

func appendUniqueString(values []string, value string) []string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendUniqueStrings(values []string, additions []string) []string {
	for _, value := range additions {
		values = appendUniqueString(values, value)
	}
	return values
}

func sortTriggerKeysBySpecificity(keys []string) []string {
	sort.SliceStable(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})
	return keys
}

func (a *App) triggerOverrideKeysEndingWith(ctx context.Context, suffix string) ([]string, error) {
	suffix = strings.Trim(strings.TrimSpace(suffix), "/")
	if suffix == "" || a == nil || a.db == nil {
		return nil, nil
	}

	rows, err := a.db.Query(ctx, `
		SELECT repository_name
		FROM triggers
		WHERE repository_name = $1
		   OR RIGHT(repository_name, LENGTH($1) + 1) = '/' || $1
		ORDER BY LENGTH(repository_name) DESC, repository_name ASC
	`, suffix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = appendUniqueString(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}

func teamedOwnerAllTriggerKey(teamedRepoPath, owner, repo string) string {
	teamedRepoPath = strings.Trim(strings.TrimSpace(teamedRepoPath), "/")
	fullName := repositoryFullName(owner, repo)
	if teamedRepoPath == "" || fullName == "" || teamedRepoPath == fullName {
		return ""
	}
	suffix := "/" + fullName
	if !strings.HasSuffix(teamedRepoPath, suffix) {
		return ""
	}
	prefix := strings.TrimSuffix(teamedRepoPath, suffix)
	return strings.Trim(prefix+"/"+repositoryFullName(owner, "all"), "/")
}
