package nopsai

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/moby/moby/client"

	"nopsai/config"
	"nopsai/services/nopsai/internal/systemlogs"
	systemlogdocker "nopsai/services/nopsai/internal/systemlogs/docker"
)

func newSystemLogBroker(cfg *config.Config, provider systemlogs.Provider) (*systemlogs.Broker, error) {
	registry := systemlogs.DefaultRegistry()
	if provider == nil {
		if cfg == nil || !cfg.SystemLogsEnabled() {
			provider = systemlogs.NewUnavailableProvider(registry, "Docker provider is not configured")
		} else {
			host := cfg.EffectiveSystemLogsDockerHost()
			if !strings.HasPrefix(host, "tcp://") && !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
				return nil, fmt.Errorf("system log Docker host must use tcp, http, or https")
			}
			dockerClient, err := client.NewClientWithOpts(client.WithHost(host), client.WithAPIVersionNegotiation())
			if err != nil {
				return nil, err
			}
			provider = systemlogdocker.NewMobyProvider(dockerClient, registry)
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
