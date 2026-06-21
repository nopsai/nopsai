package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"nopsai/pkg/buildinfo"
	"nopsai/services/docker-socket-proxy/internal/proxy"
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
	server := &http.Server{
		Addr: listenAddress, Handler: proxy.New(socketPath, allowedContainers...),
		ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second,
	}
	log.Print("Docker read proxy started")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
