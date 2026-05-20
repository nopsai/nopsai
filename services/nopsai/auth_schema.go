package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var authSchemaStatements = []string{
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN NOT NULL DEFAULT FALSE`,
	`UPDATE users
	 SET must_change_password = TRUE
	 WHERE provider = 'local'
	   AND password_hash IS NOT NULL
	   AND last_login IS NULL
	   AND must_change_password = FALSE`,
	`CREATE TABLE IF NOT EXISTS personal_access_tokens (
		id UUID PRIMARY KEY,
		user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		token_hash TEXT UNIQUE NOT NULL,
		token_suffix TEXT NOT NULL DEFAULT '',
		expires_at TIMESTAMPTZ,
		last_used_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		revoked_at TIMESTAMPTZ
	)`,
	`ALTER TABLE personal_access_tokens ALTER COLUMN expires_at DROP NOT NULL`,
	`CREATE INDEX IF NOT EXISTS idx_personal_access_tokens_user ON personal_access_tokens(user_id)`,
	`CREATE INDEX IF NOT EXISTS idx_personal_access_tokens_expiry ON personal_access_tokens(expires_at)`,
}

func ensureAuthSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin auth schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range authSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply auth schema statement %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit auth schema transaction: %w", err)
	}
	return nil
}
