package main

import (
	"context"
	"crypto/sha256"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	"nopsai/pkg/proxyhttp"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicetls"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	aaaauthz "nopsai/services/aaa/pkg/authz"
	aaastore "nopsai/services/aaa/pkg/store"
	"nopsai/services/nopsai/pkg/aaaclient"
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
	db          *pgxpool.Pool
	cfg         *config.Config
	dispatcher  proto.DispatcherServiceClient
	encKey      []byte
	httpClient  *http.Client
	store       store.Store
	configPath  string
	cfgMu       sync.RWMutex
	idleTimeout time.Duration

	configSyncMu     sync.Mutex
	configSyncStatus ConfigSyncStatus
	envFilePath      string

	authService   *auth.Service
	serviceAuth   *serviceauth.Authenticator
	aaaClient     aaaAuthorizer
	aaaLocal      aaaAuthorizer
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

func main() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yml"
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to load config from %s", configPath)
	}

	if cfg.JWTExpiryMinutes == 0 {
		cfg.JWTExpiryMinutes = 60
	}
	if cfg.IdleTimeoutMinutes == 0 {
		cfg.IdleTimeoutMinutes = 30
	}
	if cfg.RefreshTokenTTLMinutes == 0 {
		cfg.RefreshTokenTTLMinutes = 60 * 24 * 30
	}
	if cfg.RateLimitLoginPerMinute == 0 {
		cfg.RateLimitLoginPerMinute = 10
	}
	if cfg.LoginLockoutThreshold == 0 {
		cfg.LoginLockoutThreshold = 5
	}
	if cfg.LoginLockoutWindowMin == 0 {
		cfg.LoginLockoutWindowMin = 15
	}
	if !cfg.AuthProviderLocalEnabled {
		cfg.AuthProviderLocalEnabled = true
	}

	logLevel, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.Warn().Msgf("Invalid log level '%s', defaulting to 'info'", cfg.LogLevel)
		logLevel = zerolog.InfoLevel
	}
	if cfg.LogFormat == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})
	}
	zerolog.SetGlobalLevel(logLevel)

	envFilePath := os.Getenv("ENV_FILE_PATH")
	if envFilePath == "" {
		envFilePath = filepath.Join(filepath.Dir(configPath), ".env")
	}

	if strings.TrimSpace(cfg.NopsaiListenAddress) == "" {
		cfg.NopsaiListenAddress = "0.0.0.0:8080"
	}

	hardConfigMissing := strings.TrimSpace(cfg.MasterKey) == "" || strings.TrimSpace(cfg.JWTSigningKey) == ""
	dbAttempts := 5
	if hardConfigMissing {
		dbAttempts = 1
	}
	dbpool, dbErr := connectDatabaseWithRetries(context.Background(), cfg.DatabaseURL, dbAttempts, 3*time.Second)
	if hardConfigMissing || dbErr != nil {
		runSetupPreflightOnlyServer(cfg, configPath, envFilePath, dbpool, dbErr)
		if dbpool != nil {
			dbpool.Close()
		}
		return
	}
	defer dbpool.Close()

	key := sha256.Sum256([]byte(cfg.MasterKey))

	if err := ensureDefaultAdmin(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure default admin")
	}
	if err := ensureAuthSchema(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure auth schema")
	}
	if err := ensureGroupSchema(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure group schema")
	}
	if err := ensureProductAccessBootstrap(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure product access roles")
	}
	if err := ensureConfigRepositorySchema(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure config repository schema")
	}
	if err := ensureKnowledgeContextSchema(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure knowledge context schema")
	}
	if err := ensureResourceAuthorizationSchema(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure resource authorization schema")
	}
	if err := ensureExternalTriggerSchema(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure external trigger schema")
	}
	if err := ensureScheduleSchema(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure schedule schema")
	}
	if err := ensureLLMProfileSchema(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure LLM profile schema")
	}
	if err := ensureMCPSchema(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure MCP schema")
	}
	if err := ensureSetupSchema(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure setup schema")
	}
	if err := ensureApprovalSchema(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure approval schema")
	}
	if err := ensureNotificationSchema(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure notification schema")
	}
	if err := ensureDataManagementSchema(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure data management schema")
	}

	dispatcherAddr := strings.TrimSpace(cfg.DispatcherAddress)
	if dispatcherAddr == "" {
		dispatcherAddr = "localhost:9090"
	}
	dispatcherCreds, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: cfg.EffectiveServiceJWTSigningKey(),
		Issuer:     cfg.EffectiveServiceJWTIssuer(),
		Audience:   cfg.EffectiveServiceJWTAudience(),
		Role:       serviceauth.RoleNopsai,
		ServiceID:  cfg.EffectiveNopsaiServiceID(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to configure dispatcher client authentication")
	}
	dispatcherTransportCreds, err := servicetls.ClientCredentials(servicetls.Config{
		Mode:       cfg.EffectiveDispatcherTLSMode(),
		Secret:     cfg.EffectiveDispatcherTLSSecret(),
		Role:       serviceauth.RoleNopsai,
		ServiceID:  cfg.EffectiveNopsaiServiceID(),
		ServerName: cfg.EffectiveDispatcherTLSServerName(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to configure dispatcher transport security")
	}
	serviceAuthenticator, err := serviceauth.NewAuthenticator(serviceauth.Config{
		SigningKey: cfg.EffectiveServiceJWTSigningKey(),
		Issuer:     cfg.EffectiveServiceJWTIssuer(),
		Audience:   cfg.EffectiveServiceJWTAudience(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to configure service HTTP authentication")
	}
	dispatcherConn, err := grpc.Dial(
		dispatcherAddr,
		grpc.WithTransportCredentials(dispatcherTransportCreds),
		grpc.WithPerRPCCredentials(dispatcherCreds),
	)
	if err != nil {
		log.Fatal().Err(err).Str("addr", dispatcherAddr).Msg("Failed to connect to dispatcher")
	}
	defer dispatcherConn.Close()

	authCfg := auth.Config{
		LocalEnabled:       cfg.AuthProviderLocalEnabled,
		SigningKey:         cfg.JWTSigningKey,
		JWTIssuer:          cfg.JWTIssuer,
		JWTAudience:        cfg.JWTAudience,
		AccessTTL:          time.Duration(cfg.JWTExpiryMinutes) * time.Minute,
		RefreshTTL:         time.Duration(cfg.RefreshTokenTTLMinutes) * time.Minute,
		LoginRateLimit:     cfg.RateLimitLoginPerMinute,
		LoginLockoutThresh: cfg.LoginLockoutThreshold,
		LoginLockoutWindow: time.Duration(cfg.LoginLockoutWindowMin) * time.Minute,
	}

	authService, err := auth.NewService(context.Background(), dbpool, authCfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize authentication service")
	}
	authzEnforcer, err := authz.NewEnforcer(context.Background(), dbpool)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load RBAC policies")
	}
	aaaClient := aaaclient.New(cfg.AAAAPIURL, cfg.AAASharedToken)
	aaaLocal := aaaauthz.NewEvaluator(aaastore.NewPGStore(dbpool))
	auditLogger := audit.NewLogger(dbpool)

	app := &App{
		db:          dbpool,
		cfg:         cfg,
		dispatcher:  proto.NewDispatcherServiceClient(dispatcherConn),
		encKey:      key[:],
		httpClient:  proxyhttp.NewInternalAwareClient(10 * time.Second),
		store:       store.NewPGStore(dbpool),
		configPath:  configPath,
		envFilePath: envFilePath,
		authService: authService,
		serviceAuth: serviceAuthenticator,
		aaaClient:   aaaClient,
		aaaLocal:    aaaLocal,
		authz:       authzEnforcer,
		auditLogger: auditLogger,
		configSyncStatus: ConfigSyncStatus{
			Status:  "idle",
			Message: "No configuration sync has been requested yet.",
		},
		idleTimeout: time.Duration(cfg.IdleTimeoutMinutes) * time.Minute,
	}
	if err := app.loadOrSeedLLMProfilesConfig(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("Failed to load LLM profiles")
	}
	if err := app.loadOrSeedMCPRegistryConfig(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("Failed to load MCP registry")
	}
	scheduleWorkerCtx, stopScheduleWorker := context.WithCancel(context.Background())
	defer stopScheduleWorker()
	go app.runScheduleWorker(scheduleWorkerCtx)
	cleanupWorkerCtx, stopCleanupWorker := context.WithCancel(context.Background())
	defer stopCleanupWorker()
	go app.runDataCleanupScheduleWorker(cleanupWorkerCtx)

	handler := app.buildHTTPHandler()

	server := &http.Server{
		Addr:    cfg.NopsaiListenAddress,
		Handler: handler,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info().Msgf("Nopsai API server listening on %s", cfg.NopsaiListenAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	<-stop

	log.Info().Msg("Shutting down the server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	app.db.Close()
	log.Info().Msg("Server exiting")
}
