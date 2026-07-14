package runnerinstall

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"nopsai/config"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicetls"
)

func BuildKubernetesManifestResponse(cfg config.Config, r *http.Request) (KubernetesManifestResponse, error) {
	query := r.URL.Query()
	runnerID := strings.TrimSpace(query.Get("runner_id"))
	if runnerID == "" {
		runnerID = firstNonEmptyString(strings.TrimSpace(cfg.RunnerID), "k8s-runner-1")
	}
	runnerScopes := strings.TrimSpace(query.Get("runner_scopes"))
	if runnerScopes == "" {
		runnerScopes = strings.TrimSpace(cfg.RunnerScopes)
	}
	runnerCapacity := cfg.RunnerCapacity
	if runnerCapacity <= 0 {
		runnerCapacity = 1
	}
	if rawCapacity := strings.TrimSpace(query.Get("runner_capacity")); rawCapacity != "" {
		parsed, err := strconv.Atoi(rawCapacity)
		if err != nil || parsed <= 0 {
			return KubernetesManifestResponse{}, fmt.Errorf("runner_capacity must be a positive integer")
		}
		runnerCapacity = parsed
	}

	kcfg := config.NormalizeKubernetesConfig(cfg.Kubernetes)
	namespace := firstNonEmptyString(strings.TrimSpace(query.Get("namespace")), kcfg.Namespace, "nopsai-runs")
	serviceAccount := firstNonEmptyString(strings.TrimSpace(query.Get("service_account")), kcfg.ServiceAccount, "nopsai-runner")
	runnerImage := firstNonEmptyString(strings.TrimSpace(query.Get("runner_image")), DefaultK8sImage)
	agentImage := firstNonEmptyString(strings.TrimSpace(query.Get("agent_image")), strings.TrimSpace(cfg.AgentImage), "nopsai-agent:latest")
	imagePullPolicy := firstNonEmptyString(strings.TrimSpace(query.Get("image_pull_policy")), kcfg.DefaultImagePullPolicy, "IfNotPresent")
	workspaceSize := firstNonEmptyString(strings.TrimSpace(query.Get("workspace_size")), kcfg.DefaultWorkspaceSize, "5Gi")
	workspaceAccessMode := firstNonEmptyString(strings.TrimSpace(query.Get("workspace_access_mode")), kcfg.DefaultWorkspaceAccessMode, "ReadWriteOnce")
	taskTimeout := firstNonEmptyString(strings.TrimSpace(query.Get("task_timeout")), kcfg.DefaultTaskTimeout, "30m")
	runTimeout := firstNonEmptyString(strings.TrimSpace(query.Get("run_timeout")), kcfg.DefaultRunTimeout, "2h")
	storageClass := firstNonEmptyString(strings.TrimSpace(query.Get("storage_class")), kcfg.StorageClass)
	workspaceMode := firstNonEmptyString(strings.TrimSpace(query.Get("workspace_volume_mode")), kcfg.WorkspaceVolumeMode, "pvc")
	affinityEnabled := boolPtrValue(kcfg.AffinityEnabled, true)
	if rawAffinity := strings.TrimSpace(query.Get("affinity_enabled")); rawAffinity != "" {
		parsed, err := strconv.ParseBool(rawAffinity)
		if err != nil {
			return KubernetesManifestResponse{}, fmt.Errorf("affinity_enabled must be true or false")
		}
		affinityEnabled = parsed
	}

	serviceJWTSigningKey := cfg.EffectiveServiceJWTSigningKey()
	if strings.TrimSpace(serviceJWTSigningKey) == "" {
		return KubernetesManifestResponse{}, fmt.Errorf("SERVICE_JWT_SIGNING_KEY is not configured")
	}
	dispatcherAddress, adapted, warnings := ExternalDispatcherAddress(cfg, r)
	if adapted {
		warnings = append(warnings, "The configured dispatcher address is local to the NopsAI stack, so this manifest uses an external dispatcher host derived from the current request host and dispatcher port. Confirm that endpoint is reachable from the Kubernetes cluster.")
	}
	tlsSecret := cfg.EffectiveDispatcherTLSSecret()
	tlsMode := cfg.EffectiveDispatcherTLSMode()
	if servicetls.Enabled(tlsMode) && strings.TrimSpace(tlsSecret) == "" {
		return KubernetesManifestResponse{}, fmt.Errorf("DISPATCHER_TLS_SECRET is not configured")
	}
	if strings.EqualFold(workspaceMode, "emptyDir") {
		warnings = append(warnings, "emptyDir is not emitted because NopsAI Kubernetes execution uses separate agent and step pods; use PVC mode for a shared agent/step workspace.")
		workspaceMode = "pvc"
	}

	env := map[string]string{
		"RUNTIME":                                  "kubernetes",
		"RUNNER_ID":                                runnerID,
		"RUNNER_SCOPES":                            runnerScopes,
		"RUNNER_CAPACITY":                          strconv.Itoa(runnerCapacity),
		"RUNNER_SERVICE_ID":                        cfg.EffectiveRunnerServiceID(),
		"AGENT_IMAGE":                              agentImage,
		"DISPATCHER_GRPC_ADDRESS":                  dispatcherAddress,
		serviceauth.EnvIssuer:                      cfg.EffectiveServiceJWTIssuer(),
		serviceauth.EnvAudience:                    cfg.EffectiveServiceJWTAudience(),
		servicetls.EnvMode:                         tlsMode,
		servicetls.EnvServerName:                   cfg.EffectiveDispatcherTLSServerName(),
		"KUBERNETES_NAMESPACE":                     namespace,
		"KUBERNETES_SERVICE_ACCOUNT":               serviceAccount,
		"KUBERNETES_DEFAULT_IMAGE_PULL_POLICY":     imagePullPolicy,
		"KUBERNETES_DEFAULT_WORKSPACE_SIZE":        workspaceSize,
		"KUBERNETES_DEFAULT_WORKSPACE_ACCESS_MODE": workspaceAccessMode,
		"KUBERNETES_DEFAULT_TASK_TIMEOUT":          taskTimeout,
		"KUBERNETES_DEFAULT_RUN_TIMEOUT":           runTimeout,
		"KUBERNETES_WORKSPACE_VOLUME_MODE":         workspaceMode,
		"KUBERNETES_AFFINITY_ENABLED":              strconv.FormatBool(affinityEnabled),
		"KUBERNETES_CLEANUP_FINISHED_PODS":         "true",
	}
	if storageClass != "" {
		env["KUBERNETES_STORAGE_CLASS"] = storageClass
	}
	if runtimePools := config.NormalizeRuntimePools(cfg.RuntimePools); len(runtimePools) > 0 {
		if data, err := yaml.Marshal(runtimePools); err == nil {
			env["KUBERNETES_RUNTIME_POOLS"] = string(data)
		}
	}
	if cfg.Limits.MaxConcurrentRuns > 0 {
		env["LIMITS_MAX_CONCURRENT_RUNS"] = strconv.Itoa(cfg.Limits.MaxConcurrentRuns)
	}
	if cfg.Limits.MaxConcurrentTasks > 0 {
		env["LIMITS_MAX_CONCURRENT_TASKS"] = strconv.Itoa(cfg.Limits.MaxConcurrentTasks)
	}
	if cfg.Limits.MaxConcurrentTasksPerRun > 0 {
		env["LIMITS_MAX_CONCURRENT_TASKS_PER_RUN"] = strconv.Itoa(cfg.Limits.MaxConcurrentTasksPerRun)
	}
	if cfg.Limits.MaxPendingTasks > 0 {
		env["LIMITS_MAX_PENDING_TASKS"] = strconv.Itoa(cfg.Limits.MaxPendingTasks)
	}

	secretEnv := map[string]string{
		serviceauth.EnvSigningKey: serviceJWTSigningKey,
		servicetls.EnvSecret:      tlsSecret,
	}
	manifest := buildKubernetesManifest(namespace, serviceAccount, runnerID, runnerImage, env, secretEnv)
	return KubernetesManifestResponse{
		RunnerID:          runnerID,
		RunnerScopes:      runnerScopes,
		RunnerCapacity:    runnerCapacity,
		Namespace:         namespace,
		ServiceAccount:    serviceAccount,
		DispatcherAddress: dispatcherAddress,
		RunnerImage:       runnerImage,
		Manifest:          manifest,
		Command:           "kubectl apply -f nopsai-k8s-runner.yaml",
		Warnings:          warnings,
	}, nil
}

func BuildKubernetesBootstrapCommandResponse(cfg config.Config, r *http.Request, issueToken TokenIssuer) (KubernetesBootstrapCommandResponse, error) {
	if issueToken == nil {
		return KubernetesBootstrapCommandResponse{}, fmt.Errorf("runner bootstrap token issuer is required")
	}
	manifestResp, err := BuildKubernetesManifestResponse(cfg, r)
	if err != nil {
		return KubernetesBootstrapCommandResponse{}, err
	}
	appName := kubernetesManifestName("nopsai-k8s-runner", manifestResp.RunnerID)
	script := buildKubernetesBootstrapScript(manifestResp.Manifest, manifestResp.Namespace, appName)
	token, expiresAt, err := issueToken(script, 10*time.Minute, "text/x-shellscript; charset=utf-8")
	if err != nil {
		return KubernetesBootstrapCommandResponse{}, err
	}
	bootstrapURL := strings.TrimRight(RequestExternalBaseURL(r), "/") + "/v1/system/dispatcher/runner-bootstrap?token=" + url.QueryEscape(token)
	return KubernetesBootstrapCommandResponse{
		RunnerID:          manifestResp.RunnerID,
		RunnerScopes:      manifestResp.RunnerScopes,
		RunnerCapacity:    manifestResp.RunnerCapacity,
		Namespace:         manifestResp.Namespace,
		ServiceAccount:    manifestResp.ServiceAccount,
		DispatcherAddress: manifestResp.DispatcherAddress,
		RunnerImage:       manifestResp.RunnerImage,
		BootstrapCommand:  buildKubernetesBootstrapCommand(bootstrapURL),
		ExpiresAt:         expiresAt,
		Warnings: append([]string{
			"This one-time Kubernetes install command expires in 10 minutes and is consumed by the first successful download.",
			"Run this command from a machine where kubectl targets the destination cluster.",
		}, manifestResp.Warnings...),
	}, nil
}

func buildKubernetesBootstrapCommand(bootstrapURL string) string {
	return fmt.Sprintf("tmp=$(mktemp) && curl -fsSL %s -o \"$tmp\" && sh \"$tmp\"; rc=$?; rm -f \"$tmp\"; exit $rc", ShellQuote(bootstrapURL))
}

func buildKubernetesBootstrapScript(manifest, namespace, appName string) string {
	var builder strings.Builder
	builder.WriteString("#!/bin/sh\n")
	builder.WriteString("set -eu\n")
	builder.WriteString("\n")
	builder.WriteString("if ! command -v kubectl >/dev/null 2>&1; then\n")
	builder.WriteString("  echo \"kubectl is required on this host\" >&2\n")
	builder.WriteString("  exit 1\n")
	builder.WriteString("fi\n\n")
	builder.WriteString("tmp=$(mktemp)\n")
	builder.WriteString("ns=")
	builder.WriteString(ShellQuote(namespace))
	builder.WriteString("\n")
	builder.WriteString("app=")
	builder.WriteString(ShellQuote(appName))
	builder.WriteString("\n")
	builder.WriteString("cleanup() { rm -f \"$tmp\"; }\n")
	builder.WriteString("trap cleanup EXIT\n\n")
	builder.WriteString("cat > \"$tmp\" <<'NOPSAI_K8S_RUNNER_MANIFEST'\n")
	builder.WriteString(manifest)
	if !strings.HasSuffix(manifest, "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString("NOPSAI_K8S_RUNNER_MANIFEST\n\n")
	builder.WriteString("echo \"Applying NopsAI Kubernetes runner manifest...\"\n")
	builder.WriteString("kubectl apply -f \"$tmp\"\n")
	builder.WriteString("echo \"Waiting for Kubernetes runner rollout...\"\n")
	builder.WriteString("if ! kubectl -n \"$ns\" rollout status deployment/\"$app\" --timeout=120s; then\n")
	builder.WriteString("  echo \"Runner deployment did not become ready. Diagnostics:\" >&2\n")
	builder.WriteString("  kubectl -n \"$ns\" get pods -l app.kubernetes.io/instance=\"$app\" -o wide >&2 || true\n")
	builder.WriteString("  kubectl -n \"$ns\" describe deployment/\"$app\" >&2 || true\n")
	builder.WriteString("  kubectl -n \"$ns\" logs deployment/\"$app\" --tail=120 >&2 || true\n")
	builder.WriteString("  exit 1\n")
	builder.WriteString("fi\n")
	builder.WriteString("echo \"Recent runner logs:\"\n")
	builder.WriteString("kubectl -n \"$ns\" logs deployment/\"$app\" --tail=40 || true\n")
	builder.WriteString("echo \"Refresh System > Dispatcher to confirm registration. If no registration appears, run: kubectl -n $ns logs -f deployment/$app\"\n")
	return builder.String()
}

func buildKubernetesManifest(namespace, serviceAccount, runnerID, runnerImage string, env, secretEnv map[string]string) string {
	appName := kubernetesManifestName("nopsai-k8s-runner", runnerID)
	configMapName := kubernetesManifestName(appName, "config")
	secretName := kubernetesManifestName(appName, "secret")
	labels := map[string]string{
		"app.kubernetes.io/name":      "nopsai-k8s-runner",
		"app.kubernetes.io/component": "runner",
		"nopsai.io/runner-id":         kubernetesManifestLabelValue(runnerID),
	}
	podLabels := cloneStringMap(labels)
	podLabels["app.kubernetes.io/instance"] = appName

	docs := []interface{}{
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": namespace,
				"labels": map[string]string{
					"app.kubernetes.io/name": "nopsai",
				},
			},
		},
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]interface{}{
				"name":      serviceAccount,
				"namespace": namespace,
				"labels":    labels,
			},
		},
		map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "Role",
			"metadata": map[string]interface{}{
				"name":      appName,
				"namespace": namespace,
				"labels":    labels,
			},
			"rules": []map[string]interface{}{
				{"apiGroups": []string{""}, "resources": []string{"pods"}, "verbs": []string{"get", "list", "watch", "create", "delete"}},
				{"apiGroups": []string{""}, "resources": []string{"pods/log"}, "verbs": []string{"get", "list", "watch"}},
				{"apiGroups": []string{""}, "resources": []string{"pods/exec"}, "verbs": []string{"get", "create"}},
				{"apiGroups": []string{""}, "resources": []string{"persistentvolumeclaims"}, "verbs": []string{"get", "list", "watch", "create", "delete"}},
				{"apiGroups": []string{""}, "resources": []string{"events"}, "verbs": []string{"get", "list", "watch"}},
			},
		},
		map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "RoleBinding",
			"metadata": map[string]interface{}{
				"name":      appName,
				"namespace": namespace,
				"labels":    labels,
			},
			"subjects": []map[string]string{{
				"kind":      "ServiceAccount",
				"name":      serviceAccount,
				"namespace": namespace,
			}},
			"roleRef": map[string]string{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     appName,
			},
		},
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"type":       "Opaque",
			"metadata": map[string]interface{}{
				"name":      secretName,
				"namespace": namespace,
				"labels":    labels,
			},
			"stringData": secretEnv,
		},
		map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      configMapName,
				"namespace": namespace,
				"labels":    labels,
			},
			"data": env,
		},
		map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      appName,
				"namespace": namespace,
				"labels":    labels,
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"selector": map[string]interface{}{
					"matchLabels": podLabels,
				},
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": podLabels,
					},
					"spec": map[string]interface{}{
						"serviceAccountName": serviceAccount,
						"containers": []map[string]interface{}{{
							"name":            "runner",
							"image":           runnerImage,
							"imagePullPolicy": firstNonEmptyString(env["KUBERNETES_DEFAULT_IMAGE_PULL_POLICY"], "IfNotPresent"),
							"envFrom": []map[string]interface{}{
								{"configMapRef": map[string]string{"name": configMapName}},
								{"secretRef": map[string]string{"name": secretName}},
							},
						}},
					},
				},
			},
		},
	}

	var builder strings.Builder
	for i, doc := range docs {
		if i > 0 {
			builder.WriteString("---\n")
		}
		data, err := yaml.Marshal(doc)
		if err != nil {
			continue
		}
		builder.Write(data)
	}
	return builder.String()
}

func kubernetesManifestName(parts ...string) string {
	name := strings.ToLower(strings.Join(parts, "-"))
	name = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		name = "nopsai"
	}
	if len(name) <= 63 {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	return strings.Trim(name[:52], "-") + "-" + fmt.Sprintf("%x", sum[:5])
}

func kubernetesManifestLabelValue(value string) string {
	label := kubernetesManifestName(value)
	if len(label) <= 63 {
		return label
	}
	return label[:63]
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func boolPtrValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
