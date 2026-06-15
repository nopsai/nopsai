package app

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/proxyhttp"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/startupgates"
	"nopsai/services/git-bot/internal/service"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v53/github"
	"github.com/rs/zerolog"
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
	bootstrap, err := fetchGitHubBootstrap(context.Background(), cfg, httpClient, serviceCredentials)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to retrieve GitHub credentials from nopsai")
	}

	appID, err := strconv.ParseInt(bootstrap.GitHubAppID, 10, 64)
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid GitHub App ID in configuration")
	}
	installationID, err := strconv.ParseInt(bootstrap.GitHubInstallationID, 10, 64)
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid GitHub Installation ID in configuration")
	}

	itr, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appID, []byte(bootstrap.GitHubPrivateKey))
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create GitHub App transport")
	}

	installationTransport := ghinstallation.NewFromAppsTransport(itr, installationID)
	githubHTTPClient := &http.Client{
		Transport: installationTransport,
		Timeout:   15 * time.Second,
	}
	ghClient := github.NewClient(githubHTTPClient)
	gitBot := service.NewGitBotApp(
		cfg,
		ghClient,
		httpClient,
		appID,
		bootstrap.GitHubWebhookSecret,
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
