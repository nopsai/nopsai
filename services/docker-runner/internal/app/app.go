package app

import (
	"os"
	"strings"

	"nopsai/config"
	"nopsai/pkg/registryauth"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicelog"
	"nopsai/pkg/servicetls"
	"nopsai/pkg/startupgates"
	"nopsai/services/docker-runner/internal/service"

	"github.com/moby/moby/client"
	"github.com/rs/zerolog/log"
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
	if err := startupgates.ValidateRunner(cfg); err != nil {
		log.Fatal().Err(err).Msg("runner startup gates failed")
	}

	dispatcherAddr := strings.TrimSpace(cfg.DispatcherAddress)
	if dispatcherAddr == "" {
		dispatcherAddr = "localhost:9090"
	}

	runnerID := strings.TrimSpace(cfg.RunnerID)
	if runnerID == "" {
		if host, err := os.Hostname(); err == nil {
			runnerID = host
		} else {
			runnerID = "runner"
		}
	}

	dispatcherCreds, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: cfg.EffectiveServiceJWTSigningKey(),
		Issuer:     cfg.EffectiveServiceJWTIssuer(),
		Audience:   cfg.EffectiveServiceJWTAudience(),
		Role:       serviceauth.RoleRunner,
		ServiceID:  cfg.EffectiveRunnerServiceID(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to configure dispatcher client authentication")
	}

	transportCreds, err := servicetls.ClientCredentials(servicetls.Config{
		Mode:       cfg.EffectiveDispatcherTLSMode(),
		Secret:     cfg.EffectiveDispatcherTLSSecret(),
		Role:       serviceauth.RoleRunner,
		ServiceID:  cfg.EffectiveRunnerServiceID(),
		ServerName: cfg.EffectiveDispatcherTLSServerName(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to configure dispatcher transport security")
	}

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create docker client")
	}
	defer dockerClient.Close()

	registryAuth, registryHosts, err := registryauth.DockerConfigResolverFromEnv(os.Getenv)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load local Docker registry auth config")
	}
	var registryAuthResolver service.RegistryAuthResolver
	if registryAuth.Configured() {
		registryAuthResolver = registryAuth
		log.Info().Strs("registry_hosts", registryHosts).Msg("loaded local Docker registry auth config")
	}

	networkValue, networkSet := dockerNetworkFromConfig(cfg)
	runner := service.NewDockerRunner(service.RunnerOptions{
		RunnerID:                 runnerID,
		RunnerScopes:             cfg.RunnerScopes,
		Capacity:                 int32(cfg.RunnerCapacity),
		DispatcherAddr:           dispatcherAddr,
		DispatcherCreds:          dispatcherCreds,
		TransportCreds:           transportCreds,
		Docker:                   dockerClient,
		DockerNetwork:            networkValue,
		DockerNetworkSet:         networkSet,
		ContainerName:            strings.TrimSpace(os.Getenv("RUNNER_CONTAINER_NAME")),
		RegistryAuth:             registryAuthResolver,
		RegistryAuthConfigBase64: strings.TrimSpace(os.Getenv(registryauth.DockerConfigBase64Env)),
	})
	runner.ServeForever()
}

func configureLogging(cfg *config.Config) {
	if err := servicelog.Configure(cfg.LogLevel, cfg.LogFormat); err != nil {
		log.Warn().Str("log_level", cfg.LogLevel).Msg("Invalid log level; defaulting to info")
	}
}

func dockerNetworkFromConfig(cfg *config.Config) (string, bool) {
	networkValue := strings.TrimSpace(cfg.DockerNetworkName)
	if envVal, ok := os.LookupEnv("DOCKER_NETWORK_NAME"); ok {
		return envVal, true
	}
	return networkValue, networkValue != ""
}
