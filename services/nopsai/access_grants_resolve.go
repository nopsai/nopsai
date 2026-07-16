package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/auth"
)

func normalizeProductRoleName(raw string) (string, error) {
	roleName := strings.ToLower(strings.TrimSpace(raw))
	if _, ok := productRoleDefinitions[roleName]; !ok {
		return "", fmt.Errorf("role must be one of viewer, developer, owner, admin")
	}
	return roleName, nil
}

func normalizeAccessGrantSubjectType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case grantSubjectUser:
		return model.SubjectTypeUser, nil
	case model.SubjectTypeAuthTeam:
		return model.SubjectTypeAuthTeam, nil
	case grantSubjectRepository:
		return model.SubjectTypeRepository, nil
	case grantSubjectTrigger:
		return model.SubjectTypeTrigger, nil
	case grantSubjectServiceAccount:
		return model.SubjectTypeServiceAccount, nil
	case grantSubjectService, model.SubjectTypeInternalService:
		return model.SubjectTypeInternalService, nil
	default:
		return "", fmt.Errorf("subject_type must be user, auth_team, repository, trigger, service_account, or internal_service")
	}
}

func normalizeAccessGrantResourceType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case grantResourceTeam:
		return grantResourceTeam, nil
	case grantResourcePipeline:
		return grantResourcePipeline, nil
	case grantResourceRun:
		return grantResourceRun, nil
	case grantResourceSchedule:
		return grantResourceSchedule, nil
	case grantResourceTrigger:
		return grantResourceTrigger, nil
	case grantResourceExternalTrigger:
		return grantResourceExternalTrigger, nil
	case grantResourceGitWebhookSource:
		return grantResourceGitWebhookSource, nil
	case grantResourceSecret:
		return grantResourceSecret, nil
	case grantResourceVariable:
		return grantResourceVariable, nil
	case grantResourceScope:
		return grantResourceScope, nil
	case grantResourceRepo:
		return grantResourceRepo, nil
	case grantResourceStep:
		return grantResourceStep, nil
	case grantResourceRunner:
		return grantResourceRunner, nil
	case grantResourceConfig:
		return grantResourceConfig, nil
	case grantResourceDashboard:
		return grantResourceDashboard, nil
	case grantResourceKnowledgeContext:
		return grantResourceKnowledgeContext, nil
	case grantResourceKnowledgeConnection:
		return grantResourceKnowledgeConnection, nil
	case grantResourceLLMProfile:
		return grantResourceLLMProfile, nil
	case grantResourceAgentProfile:
		return grantResourceAgentProfile, nil
	case grantResourceMCPServer:
		return grantResourceMCPServer, nil
	case grantResourceMCPProfile:
		return grantResourceMCPProfile, nil
	case grantResourceCredential:
		return grantResourceCredential, nil
	case grantResourceCompany, grantResourcePlatform:
		return grantResourcePlatform, nil
	default:
		return "", fmt.Errorf("unsupported resource_type")
	}
}

func resolveAccessGrantSubject(ctx context.Context, runner queryRunner, rawType, rawID string) (accessGrantSubject, error) {
	subjectType, err := normalizeAccessGrantSubjectType(rawType)
	if err != nil {
		return accessGrantSubject{}, err
	}
	rawID = strings.TrimSpace(rawID)
	if rawID == "" {
		return accessGrantSubject{}, fmt.Errorf("subject_id is required")
	}

	switch subjectType {
	case model.SubjectTypeUser:
		return resolveAccessGrantUser(ctx, runner, rawID)
	case model.SubjectTypeAuthTeam:
		return resolveAccessGrantAuthTeam(ctx, runner, rawID)
	case model.SubjectTypeRepository, model.SubjectTypeTrigger:
		return resolveAccessGrantNamedSubject(subjectType, rawID)
	case model.SubjectTypeServiceAccount:
		return resolveAccessGrantServiceAccount(ctx, runner, rawID)
	case model.SubjectTypeInternalService:
		return resolveAccessGrantService(ctx, runner, rawID)
	default:
		return accessGrantSubject{}, fmt.Errorf("unsupported subject_type")
	}
}

func resolveAccessGrantUser(ctx context.Context, runner queryRunner, rawID string) (accessGrantSubject, error) {
	var subject accessGrantSubject
	query := `
		SELECT id::text, COALESCE(NULLIF(sub, ''), COALESCE(email, id::text))
		FROM users
		WHERE provider <> $1 AND %s
		LIMIT 1
	`

	var (
		lookup string
		args   []any
	)
	if _, err := uuid.Parse(rawID); err == nil {
		lookup = "id::text = $2"
		args = []any{auth.ProviderServiceAccount, rawID}
	} else if strings.Contains(rawID, "@") {
		lookup = "LOWER(email) = LOWER($2)"
		args = []any{auth.ProviderServiceAccount, rawID}
	} else {
		lookup = "sub = $2"
		args = []any{auth.ProviderServiceAccount, rawID}
	}

	err := runner.QueryRow(ctx, fmt.Sprintf(query, lookup), args...).Scan(&subject.ID, &subject.Display)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return accessGrantSubject{}, fmt.Errorf("subject not found")
		}
		return accessGrantSubject{}, err
	}
	subject.Type = model.SubjectTypeUser
	return subject, nil
}

func resolveAccessGrantServiceAccount(ctx context.Context, runner queryRunner, rawID string) (accessGrantSubject, error) {
	var subject accessGrantSubject
	rawID = strings.TrimSpace(rawID)
	if strings.HasPrefix(strings.ToLower(rawID), model.SubjectTypeServiceAccount+":") {
		rawID = strings.TrimSpace(rawID[len(model.SubjectTypeServiceAccount)+1:])
	}
	query := `
		SELECT sub, COALESCE(NULLIF(sub, ''), COALESCE(email, id::text))
		FROM users
		WHERE provider = $1 AND %s
		LIMIT 1
	`

	var (
		lookup string
		args   []any
	)
	if _, err := uuid.Parse(rawID); err == nil {
		lookup = "id::text = $2"
		args = []any{auth.ProviderServiceAccount, rawID}
	} else if strings.Contains(rawID, "@") {
		lookup = "LOWER(email) = LOWER($2)"
		args = []any{auth.ProviderServiceAccount, rawID}
	} else {
		lookup = "sub = $2"
		args = []any{auth.ProviderServiceAccount, strings.Trim(strings.TrimSpace(rawID), "/")}
	}

	err := runner.QueryRow(ctx, fmt.Sprintf(query, lookup), args...).Scan(&subject.ID, &subject.Display)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return accessGrantSubject{}, fmt.Errorf("service account subject not found")
		}
		return accessGrantSubject{}, err
	}
	subject.Type = model.SubjectTypeServiceAccount
	subject.ID = strings.TrimSpace(subject.ID)
	subject.Display = strings.TrimSpace(firstNonEmptyString(subject.Display, subject.ID))
	if subject.ID == "" {
		return accessGrantSubject{}, fmt.Errorf("service account subject not found")
	}
	return subject, nil
}

func resolveAccessGrantAuthTeam(ctx context.Context, runner queryRunner, rawID string) (accessGrantSubject, error) {
	teamID := strings.TrimSpace(rawID)
	if teamID == "" {
		return accessGrantSubject{}, fmt.Errorf("subject_id is required")
	}

	var subject accessGrantSubject
	query := `
		SELECT id::text, name
		FROM auth_teams
		WHERE %s
		LIMIT 1
	`
	var (
		lookup string
		args   []any
	)
	if _, err := uuid.Parse(teamID); err == nil {
		lookup = "id::text = $1"
		args = []any{teamID}
	} else {
		lookup = "name = $1"
		args = []any{teamID}
	}

	err := runner.QueryRow(ctx, fmt.Sprintf(query, lookup), args...).Scan(&subject.ID, &subject.Display)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return accessGrantSubject{}, fmt.Errorf("subject not found")
		}
		return accessGrantSubject{}, err
	}
	subject.Type = model.SubjectTypeAuthTeam
	return subject, nil
}

func resolveAccessGrantNamedSubject(subjectType, rawID string) (accessGrantSubject, error) {
	subjectID := strings.Trim(strings.TrimSpace(rawID), "/")
	if subjectID == "" {
		return accessGrantSubject{}, fmt.Errorf("subject_id is required")
	}
	for _, prefix := range []string{
		model.SubjectTypeRepository + ":",
		model.SubjectTypeTrigger + ":",
		model.SubjectTypeServiceAccount + ":",
	} {
		if strings.HasPrefix(strings.ToLower(subjectID), prefix) {
			subjectID = strings.TrimSpace(subjectID[len(prefix):])
			break
		}
	}
	subjectID = strings.Trim(strings.TrimSpace(subjectID), "/")
	if subjectID == "" {
		return accessGrantSubject{}, fmt.Errorf("subject_id is required")
	}
	return accessGrantSubject{
		Type:    subjectType,
		ID:      subjectID,
		Display: subjectID,
	}, nil
}

func resolveAccessGrantService(ctx context.Context, runner queryRunner, rawID string) (accessGrantSubject, error) {
	serviceID := strings.TrimSpace(rawID)
	if serviceID == "" {
		return accessGrantSubject{}, fmt.Errorf("subject_id is required")
	}
	var exists int
	err := runner.QueryRow(ctx, `
		SELECT 1
		WHERE $1 = 'dispatcher'
		   OR EXISTS (
				SELECT 1
				FROM auth_role_bindings
				WHERE subject_type = 'internal_service' AND subject_id = $1
		   )
	`, serviceID).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return accessGrantSubject{}, fmt.Errorf("subject not found")
		}
		return accessGrantSubject{}, err
	}
	return accessGrantSubject{
		Type:    model.SubjectTypeInternalService,
		ID:      serviceID,
		Display: serviceID,
	}, nil
}

func resolveAccessGrantResource(ctx context.Context, runner queryRunner, rawType, rawID string, requireExists bool) (accessGrantResource, error) {
	resourceType, err := normalizeAccessGrantResourceType(rawType)
	if err != nil {
		return accessGrantResource{}, err
	}
	rawID = strings.TrimSpace(rawID)

	switch resourceType {
	case grantResourcePlatform:
		return accessGrantResource{
			Type:    grantResourcePlatform,
			ID:      platformGrantID,
			Display: "platform",
		}, nil
	case grantResourceTeam:
		return resolveAccessGrantTeam(ctx, runner, rawID, requireExists)
	case grantResourcePipeline:
		return resolvePipelineOrStepGrantResource(ctx, runner, grantResourcePipeline, rawID, requireExists, "pipelines")
	case grantResourceRun:
		if rawID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		if requireExists && rawID != "*" {
			var exists int
			err := runner.QueryRow(ctx, `SELECT 1 FROM pipeline_runs WHERE run_id::text = $1 LIMIT 1`, rawID).Scan(&exists)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
					return accessGrantResource{}, fmt.Errorf("resource not found")
				}
				return accessGrantResource{}, err
			}
		}
		return accessGrantResource{Type: grantResourceRun, ID: rawID, Display: rawID}, nil
	case grantResourceSchedule:
		return resolveScheduleGrantResource(ctx, runner, rawID, requireExists)
	case grantResourceStep:
		return resolvePipelineOrStepGrantResource(ctx, runner, grantResourceStep, rawID, requireExists, "steps")
	case grantResourceTrigger:
		if rawID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		if requireExists {
			var exists int
			err := runner.QueryRow(ctx, `SELECT 1 FROM triggers WHERE repository_name = $1 LIMIT 1`, rawID).Scan(&exists)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
					return accessGrantResource{}, fmt.Errorf("resource not found")
				}
				return accessGrantResource{}, err
			}
		}
		return accessGrantResource{Type: grantResourceTrigger, ID: rawID, Display: rawID}, nil
	case grantResourceExternalTrigger:
		if rawID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		if requireExists && rawID != "*" {
			var exists int
			err := runner.QueryRow(ctx, `SELECT 1 FROM external_triggers WHERE id = $1 LIMIT 1`, rawID).Scan(&exists)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
					return accessGrantResource{}, fmt.Errorf("resource not found")
				}
				return accessGrantResource{}, err
			}
		}
		return accessGrantResource{Type: grantResourceExternalTrigger, ID: rawID, Display: rawID}, nil
	case grantResourceGitWebhookSource:
		if rawID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		if requireExists && rawID != "*" {
			var exists int
			err := runner.QueryRow(ctx, `SELECT 1 FROM git_webhook_sources WHERE id = $1 LIMIT 1`, rawID).Scan(&exists)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
					return accessGrantResource{}, fmt.Errorf("resource not found")
				}
				return accessGrantResource{}, err
			}
		}
		return accessGrantResource{Type: grantResourceGitWebhookSource, ID: rawID, Display: rawID}, nil
	case grantResourceScope:
		scopeID, scopeLookup, scopeDisplay := normalizeScopeGrantResourceID(rawID)
		if scopeDisplay == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		if requireExists && !isDefaultScopeGrantResource(scopeID, scopeDisplay) {
			var exists int
			err := runner.QueryRow(ctx, `
				SELECT 1
				FROM (
					SELECT scope FROM secrets WHERE scope = $1
					UNION
					SELECT scope FROM variables WHERE scope = $1
				) scopes
				LIMIT 1
			`, scopeLookup).Scan(&exists)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
					return accessGrantResource{}, fmt.Errorf("resource not found")
				}
				return accessGrantResource{}, err
			}
		}
		return accessGrantResource{Type: grantResourceScope, ID: scopeID, Display: scopeDisplay}, nil
	case grantResourceRepo:
		if rawID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		if requireExists {
			var exists int
			err := runner.QueryRow(ctx, `
				SELECT 1
				FROM (
					SELECT name AS value FROM teams WHERE name = $1
					UNION
					SELECT repository_name AS value FROM triggers WHERE repository_name = $1
					UNION
					SELECT repository_name AS value FROM secrets WHERE repository_name = $1
					UNION
					SELECT repository_name AS value FROM variables WHERE repository_name = $1
				) repositories
				LIMIT 1
			`, rawID).Scan(&exists)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
					return accessGrantResource{}, fmt.Errorf("resource not found")
				}
				return accessGrantResource{}, err
			}
		}
		return accessGrantResource{Type: grantResourceRepo, ID: rawID, Display: rawID}, nil
	case grantResourceRunner, grantResourceConfig:
		if rawID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		resourceID := strings.Trim(strings.TrimSpace(rawID), "/")
		if resourceID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		return accessGrantResource{Type: resourceType, ID: resourceID, Display: resourceID}, nil
	case grantResourceDashboard:
		return resolveDashboardGrantResource(ctx, runner, rawID, requireExists)
	case grantResourceKnowledgeContext:
		resourceID := strings.Trim(strings.TrimSpace(rawID), "/")
		if resourceID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		if requireExists && resourceID != "*" {
			kind, team, name, err := splitKnowledgeContextIdentifier(resourceID)
			if err != nil {
				return accessGrantResource{}, err
			}
			var exists int
			err = runner.QueryRow(ctx, `
				SELECT 1
				FROM knowledge_contexts
				WHERE kind = $1 AND team_path = $2 AND name = $3
				LIMIT 1
			`, kind, team, name).Scan(&exists)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
					return accessGrantResource{}, fmt.Errorf("resource not found")
				}
				return accessGrantResource{}, err
			}
		}
		return accessGrantResource{Type: resourceType, ID: resourceID, Display: resourceID}, nil
	case grantResourceKnowledgeConnection:
		resourceID := strings.Trim(strings.TrimSpace(rawID), "/")
		if resourceID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		if resourceID != "*" {
			team, name, err := splitKnowledgeConnectionIdentifier(resourceID)
			if err != nil {
				return accessGrantResource{}, err
			}
			resourceID = buildKnowledgeConnectionIdentifier(team, name)
			if requireExists {
				var exists int
				err = runner.QueryRow(ctx, `
					SELECT 1
					FROM knowledge_context_connections
					WHERE team_path = $1 AND name = $2
					LIMIT 1
				`, team, name).Scan(&exists)
				if err != nil {
					if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
						return accessGrantResource{}, fmt.Errorf("resource not found")
					}
					return accessGrantResource{}, err
				}
			}
		}
		return accessGrantResource{Type: resourceType, ID: resourceID, Display: resourceID}, nil
	case grantResourceLLMProfile:
		return resolveNamedTableGrantResource(ctx, runner, resourceType, rawID, requireExists, "llm_profiles", "name")
	case grantResourceAgentProfile:
		return resolveNamedTableGrantResource(ctx, runner, resourceType, rawID, requireExists, "agent_profiles", "id")
	case grantResourceMCPServer:
		return resolveNamedTableGrantResource(ctx, runner, resourceType, rawID, requireExists, "mcp_servers", "name")
	case grantResourceMCPProfile:
		return resolveNamedTableGrantResource(ctx, runner, resourceType, rawID, requireExists, "mcp_profiles", "name")
	case grantResourceCredential:
		resourceID := strings.Trim(strings.TrimSpace(rawID), "/")
		if resourceID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		if requireExists && resourceID != "*" {
			parts := strings.SplitN(resourceID, "/", 2)
			if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
				return accessGrantResource{}, fmt.Errorf("credential resource_id must be namespace/name")
			}
			var exists int
			err := runner.QueryRow(ctx, `
				SELECT 1
				FROM credentials
				WHERE namespace = $1 AND name = $2
				LIMIT 1
			`, strings.TrimSpace(parts[0]), strings.Trim(strings.TrimSpace(parts[1]), "/")).Scan(&exists)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
					return accessGrantResource{}, fmt.Errorf("resource not found")
				}
				return accessGrantResource{}, err
			}
		}
		return accessGrantResource{Type: resourceType, ID: resourceID, Display: resourceID}, nil
	case grantResourceSecret, grantResourceVariable:
		if rawID == "" {
			return accessGrantResource{}, fmt.Errorf("resource_id is required")
		}
		resourceID := runtimeNamedResourceIDForResource(rawID)
		if requireExists {
			tableName := grantResourceSecret + "s"
			if resourceType == grantResourceVariable {
				tableName = grantResourceVariable + "s"
			}
			var exists int
			err := runner.QueryRow(ctx, fmt.Sprintf(`SELECT 1 FROM %s WHERE %s LIMIT 1`, tableName, namedResourceWhereClause(resourceID)), namedResourceWhereArgs(resourceID)...).Scan(&exists)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
					return accessGrantResource{}, fmt.Errorf("resource not found")
				}
				return accessGrantResource{}, err
			}
		}
		return accessGrantResource{Type: resourceType, ID: resourceID, Display: resourceID}, nil
	default:
		return accessGrantResource{}, fmt.Errorf("unsupported resource_type")
	}
}

func resolveNamedTableGrantResource(ctx context.Context, runner queryRunner, resourceType, rawID string, requireExists bool, tableName, columnName string) (accessGrantResource, error) {
	resourceID := strings.Trim(strings.TrimSpace(rawID), "/")
	if resourceID == "" {
		return accessGrantResource{}, fmt.Errorf("resource_id is required")
	}
	if requireExists && resourceID != "*" {
		var exists int
		err := runner.QueryRow(ctx, fmt.Sprintf(`SELECT 1 FROM %s WHERE %s = $1 LIMIT 1`, tableName, columnName), resourceID).Scan(&exists)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
				return accessGrantResource{}, fmt.Errorf("resource not found")
			}
			return accessGrantResource{}, err
		}
	}
	return accessGrantResource{Type: resourceType, ID: resourceID, Display: resourceID}, nil
}

func isDefaultScopeGrantResource(id, display string) bool {
	return strings.TrimSpace(id) == "" && strings.EqualFold(strings.TrimSpace(display), "default")
}

func normalizeScopeGrantResourceID(rawID string) (id, lookup, display string) {
	rawID = strings.Trim(strings.TrimSpace(rawID), "/")
	switch strings.ToLower(rawID) {
	case "", "default":
		return "", "", "default"
	default:
		return rawID, rawID, rawID
	}
}

func resolveAccessGrantTeam(ctx context.Context, runner queryRunner, rawID string, requireExists bool) (accessGrantResource, error) {
	rawID = strings.TrimSpace(rawID)
	if isRootGrantResourceID(rawID) {
		return accessGrantResource{
			Type:    grantResourceTeam,
			ID:      generalGrantID,
			Display: rootGrantID,
		}, nil
	}
	if rawID == "" {
		return accessGrantResource{}, fmt.Errorf("resource_id is required")
	}

	if numericID, err := strconv.Atoi(rawID); err == nil {
		pathRecords, loadErr := loadTeamPathRecords(ctx, runner)
		if loadErr != nil {
			return accessGrantResource{}, loadErr
		}
		record, ok := pathRecords[numericID]
		if !ok {
			return accessGrantResource{}, fmt.Errorf("resource not found")
		}
		return accessGrantResource{
			Type:    grantResourceTeam,
			ID:      record.Path,
			Display: "/" + record.Path,
		}, nil
	}

	normalized := strings.Trim(strings.TrimSpace(rawID), "/")
	if normalized == "" {
		return accessGrantResource{}, fmt.Errorf("resource_id is required")
	}
	if requireExists {
		pathRecords, err := loadTeamPathRecords(ctx, runner)
		if err != nil {
			return accessGrantResource{}, err
		}
		for _, record := range pathRecords {
			if record.Path == normalized {
				return accessGrantResource{
					Type:    grantResourceTeam,
					ID:      normalized,
					Display: "/" + normalized,
				}, nil
			}
		}
		return accessGrantResource{}, fmt.Errorf("resource not found")
	}

	return accessGrantResource{
		Type:    grantResourceTeam,
		ID:      normalized,
		Display: "/" + normalized,
	}, nil
}

func resolvePipelineOrStepGrantResource(ctx context.Context, runner queryRunner, resourceType, rawID string, requireExists bool, tableName string) (accessGrantResource, error) {
	resourceID := strings.Trim(strings.TrimSpace(rawID), "/")
	if resourceID == "" {
		return accessGrantResource{}, fmt.Errorf("resource_id is required")
	}
	if requireExists {
		pathPart, namePart := model.SplitPipelineID(resourceID)
		query := fmt.Sprintf(`SELECT 1 FROM %s WHERE path = $1 AND name = $2 LIMIT 1`, tableName)
		var exists int
		err := runner.QueryRow(ctx, query, pathPart, namePart).Scan(&exists)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
				return accessGrantResource{}, fmt.Errorf("resource not found")
			}
			return accessGrantResource{}, err
		}
	}
	return accessGrantResource{Type: resourceType, ID: resourceID, Display: resourceID}, nil
}

func namedResourceWhereClause(resourceID string) string {
	repoName, scope, _ := model.ParseNamedResourceID(resourceID)
	storageScope := runtimeScopeForStorage(scope)
	switch {
	case repoName != "":
		return "name = $1 AND repository_name = $2 AND " + runtimeScopeEqualsSQL("scope", 3, storageScope)
	case scope != "":
		return "name = $1 AND repository_name IS NULL AND " + runtimeScopeEqualsSQL("scope", 2, storageScope)
	default:
		return "name = $1 AND repository_name IS NULL AND " + runtimeScopeEqualsSQL("scope", 2, storageScope)
	}
}

func namedResourceWhereArgs(resourceID string) []any {
	repoName, scope, name := model.ParseNamedResourceID(resourceID)
	storageScope := runtimeScopeForStorage(scope)
	switch {
	case repoName != "" && scope != "":
		return []any{name, repoName, storageScope}
	case repoName != "":
		return []any{name, repoName, storageScope}
	case scope != "":
		return []any{name, storageScope}
	default:
		return []any{name, storageScope}
	}
}

func loadTeamPathRecords(ctx context.Context, runner queryRunner) (map[int]teamPathRecord, error) {
	rows, err := runner.Query(ctx, `SELECT id, name, COALESCE(kind, 'team'), parent_id, COALESCE(description, ''), COALESCE(repo_url, ''), COALESCE(repository_full_name, '') FROM teams`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make(map[int]teamPathRecord)
	for rows.Next() {
		var (
			record      teamPathRecord
			parentIDSQL sql.NullInt32
		)
		if err := rows.Scan(&record.ID, &record.Name, &record.Kind, &parentIDSQL, &record.Description, &record.RepoURL, &record.RepositoryFullName); err != nil {
			return nil, err
		}
		record.Name = strings.Trim(strings.TrimSpace(record.Name), "/")
		record.RepoURL = strings.TrimSpace(record.RepoURL)
		record.RepositoryFullName = strings.Trim(strings.TrimSpace(record.RepositoryFullName), "/")
		if parentIDSQL.Valid {
			parent := int(parentIDSQL.Int32)
			record.ParentID = &parent
		}
		records[record.ID] = record
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	cache := make(map[int]string, len(records))
	var buildPath func(int) (string, error)
	buildPath = func(id int) (string, error) {
		if path, ok := cache[id]; ok {
			return path, nil
		}
		record, ok := records[id]
		if !ok {
			return "", fmt.Errorf("team %d not found", id)
		}
		if record.ParentID == nil {
			cache[id] = record.Name
			return cache[id], nil
		}
		parentPath, err := buildPath(*record.ParentID)
		if err != nil {
			return "", err
		}
		cache[id] = strings.Trim(strings.TrimSpace(parentPath+"/"+record.Name), "/")
		return cache[id], nil
	}

	for id, record := range records {
		path, err := buildPath(id)
		if err != nil {
			return nil, err
		}
		record.Path = path
		records[id] = record
	}
	return records, nil
}

func (a *App) teamGrantResourceByTeamID(ctx context.Context, teamID int) (accessGrantResource, error) {
	if a == nil || a.db == nil {
		return accessGrantResource{}, fmt.Errorf("database unavailable")
	}
	return resolveAccessGrantTeam(ctx, a.db, strconv.Itoa(teamID), true)
}

func (a *App) teamPathRecords(ctx context.Context) (map[int]teamPathRecord, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	return loadTeamPathRecords(ctx, a.db)
}

func authorizeGrantOperation(ctx context.Context, subject model.Subject, resource accessGrantResource, roleName string, checker func(context.Context, model.Subject, string, model.ResourceRef, map[string]any) (model.Decision, error), requestContext map[string]any) error {
	if roleName == productRoleAdmin || resource.Type == grantResourcePlatform {
		decision, err := checker(ctx, subject, "iam.admin", model.ResourceRef{Type: "iam", ID: "admin"}, requestContext)
		if err != nil {
			return err
		}
		if !decision.Allowed {
			return fmt.Errorf("forbidden")
		}
		return nil
	}

	action, resourceRef, err := managementActionForGrantResource(resource)
	if err != nil {
		return err
	}
	decision, err := checker(ctx, subject, action, resourceRef, requestContext)
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return fmt.Errorf("forbidden")
	}
	return nil
}

func managementActionForGrantResource(resource accessGrantResource) (string, model.ResourceRef, error) {
	switch resource.Type {
	case grantResourceTeam:
		return "team.manage_acl", model.ResourceRef{Type: grantResourceTeam, ID: resource.ID}, nil
	case grantResourcePipeline:
		return "pipeline.manage_acl", model.ResourceRef{Type: grantResourcePipeline, ID: resource.ID}, nil
	case grantResourceSchedule:
		return "pipeline_schedule.manage_acl", model.ResourceRef{Type: grantResourceSchedule, ID: resource.ID}, nil
	case grantResourceTrigger:
		return "trigger.manage_acl", model.ResourceRef{Type: grantResourceTrigger, ID: resource.ID}, nil
	case grantResourceExternalTrigger:
		return "external_trigger.manage_acl", model.ResourceRef{Type: grantResourceExternalTrigger, ID: resource.ID}, nil
	case grantResourceGitWebhookSource:
		return "git_webhook_source.manage_acl", model.ResourceRef{Type: grantResourceGitWebhookSource, ID: resource.ID}, nil
	case grantResourceSecret:
		return "secret.manage_acl", model.ResourceRef{Type: grantResourceSecret, ID: resource.ID}, nil
	case grantResourceVariable:
		return "variable.manage_acl", model.ResourceRef{Type: grantResourceVariable, ID: resource.ID}, nil
	case grantResourceScope:
		return "scope.manage_acl", model.ResourceRef{Type: grantResourceScope, ID: resource.ID}, nil
	case grantResourceRepo:
		return "repository.manage_acl", model.ResourceRef{Type: grantResourceRepo, ID: resource.ID}, nil
	case grantResourceStep:
		return "step.manage_acl", model.ResourceRef{Type: grantResourceStep, ID: resource.ID}, nil
	case grantResourceKnowledgeContext:
		return "knowledge_context.manage_access", model.ResourceRef{Type: grantResourceKnowledgeContext, ID: resource.ID}, nil
	case grantResourceKnowledgeConnection:
		return "knowledge_connection.manage_access", model.ResourceRef{Type: grantResourceKnowledgeConnection, ID: resource.ID}, nil
	case grantResourceLLMProfile:
		return "llm_profile.manage_acl", model.ResourceRef{Type: grantResourceLLMProfile, ID: resource.ID}, nil
	case grantResourceAgentProfile:
		return "agent_profile.manage_acl", model.ResourceRef{Type: grantResourceAgentProfile, ID: resource.ID}, nil
	case grantResourceMCPServer:
		return "mcp_server.manage_acl", model.ResourceRef{Type: grantResourceMCPServer, ID: resource.ID}, nil
	case grantResourceMCPProfile:
		return "mcp_profile.manage_acl", model.ResourceRef{Type: grantResourceMCPProfile, ID: resource.ID}, nil
	case grantResourceCredential:
		return "credential.manage_acl", model.ResourceRef{Type: grantResourceCredential, ID: resource.ID}, nil
	case grantResourceRunner:
		return "system.update", model.ResourceRef{Type: "dispatcher", ID: "runners"}, nil
	case grantResourceConfig:
		return "config_repo.manage", model.ResourceRef{Type: grantResourceConfig, ID: resource.ID}, nil
	default:
		return "", model.ResourceRef{}, fmt.Errorf("unsupported grant resource")
	}
}
