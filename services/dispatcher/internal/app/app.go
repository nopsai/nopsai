package app

import (
	"net"
	"os"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/proto"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicetls"
	"nopsai/services/dispatcher/internal/service"

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
		log.Fatal().Err(err).Msg("failed to load config")
	}

	configureLogging(cfg)

	listenAddr := cfg.DispatcherListenAddress
	if strings.TrimSpace(listenAddr) == "" {
		listenAddr = ":9090"
	}

	nopsaiBase := strings.TrimSpace(cfg.AgentNopsaiAPIURL)
	if nopsaiBase == "" {
		log.Fatal().Msg("Agent Nopsai API URL (agent_nopsai_api_url) must be configured for dispatcher")
	}

	internalCredentials, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: cfg.EffectiveServiceJWTSigningKey(),
		Issuer:     cfg.EffectiveServiceJWTIssuer(),
		Audience:   cfg.EffectiveServiceJWTAudience(),
		Role:       serviceauth.RoleDispatcher,
		ServiceID:  "dispatcher",
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to configure dispatcher internal HTTP authentication")
	}
	dispatcher := service.NewDispatcherServer(cfg.DispatcherRouting, nopsaiBase, internalCredentials)

	serviceAuthenticator, err := serviceauth.NewAuthenticator(serviceauth.Config{
		SigningKey: cfg.EffectiveServiceJWTSigningKey(),
		Issuer:     cfg.EffectiveServiceJWTIssuer(),
		Audience:   cfg.EffectiveServiceJWTAudience(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to configure dispatcher service authentication")
	}
	dispatcherAuth := service.NewDispatcherAuth(serviceAuthenticator, map[string]string{
		serviceauth.RoleNopsai: cfg.EffectiveNopsaiServiceID(),
		serviceauth.RoleRunner: cfg.EffectiveRunnerServiceID(),
		serviceauth.RoleAgent:  cfg.EffectiveAgentServiceID(),
	})

	dispatcherTransportCreds, err := servicetls.ServerCredentials(servicetls.Config{
		Mode:        cfg.EffectiveDispatcherTLSMode(),
		Secret:      cfg.EffectiveDispatcherTLSSecret(),
		ServerName:  cfg.EffectiveDispatcherTLSServerName(),
		ServerNames: dispatcherTLSServerNames(cfg, listenAddr),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to configure dispatcher transport security")
	}

	serverOptions := []grpc.ServerOption{
		grpc.UnaryInterceptor(dispatcherAuth.UnaryInterceptor),
		grpc.StreamInterceptor(dispatcherAuth.StreamInterceptor),
	}
	if dispatcherTransportCreds != nil {
		serverOptions = append(serverOptions, grpc.Creds(dispatcherTransportCreds))
	}
	grpcServer := grpc.NewServer(serverOptions...)
	proto.RegisterDispatcherServiceServer(grpcServer, dispatcher)

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatal().Err(err).Str("addr", listenAddr).Msg("failed to listen")
	}

	stop := make(chan struct{})
	go dispatcher.ReapStaleRunners(10*time.Second, 30*time.Second, stop)
	go dispatcher.SyncRoutingLoop(5*time.Second, stop)

	log.Info().
		Str("addr", listenAddr).
		Str("tls_mode", servicetls.NormalizeMode(cfg.EffectiveDispatcherTLSMode())).
		Msg("dispatcher listening")
	if err := grpcServer.Serve(lis); err != nil {
		close(stop)
		log.Fatal().Err(err).Msg("dispatcher server failed")
	}
}

func configureLogging(cfg *config.Config) {
	logLevel, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		logLevel = zerolog.InfoLevel
	}
	if cfg.LogFormat == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})
	}
	zerolog.SetGlobalLevel(logLevel)
}

func dispatcherTLSServerNames(cfg *config.Config, listenAddr string) []string {
	names := []string{"dispatcher", "localhost", listenAddr}
	if cfg != nil {
		names = append(names,
			cfg.EffectiveDispatcherTLSServerName(),
			cfg.DispatcherAddress,
			cfg.DispatcherListenAddress,
		)
	}
	if hostname, err := os.Hostname(); err == nil {
		names = append(names, hostname)
	}
	return names
}
