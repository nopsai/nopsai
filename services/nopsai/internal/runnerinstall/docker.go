package runnerinstall

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicetls"
)

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

func BuildComposeResponse(cfg config.Config, r *http.Request) (ComposeResponse, error) {
	spec, err := buildInstallSpec(cfg, r)
	if err != nil {
		return ComposeResponse{}, err
	}
	compose := buildCompose(spec)
	return ComposeResponse{
		RunnerID:          spec.RunnerID,
		RunnerScopes:      spec.RunnerScopes,
		RunnerCapacity:    spec.RunnerCapacity,
		DispatcherAddress: spec.DispatcherAddress,
		NetworkMode:       spec.NetworkMode,
		RunnerImage:       spec.RunnerImage,
		Compose:           compose,
		Command:           fmt.Sprintf("docker compose -f docker-compose.yaml up -d %s", spec.ServiceName),
		Warnings:          spec.Warnings,
	}, nil
}

func BuildBootstrapCommandResponse(cfg config.Config, r *http.Request, issueToken TokenIssuer) (BootstrapCommandResponse, error) {
	if issueToken == nil {
		return BootstrapCommandResponse{}, fmt.Errorf("runner bootstrap token issuer is required")
	}
	spec, err := buildInstallSpec(cfg, r)
	if err != nil {
		return BootstrapCommandResponse{}, err
	}
	script := buildDockerRunScript(spec)
	token, expiresAt, err := issueToken(script, 10*time.Minute, "text/x-shellscript; charset=utf-8")
	if err != nil {
		return BootstrapCommandResponse{}, err
	}
	bootstrapURL := strings.TrimRight(RequestExternalBaseURL(r), "/") + "/v1/system/dispatcher/runner-bootstrap?token=" + url.QueryEscape(token)
	return BootstrapCommandResponse{
		RunnerID:          spec.RunnerID,
		RunnerScopes:      spec.RunnerScopes,
		RunnerCapacity:    spec.RunnerCapacity,
		DispatcherAddress: spec.DispatcherAddress,
		NetworkMode:       spec.NetworkMode,
		RunnerImage:       spec.RunnerImage,
		BootstrapCommand:  fmt.Sprintf("tmp=$(mktemp) && curl -fsSL %s -o \"$tmp\" && sh \"$tmp\"; rc=$?; rm -f \"$tmp\"; exit $rc", ShellQuote(bootstrapURL)),
		ExpiresAt:         expiresAt,
		Warnings: append([]string{
			"This one-time install command expires in 10 minutes and is consumed by the first successful download.",
		}, spec.Warnings...),
	}, nil
}

func buildInstallSpec(cfg config.Config, r *http.Request) (installSpec, error) {
	query := r.URL.Query()
	runnerID := strings.TrimSpace(query.Get("runner_id"))
	if runnerID == "" {
		runnerID = "runner-prod-1"
	}
	runnerScopes := strings.TrimSpace(query.Get("runner_scopes"))
	if _, provided := query["runner_scopes"]; !provided && runnerScopes == "" {
		runnerScopes = strings.TrimSpace(cfg.RunnerScopes)
	}
	if _, provided := query["runner_scopes"]; !provided && runnerScopes == "" {
		runnerScopes = "prod"
	}
	runnerCapacity := cfg.RunnerCapacity
	if runnerCapacity <= 0 {
		runnerCapacity = 1
	}
	if rawCapacity := strings.TrimSpace(query.Get("runner_capacity")); rawCapacity != "" {
		parsed, err := strconv.Atoi(rawCapacity)
		if err != nil || parsed <= 0 {
			return installSpec{}, fmt.Errorf("runner_capacity must be a positive integer")
		}
		runnerCapacity = parsed
	}

	serviceJWTSigningKey := cfg.EffectiveServiceJWTSigningKey()
	if strings.TrimSpace(serviceJWTSigningKey) == "" {
		return installSpec{}, fmt.Errorf("SERVICE_JWT_SIGNING_KEY is not configured")
	}

	dispatcherAddress, adapted, warnings := ExternalDispatcherAddress(cfg, r)
	tlsSecret := cfg.EffectiveDispatcherTLSSecret()
	tlsMode := cfg.EffectiveDispatcherTLSMode()
	if servicetls.Enabled(tlsMode) && strings.TrimSpace(tlsSecret) == "" {
		return installSpec{}, fmt.Errorf("DISPATCHER_TLS_SECRET is not configured")
	}
	if adapted {
		warnings = append(warnings, "The configured dispatcher address is local to the NopsAI stack, so this template uses an external dispatcher host derived from the current request host and dispatcher port. Confirm that endpoint is reachable from the new runner host.")
		if LooksInternalAddress(cfg.AgentNopsaiAPIURL) {
			warnings = append(warnings, fmt.Sprintf("agent_nopsai_api_url is %q. Remote agent containers may need System > Config to use a URL reachable outside the Docker network.", cfg.AgentNopsaiAPIURL))
		}
	}
	networkMode := strings.ToLower(strings.TrimSpace(query.Get("runner_network_mode")))
	switch networkMode {
	case "", "auto":
		if adapted {
			networkMode = NetworkModeHost
		} else {
			networkMode = NetworkModeBridge
		}
	case "default", NetworkModeBridge:
		networkMode = NetworkModeBridge
	case NetworkModeHost:
	default:
		return installSpec{}, fmt.Errorf("runner_network_mode must be bridge, host, or auto")
	}
	if networkMode == NetworkModeHost {
		warnings = append(warnings, "The runner container will use Docker host networking so it follows the VM host routing to the dispatcher. This helps when the VM can reach the dispatcher but Docker bridge containers cannot.")
	}
	runnerImage := strings.TrimSpace(query.Get("runner_image"))
	if runnerImage == "" {
		runnerImage = DefaultRunnerImage
	}

	serviceName := composeServiceName(runnerID)
	env := []installEnv{
		{"RUNNER_ID", runnerID},
		{"RUNNER_SCOPES", runnerScopes},
		{"RUNNER_CAPACITY", strconv.Itoa(runnerCapacity)},
		{"DISPATCHER_ADDRESS", dispatcherAddress},
		{serviceauth.EnvSigningKey, serviceJWTSigningKey},
		{serviceauth.EnvIssuer, cfg.EffectiveServiceJWTIssuer()},
		{serviceauth.EnvAudience, cfg.EffectiveServiceJWTAudience()},
		{"RUNNER_SERVICE_ID", cfg.EffectiveRunnerServiceID()},
		{servicetls.EnvMode, tlsMode},
		{servicetls.EnvSecret, tlsSecret},
		{servicetls.EnvServerName, cfg.EffectiveDispatcherTLSServerName()},
	}
	dockerNetwork := strings.TrimSpace(cfg.DockerNetworkName)
	if adapted {
		dockerNetwork = ""
		env = append(env, installEnv{"DOCKER_NETWORK_NAME", ""})
	} else if dockerNetwork != "" {
		env = append(env, installEnv{"DOCKER_NETWORK_NAME", dockerNetwork})
	}

	return installSpec{
		RunnerID:          runnerID,
		RunnerScopes:      runnerScopes,
		RunnerCapacity:    runnerCapacity,
		DispatcherAddress: dispatcherAddress,
		ServiceName:       serviceName,
		DockerNetwork:     dockerNetwork,
		NetworkMode:       networkMode,
		RunnerImage:       runnerImage,
		IncludeNetwork:    networkMode == NetworkModeBridge && !adapted && dockerNetwork != "",
		Env:               env,
		Warnings:          warnings,
	}, nil
}

func buildCompose(spec installSpec) string {
	var builder strings.Builder
	builder.WriteString(spec.ServiceName)
	builder.WriteString(":\n")
	builder.WriteString("  image: ")
	builder.WriteString(strconv.Quote(spec.RunnerImage))
	builder.WriteString("\n")
	builder.WriteString("  restart: always\n")
	if spec.NetworkMode == NetworkModeHost {
		builder.WriteString("  network_mode: \"host\"\n")
	}
	builder.WriteString("  environment:\n")
	for _, item := range spec.Env {
		builder.WriteString("    ")
		builder.WriteString(item.key)
		builder.WriteString(": ")
		builder.WriteString(strconv.Quote(item.value))
		builder.WriteString("\n")
	}
	builder.WriteString("  volumes:\n")
	builder.WriteString("    - /var/run/docker.sock:/var/run/docker.sock\n")
	if spec.IncludeNetwork {
		builder.WriteString("  networks:\n")
		builder.WriteString("    - ")
		builder.WriteString(strconv.Quote(spec.DockerNetwork))
		builder.WriteString("\n")
	}
	return builder.String()
}

func buildDockerRunScript(spec installSpec) string {
	var builder strings.Builder
	builder.WriteString("#!/bin/sh\n")
	builder.WriteString("set -eu\n\n")
	builder.WriteString("if ! command -v docker >/dev/null 2>&1; then\n")
	builder.WriteString("  echo \"docker is required on this runner host\" >&2\n")
	builder.WriteString("  exit 1\n")
	builder.WriteString("fi\n\n")
	builder.WriteString("echo \"Installing NopsAI runner ")
	builder.WriteString(shellDoubleQuote(spec.RunnerID))
	builder.WriteString("\"\n")
	builder.WriteString("echo \"Dispatcher address: ")
	builder.WriteString(shellDoubleQuote(spec.DispatcherAddress))
	builder.WriteString("\"\n")
	builder.WriteString("echo \"Docker network mode: ")
	builder.WriteString(shellDoubleQuote(spec.NetworkMode))
	builder.WriteString("\"\n\n")
	builder.WriteString("runner_image=")
	builder.WriteString(ShellQuote(spec.RunnerImage))
	builder.WriteString("\n")
	builder.WriteString("docker pull \"$runner_image\"\n")
	builder.WriteString("host_arch=$(docker info --format '{{.Architecture}}' 2>/dev/null || uname -m)\n")
	builder.WriteString("case \"$host_arch\" in x86_64) host_arch=amd64 ;; aarch64) host_arch=arm64 ;; esac\n")
	builder.WriteString("image_arch=$(docker image inspect \"$runner_image\" --format '{{.Architecture}}' 2>/dev/null || true)\n")
	builder.WriteString("if [ -n \"$image_arch\" ] && [ \"$image_arch\" != \"$host_arch\" ]; then\n")
	builder.WriteString("  echo \"Runner image ${runner_image} architecture ${image_arch} does not match Docker host architecture ${host_arch}. Publish or select a matching/multi-arch runner image before installing this runner.\" >&2\n")
	builder.WriteString("  exit 1\n")
	builder.WriteString("fi\n")
	builder.WriteString("docker rm -f ")
	builder.WriteString(ShellQuote(spec.ServiceName))
	builder.WriteString(" >/dev/null 2>&1 || true\n")
	builder.WriteString("container_id=$(docker run -d \\\n")
	builder.WriteString("  --name ")
	builder.WriteString(ShellQuote(spec.ServiceName))
	builder.WriteString(" \\\n")
	builder.WriteString("  --restart always \\\n")
	if spec.NetworkMode == NetworkModeHost {
		builder.WriteString("  --network host \\\n")
	}
	if spec.IncludeNetwork {
		builder.WriteString("  --network ")
		builder.WriteString(ShellQuote(spec.DockerNetwork))
		builder.WriteString(" \\\n")
	}
	builder.WriteString("  -v /var/run/docker.sock:/var/run/docker.sock")
	for _, item := range spec.Env {
		builder.WriteString(" \\\n")
		builder.WriteString("  -e ")
		builder.WriteString(ShellQuote(item.key + "=" + item.value))
	}
	builder.WriteString(" \\\n")
	builder.WriteString("  \"$runner_image\")\n")
	builder.WriteString("echo \"NopsAI runner ")
	builder.WriteString(shellDoubleQuote(spec.RunnerID))
	builder.WriteString(" started as ${container_id}.\"\n")
	builder.WriteString("sleep 3\n")
	builder.WriteString("if ! docker inspect -f '{{.State.Running}}' ")
	builder.WriteString(ShellQuote(spec.ServiceName))
	builder.WriteString(" 2>/dev/null | grep -q '^true$'; then\n")
	builder.WriteString("  echo \"Runner container is not running. Recent logs:\" >&2\n")
	builder.WriteString("  docker logs --tail=120 ")
	builder.WriteString(ShellQuote(spec.ServiceName))
	builder.WriteString(" >&2 || true\n")
	builder.WriteString("  exit 1\n")
	builder.WriteString("fi\n")
	builder.WriteString("echo \"Recent runner logs:\"\n")
	builder.WriteString("docker logs --tail=40 ")
	builder.WriteString(ShellQuote(spec.ServiceName))
	builder.WriteString(" || true\n")
	builder.WriteString("echo \"Refresh System > Dispatcher to confirm registration. If no registration appears, run: docker logs -f ")
	builder.WriteString(shellDoubleQuote(spec.ServiceName))
	builder.WriteString("\"\n")
	return builder.String()
}

func composeServiceName(runnerID string) string {
	name := strings.ToLower(strings.TrimSpace(runnerID))
	name = nonAlphanumericRegex.ReplaceAllString(name, "-")
	name = strings.Trim(name, ".-_")
	if name == "" {
		return "nopsai-runner"
	}
	if strings.HasPrefix(name, "runner") {
		return name
	}
	return "runner-" + name
}
