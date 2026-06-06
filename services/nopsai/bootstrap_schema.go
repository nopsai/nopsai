package nopsai

import (
	"context"

	"nopsai/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

type databaseBootstrapStep struct {
	name string
	run  func(context.Context, *pgxpool.Pool) error
}

func databaseBootstrapSteps(cfg *config.Config) []databaseBootstrapStep {
	return []databaseBootstrapStep{
		{name: "default admin", run: func(ctx context.Context, db *pgxpool.Pool) error {
			if cfg != nil && cfg.RequiresProductionGates() {
				return ensureNoDefaultAdminPassword(ctx, db)
			}
			return ensureDefaultAdmin(ctx, db)
		}},
		{name: "auth schema", run: ensureAuthSchema},
		{name: "group schema", run: ensureGroupSchema},
		{name: "product access roles", run: ensureProductAccessBootstrap},
		{name: "config repository schema", run: ensureConfigRepositorySchema},
		{name: "knowledge context schema", run: ensureKnowledgeContextSchema},
		{name: "resource authorization schema", run: ensureResourceAuthorizationSchema},
		{name: "external trigger schema", run: ensureExternalTriggerSchema},
		{name: "schedule schema", run: ensureScheduleSchema},
		{name: "LLM profile schema", run: ensureLLMProfileSchema},
		{name: "MCP schema", run: ensureMCPSchema},
		{name: "setup schema", run: ensureSetupSchema},
		{name: "approval schema", run: ensureApprovalSchema},
		{name: "notification schema", run: ensureNotificationSchema},
		{name: "data management schema", run: ensureDataManagementSchema},
	}
}
