package app

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/proto"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicetls"
	service "nopsai/services/nopsai"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

func Run() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yml"
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to load config from %s", configPath)
	}

	applyConfigDefaults(cfg)
	configureLogging(cfg)

	envFilePath := os.Getenv("ENV_FILE_PATH")
	if envFilePath == "" {
		envFilePath = filepath.Join(filepath.Dir(configPath), ".env")
	}

	hardConfigMissing := strings.TrimSpace(cfg.MasterKey) == "" ||
		strings.TrimSpace(cfg.JWTSigningKey) == "" ||
		service.HasBlockingEnterpriseStartupGates(cfg)
	dbAttempts := 5
	if hardConfigMissing {
		dbAttempts = 1
	}
	dbpool, dbErr := service.ConnectDatabaseWithRetries(context.Background(), cfg.DatabaseURL, dbAttempts, 3*time.Second)
	if hardConfigMissing || dbErr != nil {
		service.RunSetupPreflightOnlyServer(cfg, configPath, envFilePath, dbpool, dbErr)
		if dbpool != nil {
			dbpool.Close()
		}
		return
	}
	if service.HasBlockingEnterpriseDatabaseGates(context.Background(), cfg, dbpool) {
		service.RunSetupPreflightOnlyServer(cfg, configPath, envFilePath, dbpool, nil)
		if dbpool != nil {
			dbpool.Close()
		}
		return
	}
	defer dbpool.Close()

	if err := service.EnsureDatabaseBootstrapForConfig(context.Background(), dbpool, cfg); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database bootstrap")
	}

	dispatcherConn, err := newDispatcherConnection(cfg)
	if err != nil {
		log.Fatal().Err(err).Str("addr", dispatcherAddress(cfg)).Msg("Failed to connect to dispatcher")
	}
	defer dispatcherConn.Close()

	serviceAuthenticator, err := newServiceAuthenticator(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to configure service HTTP authentication")
	}
	serviceCredentials, err := newNopsaiServiceCredentials(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to configure service HTTP credentials")
	}

	api, err := service.NewApp(context.Background(), service.AppOptions{
		Config:               cfg,
		Database:             dbpool,
		Dispatcher:           service.NewDispatcherClient(proto.NewDispatcherServiceClient(dispatcherConn)),
		ServiceAuthenticator: serviceAuthenticator,
		ServiceCredentials:   serviceCredentials,
		ConfigPath:           configPath,
		EnvFilePath:          envFilePath,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize NopsAI service")
	}

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	api.StartBackgroundWorkers(workerCtx)

	server := httpapi.NewServer(cfg.NopsaiListenAddress, api.Handler())
	serveUntilSignal(server, cfg.NopsaiListenAddress)
}

func applyConfigDefaults(cfg *config.Config) {
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
	if strings.TrimSpace(cfg.NopsaiListenAddress) == "" {
		cfg.NopsaiListenAddress = "0.0.0.0:8080"
	}
}

func configureLogging(cfg *config.Config) {
	logLevel, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.Warn().Msgf("Invalid log level '%s', defaulting to 'info'", cfg.LogLevel)
		logLevel = zerolog.InfoLevel
	}
	if cfg.LogFormat == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})
	}
	zerolog.SetGlobalLevel(logLevel)
}

func newDispatcherConnection(cfg *config.Config) (*grpc.ClientConn, error) {
	dispatcherCreds, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: cfg.EffectiveServiceJWTSigningKey(),
		Issuer:     cfg.EffectiveServiceJWTIssuer(),
		Audience:   cfg.EffectiveServiceJWTAudience(),
		Role:       serviceauth.RoleNopsai,
		ServiceID:  cfg.EffectiveNopsaiServiceID(),
	})
	if err != nil {
		return nil, err
	}
	dispatcherTransportCreds, err := servicetls.ClientCredentials(servicetls.Config{
		Mode:       cfg.EffectiveDispatcherTLSMode(),
		Secret:     cfg.EffectiveDispatcherTLSSecret(),
		Role:       serviceauth.RoleNopsai,
		ServiceID:  cfg.EffectiveNopsaiServiceID(),
		ServerName: cfg.EffectiveDispatcherTLSServerName(),
	})
	if err != nil {
		return nil, err
	}
	return grpc.Dial(
		dispatcherAddress(cfg),
		grpc.WithTransportCredentials(dispatcherTransportCreds),
		grpc.WithPerRPCCredentials(dispatcherCreds),
	)
}

func newServiceAuthenticator(cfg *config.Config) (*serviceauth.Authenticator, error) {
	return serviceauth.NewAuthenticator(serviceauth.Config{
		SigningKey: cfg.EffectiveServiceJWTSigningKey(),
		Issuer:     cfg.EffectiveServiceJWTIssuer(),
		Audience:   cfg.EffectiveServiceJWTAudience(),
	})
}

func newNopsaiServiceCredentials(cfg *config.Config) (*serviceauth.Credentials, error) {
	return serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: cfg.EffectiveServiceJWTSigningKey(),
		Issuer:     cfg.EffectiveServiceJWTIssuer(),
		Audience:   cfg.EffectiveServiceJWTAudience(),
		Role:       serviceauth.RoleNopsai,
		ServiceID:  cfg.EffectiveNopsaiServiceID(),
	})
}

func dispatcherAddress(cfg *config.Config) string {
	dispatcherAddr := strings.TrimSpace(cfg.DispatcherAddress)
	if dispatcherAddr == "" {
		return "localhost:9090"
	}
	return dispatcherAddr
}

func serveUntilSignal(server *http.Server, listenAddr string) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	go func() {
		log.Info().Msgf("Nopsai API server listening on %s", listenAddr)
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

	log.Info().Msg("Server exiting")
}
