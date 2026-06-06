package nopsai

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"nopsai/pkg/models"
)

const (
	setupStateKeyCompletedAt = "completed_at"
	setupStateKeyProfile     = "profile"

	setupProfileDev        = "dev"
	setupProfileTeam       = "team"
	setupProfileProduction = "production"
	setupProfileEmpty      = "empty"
)

type setupStarterProfile struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type setupCheck struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
	Blocking bool   `json:"blocking"`
}

type setupCounts struct {
	Users              int `json:"users"`
	Pipelines          int `json:"pipelines"`
	Steps              int `json:"steps"`
	Triggers           int `json:"triggers"`
	Groups             int `json:"groups"`
	AccessGrants       int `json:"access_grants"`
	LLMProfiles        int `json:"llm_profiles"`
	MCPServers         int `json:"mcp_servers"`
	MCPProfiles        int `json:"mcp_profiles"`
	KnowledgeContexts  int `json:"knowledge_contexts"`
	ConfigRepositories int `json:"config_repositories"`
}

type setupGitHubInfo struct {
	WebhookURL                 string            `json:"webhook_url"`
	GitBotServiceURL           string            `json:"git_bot_service_url,omitempty"`
	NopsaiAPIURL               string            `json:"nopsai_api_url,omitempty"`
	RequiredEvents             []string          `json:"required_events"`
	RequiredPermissions        map[string]string `json:"required_permissions"`
	AppIDConfigured            bool              `json:"app_id_configured"`
	InstallationIDConfigured   bool              `json:"installation_id_configured"`
	PrivateKeyConfigured       bool              `json:"private_key_configured"`
	WebhookSecretConfigured    bool              `json:"webhook_secret_configured"`
	GitBotURLConfigured        bool              `json:"git_bot_url_configured"`
	NopsaiForwardURLConfigured bool              `json:"nopsai_forward_url_configured"`
}

type setupStatusResponse struct {
	Completed        bool                     `json:"completed"`
	CompletedAt      string                   `json:"completed_at,omitempty"`
	Profile          string                   `json:"profile,omitempty"`
	RuntimeEnv       string                   `json:"runtime_env,omitempty"`
	EnvFilePath      string                   `json:"env_file_path,omitempty"`
	Counts           setupCounts              `json:"counts"`
	Checks           []setupCheck             `json:"checks"`
	StarterProfiles  []setupStarterProfile    `json:"starter_profiles"`
	GitHub           setupGitHubInfo          `json:"github"`
	GlobalConfigRepo *models.ConfigRepository `json:"global_config_repo,omitempty"`
}

type setupConfigRepositoryInput struct {
	RepoURL  string `json:"repo_url"`
	Branch   string `json:"branch"`
	BasePath string `json:"base_path"`
	Enabled  *bool  `json:"enabled"`
}

type setupLLMProfileInput struct {
	Name          string   `json:"name"`
	Provider      string   `json:"provider"`
	Model         string   `json:"model"`
	BaseURL       string   `json:"base_url"`
	APIKeySecret  string   `json:"api_key_secret"`
	APIKeyValue   string   `json:"api_key_value"`
	AllowedScopes []string `json:"allowed_scopes"`
}

type setupRepositoryGroupInput struct {
	Name         string   `json:"name"`
	Repositories []string `json:"repositories"`
}

type setupUserInput struct {
	Sub      string `json:"sub"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Password string `json:"password"`
	Group    string `json:"group"`
}

type setupBootstrapRequest struct {
	Profile                string                      `json:"profile"`
	GenerateSecrets        bool                        `json:"generate_secrets"`
	SeedStarterDatabase    bool                        `json:"seed_starter_database"`
	SeedLLMProfile         *bool                       `json:"seed_llm_profile"`
	MCPExamples            bool                        `json:"mcp_examples"`
	ProductionAcknowledged bool                        `json:"production_acknowledged"`
	SyncConfigRepository   bool                        `json:"sync_config_repository"`
	ConfigRepository       *setupConfigRepositoryInput `json:"config_repository"`
	RepositoryGroups       []setupRepositoryGroupInput `json:"repository_groups"`
	Repositories           []string                    `json:"repositories"`
	LLMProfile             setupLLMProfileInput        `json:"llm_profile"`
	Users                  []setupUserInput            `json:"users"`
}

type setupTemporaryCredential struct {
	Sub               string `json:"sub"`
	Email             string `json:"email,omitempty"`
	TemporaryPassword string `json:"temporary_password,omitempty"`
	Role              string `json:"role,omitempty"`
}

type setupBootstrapResponse struct {
	Status               setupStatusResponse        `json:"status"`
	Details              map[string]int             `json:"details,omitempty"`
	GeneratedSecrets     []string                   `json:"generated_secrets,omitempty"`
	RequiresRestart      bool                       `json:"requires_restart,omitempty"`
	TemporaryCredentials []setupTemporaryCredential `json:"temporary_credentials,omitempty"`
	Messages             []string                   `json:"messages,omitempty"`
	Warnings             []string                   `json:"warnings,omitempty"`
}

type setupTemplatesResponse struct {
	Profile string            `json:"profile"`
	Files   map[string]string `json:"files"`
}

type setupTemplateOptions struct {
	RepositoryGroups []setupRepositoryGroupInput
	Users            []setupUserInput
	IncludeLLM       bool
	IncludeMCP       bool
	LLMProfile       setupLLMProfileInput
}

func (req setupBootstrapRequest) shouldSeedLLMProfile() bool {
	return req.SeedLLMProfile == nil || *req.SeedLLMProfile
}

func ensureSetupSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS setup_state (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}
