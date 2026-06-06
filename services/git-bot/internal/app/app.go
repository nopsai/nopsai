package app

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/proxyhttp"
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
	writeGitHubPrivateKeyFile(cfg)

	appID, err := strconv.ParseInt(cfg.GitHubAppID, 10, 64)
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid GitHub App ID in configuration")
	}
	installationID, err := strconv.ParseInt(cfg.GitHubInstallID, 10, 64)
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid GitHub Installation ID in configuration")
	}

	if cfg.GitHubPrivateKeyPath == "" {
		log.Fatal().Msg("github_private_key_path must be set in the configuration.")
	}

	log.Info().Msgf("Loading GitHub private key from file path: %s", cfg.GitHubPrivateKeyPath)
	privateKeyBytes, err := os.ReadFile(cfg.GitHubPrivateKeyPath)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to read private key from path: %s", cfg.GitHubPrivateKeyPath)
	}

	itr, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appID, privateKeyBytes)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create GitHub App transport")
	}

	installationTransport := ghinstallation.NewFromAppsTransport(itr, installationID)
	githubHTTPClient := &http.Client{
		Transport: installationTransport,
		Timeout:   15 * time.Second,
	}
	ghClient := github.NewClient(githubHTTPClient)
	httpClient := proxyhttp.NewInternalAwareClient(10 * time.Second)

	gitBot := service.NewGitBotApp(cfg, ghClient, httpClient, appID)
	server := httpapi.NewServer(cfg.GitBotListenAddress, gitBot.Handler())
	log.Info().Msgf("Nopsai Git Bot server listening on %s", cfg.GitBotListenAddress)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}

func writeGitHubPrivateKeyFile(cfg *config.Config) {
	if cfg.GitHubPrivateKey == "" {
		return
	}

	correctedKey := strings.ReplaceAll(cfg.GitHubPrivateKey, "\n", "\n")
	if err := os.MkdirAll(filepath.Dir(cfg.GitHubPrivateKeyPath), 0700); err != nil {
		log.Fatal().Err(err).Msgf("Failed to create directory for private key: %s", cfg.GitHubPrivateKeyPath)
	}

	log.Info().Msgf("Writing GITHUB_PRIVATE_KEY to file: %s", cfg.GitHubPrivateKeyPath)
	if err := os.WriteFile(cfg.GitHubPrivateKeyPath, []byte(correctedKey), 0600); err != nil {
		log.Fatal().Err(err).Msgf("Failed to write private key to file: %s", cfg.GitHubPrivateKeyPath)
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
