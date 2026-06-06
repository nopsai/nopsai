package nopsai

import (
	"net/http"
	"sync"
	"time"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/pkg/serviceauth"

	"github.com/jackc/pgx/v5/pgxpool"

	"nopsai/services/nopsai/pkg/audit"
	"nopsai/services/nopsai/pkg/auth"
	"nopsai/services/nopsai/pkg/authz"
	"nopsai/services/nopsai/pkg/store"
)

const (
	defaultAdminSub          = "admin"
	defaultAdminEmail        = "admin@example.com"
	defaultAdminRole         = "nopsai-admin"
	defaultAdminPasswordHash = "$2a$10$ueFOcGRKCWDeOaTwy1hmQ.WjQ70Yu8JJLcl8ZvJprx7HPKArt8ESC" // password: admin
	defaultAdminID           = "00000000-0000-0000-0000-00000000000a"
	dockerContainerNameMax   = 255
)

// WebSocket Hub implementation

type App struct {
	db           *pgxpool.Pool
	cfg          *config.Config
	dispatcher   DispatcherClient
	encKey       []byte
	httpClient   *http.Client
	gitProvider  GitProvider
	runLauncher  RunLauncher
	configSync   ConfigSyncStore
	secretCrypto SecretCodec
	store        store.Store
	configPath   string
	cfgMu        sync.RWMutex
	idleTimeout  time.Duration

	configSyncMu     sync.Mutex
	configSyncStatus ConfigSyncStatus
	envFilePath      string

	authService   *auth.Service
	serviceAuth   *serviceauth.Authenticator
	aaaClient     AAAClient
	aaaLocal      AAAClient
	aaaRemoteMu   sync.Mutex
	aaaRetryAfter time.Time
	authz         *authz.Enforcer
	auditLogger   *audit.Logger
	tokenActivity sync.Map

	runnerBootstrapMu     sync.Mutex
	runnerBootstrapTokens map[string]runnerBootstrapToken
}

type LogLine = models.LogLine
type RunListItem = models.RunListItem
type StepConfiguration = models.StepConfiguration
type StepDetail = models.StepDetail
type TaskDetail = models.TaskDetail
type ParentRunInfo = models.ParentRunInfo
type RunDetail = models.RunDetail
type StepStatusUpdate = models.StepStatusUpdate
type SecretRequest = models.SecretRequest
type VariableRequest = models.VariableRequest
type ScopeResponse = models.ScopeResponse
type VariableValueResponse = models.VariableValueResponse
type PipelineRequest = models.PipelineRequest
type TriggerOverrideRequest = models.TriggerOverrideRequest
type FinalizeRequest = models.FinalizeRequest
type Group = models.Group
