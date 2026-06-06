package nopsai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var groupSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS groups (
		id SERIAL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		parent_id INTEGER REFERENCES groups(id) ON DELETE CASCADE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		UNIQUE(name)
	)`,
	`ALTER TABLE groups ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'group'`,
	`ALTER TABLE groups ADD COLUMN IF NOT EXISTS repo_url TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE groups ADD COLUMN IF NOT EXISTS repository_full_name TEXT NOT NULL DEFAULT ''`,
	`UPDATE groups SET kind = 'group' WHERE kind IS NULL OR kind NOT IN ('group', 'app')`,
	`UPDATE groups
	 SET kind = 'app',
	     repository_full_name = TRIM(BOTH '/' FROM name)
	 WHERE kind = 'group'
	   AND repo_url = ''
	   AND repository_full_name = ''
	   AND name LIKE '%/%'`,
	`UPDATE groups
	 SET repo_url = 'https://github.com/' || repository_full_name
	 WHERE kind = 'app'
	   AND repo_url = ''
	   AND repository_full_name <> ''`,
	`ALTER TABLE groups DROP CONSTRAINT IF EXISTS groups_kind_check`,
	`ALTER TABLE groups ADD CONSTRAINT groups_kind_check CHECK (kind IN ('group', 'app'))`,
	`CREATE INDEX IF NOT EXISTS idx_groups_kind ON groups(kind)`,
	`CREATE INDEX IF NOT EXISTS idx_groups_repository_full_name ON groups(repository_full_name) WHERE repository_full_name <> ''`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_groups_repository_full_name_unique ON groups(LOWER(repository_full_name)) WHERE repository_full_name <> ''`,
}

func ensureGroupSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin group schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range groupSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply group schema statement %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit group schema transaction: %w", err)
	}
	return nil
}
