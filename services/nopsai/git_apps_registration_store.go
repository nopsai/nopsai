package nopsai

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"nopsai/services/nopsai/pkg/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The registration and install callbacks are hit by a browser redirect from
// GitHub, which carries no bearer token. A single-use state row, created only by
// an authorized system.update request, is what authorizes the callback.
var gitHubAppRegistrationSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS github_app_registration_states (
		state_hash TEXT PRIMARY KEY,
		flow TEXT NOT NULL CHECK (flow IN ('register', 'install')),
		target TEXT NOT NULL DEFAULT '',
		organization TEXT NOT NULL DEFAULT '',
		app_name TEXT NOT NULL DEFAULT '',
		return_to TEXT NOT NULL DEFAULT '',
		actor TEXT NOT NULL DEFAULT '',
		expires_at TIMESTAMPTZ NOT NULL,
		consumed_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_github_app_registration_states_expiry ON github_app_registration_states(expires_at)`,
}

type gitHubAppRegistrationState struct {
	Flow         string
	Target       string
	Organization string
	AppName      string
	ReturnTo     string
	Actor        string
	ExpiresAt    time.Time
}

func ensureGitHubAppRegistrationSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	for idx, stmt := range gitHubAppRegistrationSchemaStatements {
		if _, err := db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply GitHub App registration schema statement %d: %w", idx+1, err)
		}
	}
	return nil
}

func generateGitHubAppRegistrationState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func createGitHubAppRegistrationState(
	ctx context.Context,
	db *pgxpool.Pool,
	state string,
	record gitHubAppRegistrationState,
) error {
	if db == nil {
		return fmt.Errorf("database is unavailable")
	}
	_, err := db.Exec(ctx, `
		INSERT INTO github_app_registration_states (
			state_hash, flow, target, organization, app_name, return_to, actor, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`,
		auth.HashToken(state),
		record.Flow,
		record.Target,
		record.Organization,
		record.AppName,
		record.ReturnTo,
		record.Actor,
		record.ExpiresAt,
	)
	return err
}

// consumeGitHubAppRegistrationState marks the state used inside the same
// transaction that reads it, so a replayed callback URL cannot register or
// install twice.
func consumeGitHubAppRegistrationState(
	ctx context.Context,
	db *pgxpool.Pool,
	flow, state string,
) (gitHubAppRegistrationState, error) {
	var record gitHubAppRegistrationState
	if db == nil {
		return record, fmt.Errorf("database is unavailable")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return record, err
	}
	defer tx.Rollback(ctx)
	stateHash := auth.HashToken(state)
	row := tx.QueryRow(ctx, `
		SELECT flow, target, organization, app_name, return_to, actor, expires_at
		FROM github_app_registration_states
		WHERE state_hash = $1
		  AND flow = $2
		  AND consumed_at IS NULL
		  AND expires_at > NOW()
		FOR UPDATE
	`, stateHash, flow)
	if err := row.Scan(
		&record.Flow,
		&record.Target,
		&record.Organization,
		&record.AppName,
		&record.ReturnTo,
		&record.Actor,
		&record.ExpiresAt,
	); err != nil {
		return record, err
	}
	if _, err := tx.Exec(
		ctx,
		`UPDATE github_app_registration_states SET consumed_at = NOW() WHERE state_hash = $1`,
		stateHash,
	); err != nil {
		return record, err
	}
	return record, tx.Commit(ctx)
}

func purgeExpiredGitHubAppRegistrationStates(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(
		ctx,
		`DELETE FROM github_app_registration_states WHERE expires_at < NOW() - INTERVAL '1 day'`,
	)
	return err
}
