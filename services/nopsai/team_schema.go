package nopsai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var teamSchemaStatements = []string{
	`DO $$
	DECLARE
		team_count BIGINT := 0;
	BEGIN
		IF to_regclass('groups') IS NOT NULL THEN
			IF to_regclass('teams') IS NOT NULL THEN
				EXECUTE 'SELECT COUNT(*) FROM teams' INTO team_count;
				IF team_count = 0 THEN
					DROP TABLE teams;
				END IF;
			END IF;
			IF to_regclass('teams') IS NULL THEN
				ALTER TABLE groups RENAME TO teams;
			END IF;
		END IF;
	END $$`,
	`CREATE TABLE IF NOT EXISTS teams (
		id SERIAL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		parent_id INTEGER REFERENCES teams(id) ON DELETE CASCADE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(name)
	)`,
	`DO $$
	BEGIN
		IF EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'teams'::regclass AND conname = 'groups_pkey')
		   AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'teams'::regclass AND conname = 'teams_pkey') THEN
			ALTER TABLE teams RENAME CONSTRAINT groups_pkey TO teams_pkey;
		END IF;
		IF EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'teams'::regclass AND conname = 'groups_name_key')
		   AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'teams'::regclass AND conname = 'teams_name_key') THEN
			ALTER TABLE teams RENAME CONSTRAINT groups_name_key TO teams_name_key;
		END IF;
		IF EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'teams'::regclass AND conname = 'groups_parent_id_fkey')
		   AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'teams'::regclass AND conname = 'teams_parent_id_fkey') THEN
			ALTER TABLE teams RENAME CONSTRAINT groups_parent_id_fkey TO teams_parent_id_fkey;
		END IF;
	END $$`,
	`ALTER TABLE teams ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'team'`,
	`ALTER TABLE teams ADD COLUMN IF NOT EXISTS repo_url TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE teams ADD COLUMN IF NOT EXISTS repository_full_name TEXT NOT NULL DEFAULT ''`,
	`DO $$
	BEGIN
		IF to_regclass('pipeline_runs') IS NOT NULL THEN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'pipeline_runs' AND column_name = 'group_id'
			) AND NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'pipeline_runs' AND column_name = 'team_id'
			) THEN
				ALTER TABLE pipeline_runs RENAME COLUMN group_id TO team_id;
			END IF;
		END IF;
	END $$`,
	`ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS team_id INTEGER REFERENCES teams(id) ON DELETE SET NULL`,
	`DO $$
	BEGIN
		IF EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'pipeline_runs'::regclass AND conname = 'pipeline_runs_group_id_fkey')
		   AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'pipeline_runs'::regclass AND conname = 'pipeline_runs_team_id_fkey') THEN
			ALTER TABLE pipeline_runs RENAME CONSTRAINT pipeline_runs_group_id_fkey TO pipeline_runs_team_id_fkey;
		END IF;
	END $$`,
	`ALTER TABLE teams DROP CONSTRAINT IF EXISTS groups_kind_check`,
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
	`DROP INDEX IF EXISTS idx_groups_kind`,
	`DROP INDEX IF EXISTS idx_groups_repository_full_name`,
	`DROP INDEX IF EXISTS idx_groups_repository_full_name_unique`,
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
