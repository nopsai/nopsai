package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/systemlogs"
	"nopsai/services/nopsai/pkg/auth"
)

type authenticatedUserRecord struct {
	ID                 uuid.UUID
	Provider           string
	Status             string
	PasswordHash       sql.NullString
	MustChangePassword bool
}

func (r authenticatedUserRecord) IsActive() bool {
	return strings.EqualFold(strings.TrimSpace(r.Status), "active")
}

func (a *App) loadAuthenticatedUserRecord(ctx context.Context, sub string) (authenticatedUserRecord, error) {
	var record authenticatedUserRecord
	err := a.db.QueryRow(ctx, `
		SELECT id, provider, status, password_hash, must_change_password
		FROM users
		WHERE sub = $1
	`, strings.TrimSpace(sub)).Scan(&record.ID, &record.Provider, &record.Status, &record.PasswordHash, &record.MustChangePassword)
	return record, err
}

func normalizeOptionalEmail(raw string) (string, error) {
	email := strings.TrimSpace(raw)
	if email == "" {
		return "", nil
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil {
		return "", fmt.Errorf("invalid email")
	}
	return strings.TrimSpace(parsed.Address), nil
}

func (a *App) userEmailInUse(ctx context.Context, email string, excludeUserID *uuid.UUID) (bool, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return false, nil
	}

	var (
		query = `SELECT 1 FROM users WHERE LOWER(email) = LOWER($1)`
		args  = []any{email}
	)
	if excludeUserID != nil {
		query += ` AND id <> $2`
		args = append(args, *excludeUserID)
	}
	query += ` LIMIT 1`

	var exists int
	err := a.db.QueryRow(ctx, query, args...).Scan(&exists)
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, err
	default:
		return true, nil
	}
}

func (a *App) authCapabilities(claims *auth.Claims) *authCapabilitiesResponse {
	if claims == nil {
		return &authCapabilitiesResponse{}
	}
	if a == nil || !a.aaaAvailable() {
		return &authCapabilitiesResponse{}
	}

	subject := a.buildAAASubject(claims)
	ctx := context.Background()
	pipelineWrite := a.checkCapabilityOrScopedGrant(ctx, subject, "pipeline.update", model.ResourceRef{Type: "pipeline", ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "pipeline.create", model.ResourceRef{Type: "pipeline", ID: "*"})
	configRead := a.checkCapability(subject, "system.read", model.ResourceRef{Type: "system", ID: "config"}) &&
		a.checkCapability(subject, "system.read", model.ResourceRef{Type: "system", ID: "config-sync"})
	configWrite := a.checkCapability(subject, "system.update", model.ResourceRef{Type: "system", ID: "config"}) &&
		a.checkCapability(subject, "system.update", model.ResourceRef{Type: "system", ID: "config-sync"})
	llmProfilesRead := a.checkCapability(subject, "system.read", model.ResourceRef{Type: "system", ID: "llm-profiles"})
	llmProfilesWrite := a.checkCapability(subject, "system.update", model.ResourceRef{Type: "system", ID: "llm-profiles"})
	agentProfilesRead := a.checkCapability(subject, "system.read", model.ResourceRef{Type: "system", ID: "agent-profiles"})
	agentProfilesWrite := a.checkCapability(subject, "system.update", model.ResourceRef{Type: "system", ID: "agent-profiles"})
	mcpRead := a.checkCapability(subject, "system.read", model.ResourceRef{Type: "system", ID: "mcp"})
	mcpWrite := a.checkCapability(subject, "system.update", model.ResourceRef{Type: "system", ID: "mcp"})
	credentialsRead := a.checkCapability(subject, "credential.list_metadata", model.ResourceRef{Type: "credential", ID: "*"})
	credentialsWrite := a.checkCapability(subject, "credential.write_value", model.ResourceRef{Type: "credential", ID: "*"}) ||
		a.checkCapability(subject, "credential.create", model.ResourceRef{Type: "credential", ID: "*"}) ||
		a.checkCapability(subject, "credential.rotate", model.ResourceRef{Type: "credential", ID: "*"}) ||
		a.checkCapability(subject, "credential.disable", model.ResourceRef{Type: "credential", ID: "*"}) ||
		a.checkCapability(subject, "credential.enable", model.ResourceRef{Type: "credential", ID: "*"}) ||
		a.checkCapability(subject, "credential.delete_version", model.ResourceRef{Type: "credential", ID: "*"}) ||
		a.checkCapability(subject, "credential.delete", model.ResourceRef{Type: "credential", ID: "*"})
	configReposRead := a.checkCapability(subject, "system.read", model.ResourceRef{Type: "system", ID: "config-repos"})
	configReposWrite := a.checkCapability(subject, "system.update", model.ResourceRef{Type: "system", ID: "config-repos"})
	dispatcherRead := a.checkCapability(subject, "system.read", model.ResourceRef{Type: "dispatcher", ID: "status"})
	dispatcherWrite := a.checkCapability(subject, "system.update", model.ResourceRef{Type: "dispatcher", ID: "runners"})
	systemLogsRead := false
	registry := systemlogs.DefaultRegistry()
	if a.checkCapability(subject, "system_log.read", model.ResourceRef{Type: "system_log", ID: "*"}) {
		systemLogsRead = true
	} else {
		for _, source := range registry.Sources() {
			if !a.checkCapability(subject, "system_log.read", model.ResourceRef{Type: "system_log", ID: source.ID}) {
				continue
			}
			systemLogsRead = true
			break
		}
	}
	triggerRead := a.checkCapabilityOrScopedGrant(ctx, subject, "trigger.read", model.ResourceRef{Type: "trigger", ID: "*"})
	triggerWrite := a.checkCapabilityOrScopedGrant(ctx, subject, "trigger.update", model.ResourceRef{Type: "trigger", ID: "*"})
	triggerDelete := a.checkCapabilityOrScopedGrant(ctx, subject, "trigger.delete", model.ResourceRef{Type: "trigger", ID: "*"})
	externalTriggerRead := a.checkCapabilityOrScopedGrant(ctx, subject, "external_trigger.read", model.ResourceRef{Type: grantResourceExternalTrigger, ID: "*"})
	externalTriggerWrite := a.checkCapabilityOrScopedGrant(ctx, subject, "external_trigger.update", model.ResourceRef{Type: grantResourceExternalTrigger, ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "external_trigger.create", model.ResourceRef{Type: grantResourceExternalTrigger, ID: "*"})
	externalTriggerDelete := a.checkCapabilityOrScopedGrant(ctx, subject, "external_trigger.delete", model.ResourceRef{Type: grantResourceExternalTrigger, ID: "*"})
	gitWebhookSourceRead := a.checkCapabilityOrScopedGrant(ctx, subject, "git_webhook_source.read", model.ResourceRef{Type: grantResourceGitWebhookSource, ID: "*"})
	gitWebhookSourceWrite := a.checkCapabilityOrScopedGrant(ctx, subject, "git_webhook_source.update", model.ResourceRef{Type: grantResourceGitWebhookSource, ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "git_webhook_source.create", model.ResourceRef{Type: grantResourceGitWebhookSource, ID: "*"})
	gitWebhookSourceDelete := a.checkCapabilityOrScopedGrant(ctx, subject, "git_webhook_source.delete", model.ResourceRef{Type: grantResourceGitWebhookSource, ID: "*"})
	scheduleRead := a.checkCapabilityOrScopedGrant(ctx, subject, "pipeline_schedule.read", model.ResourceRef{Type: grantResourceSchedule, ID: "*"})
	scheduleWrite := a.checkCapabilityOrScopedGrant(ctx, subject, "pipeline_schedule.update", model.ResourceRef{Type: grantResourceSchedule, ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "pipeline_schedule.create", model.ResourceRef{Type: grantResourceSchedule, ID: "*"})
	scheduleDelete := a.checkCapabilityOrScopedGrant(ctx, subject, "pipeline_schedule.delete", model.ResourceRef{Type: grantResourceSchedule, ID: "*"})
	scopeRead := a.checkCapabilityOrScopedGrant(ctx, subject, "secret.list_metadata", model.ResourceRef{Type: "secret", ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "variable.list_metadata", model.ResourceRef{Type: "variable", ID: "*"}) ||
		a.hasScopedProductGrantCapability(ctx, subject, "scope.read")
	scopeWrite := a.checkCapabilityOrScopedGrant(ctx, subject, "scope.update", model.ResourceRef{Type: "scope", ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "secret.write_value", model.ResourceRef{Type: "secret", ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "variable.write_value", model.ResourceRef{Type: "variable", ID: "*"})
	scopeDelete := a.checkCapabilityOrScopedGrant(ctx, subject, "scope.delete", model.ResourceRef{Type: "scope", ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "secret.delete", model.ResourceRef{Type: "secret", ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "variable.delete", model.ResourceRef{Type: "variable", ID: "*"})
	knowledgeRead := a.checkCapabilityOrScopedGrant(ctx, subject, "knowledge_context.read", model.ResourceRef{Type: "knowledge_context", ID: "*"})
	knowledgeWrite := a.checkCapabilityOrScopedGrant(ctx, subject, "knowledge_context.update", model.ResourceRef{Type: "knowledge_context", ID: "*"}) ||
		a.checkCapabilityOrScopedGrant(ctx, subject, "knowledge_context.create", model.ResourceRef{Type: "knowledge_context", ID: "*"})
	knowledgeDelete := a.checkCapabilityOrScopedGrant(ctx, subject, "knowledge_context.delete", model.ResourceRef{Type: "knowledge_context", ID: "*"})

	return &authCapabilitiesResponse{
		Pipelines: authResourceCapabilities{
			Write:  pipelineWrite,
			Delete: a.checkCapabilityOrScopedGrant(ctx, subject, "pipeline.delete", model.ResourceRef{Type: "pipeline", ID: "*"}),
		},
		Steps: authResourceCapabilities{
			Write: a.checkCapabilityOrScopedGrant(ctx, subject, "step.update", model.ResourceRef{Type: "step", ID: "*"}) ||
				a.checkCapabilityOrScopedGrant(ctx, subject, "step.create", model.ResourceRef{Type: "step", ID: "*"}),
			Delete: a.checkCapabilityOrScopedGrant(ctx, subject, "step.delete", model.ResourceRef{Type: "step", ID: "*"}),
		},
		Schedules: authReadCapabilities{
			Read:   scheduleRead,
			Write:  scheduleWrite,
			Delete: scheduleDelete,
		},
		Triggers: authReadCapabilities{
			Read:   triggerRead,
			Write:  triggerWrite,
			Delete: triggerDelete,
		},
		ExternalTriggers: authReadCapabilities{
			Read:   externalTriggerRead,
			Write:  externalTriggerWrite,
			Delete: externalTriggerDelete,
		},
		GitWebhookSources: authReadCapabilities{
			Read:   gitWebhookSourceRead,
			Write:  gitWebhookSourceWrite,
			Delete: gitWebhookSourceDelete,
		},
		Scopes: authReadCapabilities{
			Read:   scopeRead,
			Write:  scopeWrite,
			Delete: scopeDelete,
		},
		Knowledge: authReadCapabilities{
			Read:   knowledgeRead,
			Write:  knowledgeWrite,
			Delete: knowledgeDelete,
		},
		System: authSystemCapabilities{
			ConfigRead:         configRead,
			ConfigWrite:        configWrite,
			LLMProfilesRead:    llmProfilesRead,
			LLMProfilesWrite:   llmProfilesWrite,
			AgentProfilesRead:  agentProfilesRead,
			AgentProfilesWrite: agentProfilesWrite,
			MCPRead:            mcpRead,
			MCPWrite:           mcpWrite,
			CredentialsRead:    credentialsRead,
			CredentialsWrite:   credentialsWrite,
			ConfigReposRead:    configReposRead,
			ConfigReposWrite:   configReposWrite,
			DispatcherRead:     dispatcherRead,
			DispatcherWrite:    dispatcherWrite,
			LogsRead:           systemLogsRead,
			Access:             a.checkCapability(subject, "iam.admin", model.ResourceRef{Type: "iam", ID: "admin"}),
		},
	}
}

func (a *App) checkCapabilityOrScopedGrant(ctx context.Context, subject model.Subject, action string, resource model.ResourceRef) bool {
	if a.checkCapability(subject, action, resource) {
		return true
	}
	return a.hasScopedProductGrantCapability(ctx, subject, action)
}

func (a *App) hasScopedProductGrantCapability(ctx context.Context, subject model.Subject, action string) bool {
	if a == nil || a.db == nil {
		return false
	}

	refs := a.scopedGrantSubjectRefs(ctx, subject)
	if len(refs) == 0 {
		return false
	}

	conditions := make([]string, 0, len(refs))
	args := make([]any, 0, len(refs)*2)
	for _, ref := range refs {
		if strings.TrimSpace(ref.Type) == "" || strings.TrimSpace(ref.ID) == "" {
			continue
		}
		args = append(args, ref.Type, ref.ID)
		conditions = append(conditions, fmt.Sprintf("(subject_type = $%d AND subject_id = $%d)", len(args)-1, len(args)))
	}
	if len(conditions) == 0 {
		return false
	}

	rows, err := a.db.Query(ctx, `
		SELECT role_name, resource_type
		FROM access_grants
		WHERE `+strings.Join(conditions, " OR "), args...)
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var roleName, resourceType string
		if err := rows.Scan(&roleName, &resourceType); err != nil {
			return false
		}
		if productGrantIncludesAction(roleName, resourceType, action) {
			return true
		}
	}
	return false
}

func (a *App) scopedGrantSubjectRefs(ctx context.Context, subject model.Subject) []model.SubjectRef {
	subjectType := model.NormalizeType(subject.Type)
	subjectID := strings.TrimSpace(subject.ID)

	switch subjectType {
	case model.SubjectTypeUser:
		resolvedID, err := a.lookupScopedGrantUserID(ctx, subject)
		if err != nil || resolvedID == "" {
			return nil
		}
		subjectID = resolvedID
	case model.SubjectTypeAuthGroup, model.SubjectTypeInternalService:
		if subjectID == "" {
			return nil
		}
	default:
		return nil
	}

	refs := []model.SubjectRef{{Type: subjectType, ID: subjectID}}
	rows, err := a.db.Query(ctx, `
		SELECT group_id::text
		FROM auth_group_members
		WHERE subject_type = $1 AND subject_id = $2
		ORDER BY group_id ASC
	`, subjectType, subjectID)
	if err != nil {
		return refs
	}
	defer rows.Close()

	for rows.Next() {
		var groupID string
		if err := rows.Scan(&groupID); err != nil {
			return refs
		}
		groupID = strings.TrimSpace(groupID)
		if groupID == "" {
			continue
		}
		refs = append(refs, model.SubjectRef{Type: model.SubjectTypeAuthGroup, ID: groupID})
	}
	return refs
}

func (a *App) lookupScopedGrantUserID(ctx context.Context, subject model.Subject) (string, error) {
	switch {
	case strings.TrimSpace(subject.ID) != "":
		return strings.TrimSpace(subject.ID), nil
	case strings.TrimSpace(subject.Sub) != "":
		var id string
		err := a.db.QueryRow(ctx, `SELECT id::text FROM users WHERE sub = $1 LIMIT 1`, strings.TrimSpace(subject.Sub)).Scan(&id)
		return id, err
	case strings.TrimSpace(subject.Email) != "":
		var id string
		err := a.db.QueryRow(ctx, `SELECT id::text FROM users WHERE LOWER(email) = LOWER($1) LIMIT 1`, strings.TrimSpace(subject.Email)).Scan(&id)
		return id, err
	default:
		return "", pgx.ErrNoRows
	}
}

func productGrantIncludesAction(roleName, resourceType, action string) bool {
	actions := applicableProductRoleActions(strings.TrimSpace(roleName), strings.TrimSpace(resourceType))
	for _, candidate := range actions {
		if candidate == "*" || candidate == action {
			return true
		}
	}
	return false
}

func (a *App) checkCapability(subject model.Subject, action string, resource model.ResourceRef) bool {
	if a == nil || !a.aaaAvailable() {
		return false
	}
	decision, err := a.aaaCheck(context.Background(), subject, action, resource, nil)
	return err == nil && decision.Allowed
}
