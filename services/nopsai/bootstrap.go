package nopsai

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	"nopsai/config"
	"nopsai/pkg/serviceauth"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/internal/systemlogs"

	"github.com/jackc/pgx/v5/pgxpool"

	"nopsai/services/nopsai/pkg/store"
)

type AppOptions struct {
	Config               *config.Config
	Database             *pgxpool.Pool
	Dispatcher           DispatcherClient
	ServiceAuthenticator *serviceauth.Authenticator
	ServiceCredentials   *serviceauth.Credentials
	GitProvider          GitProvider
	RunLauncher          RunLauncher
	ConfigSyncStore      ConfigSyncStore
	SecretCodec          SecretCodec
	CredentialResolver   CredentialResolver
	AAAClient            AAAClient
	LocalAAAClient       AAAClient
	ConfigPath           string
	EnvFilePath          string
	SystemLogProvider    systemlogs.Provider
}

func NewApp(ctx context.Context, options AppOptions) (*App, error) {
	if options.Config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if options.Database == nil {
		return nil, fmt.Errorf("database pool is required")
	}
	if options.Dispatcher == nil {
		return nil, fmt.Errorf("dispatcher client is required")
	}
	if options.ServiceAuthenticator == nil {
		return nil, fmt.Errorf("service authenticator is required")
	}
	if err := ApplyPersistedRuntimeSettings(ctx, options.Database, options.Config); err != nil {
		return nil, fmt.Errorf("load runtime settings: %w", err)
	}

	key := sha256.Sum256([]byte(options.Config.MasterKey))
	security, err := newAppSecurityRuntime(ctx, options)
	if err != nil {
		return nil, err
	}

	pgStore := store.NewPGStore(options.Database)
	credentialCodec, err := credentials.NewEnvelopeCodec(options.Config.MasterKey)
	if err != nil {
		return nil, fmt.Errorf("initialize credential encryption: %w", err)
	}
	credentialService, err := newCredentialService(pgStore, credentialCodec, security.auditLogger)
	if err != nil {
		return nil, fmt.Errorf("initialize credential service: %w", err)
	}
	systemLogBroker, err := newSystemLogBroker(options.Config, options.SystemLogProvider)
	if err != nil {
		return nil, fmt.Errorf("initialize system logs: %w", err)
	}

	app := &App{
		db:                 options.Database,
		cfg:                options.Config,
		dispatcher:         options.Dispatcher,
		encKey:             key[:],
		httpClient:         security.internalHTTP,
		gitProvider:        security.gitProvider,
		runLauncher:        options.RunLauncher,
		configSync:         options.ConfigSyncStore,
		secretCrypto:       options.SecretCodec,
		store:              pgStore,
		credentialStore:    pgStore,
		credentialResolver: credentialService,
		credentials:        credentialService,
		configPath:         options.ConfigPath,
		envFilePath:        options.EnvFilePath,
		authService:        security.authService,
		serviceAuth:        options.ServiceAuthenticator,
		serviceCredentials: options.ServiceCredentials,
		aaaClient:          security.aaaClient,
		aaaLocal:           security.localAAA,
		authz:              security.authz,
		auditLogger:        security.auditLogger,
		systemLogs:         systemLogBroker,
		systemLogLimiter:   newSystemLogRateLimiter(30, time.Minute),
		configSyncStatus: ConfigSyncStatus{
			Status:  "idle",
			Message: "No configuration sync has been requested yet.",
		},
		idleTimeout: time.Duration(options.Config.IdleTimeoutMinutes) * time.Minute,
	}
	if app.runLauncher == nil {
		app.runLauncher = appRunLauncher{app: app}
	}
	if app.configSync == nil {
		app.configSync = appConfigSyncStore{app: app}
	}
	if app.secretCrypto == nil {
		app.secretCrypto = aesGCMSecretCodec{key: key[:]}
	}
	if options.CredentialResolver != nil {
		app.credentialResolver = options.CredentialResolver
	}
	if err := app.loadOrSeedLLMProfilesConfig(ctx); err != nil {
		return nil, fmt.Errorf("load LLM profiles: %w", err)
	}
	if err := app.loadOrSeedMCPRegistryConfig(ctx); err != nil {
		return nil, fmt.Errorf("load MCP registry: %w", err)
	}
	return app, nil
}

func ConnectDatabaseWithRetries(ctx context.Context, databaseURL string, attempts int, delay time.Duration) (*pgxpool.Pool, error) {
	return connectDatabaseWithRetries(ctx, databaseURL, attempts, delay)
}

func RunSetupPreflightOnlyServer(cfg *config.Config, configPath, envFilePath string, db *pgxpool.Pool, dbErr error) {
	runSetupPreflightOnlyServer(cfg, configPath, envFilePath, db, dbErr)
}

func RunSetupPreflightUntilDatabaseReadyServer(cfg *config.Config, configPath, envFilePath string, dbErr error, retryDelay time.Duration) (*pgxpool.Pool, bool) {
	return runSetupPreflightUntilDatabaseReadyServer(cfg, configPath, envFilePath, dbErr, retryDelay)
}

func EnsureDatabaseBootstrap(ctx context.Context, db *pgxpool.Pool) error {
	return EnsureDatabaseBootstrapForConfig(ctx, db, nil)
}

func EnsureDatabaseBootstrapForConfig(ctx context.Context, db *pgxpool.Pool, cfg *config.Config) error {
	for _, step := range databaseBootstrapSteps(cfg) {
		if err := step.run(ctx, db); err != nil {
			return fmt.Errorf("ensure %s: %w", step.name, err)
		}
	}
	return nil
}

func (a *App) Handler() http.Handler {
	return a.buildHTTPHandler()
}

func (a *App) StartBackgroundWorkers(ctx context.Context) {
	go a.runPendingRunRecoveryWorker(ctx)
	go a.runScheduleWorker(ctx)
	go a.runDashboardRefreshScheduleWorker(ctx)
	go a.runDataCleanupScheduleWorker(ctx)
	go a.runOIDCEntitlementSyncWorker(ctx)
	go a.runKnowledgeContextPeriodicSyncWorker(ctx)
}
