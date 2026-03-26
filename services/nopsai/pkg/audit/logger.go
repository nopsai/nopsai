package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Entry struct {
	ActorSub   string
	ActorEmail string
	Provider   string
	TenantID   string
	Action     string
	Resource   string
	Result     string
	Metadata   map[string]any
}

type Logger struct {
	db *pgxpool.Pool
}

func NewLogger(db *pgxpool.Pool) *Logger {
	return &Logger{db: db}
}

func (l *Logger) Write(ctx context.Context, e Entry) error {
	if l == nil || l.db == nil {
		return nil
	}
	var metaJSON []byte
	if len(e.Metadata) > 0 {
		metaJSON, _ = json.Marshal(e.Metadata)
	}
	var tenantUUID *uuid.UUID
	if e.TenantID != "" {
		if parsed, err := uuid.Parse(e.TenantID); err == nil {
			tenantUUID = &parsed
		}
	}
	_, err := l.db.Exec(ctx, `
		INSERT INTO audit_logs (actor_sub, actor_email, provider, tenant_id, action, resource, result, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, e.ActorSub, e.ActorEmail, e.Provider, tenantUUID, e.Action, e.Resource, e.Result, metaJSON, time.Now())
	return err
}
