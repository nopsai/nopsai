package nopsai

import (
	"net"
	"net/url"
	"strings"

	"nopsai/config"
)

func (a *App) getConfigSnapshot() config.Config {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return *a.cfg
}

func (a *App) getAutoRemovalAgentContainer() bool {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return a.cfg.AutoRemovalAgentContainer
}

func (a *App) getDefaultPipelineTimeout() string {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return a.cfg.DefaultPipelineTimeout
}

func (a *App) getAgentImage() string {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return strings.TrimSpace(a.cfg.AgentImage)
}

func (a *App) getDockerNetworkName() string {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return strings.TrimSpace(a.cfg.DockerNetworkName)
}

func (a *App) getLLMAgentTimeout() string {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return strings.TrimSpace(a.cfg.LLMAgentTimeout)
}

func containerReachableLMStudioBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}

	host := parsed.Hostname()
	if !strings.EqualFold(host, "localhost") && host != "127.0.0.1" && host != "::1" {
		return trimmed
	}

	port := parsed.Port()
	if port != "" {
		parsed.Host = net.JoinHostPort("host.docker.internal", port)
	} else {
		parsed.Host = "host.docker.internal"
	}

	return parsed.String()
}
