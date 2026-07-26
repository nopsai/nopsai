package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

const repositoryTriggerSelect = `
	SELECT repository_name, trigger_definition, COALESCE(source, 'database'), COALESCE(visibility, 'team'),
	       COALESCE(provider, 'github'), COALESCE(team_path, ''), COALESCE(management, 'nopsai'),
	       COALESCE(webhook_source_id, ''), COALESCE(config_source_path, ''), managed_by_config_repo, created_at
	FROM triggers`

func scanRepositoryTrigger(scanner interface{ Scan(...any) error }) (repositoryTriggerRecord, error) {
	var record repositoryTriggerRecord
	err := scanner.Scan(
		&record.RepositoryName,
		&record.Definition,
		&record.Source,
		&record.Visibility,
		&record.Provider,
		&record.TeamPath,
		&record.Management,
		&record.WebhookSourceID,
		&record.ConfigSourcePath,
		&record.ManagedByConfigRepo,
		&record.CreatedAt,
	)
	if err != nil {
		return record, err
	}
	record.Source = normalizeTriggerSource(record.Source)
	record.Visibility = normalizeResourceVisibility(record.Visibility)
	record.Provider = normalizeTriggerProviderString(record.Provider)
	if management, err := normalizeRepositoryTriggerManagement(record.Management); err == nil {
		record.Management = management
	}
	if teamPath, err := normalizeRepositoryTriggerTeamPath(record.TeamPath); err == nil {
		record.TeamPath = teamPath
	}
	record.RepositoryForWebhook = repositoryTriggerProviderRepository(record.RepositoryName, record.TeamPath)
	return record, nil
}

func (a *App) getRepositoryTriggerRecord(ctx context.Context, repositoryName string) (repositoryTriggerRecord, bool, error) {
	repositoryName = strings.Trim(strings.TrimSpace(repositoryName), "/")
	if repositoryName == "" || a == nil || a.db == nil {
		return repositoryTriggerRecord{}, false, nil
	}
	record, err := scanRepositoryTrigger(a.db.QueryRow(ctx, repositoryTriggerSelect+` WHERE repository_name = $1`, repositoryName))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return repositoryTriggerRecord{}, false, nil
		}
		return repositoryTriggerRecord{}, false, err
	}
	return record, true, nil
}

func (a *App) enrichRepositoryTriggerRecord(ctx context.Context, record repositoryTriggerRecord) repositoryTriggerRecord {
	if a == nil || a.db == nil {
		record.Ingress = repositoryTriggerIngress(record.Provider, record.WebhookSourceName, record.WebhookSourceID)
		return record
	}
	status, sourceName := repositoryTriggerAllowlistStatus(ctx, a.db, record)
	record.AllowlistStatus = status
	record.WebhookSourceName = sourceName
	record.Ingress = repositoryTriggerIngress(record.Provider, sourceName, record.WebhookSourceID)
	return record
}

func (a *App) repositoryTriggerTeamPathForRepository(ctx context.Context, repositoryID string) (string, bool, error) {
	repositoryID = strings.Trim(strings.TrimSpace(repositoryID), "/")
	if repositoryID == "" || a == nil || a.db == nil {
		return "", false, nil
	}
	var teamPath string
	err := a.db.QueryRow(ctx, `
		SELECT COALESCE(team_path, '')
		FROM triggers
		WHERE repository_name = $1
		   OR RIGHT(repository_name, LENGTH($1) + 1) = '/' || $1
		ORDER BY LENGTH(repository_name) DESC, repository_name ASC
		LIMIT 1
	`, repositoryID).Scan(&teamPath)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	teamPath, err = normalizeRepositoryTriggerTeamPath(teamPath)
	if err != nil {
		return "", false, err
	}
	return teamPath, true, nil
}

func (a *App) loadRepositoryTriggerOverride(ctx context.Context, owner, repo string) (repositoryTriggerRecord, bool, error) {
	fullName := repositoryFullName(owner, repo)
	matches, err := a.repositoryTeamMatches(ctx, owner, repo)
	if err != nil {
		return repositoryTriggerRecord{}, false, err
	}
	teamPaths := make([]string, 0, len(matches))
	for _, match := range matches {
		teamPaths = append(teamPaths, match.Path)
	}
	specificKeys, ownerWideKeys := repositoryTriggerOverrideKeys(owner, repo, teamPaths)
	dbSpecificKeys, err := a.triggerOverrideKeysEndingWith(ctx, fullName)
	if err != nil {
		return repositoryTriggerRecord{}, false, err
	}
	dbOwnerWideKeys, err := a.triggerOverrideKeysEndingWith(ctx, repositoryFullName(owner, "all"))
	if err != nil {
		return repositoryTriggerRecord{}, false, err
	}
	specificKeys = sortTriggerKeysBySpecificity(appendUniqueStrings(specificKeys, dbSpecificKeys))
	ownerWideKeys = sortTriggerKeysBySpecificity(appendUniqueStrings(ownerWideKeys, dbOwnerWideKeys))

	for _, key := range specificKeys {
		record, found, err := a.getRepositoryTriggerRecord(ctx, key)
		if err != nil || !found {
			if err != nil {
				return repositoryTriggerRecord{}, false, err
			}
			continue
		}
		if record.Management == repositoryTriggerManagementRepository {
			continue
		}
		return record, true, nil
	}

	for _, key := range ownerWideKeys {
		record, found, err := a.getRepositoryTriggerRecord(ctx, key)
		if err != nil || !found {
			if err != nil {
				return repositoryTriggerRecord{}, false, err
			}
			continue
		}
		if record.Management == repositoryTriggerManagementRepository {
			continue
		}
		return record, true, nil
	}
	return repositoryTriggerRecord{}, false, nil
}
