package app

import (
	"context"
	"net/http"
	"os"
	"time"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/proxyhttp"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicelog"
	"nopsai/pkg/startupgates"
	"nopsai/services/git-bot/internal/service"

	"github.com/rs/zerolog/log"
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

	configureLogging(cfg)
	if err := startupgates.ValidateGitBot(cfg); err != nil {
		log.Fatal().Err(err).Msg("git-bot startup gates failed")
	}
	serviceCredentials, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: cfg.EffectiveServiceJWTSigningKey(),
		Issuer:     cfg.EffectiveServiceJWTIssuer(),
		Audience:   cfg.EffectiveServiceJWTAudience(),
		Role:       serviceauth.RoleGitBot,
		ServiceID:  cfg.EffectiveGitBotServiceID(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to configure git-bot service credentials")
	}
	serviceAuthenticator, err := serviceauth.NewAuthenticator(serviceauth.Config{
		SigningKey: cfg.EffectiveServiceJWTSigningKey(),
		Issuer:     cfg.EffectiveServiceJWTIssuer(),
		Audience:   cfg.EffectiveServiceJWTAudience(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to configure git-bot service authentication")
	}
	httpClient := proxyhttp.NewInternalAwareClient(10 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	githubRuntime := newGitHubRuntime(cfg, httpClient, serviceCredentials)
	if err := githubRuntime.refresh(ctx); err != nil {
		log.Warn().Err(err).Msg("GitHub App credentials are not available; git-bot starts in degraded mode and keeps retrying")
	}
	go githubRuntime.watch(ctx, gitHubCredentialRefreshInterval)

	gitBot := service.NewGitBotAppWithCredentials(
		cfg,
		githubRuntime.resolver,
		httpClient,
		githubRuntime,
		serviceAuthenticator,
		serviceCredentials,
	)

	server := httpapi.NewServer(cfg.GitBotListenAddress, gitBot.Handler())
	log.Info().Msgf("Nopsai Git Bot server listening on %s", cfg.GitBotListenAddress)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}

func configureLogging(cfg *config.Config) {
	if err := servicelog.Configure(cfg.LogLevel, cfg.LogFormat); err != nil {
		log.Warn().Msgf("Invalid log level '%s', defaulting to 'info'", cfg.LogLevel)
	}
}
