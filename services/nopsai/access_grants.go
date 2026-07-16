package nopsai

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nopsai/services/aaa/pkg/model"
)

const (
	productRoleViewer    = "viewer"
	productRoleDeveloper = "developer"
	productRoleOwner     = "owner"
	productRoleAdmin     = "admin"

	grantResourceTeam                = "team"
	grantResourcePipeline            = "pipeline"
	grantResourceRun                 = "pipeline_run"
	grantResourceSchedule            = "pipeline_schedule"
	grantResourceTrigger             = "trigger"
	grantResourceExternalTrigger     = "external_trigger"
	grantResourceGitWebhookSource    = "git_webhook_source"
	grantResourceSecret              = "secret"
	grantResourceVariable            = "variable"
	grantResourceScope               = "scope"
	grantResourceRepo                = "repository"
	grantResourceStep                = "step"
	grantResourceRunner              = "runner"
	grantResourceConfig              = "config_repo"
	grantResourceDashboard           = "dashboard"
	grantResourceKnowledgeContext    = "knowledge_context"
	grantResourceKnowledgeConnection = "knowledge_connection"
	grantResourceLLMProfile          = "llm_profile"
	grantResourceAgentProfile        = "agent_profile"
	grantResourceMCPServer           = "mcp_server"
	grantResourceMCPProfile          = "mcp_profile"
	grantResourceCredential          = "credential"
	grantResourceCompany             = "company"
	grantResourcePlatform            = "platform"

	grantSubjectService        = "service"
	grantSubjectUser           = "user"
	grantSubjectTeam           = "team"
	grantSubjectRepository     = "repository"
	grantSubjectTrigger        = "trigger"
	grantSubjectServiceAccount = "service_account"

	platformGrantID = "default"
	generalGrantID  = model.TeamGeneralID
	rootGrantID     = "root"
)

var (
	errEveryTeamMustRetainOwner             = errors.New("every team must retain at least one owner")
	errExternallyManagedUserRoleAssignments = errors.New("user role assignments are managed by the identity provider")
)

type productRoleDefinition struct {
	Description string
	Actions     []string
}

type accessGrantRecord struct {
	ID                           int64
	SubjectType                  string
	SubjectID                    string
	SubjectDisplay               string
	RoleName                     string
	ResourceType                 string
	ResourceID                   string
	ResourceDisplay              string
	Inherit                      bool
	GrantedBy                    string
	CreatedAt                    time.Time
	ManagedByConfig              bool
	ConfigSourcePath             string
	ConfigSourceCommitSHA        string
	ManagedByIdentityProvider    bool
	IdentityProviderID           string
	ExternalTeamName             string
	InheritedFromResourceType    string
	InheritedFromResourceID      string
	InheritedFromResourceDisplay string
}

type GrantProductRoleInput struct {
	SubjectType  string
	SubjectID    string
	RoleName     string
	ResourceType string
	ResourceID   string
	Inherit      bool
	GrantedBy    string
}

type createAccessGrantRequest struct {
	SubjectType  string `json:"subject_type"`
	SubjectID    string `json:"subject_id"`
	Role         string `json:"role"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Inherit      *bool  `json:"inherit"`
}

type accessGrantResponse struct {
	ID                        string    `json:"id"`
	SubjectType               string    `json:"subject_type"`
	SubjectID                 string    `json:"subject_id"`
	SubjectDisplay            string    `json:"subject_display,omitempty"`
	Role                      string    `json:"role"`
	ResourceType              string    `json:"resource_type"`
	ResourceID                string    `json:"resource_id"`
	Inherit                   bool      `json:"inherit"`
	GrantedBy                 string    `json:"granted_by,omitempty"`
	CreatedAt                 time.Time `json:"created_at,omitempty"`
	ManagedByConfigRepo       bool      `json:"managed_by_config_repo"`
	ConfigSourcePath          string    `json:"config_source_path,omitempty"`
	ConfigSourceCommitSHA     string    `json:"config_source_commit_sha,omitempty"`
	ManagedByIdentityProvider bool      `json:"managed_by_identity_provider"`
	IdentityProviderID        string    `json:"identity_provider_id,omitempty"`
	ExternalTeamName          string    `json:"external_team_name,omitempty"`
	Source                    string    `json:"source"`
	InheritedFromResourceType string    `json:"inherited_from_resource_type,omitempty"`
	InheritedFromResourceID   string    `json:"inherited_from_resource_id,omitempty"`
	InheritedFromResource     string    `json:"inherited_from_resource,omitempty"`
}

type effectivePermissionResponse struct {
	Allowed              bool           `json:"allowed"`
	Action               string         `json:"action"`
	Resource             string         `json:"resource"`
	Reason               string         `json:"reason"`
	MatchedRole          string         `json:"matched_role,omitempty"`
	MatchedSubject       string         `json:"matched_subject,omitempty"`
	MatchedResource      string         `json:"matched_resource,omitempty"`
	Inherited            bool           `json:"inherited"`
	SourceParentResource string         `json:"source_parent_resource,omitempty"`
	LowLevelPermission   string         `json:"low_level_permission,omitempty"`
	MatchedPolicy        map[string]any `json:"matched_policy,omitempty"`
}

type teamPathRecord struct {
	ID                 int
	Name               string
	Kind               string
	ParentID           *int
	Description        string
	RepoURL            string
	RepositoryFullName string
	Path               string
}

type accessGrantSubject struct {
	Type    string
	ID      string
	Display string
}

type accessGrantResource struct {
	Type    string
	ID      string
	Display string
}

type queryRunner interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type execRunner interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

var productRoleDefinitions = map[string]productRoleDefinition{
	productRoleViewer: {
		Description: "Read-only access to teams, pipelines, runs, schedules, triggers, steps, scope metadata, repositories, credentials, secrets, and variables.",
		Actions: []string{
			"team.list",
			"team.read",
			"pipeline.list",
			"pipeline.read",
			"pipeline_run.list",
			"pipeline_run.read",
			"pipeline_run.read_logs",
			"pipeline_schedule.list",
			"pipeline_schedule.read",
			"trigger.read",
			"external_trigger.read",
			"git_webhook_source.read",
			"secret.list_metadata",
			"variable.list_metadata",
			"scope.read",
			"repository.read",
			"step.read",
			"config_repo.read",
			"dashboard.list",
			"dashboard.read",
			"knowledge_context.read",
			"knowledge_connection.read",
			"llm_profile.read",
			"agent_profile.read",
			"mcp_server.read",
			"mcp_profile.read",
			"credential.list_metadata",
		},
	},
	productRoleDeveloper: {
		Description: "Viewer access plus non-destructive create, update, and execution capabilities.",
		Actions: []string{
			"team.list",
			"team.read",
			"pipeline.list",
			"pipeline.read",
			"pipeline_run.list",
			"pipeline_run.read",
			"pipeline_run.read_logs",
			"trigger.read",
			"secret.list_metadata",
			"variable.list_metadata",
			"scope.read",
			"repository.read",
			"step.read",
			"config_repo.read",
			"knowledge_context.read",
			"knowledge_connection.read",
			"pipeline.create",
			"pipeline.update",
			"pipeline.execute",
			"pipeline.use",
			"approval.approve",
			"pipeline_run.rerun",
			"pipeline_run.cancel",
			"pipeline_schedule.create",
			"pipeline_schedule.update",
			"pipeline_schedule.execute",
			"trigger.update",
			"external_trigger.create",
			"external_trigger.update",
			"external_trigger.invoke",
			"git_webhook_source.create",
			"git_webhook_source.update",
			"secret.use",
			"secret.write_value",
			"variable.use",
			"variable.write_value",
			"scope.use",
			"scope.update",
			"repository.update",
			"step.create",
			"step.update",
			"step.use",
			"runner.use",
			"config_repo.use",
			"dashboard.create",
			"dashboard.update",
			"dashboard.publish",
			"dashboard.refresh",
			"dashboard.manage_sources",
			"knowledge_context.use",
			"knowledge_connection.use",
			"llm_profile.use",
			"agent_profile.use",
			"mcp_server.use",
			"mcp_profile.use",
			"credential.list_metadata",
			"credential.create",
			"credential.write_value",
			"credential.rotate",
			"credential.use",
		},
	},
	productRoleOwner: {
		Description: "Developer access plus deletes, secret reads, and permission management inside the owned scope.",
		Actions: []string{
			"team.list",
			"team.read",
			"pipeline.list",
			"pipeline.read",
			"pipeline_run.list",
			"pipeline_run.read",
			"pipeline_run.read_logs",
			"trigger.read",
			"secret.list_metadata",
			"variable.list_metadata",
			"scope.read",
			"repository.read",
			"step.read",
			"config_repo.read",
			"knowledge_context.read",
			"knowledge_connection.read",
			"llm_profile.read",
			"agent_profile.read",
			"mcp_server.read",
			"mcp_profile.read",
			"credential.list_metadata",
			"pipeline.create",
			"pipeline.update",
			"pipeline.execute",
			"pipeline.use",
			"approval.approve",
			"pipeline_run.rerun",
			"pipeline_run.cancel",
			"pipeline_run.finalize",
			"pipeline_run.write_logs",
			"pipeline_run.task_update",
			"trigger.update",
			"external_trigger.create",
			"external_trigger.update",
			"external_trigger.invoke",
			"git_webhook_source.create",
			"git_webhook_source.update",
			"secret.use",
			"secret.write_value",
			"variable.read_value",
			"variable.use",
			"variable.write_value",
			"scope.use",
			"scope.update",
			"repository.update",
			"step.create",
			"step.update",
			"step.use",
			"runner.use",
			"config_repo.use",
			"dashboard.create",
			"dashboard.update",
			"dashboard.publish",
			"dashboard.refresh",
			"dashboard.manage_sources",
			"knowledge_context.use",
			"knowledge_connection.use",
			"llm_profile.use",
			"agent_profile.use",
			"mcp_server.use",
			"mcp_profile.use",
			"credential.create",
			"credential.write_value",
			"credential.rotate",
			"credential.disable",
			"credential.enable",
			"credential.use",
			"knowledge_context.create",
			"knowledge_context.update",
			"knowledge_context.delete",
			"knowledge_context.manage_access",
			"knowledge_connection.create",
			"knowledge_connection.update",
			"knowledge_connection.test",
			"knowledge_connection.delete",
			"knowledge_connection.manage_access",
			"llm_profile.manage_acl",
			"agent_profile.manage_acl",
			"mcp_server.manage_acl",
			"mcp_profile.manage_acl",
			"team.create",
			"team.update",
			"team.move",
			"team.delete",
			"team.manage_acl",
			"config_repo.manage",
			"config_repo.sync",
			"dashboard.delete",
			"dashboard.manage_acl",
			"pipeline.delete",
			"pipeline.manage_acl",
			"pipeline_run.delete",
			"pipeline_schedule.delete",
			"pipeline_schedule.manage_acl",
			"trigger.delete",
			"trigger.manage_acl",
			"external_trigger.delete",
			"external_trigger.manage_acl",
			"git_webhook_source.delete",
			"git_webhook_source.manage_acl",
			"secret.delete",
			"secret.read_value",
			"secret.manage_acl",
			"variable.delete",
			"variable.manage_acl",
			"scope.delete",
			"scope.manage_acl",
			"repository.delete",
			"repository.manage_acl",
			"step.delete",
			"step.manage_acl",
			"credential.delete_version",
			"credential.delete",
			"credential.manage_acl",
		},
	},
	productRoleAdmin: {
		Description: "Platform-wide administrator.",
		Actions:     []string{"*"},
	},
}

var productRoleIncludes = map[string][]string{
	productRoleDeveloper: {productRoleViewer},
	productRoleOwner:     {productRoleDeveloper},
}
