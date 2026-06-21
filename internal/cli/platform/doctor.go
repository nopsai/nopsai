package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"nopsai/internal/cli/client"
)

type Severity string

const (
	SeverityOK      Severity = "ok"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type Check struct {
	Name     string   `json:"name" yaml:"name"`
	Severity Severity `json:"severity" yaml:"severity"`
	Message  string   `json:"message" yaml:"message"`
}

type Doctor struct {
	Client          *client.Client
	TokenConfigured bool
	LookPath        func(string) (string, error)
	RunCommand      func(context.Context, string, ...string) error
}

func (d Doctor) Run(ctx context.Context) []Check {
	checks := make([]Check, 0, 8)
	lookPath := d.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	foundTools := map[string]bool{}
	for _, tool := range []string{"helm", "kubectl", "docker"} {
		if path, err := lookPath(tool); err != nil {
			checks = append(checks, Check{Name: "tool/" + tool, Severity: SeverityWarning, Message: "not found in PATH"})
		} else {
			foundTools[tool] = true
			checks = append(checks, Check{Name: "tool/" + tool, Severity: SeverityOK, Message: path})
		}
	}
	runCommand := d.RunCommand
	if runCommand == nil {
		runCommand = runPlatformCommand
	}
	if foundTools["kubectl"] {
		checks = append(checks, checkCommandConnectivity(ctx, runCommand, "kubernetes/connectivity", "kubectl", "version", "--request-timeout=5s"))
	}
	if foundTools["docker"] {
		checks = append(checks, checkCommandConnectivity(ctx, runCommand, "docker/connectivity", "docker", "info"))
	}
	if d.Client == nil {
		return append(checks, Check{Name: "api/connectivity", Severity: SeverityError, Message: "API client is not configured"})
	}

	checks = append(checks, d.checkPreflight(ctx))
	checks = append(checks, d.checkMetrics(ctx))
	if !d.TokenConfigured {
		checks = append(checks,
			Check{Name: "aaa/authentication", Severity: SeverityWarning, Message: "no token configured; run `nopsai login --token`"},
			Check{Name: "monitoring/dispatcher", Severity: SeverityWarning, Message: "skipped because no token is configured"},
		)
		return checks
	}
	checks = append(checks, d.checkAuthenticatedEndpoint(ctx, "aaa/authentication", "/v1/auth/me"))
	checks = append(checks, d.checkDispatcher(ctx))
	return checks
}

func checkCommandConnectivity(ctx context.Context, run func(context.Context, string, ...string) error, checkName, command string, args ...string) Check {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := run(commandCtx, command, args...); err != nil {
		return Check{Name: checkName, Severity: SeverityWarning, Message: "unreachable: " + err.Error()}
	}
	return Check{Name: checkName, Severity: SeverityOK, Message: "reachable"}
}

func runPlatformCommand(ctx context.Context, command string, args ...string) error {
	// The caller only supplies the fixed kubectl and docker probes above.
	return exec.CommandContext(ctx, command, args...).Run() // #nosec G204 -- command names are fixed by the caller.
}

func HasErrors(checks []Check) bool {
	for _, check := range checks {
		if check.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (d Doctor) checkPreflight(ctx context.Context) Check {
	status, body, err := d.get(ctx, "/v1/setup/preflight")
	if err != nil {
		return Check{Name: "api/preflight", Severity: SeverityError, Message: err.Error()}
	}
	var payload struct {
		Ready bool   `json:"ready"`
		Mode  string `json:"mode"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Check{Name: "api/preflight", Severity: SeverityError, Message: "invalid preflight response"}
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices || !payload.Ready {
		return Check{Name: "api/preflight", Severity: SeverityError, Message: fmt.Sprintf("not ready (HTTP %d, mode %s)", status, valueOrUnknown(payload.Mode))}
	}
	return Check{Name: "api/preflight", Severity: SeverityOK, Message: "ready (mode " + valueOrUnknown(payload.Mode) + ")"}
}

func (d Doctor) checkMetrics(ctx context.Context) Check {
	status, _, err := d.get(ctx, "/metrics")
	if err != nil {
		return Check{Name: "monitoring/metrics", Severity: SeverityError, Message: err.Error()}
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return Check{Name: "monitoring/metrics", Severity: SeverityError, Message: fmt.Sprintf("HTTP %d", status)}
	}
	return Check{Name: "monitoring/metrics", Severity: SeverityOK, Message: "reachable"}
}

func (d Doctor) checkAuthenticatedEndpoint(ctx context.Context, name, path string) Check {
	status, _, err := d.get(ctx, path)
	if err != nil {
		return Check{Name: name, Severity: SeverityError, Message: err.Error()}
	}
	if status == http.StatusUnauthorized {
		return Check{Name: name, Severity: SeverityError, Message: "token was rejected"}
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return Check{Name: name, Severity: SeverityWarning, Message: fmt.Sprintf("HTTP %d", status)}
	}
	return Check{Name: name, Severity: SeverityOK, Message: "token accepted"}
}

func (d Doctor) checkDispatcher(ctx context.Context) Check {
	status, body, err := d.get(ctx, "/v1/system/dispatcher")
	if err != nil {
		return Check{Name: "monitoring/dispatcher", Severity: SeverityError, Message: err.Error()}
	}
	if status == http.StatusForbidden {
		return Check{Name: "monitoring/dispatcher", Severity: SeverityWarning, Message: "token lacks dispatcher read permission"}
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return Check{Name: "monitoring/dispatcher", Severity: SeverityError, Message: fmt.Sprintf("HTTP %d", status)}
	}
	var payload struct {
		DispatcherError string            `json:"dispatcher_error"`
		Runners         []json.RawMessage `json:"runners"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Check{Name: "monitoring/dispatcher", Severity: SeverityError, Message: "invalid dispatcher response"}
	}
	if strings.TrimSpace(payload.DispatcherError) != "" {
		return Check{Name: "monitoring/dispatcher", Severity: SeverityWarning, Message: payload.DispatcherError}
	}
	return Check{Name: "monitoring/dispatcher", Severity: SeverityOK, Message: fmt.Sprintf("reachable; %d runner(s) registered", len(payload.Runners))}
}

func (d Doctor) get(ctx context.Context, path string) (int, []byte, error) {
	request, err := d.Client.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		return 0, nil, err
	}
	request = request.WithContext(ctx)
	response, err := d.Client.Do(request)
	if err != nil {
		return 0, nil, fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return 0, nil, fmt.Errorf("read response: %w", err)
	}
	return response.StatusCode, body, nil
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}
