package app

import (
	"os"
	"path/filepath"
	"strings"

	"nopsai/config"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicelog"
	"nopsai/pkg/servicetls"
	"nopsai/pkg/startupgates"
	"nopsai/services/k8s-runner/internal/service"

	"github.com/rs/zerolog/log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
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
	if err := startupgates.ValidateKubernetesRunner(cfg); err != nil {
		log.Fatal().Err(err).Msg("kubernetes runner startup gates failed")
	}

	dispatcherAddr := strings.TrimSpace(cfg.DispatcherAddress)
	if dispatcherAddr == "" {
		dispatcherAddr = "dispatcher:9090"
	}

	runnerID := strings.TrimSpace(cfg.RunnerID)
	if runnerID == "" {
		if host, err := os.Hostname(); err == nil {
			runnerID = host
		} else {
			runnerID = "k8s-runner"
		}
	}

	capacity := int32(cfg.RunnerCapacity)
	if capacity <= 0 {
		capacity = 1
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

	restConfig, err := kubernetesRESTConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load kubernetes config")
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create kubernetes client")
	}

	runner := service.NewKubernetesRunner(service.RunnerOptions{
		Config:          cfg,
		RunnerID:        runnerID,
		DispatcherAddr:  dispatcherAddr,
		Capacity:        capacity,
		DispatcherCreds: dispatcherCreds,
		TransportCreds:  transportCreds,
		Client:          clientset,
	})
	runner.ServeForever()
}

func configureLogging(cfg *config.Config) {
	if err := servicelog.Configure(cfg.LogLevel, cfg.LogFormat); err != nil {
		log.Warn().Str("log_level", cfg.LogLevel).Msg("Invalid log level; defaulting to info")
	}
}

func kubernetesRESTConfig() (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	kubeconfig := strings.TrimSpace(os.Getenv("KUBECONFIG"))
	if kubeconfig == "" {
		if home, err := os.UserHomeDir(); err == nil {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
