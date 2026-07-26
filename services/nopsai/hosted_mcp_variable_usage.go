package nopsai

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/routeauthz"
)

type hostedMCPVariableUsageAggregate struct {
	Name      string
	Scopes    map[string]struct{}
	Repos     map[string]struct{}
	Sources   map[string]struct{}
	Locations []map[string]any
	UpdatedAt time.Time
}

func (a *App) hostedMCPAnalyzeVariableUsage(ctx context.Context, subject aaamodel.Subject, args map[string]any) (map[string]any, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	queryArgs := []any{}
	conditions := []string{"TRUE"}
	scopeFilter := strings.TrimSpace(stringArg(args, "scope"))
	if scopeFilter != "" {
		scope := runtimeScopeForStorage(scopeFilter)
		queryArgs = append(queryArgs, scope)
		conditions = append(conditions, runtimeScopeEqualsSQL("scope", len(queryArgs)))
	}
	repositoryFilter := hostedMCPRepositoryFullName(args)
	if repositoryFilter != "" {
		queryArgs = append(queryArgs, repositoryFilter)
		conditions = append(conditions, fmt.Sprintf("repository_name = $%d", len(queryArgs)))
	}

	rows, err := a.db.Query(ctx, `
		SELECT name, COALESCE(repository_name, ''), scope, COALESCE(source, 'database'), created_at, updated_at
		FROM variables
		WHERE `+strings.Join(conditions, " AND ")+`
		ORDER BY LOWER(name), scope, repository_name NULLS FIRST
	`, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aggregates := map[string]*hostedMCPVariableUsageAggregate{}
	totalVisible := 0
	visibleScopes := map[string]struct{}{}
	for rows.Next() {
		var name, repository, scope, source string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&name, &repository, &scope, &source, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		displayScope := runtimeScopeForDisplay(scope)
		resource := routeauthz.BuildVariableResource(repository, runtimeScopeForResource(displayScope), name)
		if !a.hostedMCPAllowed(ctx, subject, hostedMCPPermission{Action: "variable.list_metadata", Resource: resource}) {
			continue
		}

		totalVisible++
		visibleScopes[displayScope] = struct{}{}
		key := strings.ToLower(name)
		aggregate := aggregates[key]
		if aggregate == nil {
			aggregate = &hostedMCPVariableUsageAggregate{
				Name:    name,
				Scopes:  map[string]struct{}{},
				Repos:   map[string]struct{}{},
				Sources: map[string]struct{}{},
			}
			aggregates[key] = aggregate
		}
		aggregate.Scopes[displayScope] = struct{}{}
		repositoryLabel := repository
		if repositoryLabel == "" {
			repositoryLabel = "global"
		}
		aggregate.Repos[repositoryLabel] = struct{}{}
		aggregate.Sources[normalizeVariableSourceKey(source)] = struct{}{}
		if updatedAt.After(aggregate.UpdatedAt) {
			aggregate.UpdatedAt = updatedAt
		}
		aggregate.Locations = append(aggregate.Locations, map[string]any{
			"name":       name,
			"repository": repositoryLabel,
			"scope":      displayScope,
			"source":     normalizeVariableSourceKey(source),
			"created_at": createdAt,
			"updated_at": updatedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	duplicates := []map[string]any{}
	for _, aggregate := range aggregates {
		if len(aggregate.Locations) <= 1 {
			continue
		}
		duplicates = append(duplicates, map[string]any{
			"name":         aggregate.Name,
			"occurrences":  len(aggregate.Locations),
			"scopes":       sortedKeys(aggregate.Scopes),
			"repositories": sortedKeys(aggregate.Repos),
			"sources":      sortedKeys(aggregate.Sources),
			"locations":    aggregate.Locations,
			"updated_at":   aggregate.UpdatedAt,
		})
	}
	sort.Slice(duplicates, func(i, j int) bool {
		left, _ := duplicates[i]["occurrences"].(int)
		right, _ := duplicates[j]["occurrences"].(int)
		if left != right {
			return left > right
		}
		leftName, _ := duplicates[i]["name"].(string)
		rightName, _ := duplicates[j]["name"].(string)
		return leftName < rightName
	})
	limit := limitArg(args, 20, 100)
	if len(duplicates) > limit {
		duplicates = duplicates[:limit]
	}

	return map[string]any{
		"metadata_only":               true,
		"total_visible_variables":     totalVisible,
		"unique_variable_names":       len(aggregates),
		"repetitive_variable_names":   countRepeatedVariableNames(aggregates),
		"visible_scope_count":         len(visibleScopes),
		"duplicates":                  duplicates,
		"scope_filter":                scopeFilter,
		"repository_filter":           repositoryFilter,
		"values_read":                 false,
		"values_intentionally_hidden": true,
	}, nil
}

func countRepeatedVariableNames(aggregates map[string]*hostedMCPVariableUsageAggregate) int {
	count := 0
	for _, aggregate := range aggregates {
		if len(aggregate.Locations) > 1 {
			count++
		}
	}
	return count
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if strings.TrimSpace(key) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
