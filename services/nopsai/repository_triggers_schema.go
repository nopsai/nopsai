package nopsai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var repositoryTriggerSchemaStatements = []string{
	`ALTER TABLE triggers ADD COLUMN IF NOT EXISTS provider TEXT NOT NULL DEFAULT 'github'`,
	`UPDATE triggers SET provider = 'github' WHERE BTRIM(provider) = ''`,
	`ALTER TABLE triggers DROP CONSTRAINT IF EXISTS triggers_provider_check`,
	`ALTER TABLE triggers ADD CONSTRAINT triggers_provider_check CHECK (provider IN ('github', 'generic', 'gitlab', 'bitbucket', 'gitea'))`,
	`ALTER TABLE triggers ADD COLUMN IF NOT EXISTS team_path TEXT NOT NULL DEFAULT ''`,
	`UPDATE triggers SET team_path = 'global' WHERE BTRIM(team_path) = '' OR LOWER(BTRIM(team_path)) IN ('root', 'general', '__general__')`,
	`ALTER TABLE triggers ADD COLUMN IF NOT EXISTS management TEXT NOT NULL DEFAULT 'nopsai'`,
	`UPDATE triggers SET management = 'nopsai' WHERE BTRIM(management) = ''`,
	`ALTER TABLE triggers DROP CONSTRAINT IF EXISTS triggers_management_check`,
	`ALTER TABLE triggers ADD CONSTRAINT triggers_management_check CHECK (management IN ('nopsai', 'repository'))`,
	`ALTER TABLE triggers ADD COLUMN IF NOT EXISTS webhook_source_id TEXT`,
	`DO $$
	BEGIN
		IF NOT EXISTS (
			SELECT 1 FROM pg_constraint WHERE conname = 'triggers_webhook_source_id_fkey'
		) THEN
			ALTER TABLE triggers
				ADD CONSTRAINT triggers_webhook_source_id_fkey
				FOREIGN KEY (webhook_source_id)
				REFERENCES git_webhook_sources(id)
				ON DELETE SET NULL;
		END IF;
	END $$`,
	`CREATE INDEX IF NOT EXISTS idx_triggers_provider ON triggers(provider, repository_name)`,
	`CREATE INDEX IF NOT EXISTS idx_triggers_team_path ON triggers(team_path, repository_name)`,
	`CREATE INDEX IF NOT EXISTS idx_triggers_webhook_source ON triggers(webhook_source_id)`,
}

func ensureRepositoryTriggerSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin repository trigger schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range repositoryTriggerSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply repository trigger schema statement %d: %w", idx+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit repository trigger schema transaction: %w", err)
	}
	return nil
}
