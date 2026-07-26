package nopsai

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/moby/client"
	clientkubernetes "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"nopsai/config"
	"nopsai/services/nopsai/internal/systemlogs"
	systemlogdocker "nopsai/services/nopsai/internal/systemlogs/docker"
	systemlogkubernetes "nopsai/services/nopsai/internal/systemlogs/kubernetes"
)

func newSystemLogBroker(cfg *config.Config, provider systemlogs.Provider) (*systemlogs.Broker, error) {
	registry := systemlogs.DefaultRegistry()
	if provider == nil {
		if cfg == nil || !cfg.SystemLogsEnabled() {
			provider = systemlogs.NewUnavailableProvider(registry, "system log provider is not configured")
		} else {
			switch cfg.EffectiveSystemLogsProvider() {
			case "docker":
				host := cfg.EffectiveSystemLogsDockerHost()
				if !strings.HasPrefix(host, "tcp://") && !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
					return nil, fmt.Errorf("system log Docker host must use tcp, http, or https")
				}
				dockerClient, err := client.New(client.WithHost(host))
				if err != nil {
					return nil, err
				}
				provider = systemlogdocker.NewMobyProvider(dockerClient, registry)
			case "kubernetes":
				kubernetesClient, err := newSystemLogsKubernetesClient()
				if err != nil {
					return nil, err
				}
				kubernetesConfig := cfg.SystemLogs.Kubernetes
				kubernetesConfig.Namespace = effectiveSystemLogsKubernetesNamespace(kubernetesConfig.Namespace)
				provider = systemlogkubernetes.NewClientProvider(kubernetesClient, registry, systemlogkubernetes.Options{
					Namespace:     kubernetesConfig.Namespace,
					LabelSelector: kubernetesConfig.LabelSelector,
					Container:     kubernetesConfig.Container,
				})
			default:
				provider = systemlogs.NewUnavailableProvider(registry, "system log provider is not configured")
			}
		}
	}

	settings := config.SystemLogsConfig{}
	key := sha256.Sum256([]byte("system-logs-cursor-key"))
	if cfg != nil {
		settings = cfg.SystemLogs
		key = sha256.Sum256([]byte("system-logs-cursor-key:" + cfg.MasterKey))
	}
	bufferAge := time.Duration(settings.BufferAgeMinutes) * time.Minute
	return systemlogs.NewBroker(
		provider,
		registry,
		systemlogs.NewRedactor(settings.MaxLineBytes),
		systemlogs.NewCursorCodec(key[:]),
		systemlogs.BrokerOptions{
			BufferLines: settings.BufferLines, BufferAge: bufferAge, MaxTailLines: settings.MaxTailLines,
			MaxStreams: settings.MaxStreams, MaxStreamsPerSource: settings.MaxStreamsPerSource,
		},
	), nil
}

func newSystemLogsKubernetesClient() (clientkubernetes.Interface, error) {
	restConfig, err := systemLogsKubernetesRESTConfig()
	if err != nil {
		return nil, err
	}
	return clientkubernetes.NewForConfig(restConfig)
}

func systemLogsKubernetesRESTConfig() (*rest.Config, error) {
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

func effectiveSystemLogsKubernetesNamespace(configured string) string {
	if namespace := strings.TrimSpace(configured); namespace != "" {
		return namespace
	}
	if namespace, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if trimmed := strings.TrimSpace(string(namespace)); trimmed != "" {
			return trimmed
		}
	}
	return "default"
}
