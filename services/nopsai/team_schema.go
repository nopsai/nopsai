package nopsai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var teamSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS teams (
		id SERIAL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		parent_id INTEGER REFERENCES teams(id) ON DELETE CASCADE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(name)
	)`,
	`ALTER TABLE teams ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'team'`,
	`ALTER TABLE teams ADD COLUMN IF NOT EXISTS repo_url TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE teams ADD COLUMN IF NOT EXISTS repository_full_name TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL`,
	`ALTER TABLE teams DROP CONSTRAINT IF EXISTS teams_kind_check`,
	`UPDATE teams SET kind = 'team' WHERE kind IS NULL OR kind NOT IN ('team', 'app')`,
	`UPDATE teams
	 SET kind = 'app',
	     repository_full_name = TRIM(BOTH '/' FROM name)
	 WHERE kind = 'team'
	   AND repo_url = ''
	   AND repository_full_name = ''
	   AND name LIKE '%/%'`,
	`UPDATE teams
	 SET repo_url = 'https://github.com/' || repository_full_name
	 WHERE kind = 'app'
	   AND repo_url = ''
	   AND repository_full_name <> ''`,
	`ALTER TABLE teams ADD CONSTRAINT teams_kind_check CHECK (kind IN ('team', 'app'))`,
	`CREATE INDEX IF NOT EXISTS idx_teams_kind ON teams(kind)`,
	`CREATE INDEX IF NOT EXISTS idx_teams_repository_full_name ON teams(repository_full_name) WHERE repository_full_name <> ''`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_teams_repository_full_name_unique ON teams(LOWER(repository_full_name)) WHERE repository_full_name <> ''`,
}

func ensureTeamSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin team schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range teamSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply team schema statement %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit team schema transaction: %w", err)
	}
	return nil
}
