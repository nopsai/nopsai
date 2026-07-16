package nopsai

import (
	"context"

	"nopsai/config"
	aaastore "nopsai/services/aaa/pkg/store"

	"github.com/jackc/pgx/v5/pgxpool"
)

type databaseBootstrapStep struct {
	name string
	run  func(context.Context, *pgxpool.Pool) error
}

func databaseBootstrapSteps(cfg *config.Config) []databaseBootstrapStep {
	return []databaseBootstrapStep{
		{name: "aaa policy schema", run: func(ctx context.Context, db *pgxpool.Pool) error {
			if db == nil {
				return nil
			}
			return aaastore.NewPGStore(db).EnsureSchema(ctx)
		}},
		{name: "default admin", run: func(ctx context.Context, db *pgxpool.Pool) error {
			if cfg != nil && cfg.RequiresProductionGates() {
				return ensureNoDefaultAdminPassword(ctx, db)
			}
			return ensureDefaultAdmin(ctx, db)
		}},
		{name: "auth schema", run: ensureAuthSchema},
		{name: "config repository schema", run: ensureConfigRepositorySchema},
		{name: "runtime settings schema", run: ensureRuntimeSettingsSchema},
		{name: "credential schema", run: ensureCredentialSchema},
		{name: "team schema", run: ensureTeamSchema},
		{name: "product access roles", run: ensureProductAccessBootstrap},
		{name: "knowledge context schema", run: ensureKnowledgeContextSchema},
		{name: "resource authorization schema", run: ensureResourceAuthorizationSchema},
		{name: "external trigger schema", run: ensureExternalTriggerSchema},
		{name: "git webhook source schema", run: ensureGitWebhookSourceSchema},
		{name: "repository trigger schema", run: ensureRepositoryTriggerSchema},
		{name: "schedule schema", run: ensureScheduleSchema},
		{name: "LLM profile schema", run: ensureLLMProfileSchema},
		{name: "agent profile schema", run: ensureAgentProfileSchema},
		{name: "MCP schema", run: ensureMCPSchema},
		{name: "team profile schema", run: ensureTeamProfileSchema},
		{name: "assistant schema", run: ensureAssistantSchema},
		{name: "pipeline final output schema", run: ensurePipelineFinalOutputSchema},
		{name: "dashboard schema", run: ensureDashboardSchema},
		{name: "setup schema", run: ensureSetupSchema},
		{name: "approval schema", run: ensureApprovalSchema},
		{name: "notification schema", run: ensureNotificationSchema},
		{name: "monitoring analytics schema", run: ensureMonitoringAnalyticsSchema},
		{name: "data management schema", run: ensureDataManagementSchema},
		{name: "legacy credential migration", run: func(ctx context.Context, db *pgxpool.Pool) error {
			return migrateLegacyCredentialSources(ctx, db, cfg)
		}},
		{name: "auth oidc config", run: func(ctx context.Context, db *pgxpool.Pool) error {
			return seedOIDCConfigProviders(ctx, db, cfg)
		}},
		{name: "oidc auth team mappings", run: reconcileOIDCAuthTeamMappings},
		{name: "oidc basic role mappings", run: reconcileOIDCBasicRoleMappings},
	}
}
