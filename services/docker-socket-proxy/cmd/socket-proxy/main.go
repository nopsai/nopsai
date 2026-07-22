package main

import (
	"net/http"
	"os"
	"strings"
	"time"

	"nopsai/pkg/buildinfo"
	"nopsai/pkg/servicelog"
	"nopsai/services/docker-socket-proxy/internal/proxy"

	"github.com/rs/zerolog/log"
)

func main() {
	if buildinfo.Requested(os.Args[1:]) {
		_ = buildinfo.WriteVersion(os.Stdout, "nopsai-docker-socket-proxy")
		return
	}
	listenAddress := strings.TrimSpace(os.Getenv("LISTEN_ADDRESS"))
	if listenAddress == "" {
		listenAddress = ":2375"
	}
	socketPath := strings.TrimSpace(os.Getenv("DOCKER_SOCKET_PATH"))
	if socketPath == "" {
		socketPath = "/var/run/docker.sock"
	}
	allowedContainers := proxy.DefaultAllowedContainers
	if configured := strings.TrimSpace(os.Getenv("ALLOWED_CONTAINERS")); configured != "" {
		allowedContainers = strings.Split(configured, ",")
	}
	if err := servicelog.Configure(os.Getenv("LOG_LEVEL"), os.Getenv("LOG_FORMAT")); err != nil {
		log.Warn().Str("log_level", os.Getenv("LOG_LEVEL")).Msg("invalid log level; defaulting to info")
	}
	server := &http.Server{
		Addr: listenAddress, Handler: servicelog.HTTPMiddleware(proxy.New(socketPath, allowedContainers...)),
		ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second,
	}
	log.Info().Str("addr", listenAddress).Msg("docker read proxy started")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("docker read proxy failed")
	}
}
