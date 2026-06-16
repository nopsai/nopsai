package systemconfig

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"nopsai/config"
)

func BuildResponse(cfg config.Config, envFilePath string) map[string]interface{} {
	return map[string]interface{}{
		"agent_nopsai_api_url":              cfg.AgentNopsaiAPIURL,
		"git_bot_nopsai_api_url":            cfg.GitBotNopsaiAPIURL,
		"nopsai_git_bot_api_url":            cfg.NopsaiGitBotAPIURL,
		"dispatcher_address":                cfg.DispatcherAddress,
		"agent_image":                       cfg.AgentImage,
		"docker_network_name":               cfg.DockerNetworkName,
		"auto_removal_agent_container":      cfg.AutoRemovalAgentContainer,
		"default_pipeline_timeout":          cfg.DefaultPipelineTimeout,
		"llm_agent_timeout":                 cfg.LLMAgentTimeout,
		"dispatcher_routing":                CloneDispatcherRouting(cfg.DispatcherRouting),
		"runner_id":                         cfg.RunnerID,
		"runner_scopes":                     cfg.RunnerScopes,
		"runner_capacity":                   cfg.RunnerCapacity,
		"github_app_id":                     cfg.GitHubAppID,
		"github_installation_id":            cfg.GitHubInstallID,
		"github_private_key_credential_ref": cfg.GitHubPrivateKeyCredentialRef,
		"github_webhook_credential_ref":     cfg.GitHubWebhookCredentialRef,
		"runtime":                           config.NormalizeRuntime(cfg.Runtime),
		"kubernetes":                        config.NormalizeKubernetesConfig(cfg.Kubernetes),
		"limits":                            cfg.Limits,
		"runtime_pools":                     config.NormalizeRuntimePools(cfg.RuntimePools),
		"env_file_path":                     envFilePath,
	}
}

func CloneDispatcherRouting(routing map[string][]string) map[string][]string {
	if len(routing) == 0 {
		return map[string][]string{}
	}
	cloned := make(map[string][]string, len(routing))
	for scope, runners := range routing {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			scope = "*"
		}
		next := make([]string, 0, len(runners))
		for _, runner := range runners {
			runner = strings.TrimSpace(runner)
			if runner != "" {
				next = append(next, runner)
			}
		}
		if len(next) > 0 {
			cloned[scope] = next
		}
	}
	return cloned
}

func NormalizeDispatcherRoutingConfig(routing map[string][]string) map[string][]string {
	if routing == nil {
		return nil
	}
	return CloneDispatcherRouting(routing)
}

func NormalizeRunnerScopes(raw string) string {
	parts := strings.Split(raw, ",")
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		scope := strings.Trim(strings.TrimSpace(part), "/")
		if scope == "" {
			continue
		}
		key := strings.ToLower(scope)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, scope)
	}
	return strings.Join(normalized, ",")
}

func ValidateRunnerLimits(limits config.RunnerLimits) error {
	if limits.MaxConcurrentRuns < 0 ||
		limits.MaxConcurrentTasks < 0 ||
		limits.MaxConcurrentTasksPerRun < 0 ||
		limits.MaxPendingTasks < 0 {
		return fmt.Errorf("runner limits cannot be negative")
	}
	return nil
}

func EncodeEnvJSON(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func WriteEnvFile(path string, updates map[string]string) error {
	var lines []string
	used := make(map[string]bool, len(updates))

	if data, err := os.ReadFile(path); err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		for scanner.Scan() {
			line := scanner.Text()
			if key, ok := parseEnvKey(line); ok {
				if value, shouldReplace := updates[key]; shouldReplace {
					line = formatEnvLine(key, value)
					used[key] = true
				}
			}
			lines = append(lines, line)
		}
		if scanErr := scanner.Err(); scanErr != nil {
			return scanErr
		}
	}

	for key, value := range updates {
		if used[key] {
			continue
		}
		lines = append(lines, formatEnvLine(key, value))
	}

	output := strings.Join(lines, "\n")
	return os.WriteFile(path, []byte(output), 0o644)
}

func parseEnvKey(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	parts := strings.SplitN(trimmed, "=", 2)
	if len(parts) != 2 {
		return "", false
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return "", false
	}
	return key, true
}

func formatEnvLine(key, value string) string {
	escaped := strings.ReplaceAll(value, `"`, `\"`)
	return fmt.Sprintf(`%s="%s"`, key, escaped)
}
