package nopsai

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ensureAuthTeamsForNames(ctx context.Context, runner execRunner, rawNames []string, description string) (int, error) {
	if runner == nil || len(rawNames) == 0 {
		return 0, nil
	}
	description = strings.TrimSpace(description)
	if description == "" {
		description = "Created from NopsAI team sync"
	}

	teamSet := map[string]struct{}{}
	for _, rawName := range rawNames {
		name := strings.Trim(strings.TrimSpace(rawName), "/")
		if name == "" {
			continue
		}
		teamSet[name] = struct{}{}
	}
	if len(teamSet) == 0 {
		return 0, nil
	}

	teamNames := make([]string, 0, len(teamSet))
	for name := range teamSet {
		teamNames = append(teamNames, name)
	}
	sort.Strings(teamNames)

	created := 0
	for _, name := range teamNames {
		tag, err := runner.Exec(ctx, `
			INSERT INTO auth_teams (name, description)
			VALUES ($1, $2)
			ON CONFLICT (name) DO NOTHING
		`, name, description)
		if err != nil {
			return created, fmt.Errorf("failed to ensure auth team %q: %w", name, err)
		}
		if tag.RowsAffected() > 0 {
			created++
		}
	}
	return created, nil
}

func reconcileTeamAuthTeamSubjects(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	records, err := loadTeamPathRecords(ctx, db)
	if err != nil {
		return fmt.Errorf("failed to load team paths for auth-team subject reconciliation: %w", err)
	}
	names := make([]string, 0, len(records))
	for _, record := range records {
		if record.Kind != "team" || record.Path == "" {
			continue
		}
		names = append(names, record.Path)
	}
	_, err = ensureAuthTeamsForNames(ctx, db, names, "Created from NopsAI team sync")
	return err
}
