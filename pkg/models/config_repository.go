package models

import "time"

const (
	ConfigRepositoryScopeTeam      = "team"
	ConfigRepositoryScopeSystem    = "system"
	ConfigRepositorySystemGlobalID = "global"
)

type ConfigRepository struct {
	ID                    int64      `json:"id"`
	ScopeType             string     `json:"scope_type"`
	ScopeID               string     `json:"scope_id"`
	RepoURL               string     `json:"repo_url"`
	Branch                string     `json:"branch"`
	BasePath              string     `json:"base_path"`
	Enabled               bool       `json:"enabled"`
	WriteEnabled          bool       `json:"write_enabled"`
	WriteBranch           string     `json:"write_branch"`
	ConfigRepoID          *int64     `json:"config_repo_id,omitempty"`
	ConfigSourcePath      string     `json:"config_source_path,omitempty"`
	ConfigSourceCommitSHA string     `json:"config_source_commit_sha,omitempty"`
	ManagedByConfigRepo   bool       `json:"managed_by_config_repo"`
	LastSyncStatus        string     `json:"last_sync_status"`
	LastSyncMessage       string     `json:"last_sync_message"`
	LastSyncStartedAt     *time.Time `json:"last_sync_started_at,omitempty"`
	LastSyncCompletedAt   *time.Time `json:"last_sync_completed_at,omitempty"`
	LastSyncCommitSHA     string     `json:"last_sync_commit_sha"`
	CreatedBy             string     `json:"created_by"`
	UpdatedBy             string     `json:"updated_by"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type ConfigRepositoryInput struct {
	ScopeType    string
	ScopeID      string
	RepoURL      string
	Branch       string
	BasePath     string
	Enabled      bool
	WriteEnabled bool
	WriteBranch  string
	Actor        string
}

type ConfigRepositoryFilter struct {
	ScopeType string
	ScopeID   string
	Enabled   *bool
}
