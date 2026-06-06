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

	grantResourceFolder           = "folder"
	grantResourceTeam             = "team"
	grantResourcePipeline         = "pipeline"
	grantResourceRun              = "pipeline_run"
	grantResourceSchedule         = "pipeline_schedule"
	grantResourceTrigger          = "trigger"
	grantResourceExternalTrigger  = "external_trigger"
	grantResourceSecret           = "secret"
	grantResourceVariable         = "variable"
	grantResourceScope            = "scope"
	grantResourceRepo             = "repository"
	grantResourceStep             = "step"
	grantResourceRunner           = "runner"
	grantResourceConfig           = "config_repo"
	grantResourceKnowledgeContext = "knowledge_context"
	grantResourceCompany          = "company"
	grantResourcePlatform         = "platform"

	grantSubjectService        = "service"
	grantSubjectUser           = "user"
	grantSubjectGroup          = "group"
	grantSubjectRepository     = "repository"
	grantSubjectTrigger        = "trigger"
	grantSubjectServiceAccount = "service_account"

	platformGrantID = "default"
	generalGrantID  = model.FolderGeneralID
	rootGrantID     = "root"
)

var errEveryFolderMustRetainOwner = errors.New("every folder must retain at least one owner")

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

type groupPathRecord struct {
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
		Description: "Read-only access to folders, pipelines, runs, schedules, triggers, steps, scope metadata, repositories, secrets, and variables.",
		Actions: []string{
			"folder.list",
			"folder.read",
			"pipeline.list",
			"pipeline.read",
			"pipeline_run.list",
			"pipeline_run.read",
			"pipeline_run.read_logs",
			"pipeline_schedule.list",
			"pipeline_schedule.read",
			"trigger.read",
			"external_trigger.read",
			"secret.list_metadata",
			"variable.list_metadata",
			"scope.read",
			"repository.read",
			"step.read",
			"config_repo.read",
			"knowledge_context.read",
		},
	},
	productRoleDeveloper: {
		Description: "Viewer access plus non-destructive create, update, and execution capabilities.",
		Actions: []string{
			"folder.list",
			"folder.read",
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
			"knowledge_context.use",
		},
	},
	productRoleOwner: {
		Description: "Developer access plus deletes, secret reads, and permission management inside the owned scope.",
		Actions: []string{
			"folder.list",
			"folder.read",
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
			"knowledge_context.use",
			"knowledge_context.create",
			"knowledge_context.update",
			"knowledge_context.delete",
			"knowledge_context.manage_access",
			"folder.create",
			"folder.update",
			"folder.move",
			"folder.delete",
			"folder.manage_acl",
			"config_repo.manage",
			"config_repo.sync",
			"pipeline.delete",
			"pipeline.manage_acl",
			"pipeline_run.delete",
			"pipeline_schedule.delete",
			"pipeline_schedule.manage_acl",
			"trigger.delete",
			"trigger.manage_acl",
			"external_trigger.delete",
			"external_trigger.manage_acl",
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
