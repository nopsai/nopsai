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

type FieldMetadata struct {
	Scope   config.ConfigScope `json:"scope"`
	Label   string             `json:"label"`
	Section string             `json:"section"`
	Apply   string             `json:"apply"`
}

var FieldMetadataByKey = map[string]FieldMetadata{
	"log_level":                        runtimeLive("Log level", "General"),
	"log_format":                       runtimeLive("Log format", "General"),
	"environment":                      runtimeLive("Environment", "General"),
	"public_url":                       runtimeLive("Public URL", "General"),
	"notification_mail_logo_url":       runtimeLive("Mail logo URL", "General"),
	"notification_mail_website_url":    runtimeLive("Mail website URL", "General"),
	"notification_mail_support_url":    runtimeLive("Mail support URL", "General"),
	"notification_mail_footer_address": runtimeLive("Mail footer address", "General"),
	"require_production_gates":         bootstrapOnly("Production startup gates", "General"),

	"agent_image":                  nextRunOnly("Agent image", "Runtime"),
	"docker_network_name":          nextRunOnly("Docker network", "Runtime"),
	"auto_removal_agent_container": nextRunOnly("Auto-remove agent containers", "Runtime"),
	"default_pipeline_timeout":     nextRunOnly("Default pipeline timeout", "Runtime"),
	"llm_agent_timeout":            nextRunOnly("LLM agent timeout", "Runtime"),
	"runtime":                      nextRunOnly("Runtime", "Runtime"),
	"kubernetes":                   nextRunOnly("Kubernetes defaults", "Runtime"),
	"limits":                       nextRunOnly("Runner limits", "Runtime"),
	"runtime_pools":                runtimeLive("Runtime pools", "Runtime"),
	"assistant":                    runtimeLive("Assistant", "Assistant"),

	"nopsai_api_url":          runtimeReload("NopsAI API URL", "Services"),
	"dispatcher_grpc_address": runtimeReload("Dispatcher gRPC address", "Dispatcher"),
	"dispatcher_routing":      runtimeLive("Dispatcher routing", "Dispatcher"),
	"git_bot_api_url":         runtimeLive("GitBot API URL", "GitHub App"),

	"github_app_id":                     runtimeReload("GitHub App ID", "GitHub App"),
	"github_installation_id":            runtimeReload("GitHub installation ID", "GitHub App"),
	"github_private_key_credential_ref": runtimeReload("GitHub private key credential", "GitHub App"),
	"github_webhook_credential_ref":     runtimeReload("GitHub webhook credential", "GitHub App"),

	"runner_id":       bootstrapOnly("Runner ID", "Runners"),
	"runner_scopes":   runtimeReload("Runner scopes", "Runners"),
	"runner_capacity": runtimeReload("Runner capacity", "Runners"),
}

func runtimeLive(label, section string) FieldMetadata {
	return FieldMetadata{Scope: config.ConfigScopeRuntimeLive, Label: label, Section: section, Apply: "Applied immediately"}
}

func runtimeReload(label, section string) FieldMetadata {
	return FieldMetadata{Scope: config.ConfigScopeRuntimeReload, Label: label, Section: section, Apply: "Applies after reconnect"}
}

func nextRunOnly(label, section string) FieldMetadata {
	return FieldMetadata{Scope: config.ConfigScopeNextRunOnly, Label: label, Section: section, Apply: "Applies to new runs only"}
}

func bootstrapOnly(label, section string) FieldMetadata {
	return FieldMetadata{Scope: config.ConfigScopeBootstrapOnly, Label: label, Section: section, Apply: "Requires service restart"}
}

func BuildResponse(cfg config.Config, envFilePath string) map[string]interface{} {
	return map[string]interface{}{
		"log_level":                         cfg.LogLevel,
		"log_format":                        cfg.LogFormat,
		"environment":                       cfg.Environment,
		"public_url":                        cfg.PublicURL,
		"notification_mail_logo_url":        cfg.NotificationMailLogoURL,
		"notification_mail_website_url":     cfg.NotificationMailWebsiteURL,
		"notification_mail_support_url":     cfg.NotificationMailSupportURL,
		"notification_mail_footer_address":  cfg.NotificationMailFooterAddress,
		"require_production_gates":          cfg.RequireProductionGates,
		"nopsai_api_url":                    cfg.EffectiveNopsaiAPIURL(),
		"git_bot_api_url":                   cfg.NopsaiGitBotAPIURL,
		"dispatcher_grpc_address":           cfg.DispatcherAddress,
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
		"assistant":                         cfg.EffectiveAssistantConfig(),
		"env_file_path":                     envFilePath,
		"field_metadata":                    FieldMetadataByKey,
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
