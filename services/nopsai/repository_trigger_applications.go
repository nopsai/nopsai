package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/gitwebhook"
)

type repositoryTriggerApplication struct {
	TeamPath           string
	Name               string
	RepoURL            string
	RepositoryFullName string
}

func repositoryTriggerApplicationFromRecord(record repositoryTriggerRecord) (repositoryTriggerApplication, bool, error) {
	teamPath := strings.Trim(strings.TrimSpace(record.TeamPath), "/")
	if teamPath == "" || isGlobalGrantResourceID(teamPath) || strings.EqualFold(teamPath, rootGrantID) {
		return repositoryTriggerApplication{}, false, nil
	}

	repositoryFullName := strings.Trim(strings.TrimSpace(record.RepositoryForWebhook), "/")
	if repositoryFullName == "" {
		repositoryFullName = repositoryTriggerProviderRepository(record.RepositoryName, teamPath)
	}
	repositoryFullName = strings.Trim(strings.TrimSpace(repositoryFullName), "/")
	if repositoryFullName == "" || !strings.Contains(repositoryFullName, "/") {
		return repositoryTriggerApplication{}, false, nil
	}

	repoURL, ok := repositoryTriggerApplicationRepoURL(record.Provider, repositoryFullName)
	if !ok {
		return repositoryTriggerApplication{}, false, nil
	}
	name, err := configsync.NormalizeStructureName(configsync.RepositoryDisplayNameFromFullName(repositoryFullName))
	if err != nil {
		return repositoryTriggerApplication{}, false, err
	}
	return repositoryTriggerApplication{
		TeamPath:           teamPath,
		Name:               name,
		RepoURL:            repoURL,
		RepositoryFullName: repositoryFullName,
	}, true, nil
}

func repositoryTriggerApplicationRepoURL(provider, repositoryFullName string) (string, bool) {
	repositoryFullName = strings.Trim(strings.TrimSpace(repositoryFullName), "/")
	if repositoryFullName == "" {
		return "", false
	}
	provider = normalizeTriggerProviderString(provider)
	switch provider {
	case repositoryTriggerProviderGitHub:
		return configsync.CanonicalRepositoryURL(repositoryFullName), true
	case gitwebhook.ProviderGitLab:
		return "https://gitlab.com/" + repositoryFullName, true
	case gitwebhook.ProviderBitbucket:
		return "https://bitbucket.org/" + repositoryFullName, true
	default:
		return "", false
	}
}

func mergeRepositoryTriggerApplicationsIntoStructure(
	structure map[string]*configsync.PipelineRunStructureNode,
	triggers map[string]storedTrigger,
) error {
	for _, stored := range triggers {
		app, ok, err := repositoryTriggerApplicationFromRecord(stored.record)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		segments, err := configsync.CleanPathSegments(app.TeamPath, false)
		if err != nil {
			return err
		}
		node := configsync.EnsurePipelineRunStructurePath(structure, segments)
		if pipelineRunStructureNodeHasApp(node, app.RepositoryFullName) {
			continue
		}
		node.Apps = append(node.Apps, configsync.PipelineRunStructureApp{
			Name:               app.Name,
			RepoURL:            app.RepoURL,
			RepositoryFullName: app.RepositoryFullName,
		})
	}
	return nil
}

func pipelineRunStructureNodeHasApp(node *configsync.PipelineRunStructureNode, repositoryFullName string) bool {
	if node == nil {
		return false
	}
	repositoryFullName = strings.Trim(strings.TrimSpace(repositoryFullName), "/")
	for _, app := range node.Apps {
		if strings.EqualFold(strings.Trim(strings.TrimSpace(app.RepositoryFullName), "/"), repositoryFullName) {
			return true
		}
		if app.RepositoryFullName == "" {
			if fullName, err := configsync.RepositoryFullNameFromURL(app.RepoURL); err == nil && strings.EqualFold(fullName, repositoryFullName) {
				return true
			}
		}
	}
	return false
}

func (a *App) ensureRepositoryTriggerApplication(ctx context.Context, runner queryRunner, record repositoryTriggerRecord) (int, bool, error) {
	app, ok, err := repositoryTriggerApplicationFromRecord(record)
	if err != nil || !ok {
		return 0, false, err
	}

	parentID, err := repositoryTriggerApplicationParentID(ctx, runner, app.TeamPath)
	if err != nil || parentID == nil {
		return 0, false, err
	}

	existingTeams, err := loadExistingTeamRecords(ctx, runner)
	if err != nil {
		return 0, false, err
	}
	repoKey := strings.ToLower(app.RepositoryFullName)
	if existing := existingTeams.byRepo[repoKey]; existing != nil {
		if !parentPointersEqual(existing.ParentID, parentID) {
			return 0, false, fmt.Errorf("application for repository %s already belongs to another team", app.RepositoryFullName)
		}
		err := reassignRepositoryRunsToApplication(ctx, runner, existing.ID, parentID, app.RepositoryFullName)
		return existing.ID, false, err
	}
	if existing := existingTeams.byName[app.Name]; existing != nil {
		return 0, false, fmt.Errorf("application %s conflicts with an existing team or application", app.Name)
	}

	var appID int
	if err := runner.QueryRow(ctx, `
		INSERT INTO teams (name, kind, parent_id, description, repo_url, repository_full_name)
		VALUES ($1, 'app', $2, '', $3, $4)
		RETURNING id
	`, app.Name, parentID, app.RepoURL, app.RepositoryFullName).Scan(&appID); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			existingTeams, loadErr := loadExistingTeamRecords(ctx, runner)
			if loadErr != nil {
				return 0, false, loadErr
			}
			if existing := existingTeams.byRepo[repoKey]; existing != nil {
				if !parentPointersEqual(existing.ParentID, parentID) {
					return 0, false, fmt.Errorf("application for repository %s already belongs to another team", app.RepositoryFullName)
				}
				reassignErr := reassignRepositoryRunsToApplication(ctx, runner, existing.ID, parentID, app.RepositoryFullName)
				return existing.ID, false, reassignErr
			}
		}
		return 0, false, err
	}
	err = reassignRepositoryRunsToApplication(ctx, runner, appID, parentID, app.RepositoryFullName)
	return appID, true, err
}

func repositoryTriggerApplicationParentID(ctx context.Context, runner queryRunner, teamPath string) (*int, error) {
	teamPath = strings.Trim(strings.TrimSpace(teamPath), "/")
	if teamPath == "" || isGlobalGrantResourceID(teamPath) || strings.EqualFold(teamPath, rootGrantID) {
		return nil, nil
	}
	records, err := loadTeamPathRecords(ctx, runner)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		if record.Path != teamPath {
			continue
		}
		if record.Kind == "app" || record.RepositoryFullName != "" || record.RepoURL != "" {
			return nil, fmt.Errorf("trigger team path %s points to an application", teamPath)
		}
		id := record.ID
		return &id, nil
	}
	return nil, nil
}

func reassignRepositoryRunsToApplication(ctx context.Context, runner queryRunner, appID int, parentID *int, repositoryFullName string) error {
	owner, repo := splitRepositoryFullName(repositoryFullName)
	if strings.TrimSpace(repo) == "" {
		return nil
	}
	var parent any
	if parentID != nil {
		parent = *parentID
	}
	_, err := runner.Exec(ctx, `
		UPDATE pipeline_runs
		SET team_id = $1
		WHERE LOWER(COALESCE(git_repo_owner, '')) = LOWER($2)
		  AND LOWER(COALESCE(git_repo_name, '')) = LOWER($3)
		  AND (
		    team_id IS NULL
		    OR team_id = $1
		    OR ($4::int IS NOT NULL AND team_id = $4::int)
		  )
	`, appID, owner, repo, parent)
	if err != nil {
		return err
	}
	return nil
}

func repositoryApplicationExists(ctx context.Context, runner queryRunner, record repositoryTriggerRecord) (bool, *int, error) {
	app, ok, err := repositoryTriggerApplicationFromRecord(record)
	if err != nil || !ok {
		return false, nil, err
	}
	parentID, err := repositoryTriggerApplicationParentID(ctx, runner, app.TeamPath)
	if err != nil || parentID == nil {
		return false, parentID, err
	}
	var id int
	err = runner.QueryRow(ctx, `
		SELECT id
		FROM teams
		WHERE LOWER(repository_full_name) = LOWER($1)
		LIMIT 1
	`, app.RepositoryFullName).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return false, parentID, nil
		}
		return false, parentID, err
	}
	return true, parentID, nil
}
