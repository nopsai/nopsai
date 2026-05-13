package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"nopsai/config"
	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	"nopsai/pkg/proxyhttp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v3"

	aaaauthz "nopsai/services/aaa/pkg/authz"
	"nopsai/services/aaa/pkg/model"
	aaastore "nopsai/services/aaa/pkg/store"
	"nopsai/services/nopsai/pkg/aaaclient"
	"nopsai/services/nopsai/pkg/audit"
	"nopsai/services/nopsai/pkg/auth"
	"nopsai/services/nopsai/pkg/authz"
	"nopsai/services/nopsai/pkg/routeauthz"
	"nopsai/services/nopsai/pkg/store"
	"nopsai/services/nopsai/pkg/validation"
)

const (
	defaultAdminSub          = "admin"
	defaultAdminEmail        = "admin@example.com"
	defaultAdminRole         = "nopsai-admin"
	defaultAdminPasswordHash = "$2a$10$ueFOcGRKCWDeOaTwy1hmQ.WjQ70Yu8JJLcl8ZvJprx7HPKArt8ESC" // password: admin
	defaultAdminID           = "00000000-0000-0000-0000-00000000000a"
)

// WebSocket Hub implementation

type App struct {
	db          *pgxpool.Pool
	cfg         *config.Config
	dispatcher  proto.DispatcherServiceClient
	encKey      []byte
	httpClient  *http.Client
	store       store.Store
	configPath  string
	cfgMu       sync.RWMutex
	idleTimeout time.Duration

	configSyncMu     sync.Mutex
	configSyncStatus ConfigSyncStatus
	envFilePath      string

	authService   *auth.Service
	aaaClient     aaaAuthorizer
	aaaLocal      aaaAuthorizer
	aaaRemoteMu   sync.Mutex
	aaaRetryAfter time.Time
	authz         *authz.Enforcer
	auditLogger   *audit.Logger
	tokenActivity sync.Map
}

type LogLine = models.LogLine
type RunListItem = models.RunListItem
type StepConfiguration = models.StepConfiguration
type StepDetail = models.StepDetail
type TaskDetail = models.TaskDetail
type ParentRunInfo = models.ParentRunInfo
type RunDetail = models.RunDetail
type StepStatusUpdate = models.StepStatusUpdate
type SecretRequest = models.SecretRequest
type VariableRequest = models.VariableRequest
type ScopeResponse = models.ScopeResponse
type VariableValueResponse = models.VariableValueResponse
type PipelineRequest = models.PipelineRequest
type TriggerOverrideRequest = models.TriggerOverrideRequest
type FinalizeRequest = models.FinalizeRequest
type Group = models.Group

type ConfigSyncStatus struct {
	Status      string         `json:"status"`
	Message     string         `json:"message,omitempty"`
	Details     map[string]int `json:"details,omitempty"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
}

type authLoginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type authRefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type authLoginResponse struct {
	AccessToken  string                    `json:"access_token"`
	RefreshToken string                    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time                 `json:"expires_at"`
	Roles        []string                  `json:"roles,omitempty"`
	Provider     string                    `json:"provider,omitempty"`
	Email        string                    `json:"email,omitempty"`
	Sub          string                    `json:"sub,omitempty"`
	Capabilities *authCapabilitiesResponse `json:"capabilities,omitempty"`
}

type authCapabilitiesResponse struct {
	Pipelines authResourceCapabilities `json:"pipelines"`
	Steps     authResourceCapabilities `json:"steps"`
	Triggers  authReadCapabilities     `json:"triggers"`
	Scopes    authReadCapabilities     `json:"scopes"`
	System    authSystemCapabilities   `json:"system"`
}

type authResourceCapabilities struct {
	Write  bool `json:"write"`
	Delete bool `json:"delete"`
}

type authReadCapabilities struct {
	Read   bool `json:"read"`
	Write  bool `json:"write"`
	Delete bool `json:"delete"`
}

type authSystemCapabilities struct {
	ConfigRead       bool `json:"config_read"`
	ConfigWrite      bool `json:"config_write"`
	ConfigReposRead  bool `json:"config_repos_read"`
	ConfigReposWrite bool `json:"config_repos_write"`
	DispatcherRead   bool `json:"dispatcher_read"`
	DispatcherWrite  bool `json:"dispatcher_write"`
	Access           bool `json:"access"`
}

type authChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type authUpdateEmailRequest struct {
	Email string `json:"email"`
}

type userRoleBinding struct {
	Role string `json:"role"`
}

type userSummary struct {
	ID        string            `json:"id"`
	Sub       string            `json:"sub"`
	Email     string            `json:"email"`
	Provider  string            `json:"provider"`
	Status    string            `json:"status"`
	LastLogin *time.Time        `json:"last_login,omitempty"`
	Roles     []userRoleBinding `json:"roles,omitempty"`
}

type createUserRequest struct {
	Sub      string `json:"sub"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type updateUserRequest struct {
	Email    string `json:"email"`
	Status   string `json:"status"`
	Password string `json:"password"`
}

type userRoleRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

type createRoleRequest struct {
	Role         string `json:"role"`
	Name         string `json:"name"`
	Object       string `json:"obj"`
	Action       string `json:"act"`
	Effect       string `json:"effect"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
}

// Keep these local for now if not in models
type suiteCheckRunResponse struct {
	CheckRunID         int64  `json:"check_run_id"`
	HeadSHA            string `json:"head_sha"`
	PullRequestHeadRef string `json:"pull_request_head_ref,omitempty"`
	HeadBranch         string `json:"head_branch,omitempty"`
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)
var envKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func defaultPolicyName(obj, act string) string {
	cleanedObj := strings.Trim(strings.TrimSpace(obj), "/")
	base := cleanedObj
	if base == "" {
		base = "policy"
	} else {
		parts := strings.Split(base, "/")
		base = parts[len(parts)-1]
		if base == "" {
			base = cleanedObj
		}
	}
	action := strings.TrimSpace(act)
	if action == "" {
		action = "ANY"
	}
	return fmt.Sprintf("%s • %s", base, action)
}

func deriveTriggerEventID(gitContext map[string]string) string {
	if gitContext == nil {
		return ""
	}
	owner := strings.ToLower(strings.TrimSpace(gitContext["repo_owner"]))
	name := strings.ToLower(strings.TrimSpace(gitContext["repo_name"]))
	ref := strings.ToLower(strings.TrimSpace(gitContext["ref"]))
	sha := strings.ToLower(strings.TrimSpace(gitContext["commit_sha"]))
	if owner == "" && name == "" && ref == "" && sha == "" {
		return ""
	}
	return fmt.Sprintf("%s|%s|%s|%s", owner, name, ref, sha)
}

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

func (a *App) setConfigSyncStatus(status ConfigSyncStatus) {
	a.configSyncMu.Lock()
	defer a.configSyncMu.Unlock()
	a.configSyncStatus = cloneConfigSyncStatus(status)
}

func (a *App) getConfigSyncStatus() ConfigSyncStatus {
	a.configSyncMu.Lock()
	defer a.configSyncMu.Unlock()
	return cloneConfigSyncStatus(a.configSyncStatus)
}

func cloneConfigSyncStatus(status ConfigSyncStatus) ConfigSyncStatus {
	statusCopy := status
	if status.Details != nil {
		detailsCopy := make(map[string]int, len(status.Details))
		for k, v := range status.Details {
			detailsCopy[k] = v
		}
		statusCopy.Details = detailsCopy
	}
	if status.StartedAt != nil {
		startedAt := *status.StartedAt
		statusCopy.StartedAt = &startedAt
	}
	if status.CompletedAt != nil {
		completedAt := *status.CompletedAt
		statusCopy.CompletedAt = &completedAt
	}
	return statusCopy
}

func (a *App) startConfigSync(startedAt time.Time) (ConfigSyncStatus, bool) {
	a.configSyncMu.Lock()
	defer a.configSyncMu.Unlock()

	if strings.EqualFold(a.configSyncStatus.Status, "running") {
		return cloneConfigSyncStatus(a.configSyncStatus), false
	}

	status := ConfigSyncStatus{
		Status:    "running",
		Message:   "Configuration synchronization started.",
		StartedAt: &startedAt,
	}
	a.configSyncStatus = cloneConfigSyncStatus(status)
	return cloneConfigSyncStatus(a.configSyncStatus), true
}

// This new helper function fetches and builds a RunListItem for a given run ID.
func (a *App) getRunListItem(runID string) (*RunListItem, error) {
	return a.store.GetRunListItem(context.Background(), runID)
}

// The broadcast function is updated to send a more specific 'run_summary_update' message
// with the full RunListItem as the payload.

func matchBranchPattern(pattern, name string) bool {
	if pattern == "" {
		return false
	}
	var builder strings.Builder
	builder.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				builder.WriteString(".*")
				i++
			} else {
				builder.WriteString("[^/]*")
			}
		case '?':
			builder.WriteString(".")
		case '.', '(', ')', '+', '|', '^', '$', '{', '}', '[', ']', '\\':
			builder.WriteByte('\\')
			builder.WriteByte(pattern[i])
		default:
			builder.WriteByte(pattern[i])
		}
	}
	builder.WriteString("$")
	re, err := regexp.Compile(builder.String())
	if err != nil {
		return pattern == name
	}
	return re.MatchString(name)
}

var (
	errManifestNotFound = errors.New("manifest not found")
	errPipelineNotFound = errors.New("pipeline not found")
)

// corsMiddleware allows cross-origin requests from the UI development server.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*") // Allow any origin for simplicity in POC
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

type requestContextKey string

const (
	ctxKeyRequestID requestContextKey = "request-id"
)

func isPublicPath(path string) bool {
	switch path {
	case "/v1/auth/login", "/v1/auth/refresh", "/v1/auth/logout", "/v1/git/events":
		return true
	default:
		return false
	}
}

func isDispatcherInternalPath(path string) bool {
	switch {
	case path == "/v1/run":
		return true
	case strings.HasPrefix(path, "/v1/pipelines/"):
		return true
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/logs/ingest"):
		return true
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/finalize"):
		return true
	case strings.HasPrefix(path, "/v1/runs/") && strings.HasSuffix(path, "/status"):
		return true
	case strings.HasPrefix(path, "/v1/runs/") && strings.Contains(path, "/steps/") && strings.Contains(path, "/tasks/"):
		return true
	default:
		return false
	}
}

func isDispatcherInternalClaims(claims *auth.Claims) bool {
	if claims == nil {
		return false
	}
	return strings.TrimSpace(claims.Sub) == "dispatcher" && strings.TrimSpace(claims.Provider) == "internal-service"
}

func isTrustedInternalDispatcherRequest(r *http.Request) bool {
	if r == nil || !isDispatcherInternalPath(r.URL.Path) {
		return false
	}
	claims, ok := auth.ClaimsFromContext(r.Context())
	return ok && isDispatcherInternalClaims(claims)
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", reqID)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	length int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.status = code
	lrw.ResponseWriter.WriteHeader(code)
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	n, err := lrw.ResponseWriter.Write(b)
	lrw.length += n
	return n, err
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lrw, r)
		reqID, _ := r.Context().Value(ctxKeyRequestID).(string)
		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", lrw.status).
			Int("bytes", lrw.length).
			Str("request_id", reqID).
			Dur("duration_ms", time.Since(start)).
			Msg("http_request")
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error().Interface("panic", rec).Msg("panic recovered")
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *App) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if a.authService == nil {
			http.Error(w, "authentication not configured", http.StatusServiceUnavailable)
			return
		}
		authzHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		if authzHeader == "" || !strings.HasPrefix(strings.ToLower(authzHeader), "bearer ") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		token := strings.TrimSpace(authzHeader[len("Bearer "):])
		claims, err := a.authService.AuthenticateToken(r.Context(), token)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		if a.idleTimeout > 0 {
			now := time.Now()
			if lastRaw, ok := a.tokenActivity.Load(token); ok {
				if lastSeen, ok := lastRaw.(time.Time); ok && now.Sub(lastSeen) > a.idleTimeout {
					a.tokenActivity.Delete(token)
					http.Error(w, "session expired due to inactivity", http.StatusUnauthorized)
					return
				}
			}
			a.tokenActivity.Store(token, now)
		}

		ctx := auth.WithClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *App) authzMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok {
			http.Error(w, "missing claims", http.StatusUnauthorized)
			return
		}

		subject := a.buildAAASubject(claims)
		ctx := withAAASubject(r.Context(), subject)
		if isAuthenticatedOnlyPath(r.URL.Path) {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if !a.aaaAvailable() {
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return
		}

		action, resource, requiresFilter, err := routeauthz.MapRequest(r)
		if err != nil {
			http.Error(w, "invalid authorization mapping", http.StatusBadRequest)
			return
		}
		if action == "" || requiresFilter {
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		decision, err := a.aaaCheck(ctx, subject, action, resource, a.aaaRequestContext(r))
		if err != nil {
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return
		}
		if !decision.Allowed {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type auditRecorder struct {
	http.ResponseWriter
	status int
}

func (a *auditRecorder) WriteHeader(code int) {
	a.status = code
	a.ResponseWriter.WriteHeader(code)
}

func (a *App) auditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &auditRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		if a.auditLogger == nil {
			return
		}
		claims, _ := auth.ClaimsFromContext(r.Context())
		requestID, _ := r.Context().Value(ctxKeyRequestID).(string)

		entry := audit.Entry{
			ActorSub:   "",
			ActorEmail: "",
			Provider:   "",
			Action:     r.Method,
			Resource:   r.URL.Path,
			Result:     fmt.Sprintf("%d", rec.status),
			Metadata: map[string]any{
				"request_id": requestID,
				"remote_ip":  r.RemoteAddr,
			},
		}
		if claims != nil {
			entry.ActorSub = claims.Sub
			entry.ActorEmail = claims.Email
			entry.Provider = claims.Provider
		}
		_ = a.auditLogger.Write(r.Context(), entry)
	})
}

func validatePipeline(pipeline *models.Pipeline) error {
	return validation.ValidatePipeline(pipeline)
}

type systemConfigPayload struct {
	AgentNopsaiAPIURL         *string `json:"agent_nopsai_api_url"`
	GitBotNopsaiAPIURL        *string `json:"git_bot_nopsai_api_url"`
	NopsaiGitBotAPIURL        *string `json:"nopsai_git_bot_api_url"`
	AgentImage                *string `json:"agent_image"`
	DockerNetworkName         *string `json:"docker_network_name"`
	AutoRemovalAgentContainer *bool   `json:"auto_removal_agent_container"`
	DefaultPipelineTimeout    *string `json:"default_pipeline_timeout"`
	LLMAgentTimeout           *string `json:"llm_agent_timeout"`
}

func (a *App) buildSystemConfigResponse(cfg config.Config) map[string]interface{} {
	return map[string]interface{}{
		"agent_nopsai_api_url":         cfg.AgentNopsaiAPIURL,
		"git_bot_nopsai_api_url":       cfg.GitBotNopsaiAPIURL,
		"nopsai_git_bot_api_url":       cfg.NopsaiGitBotAPIURL,
		"agent_image":                  cfg.AgentImage,
		"docker_network_name":          cfg.DockerNetworkName,
		"auto_removal_agent_container": cfg.AutoRemovalAgentContainer,
		"default_pipeline_timeout":     cfg.DefaultPipelineTimeout,
		"llm_agent_timeout":            cfg.LLMAgentTimeout,
		"env_file_path":                a.envFilePath,
	}
}

func (a *App) applySystemConfig(payload systemConfigPayload) config.Config {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()

	if payload.AgentNopsaiAPIURL != nil {
		a.cfg.AgentNopsaiAPIURL = strings.TrimSpace(*payload.AgentNopsaiAPIURL)
	}
	if payload.GitBotNopsaiAPIURL != nil {
		a.cfg.GitBotNopsaiAPIURL = strings.TrimSpace(*payload.GitBotNopsaiAPIURL)
	}
	if payload.NopsaiGitBotAPIURL != nil {
		a.cfg.NopsaiGitBotAPIURL = strings.TrimSpace(*payload.NopsaiGitBotAPIURL)
	}
	if payload.AgentImage != nil {
		a.cfg.AgentImage = strings.TrimSpace(*payload.AgentImage)
	}
	if payload.DockerNetworkName != nil {
		a.cfg.DockerNetworkName = strings.TrimSpace(*payload.DockerNetworkName)
	}
	if payload.AutoRemovalAgentContainer != nil {
		a.cfg.AutoRemovalAgentContainer = *payload.AutoRemovalAgentContainer
	}
	if payload.DefaultPipelineTimeout != nil {
		a.cfg.DefaultPipelineTimeout = strings.TrimSpace(*payload.DefaultPipelineTimeout)
	}
	if payload.LLMAgentTimeout != nil {
		a.cfg.LLMAgentTimeout = strings.TrimSpace(*payload.LLMAgentTimeout)
	}

	return *a.cfg
}

func (a *App) persistSystemConfig(cfg config.Config, payload systemConfigPayload) error {
	if a.configPath == "" {
		return nil
	}

	existing := map[string]interface{}{}
	if contents, err := os.ReadFile(a.configPath); err == nil {
		if len(contents) > 0 {
			if unmarshalErr := yaml.Unmarshal(contents, &existing); unmarshalErr != nil {
				log.Warn().Err(unmarshalErr).Msg("Failed to parse existing config file; rewriting allowed fields")
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if payload.AgentImage != nil {
		existing["agent_image"] = cfg.AgentImage
	}
	if payload.AgentNopsaiAPIURL != nil {
		existing["agent_nopsai_api_url"] = cfg.AgentNopsaiAPIURL
	}
	if payload.GitBotNopsaiAPIURL != nil {
		existing["git_bot_nopsai_api_url"] = cfg.GitBotNopsaiAPIURL
	}
	if payload.NopsaiGitBotAPIURL != nil {
		existing["nopsai_git_bot_api_url"] = cfg.NopsaiGitBotAPIURL
	}
	if payload.DockerNetworkName != nil {
		existing["docker_network_name"] = cfg.DockerNetworkName
	}
	if payload.AutoRemovalAgentContainer != nil {
		existing["auto_removal_agent_container"] = cfg.AutoRemovalAgentContainer
	}
	if payload.DefaultPipelineTimeout != nil {
		existing["default_pipeline_timeout"] = cfg.DefaultPipelineTimeout
	}
	if payload.LLMAgentTimeout != nil {
		existing["llm_agent_timeout"] = cfg.LLMAgentTimeout
	}

	contents, err := yaml.Marshal(existing)
	if err != nil {
		return err
	}

	return os.WriteFile(a.configPath, contents, 0o644)
}

func (a *App) persistEnvOverrides(cfg config.Config, payload systemConfigPayload) error {
	if a.envFilePath == "" {
		return nil
	}

	updates := map[string]string{}

	if payload.AgentNopsaiAPIURL != nil {
		updates["AGENT_NOPSAI_API_URL"] = cfg.AgentNopsaiAPIURL
	}
	if payload.GitBotNopsaiAPIURL != nil {
		updates["GIT_BOT_NOPSAI_API_URL"] = cfg.GitBotNopsaiAPIURL
	}
	if payload.NopsaiGitBotAPIURL != nil {
		updates["NOPSAI_GIT_BOT_API_URL"] = cfg.NopsaiGitBotAPIURL
	}
	if payload.AgentImage != nil {
		updates["AGENT_IMAGE"] = cfg.AgentImage
	}
	if payload.DockerNetworkName != nil {
		updates["DOCKER_NETWORK_NAME"] = cfg.DockerNetworkName
	}
	if payload.AutoRemovalAgentContainer != nil {
		updates["AUTO_REMOVAL_AGENT_CONTAINER"] = strconv.FormatBool(cfg.AutoRemovalAgentContainer)
	}
	if payload.DefaultPipelineTimeout != nil {
		updates["DEFAULT_PIPELINE_TIMEOUT"] = cfg.DefaultPipelineTimeout
	}
	if payload.LLMAgentTimeout != nil {
		updates["LLM_AGENT_TIMEOUT"] = cfg.LLMAgentTimeout
	}

	if len(updates) == 0 {
		return nil
	}

	return writeEnvFile(a.envFilePath, updates)
}

func writeEnvFile(path string, updates map[string]string) error {
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

func (a *App) handleGetSystemConfig(w http.ResponseWriter, r *http.Request) {
	cfg := a.getConfigSnapshot()
	resp := a.buildSystemConfigResponse(cfg)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode system config response")
	}
}

func (a *App) handleUpdateSystemConfig(w http.ResponseWriter, r *http.Request) {
	var payload systemConfigPayload
	if err := httpapi.DecodeJSON(r, &payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	cfg := a.applySystemConfig(payload)
	if err := a.persistSystemConfig(cfg, payload); err != nil {
		log.Warn().Err(err).Msg("Failed to persist system config; keeping in-memory settings only")
	}
	if err := a.persistEnvOverrides(cfg, payload); err != nil {
		log.Warn().Err(err).Msg("Failed to persist .env overrides; keeping in-memory settings only")
	}

	resp := a.buildSystemConfigResponse(cfg)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode updated system config response")
	}
}

func (a *App) handleGetConfigSyncStatus(w http.ResponseWriter, r *http.Request) {
	status := a.getConfigSyncStatus()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Warn().Err(err).Msg("Failed to encode config sync status")
	}
}

func (a *App) handleDispatcherStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status, err := a.dispatcher.GetStatus(ctx, &emptypb.Empty{})
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch dispatcher status")
		http.Error(w, "Failed to fetch dispatcher status", http.StatusBadGateway)
		return
	}

	a.cfgMu.RLock()
	routing := a.cfg.DispatcherRouting
	a.cfgMu.RUnlock()

	runners := make([]map[string]interface{}, 0, len(status.GetRunners()))
	for _, runner := range status.GetRunners() {
		runners = append(runners, map[string]interface{}{
			"runner_id":           runner.GetRunnerId(),
			"scopes":              runner.GetScopes(),
			"capacity":            runner.GetCapacity(),
			"active_jobs":         runner.GetActiveJobs(),
			"inflight_jobs":       runner.GetInflightJobs(),
			"last_heartbeat_unix": runner.GetLastHeartbeatUnix(),
			"metadata":            runner.GetMetadata(),
			"allow_dispatch":      runner.GetAllowDispatch(),
		})
	}

	resp := map[string]interface{}{
		"queued_jobs": status.GetQueuedJobs(),
		"runners":     runners,
	}
	if len(routing) > 0 {
		resp["routing"] = routing
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Warn().Err(err).Msg("Failed to encode dispatcher status")
	}
}

func (a *App) handleUpdateRunnerDispatch(w http.ResponseWriter, r *http.Request) {
	runnerID := strings.TrimSpace(r.PathValue("runnerID"))
	if runnerID == "" {
		http.Error(w, "runner_id is required", http.StatusBadRequest)
		return
	}

	var payload struct {
		AllowDispatch *bool  `json:"allow_dispatch"`
		ConnectionID  string `json:"connection_id"`
	}
	if err := httpapi.DecodeJSON(r, &payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	if payload.AllowDispatch == nil {
		http.Error(w, "allow_dispatch is required", http.StatusBadRequest)
		return
	}

	resp, err := a.dispatcher.UpdateRunnerDispatch(r.Context(), &proto.UpdateRunnerDispatchRequest{
		RunnerId:      runnerID,
		AllowDispatch: *payload.AllowDispatch,
		ConnectionId:  strings.TrimSpace(payload.ConnectionID),
	})
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("Failed to update runner dispatch state")
		statusCode := http.StatusBadGateway
		if st, ok := grpcstatus.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument:
				statusCode = http.StatusBadRequest
			case codes.NotFound:
				statusCode = http.StatusNotFound
			case codes.Unavailable:
				statusCode = http.StatusBadGateway
			default:
				statusCode = http.StatusInternalServerError
			}
			http.Error(w, st.Message(), statusCode)
			return
		}
		http.Error(w, "Failed to update runner dispatch", statusCode)
		return
	}

	if resp == nil || resp.Runner == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp.Runner); err != nil {
		log.Warn().Err(err).Msg("Failed to encode runner dispatch response")
	}
}

func (a *App) prepareSecretsForPipeline(pipeline models.Pipeline, gitContext map[string]string, scope string) (map[string]string, error) {
	requiredSecrets := make(map[string]struct{})
	for _, step := range pipeline.Steps {
		for _, secretName := range step.GetSecrets() {
			requiredSecrets[secretName] = struct{}{}
		}
	}

	if len(requiredSecrets) == 0 {
		return nil, nil
	}

	finalSecrets := make(map[string]string)
	repoFullName := fmt.Sprintf("%s/%s", gitContext["repo_owner"], gitContext["repo_name"])

	for secretName := range requiredSecrets {
		encryptedValue, found, err := a.findEncryptedSecret(secretName, repoFullName, scope)
		if err != nil {
			return nil, fmt.Errorf("pipeline aborted: failed to resolve secret '%s': %w", secretName, err)
		}
		if !found {
			if scope != "" {
				return nil, fmt.Errorf("pipeline aborted: required secret '%s' not found for scope '%s'", secretName, scope)
			}
			return nil, fmt.Errorf("pipeline aborted: required secret '%s' not found in the default scope", secretName)
		}

		decryptedValue, decryptErr := a.decrypt(encryptedValue)
		if decryptErr != nil {
			log.Error().Err(decryptErr).Str("secret_name", secretName).Msg("Failed to decrypt secret; this will cause a failure.")
			return nil, fmt.Errorf("pipeline aborted: failed to decrypt secret '%s'", secretName)
		}
		finalSecrets[secretName] = decryptedValue
	}

	return finalSecrets, nil
}

func (a *App) handleConfigSync(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	status, started := a.startConfigSync(startedAt)
	if !started {
		http.Error(w, "A configuration sync is already in progress", http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Warn().Err(err).Msg("Failed to encode config sync response")
	}

	go func(started time.Time) {
		log.Info().Msg("Starting synchronization for all config repositories")
		a.setConfigSyncStatus(a.syncAllConfigRepositories(context.Background(), started))
	}(startedAt)
}

func (a *App) syncAllConfigRepositories(ctx context.Context, started time.Time) ConfigSyncStatus {
	details := map[string]int{
		"repositories_synced": 0,
		"repositories_failed": 0,
	}
	var messages []string

	enabled := true
	syncedRepoIDs := map[int64]struct{}{}
	syncRepos := func(scopeType string) {
		repos, err := a.store.ListConfigRepositories(ctx, models.ConfigRepositoryFilter{ScopeType: scopeType, Enabled: &enabled})
		if err != nil {
			details["repositories_failed"]++
			messages = append(messages, fmt.Sprintf("%s:*: %v", scopeType, err))
			return
		}
		for _, repo := range repos {
			if _, alreadySynced := syncedRepoIDs[repo.ID]; alreadySynced {
				continue
			}
			syncedRepoIDs[repo.ID] = struct{}{}
			repoStartedAt := time.Now()
			if err := a.store.UpdateConfigRepositorySyncStatus(ctx, repo.ID, "running", "Configuration synchronization started.", repo.LastSyncCommitSHA, &repoStartedAt, nil); err != nil {
				details["repositories_failed"]++
				messages = append(messages, fmt.Sprintf("%s:%s: %v", repo.ScopeType, repo.ScopeID, err))
				continue
			}
			status := a.syncConfigRepository(ctx, repo, repoStartedAt)
			if strings.EqualFold(status.Status, "success") {
				details["repositories_synced"]++
				for key, value := range status.Details {
					details[key] += value
				}
				continue
			}
			details["repositories_failed"]++
			messages = append(messages, fmt.Sprintf("%s:%s: %s", repo.ScopeType, repo.ScopeID, status.Message))
		}
	}

	syncRepos(models.ConfigRepositoryScopeSystem)
	for {
		before := len(syncedRepoIDs)
		syncRepos(models.ConfigRepositoryScopeFolder)
		if len(syncedRepoIDs) == before {
			break
		}
	}

	completedAt := time.Now()
	if details["repositories_failed"] > 0 {
		return ConfigSyncStatus{
			Status:      "error",
			Message:     "One or more config repositories failed to synchronize: " + strings.Join(messages, "; "),
			Details:     details,
			StartedAt:   &started,
			CompletedAt: &completedAt,
		}
	}

	return ConfigSyncStatus{
		Status:      "success",
		Message:     "Configuration synchronization completed successfully.",
		Details:     details,
		StartedAt:   &started,
		CompletedAt: &completedAt,
	}
}

func (a *App) syncConfigRepository(ctx context.Context, repo models.ConfigRepository, started time.Time) ConfigSyncStatus {
	log.Info().
		Int64("config_repo_id", repo.ID).
		Str("scope_type", repo.ScopeType).
		Str("scope_id", repo.ScopeID).
		Msg("Starting configuration synchronization from Git")

	details, commitSHA, syncErr := a.syncConfigurationFromGit(ctx, repo)
	completedAt := time.Now()
	if syncErr != nil {
		message := fmt.Sprintf("Configuration synchronization failed: %v", syncErr)
		log.Error().Err(syncErr).Int64("config_repo_id", repo.ID).Msg("Configuration synchronization failed")
		if err := a.store.UpdateConfigRepositorySyncStatus(ctx, repo.ID, "error", message, commitSHA, &started, &completedAt); err != nil {
			log.Warn().Err(err).Int64("config_repo_id", repo.ID).Msg("Failed to update config repository sync status")
		}
		return ConfigSyncStatus{
			Status:      "error",
			Message:     message,
			StartedAt:   &started,
			CompletedAt: &completedAt,
		}
	}

	message := "Configuration synchronization completed successfully."
	if err := a.store.UpdateConfigRepositorySyncStatus(ctx, repo.ID, "success", message, commitSHA, &started, &completedAt); err != nil {
		log.Warn().Err(err).Int64("config_repo_id", repo.ID).Msg("Failed to update config repository sync status")
	}
	log.Info().Interface("details", details).Int64("config_repo_id", repo.ID).Msg("Configuration synchronization succeeded")
	return ConfigSyncStatus{
		Status:      "success",
		Message:     message,
		Details:     details,
		StartedAt:   &started,
		CompletedAt: &completedAt,
	}
}

func (a *App) syncConfigurationFromGit(ctx context.Context, binding models.ConfigRepository) (map[string]int, string, error) {
	details := map[string]int{
		"pipelines_synced":           0,
		"steps_synced":               0,
		"general_vars_synced":        0,
		"repo_vars_synced":           0,
		"triggers_synced":            0,
		"config_repositories_synced": 0,
		"run_groups_created":         0,
		"run_groups_updated":         0,
	}

	repoURL := strings.TrimSpace(binding.RepoURL)
	if repoURL == "" {
		return nil, "", fmt.Errorf("config repository URL is not configured")
	}
	branch := strings.TrimSpace(binding.Branch)
	if branch == "" {
		branch = "main"
	}
	basePath := normalizeConfigRepositoryBasePathValue(binding.BasePath)
	commitSHA := ""
	boundFolder := strings.Trim(strings.TrimSpace(binding.ScopeID), "/")
	if binding.ScopeType == models.ConfigRepositoryScopeFolder && boundFolder == "" {
		return nil, commitSHA, fmt.Errorf("group-scoped config repository is missing its scope_id")
	}

	owner, repo, err := parseGitHubRepoURL(repoURL)
	if err != nil {
		return nil, commitSHA, fmt.Errorf("failed to parse config repository URL: %w", err)
	}
	if err := a.ensureConfigRepoAccessible(owner, repo); err != nil {
		return nil, commitSHA, err
	}

	// --- 1. Fetch all configurations from Git ---

	pipelineDir := configRepoJoinPath(basePath, "pipelines")
	stepDir := configRepoJoinPath(basePath, "steps")
	triggerDir := configRepoJoinPath(basePath, "triggers")
	environmentDir := configRepoJoinPath(basePath, "environments")
	pipelineRunDir := configRepoJoinPath(basePath, "pipelineruns")
	configRepositoryDir := configRepoJoinPath(basePath, "config-repositories")

	pipelineFiles, err := a.requestGitBotDirectory(owner, repo, branch, pipelineDir)
	if err != nil {
		return nil, commitSHA, fmt.Errorf("failed to fetch pipeline definitions: %w", err)
	}
	stepFiles, err := a.requestGitBotDirectory(owner, repo, branch, stepDir)
	if err != nil {
		return nil, commitSHA, fmt.Errorf("failed to fetch reusable steps: %w", err)
	}
	triggerFiles, err := a.requestGitBotDirectory(owner, repo, branch, triggerDir)
	if err != nil {
		return nil, commitSHA, fmt.Errorf("failed to fetch trigger manifests: %w", err)
	}
	environmentFiles, err := a.requestGitBotDirectory(owner, repo, branch, environmentDir)
	if err != nil {
		return nil, commitSHA, fmt.Errorf("failed to fetch environment definitions: %w", err)
	}

	pipelineRunFiles, err := a.requestGitBotDirectory(owner, repo, branch, pipelineRunDir)
	if err != nil {
		return nil, commitSHA, fmt.Errorf("failed to fetch pipeline run structure definitions: %w", err)
	}

	configRepositoryFiles, err := a.requestGitBotDirectory(owner, repo, branch, configRepositoryDir)
	if err != nil {
		return nil, commitSHA, fmt.Errorf("failed to fetch config repository bindings: %w", err)
	}

	var pipelineRunStructure map[string]*pipelineRunStructureNode
	for path, content := range pipelineRunFiles {
		normalized := filepath.ToSlash(path)
		rel, ok := relativeConfigPath(normalized, pipelineRunDir)
		if !ok {
			continue
		}
		if rel == "structure.yaml" || rel == "structure.yml" {
			parsed, err := parsePipelineRunStructure(content)
			if err != nil {
				return nil, commitSHA, fmt.Errorf("failed to parse pipeline run structure '%s': %w", normalized, err)
			}
			if binding.ScopeType == models.ConfigRepositoryScopeFolder {
				parsed, err = normalizePipelineRunStructureForFolder(boundFolder, parsed)
				if err != nil {
					return nil, commitSHA, fmt.Errorf("failed to normalize pipeline run structure '%s': %w", normalized, err)
				}
			}
			pipelineRunStructure = parsed
			break
		}
	}

	// --- 2. Parse Files ---

	configRepositories := make(map[string]storedConfigRepository)
	for path, content := range configRepositoryFiles {
		normalized := filepath.ToSlash(path)
		rel, ok := relativeConfigPath(normalized, configRepositoryDir)
		if !ok {
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}

		scopeType, scopeID, err := parseConfigRepositoryBindingPath(rel)
		if err != nil {
			return nil, commitSHA, fmt.Errorf("invalid config repository binding '%s': %w", normalized, err)
		}

		var file configRepositoryBindingFile
		if err := yaml.Unmarshal([]byte(content), &file); err != nil {
			return nil, commitSHA, fmt.Errorf("failed to parse config repository binding '%s': %w", normalized, err)
		}
		if err := validateConfigRepositoryBindingFile(file, scopeType, scopeID, normalized); err != nil {
			return nil, commitSHA, err
		}
		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			scopeID, err = normalizeConfigPathForFolder(boundFolder, scopeID)
			if err != nil {
				return nil, commitSHA, fmt.Errorf("invalid group-scoped config repository binding '%s': %w", normalized, err)
			}
		}
		basePath, err := normalizeConfigRepositoryBasePathForRequest(file.BasePath)
		if err != nil {
			return nil, commitSHA, fmt.Errorf("invalid base_path in config repository binding '%s': %w", normalized, err)
		}
		enabled := true
		if file.Enabled != nil {
			enabled = *file.Enabled
		}
		branch := strings.TrimSpace(file.Branch)
		if branch == "" {
			branch = "main"
		}

		key := scopeType + "/" + scopeID
		if _, exists := configRepositories[key]; exists {
			return nil, commitSHA, fmt.Errorf("duplicate config repository binding for '%s' detected", key)
		}
		configRepositories[key] = storedConfigRepository{
			scopeType:  scopeType,
			scopeID:    scopeID,
			repoURL:    strings.TrimSpace(file.RepoURL),
			branch:     branch,
			basePath:   basePath,
			enabled:    enabled,
			sourcePath: normalized,
		}
	}

	pipelines := make(map[string]storedPipeline)
	for path, content := range pipelineFiles {
		normalized := filepath.ToSlash(path)
		rel, ok := relativeConfigPath(normalized, pipelineDir)
		if !ok {
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}

		var pipeline models.Pipeline
		if err := yaml.Unmarshal([]byte(content), &pipeline); err != nil {
			return nil, commitSHA, fmt.Errorf("failed to parse pipeline '%s': %w", normalized, err)
		}
		if err := validatePipeline(&pipeline); err != nil {
			return nil, commitSHA, fmt.Errorf("pipeline validation failed for '%s': %w", normalized, err)
		}

		pipelinePath, fileBase, _, err := splitPipelineIdentifier(rel)
		if err != nil {
			return nil, commitSHA, fmt.Errorf("invalid pipeline path '%s': %w", normalized, err)
		}
		if pipeline.Name != fileBase {
			return nil, commitSHA, fmt.Errorf("pipeline '%s' name '%s' must match file name '%s'", normalized, pipeline.Name, fileBase)
		}

		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			targetID, err := normalizeConfigPathForFolder(boundFolder, rel)
			if err != nil {
				return nil, commitSHA, fmt.Errorf("invalid group-scoped pipeline path '%s': %w", normalized, err)
			}
			pipelinePath, fileBase, _, err = splitPipelineIdentifier(targetID)
			if err != nil {
				return nil, commitSHA, fmt.Errorf("invalid normalized pipeline path '%s': %w", targetID, err)
			}
		}

		key := buildPipelineIdentifier(pipelinePath, fileBase)
		if _, exists := pipelines[key]; exists {
			return nil, commitSHA, fmt.Errorf("duplicate pipeline '%s' detected in config repository", key)
		}

		pipelines[key] = storedPipeline{
			definition: content,
			version:    normalizePipelineVersion(pipeline.Version),
			path:       pipelinePath,
			name:       fileBase,
			sourcePath: normalized,
		}
	}

	steps := make(map[string]storedStep)
	for path, content := range stepFiles {
		normalized := filepath.ToSlash(path)
		rel, ok := relativeConfigPath(normalized, stepDir)
		if !ok {
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}

		var step models.PipelineStep
		if err := yaml.Unmarshal([]byte(content), &step); err != nil {
			return nil, commitSHA, fmt.Errorf("failed to parse reusable step '%s': %w", normalized, err)
		}
		stepName := step.GetName()
		if stepName == "" {
			return nil, commitSHA, fmt.Errorf("reusable step '%s' is missing the required 'name' field", normalized)
		}

		stepPath, fileBase, _, err := splitStepIdentifier(rel)
		if err != nil {
			return nil, commitSHA, fmt.Errorf("invalid reusable step path '%s': %w", normalized, err)
		}
		if stepName != fileBase {
			return nil, commitSHA, fmt.Errorf("reusable step '%s' name '%s' must match file name '%s'", normalized, stepName, fileBase)
		}

		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			targetID, err := normalizeConfigPathForFolder(boundFolder, rel)
			if err != nil {
				return nil, commitSHA, fmt.Errorf("invalid group-scoped reusable step path '%s': %w", normalized, err)
			}
			stepPath, fileBase, _, err = splitStepIdentifier(targetID)
			if err != nil {
				return nil, commitSHA, fmt.Errorf("invalid normalized reusable step path '%s': %w", targetID, err)
			}
		}

		key := buildStepIdentifier(stepPath, fileBase)
		if _, exists := steps[key]; exists {
			return nil, commitSHA, fmt.Errorf("duplicate reusable step '%s' detected in config repository", key)
		}

		steps[key] = storedStep{
			definition: content,
			path:       stepPath,
			name:       fileBase,
			sourcePath: normalized,
		}
	}

	generalEnvs := make(map[generalEnvKey]storedEnvVar)
	repoEnvs := make(map[repoEnvKey]storedEnvVar)

	for path, content := range environmentFiles {
		normalized := filepath.ToSlash(path)
		rel, ok := relativeConfigPath(normalized, environmentDir)
		if !ok {
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") {
			continue
		}

		envPath, ok, err := parseEnvironmentFilePath(rel)
		if err != nil {
			return nil, commitSHA, fmt.Errorf("invalid environment file '%s': %w", normalized, err)
		}
		if !ok {
			continue
		}
		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			envPath, err = normalizeConfigPathForFolder(boundFolder, envPath)
			if err != nil {
				return nil, commitSHA, fmt.Errorf("invalid group-scoped environment path '%s': %w", normalized, err)
			}
		}

		var raw map[string]interface{}
		if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
			return nil, commitSHA, fmt.Errorf("failed to parse environment file '%s': %w", normalized, err)
		}

		for key, value := range raw {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				return nil, commitSHA, fmt.Errorf("environment file '%s' contains an empty key", normalized)
			}

			strValue, ok := value.(string)
			if !ok {
				return nil, commitSHA, fmt.Errorf("environment entry '%s' in '%s' must be a string", trimmedKey, normalized)
			}

			parts := strings.Split(trimmedKey, "/")
			switch len(parts) {
			case 1:
				gKey := generalEnvKey{envPath: envPath, name: trimmedKey}
				if _, exists := generalEnvs[gKey]; exists {
					return nil, commitSHA, fmt.Errorf("duplicate environment variable '%s' for '%s' detected", trimmedKey, envPath)
				}
				generalEnvs[gKey] = storedEnvVar{value: strValue, sourcePath: normalized}
			case 3:
				repoName := fmt.Sprintf("%s/%s", strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
				varName := strings.TrimSpace(parts[2])
				if repoName == "" || varName == "" {
					return nil, commitSHA, fmt.Errorf("invalid repository-scoped environment key '%s' in '%s'", trimmedKey, normalized)
				}
				if binding.ScopeType == models.ConfigRepositoryScopeFolder {
					repoName, err = normalizeConfigPathForFolder(boundFolder, repoName)
					if err != nil {
						return nil, commitSHA, fmt.Errorf("invalid group-scoped repository variable key '%s' in '%s': %w", trimmedKey, normalized, err)
					}
				}
				rKey := repoEnvKey{repo: repoName, envPath: envPath, name: varName}
				if _, exists := repoEnvs[rKey]; exists {
					return nil, commitSHA, fmt.Errorf("duplicate repository environment variable '%s' for '%s' detected", trimmedKey, envPath)
				}
				repoEnvs[rKey] = storedEnvVar{value: strValue, sourcePath: normalized}
			default:
				return nil, commitSHA, fmt.Errorf("environment key '%s' in '%s' has an unsupported format", trimmedKey, normalized)
			}
		}
	}

	triggers := make(map[string]storedTrigger)
	for path, content := range triggerFiles {
		normalized := filepath.ToSlash(path)
		rel, ok := relativeConfigPath(normalized, triggerDir)
		if !ok {
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}

		repoKey := strings.TrimSuffix(rel, filepath.Ext(rel))
		repoKey = strings.Trim(repoKey, "/")
		if repoKey == "" {
			return nil, commitSHA, fmt.Errorf("trigger file '%s' does not specify a repository", normalized)
		}
		if strings.Contains(repoKey, "..") {
			return nil, commitSHA, fmt.Errorf("trigger file '%s' contains invalid path segments", normalized)
		}
		repoKey = filepath.ToSlash(repoKey)
		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			repoKey, err = normalizeConfigPathForFolder(boundFolder, repoKey)
			if err != nil {
				return nil, commitSHA, fmt.Errorf("invalid group-scoped trigger path '%s': %w", normalized, err)
			}
		}

		if err := yaml.Unmarshal([]byte(content), &models.Manifest{}); err != nil {
			return nil, commitSHA, fmt.Errorf("failed to parse trigger manifest '%s': %w", normalized, err)
		}

		if _, exists := triggers[repoKey]; exists {
			return nil, commitSHA, fmt.Errorf("duplicate trigger manifest for repository '%s' detected", repoKey)
		}

		triggers[repoKey] = storedTrigger{definition: content, sourcePath: normalized}
	}

	// --- 3. Database Transaction (Upsert + Prune) ---
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, commitSHA, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	overrideScopes, err := configRepositoryOverrideScopes(ctx, tx, binding, configRepositories)
	if err != nil {
		return nil, commitSHA, err
	}
	filterDelegatedConfigResources(binding, overrideScopes, pipelines, steps, generalEnvs, repoEnvs, triggers)
	effectivePipelineRunStructure, err := effectivePipelineRunStructureForConfigSync(binding, configRepositories, pipelineRunStructure, overrideScopes)
	if err != nil {
		return nil, commitSHA, err
	}

	const pipelineUpsert = `INSERT INTO pipelines (
			path, name, version, definition, source,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at
		) VALUES ($1, $2, $3, $4, 'git', $5, $6, $7, TRUE, NOW())
		ON CONFLICT (path, name) DO UPDATE SET
			version = EXCLUDED.version,
			definition = EXCLUDED.definition,
			source = 'git',
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE,
			updated_at = NOW()`
	const stepUpsert = `INSERT INTO steps (
			path, name, definition, source,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at
		) VALUES ($1, $2, $3, 'git', $4, $5, $6, TRUE, NOW())
		ON CONFLICT (path, name) DO UPDATE SET
			definition = EXCLUDED.definition,
			source = 'git',
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE,
			updated_at = NOW()`
	const envUpsert = `INSERT INTO variables (
			name, value, repository_name, scope, source,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo, updated_at
		) VALUES ($1, $2, $3, $4, 'git', $5, $6, $7, TRUE, NOW())
		ON CONFLICT (name, repository_name, scope) DO UPDATE SET
			value = EXCLUDED.value,
			source = 'git',
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE,
			updated_at = NOW()`
	const triggerUpsert = `INSERT INTO triggers (
			repository_name, trigger_definition, source,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo
		) VALUES ($1, $2, 'git', $3, $4, $5, TRUE)
		ON CONFLICT (repository_name) DO UPDATE SET
			trigger_definition = EXCLUDED.trigger_definition,
			source = 'git',
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE`
	const configRepositoryUpsert = `INSERT INTO config_repositories (
			scope_type, scope_id, repo_url, branch, base_path, enabled,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo,
			created_by, updated_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, TRUE, 'config-repo', 'config-repo')
		ON CONFLICT (scope_type, scope_id) DO UPDATE SET
			repo_url = EXCLUDED.repo_url,
			branch = EXCLUDED.branch,
			base_path = EXCLUDED.base_path,
			enabled = EXCLUDED.enabled,
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = TRUE,
			updated_by = EXCLUDED.updated_by,
			updated_at = NOW()`

	// A. Upsert Config Repository Bindings
	for key, stored := range configRepositories {
		writable, err := ensureConfigResourceWritable(ctx, tx, "config_repositories", "config repository", key, binding, stored.scopeID, "scope_type = $1 AND scope_id = $2", stored.scopeType, stored.scopeID)
		if err != nil {
			return nil, commitSHA, err
		}
		if !writable {
			continue
		}
		if _, err := tx.Exec(ctx, configRepositoryUpsert, stored.scopeType, stored.scopeID, stored.repoURL, stored.branch, stored.basePath, stored.enabled, binding.ID, stored.sourcePath, commitSHA); err != nil {
			return nil, commitSHA, fmt.Errorf("failed to upsert config repository binding '%s': %w", key, err)
		}
		details["config_repositories_synced"]++
	}

	// B. Upsert Pipelines
	for key, stored := range pipelines {
		writable, err := ensureConfigResourceWritable(ctx, tx, "pipelines", "pipeline", key, binding, key, "path = $1 AND name = $2", stored.path, stored.name)
		if err != nil {
			return nil, commitSHA, err
		}
		if !writable {
			continue
		}
		if _, err := tx.Exec(ctx, pipelineUpsert, stored.path, stored.name, stored.version, stored.definition, binding.ID, stored.sourcePath, commitSHA); err != nil {
			return nil, commitSHA, fmt.Errorf("failed to upsert pipeline '%s': %w", key, err)
		}
		details["pipelines_synced"]++
	}

	// C. Upsert Steps
	for key, stored := range steps {
		writable, err := ensureConfigResourceWritable(ctx, tx, "steps", "reusable step", key, binding, key, "path = $1 AND name = $2", stored.path, stored.name)
		if err != nil {
			return nil, commitSHA, err
		}
		if !writable {
			continue
		}
		if _, err := tx.Exec(ctx, stepUpsert, stored.path, stored.name, stored.definition, binding.ID, stored.sourcePath, commitSHA); err != nil {
			return nil, commitSHA, fmt.Errorf("failed to upsert reusable step '%s': %w", key, err)
		}
		details["steps_synced"]++
	}

	// D. Upsert General Envs
	for key, stored := range generalEnvs {
		var envParam interface{}
		if key.envPath != "" {
			envParam = key.envPath
		}
		resourceID := fmt.Sprintf("scope=%s name=%s", key.envPath, key.name)
		writable, err := ensureConfigResourceWritable(ctx, tx, "variables", "variable", resourceID, binding, key.envPath, "name = $1 AND repository_name IS NULL AND scope IS NOT DISTINCT FROM $2", key.name, envParam)
		if err != nil {
			return nil, commitSHA, err
		}
		if !writable {
			continue
		}
		if _, err := tx.Exec(ctx, envUpsert, key.name, stored.value, nil, envParam, binding.ID, stored.sourcePath, commitSHA); err != nil {
			return nil, commitSHA, fmt.Errorf("failed to upsert variable '%s' for scope '%s': %w", key.name, key.envPath, err)
		}
		details["general_vars_synced"]++
	}

	// E. Upsert Repo Envs
	for key, stored := range repoEnvs {
		var envParam interface{}
		if key.envPath != "" {
			envParam = key.envPath
		}
		resourceID := fmt.Sprintf("repo=%s scope=%s name=%s", key.repo, key.envPath, key.name)
		writable, err := ensureConfigResourceWritable(ctx, tx, "variables", "variable", resourceID, binding, key.repo, "name = $1 AND repository_name = $2 AND scope IS NOT DISTINCT FROM $3", key.name, key.repo, envParam)
		if err != nil {
			return nil, commitSHA, err
		}
		if !writable {
			continue
		}
		if _, err := tx.Exec(ctx, envUpsert, key.name, stored.value, key.repo, envParam, binding.ID, stored.sourcePath, commitSHA); err != nil {
			return nil, commitSHA, fmt.Errorf("failed to upsert repository variable '%s' for repo '%s' scope '%s': %w", key.name, key.repo, key.envPath, err)
		}
		details["repo_vars_synced"]++
	}

	// F. Upsert Triggers
	for repoName, stored := range triggers {
		writable, err := ensureConfigResourceWritable(ctx, tx, "triggers", "trigger", repoName, binding, repoName, "repository_name = $1", repoName)
		if err != nil {
			return nil, commitSHA, err
		}
		if !writable {
			continue
		}
		if _, err := tx.Exec(ctx, triggerUpsert, repoName, stored.definition, binding.ID, stored.sourcePath, commitSHA); err != nil {
			return nil, commitSHA, fmt.Errorf("failed to upsert trigger override '%s': %w", repoName, err)
		}
		details["triggers_synced"]++
	}

	// --- PRUNING PHASE: Remove items that exist in DB as source='git' but were not in the Git payload ---

	// 0. Prune Config Repository Bindings
	if binding.ScopeType == models.ConfigRepositoryScopeSystem {
		var scopeTypes, scopeIDs []string
		for _, cfgRepo := range configRepositories {
			scopeTypes = append(scopeTypes, cfgRepo.scopeType)
			scopeIDs = append(scopeIDs, cfgRepo.scopeID)
		}
		prunedRepoIDs := []int64{}
		if len(scopeTypes) == 0 {
			rows, err := tx.Query(ctx, "SELECT id FROM config_repositories WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID)
			if err != nil {
				return nil, commitSHA, fmt.Errorf("failed to prune config repository bindings: %w", err)
			}
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return nil, commitSHA, fmt.Errorf("failed to scan pruned config repository binding: %w", err)
				}
				prunedRepoIDs = append(prunedRepoIDs, id)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return nil, commitSHA, fmt.Errorf("failed to read pruned config repository bindings: %w", err)
			}
			rows.Close()
		} else {
			rows, err := tx.Query(ctx, `
				SELECT id FROM config_repositories
				WHERE managed_by_config_repo = TRUE
				AND config_repo_id = $3
				AND NOT EXISTS (
					SELECT 1 FROM unnest($1::text[], $2::text[]) AS t(scope_type, scope_id)
					WHERE config_repositories.scope_type = t.scope_type
					AND config_repositories.scope_id = t.scope_id
				)`, scopeTypes, scopeIDs, binding.ID)
			if err != nil {
				return nil, commitSHA, fmt.Errorf("failed to prune config repository bindings: %w", err)
			}
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return nil, commitSHA, fmt.Errorf("failed to scan pruned config repository binding: %w", err)
				}
				prunedRepoIDs = append(prunedRepoIDs, id)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return nil, commitSHA, fmt.Errorf("failed to read pruned config repository bindings: %w", err)
			}
			rows.Close()
		}
		if len(prunedRepoIDs) > 0 {
			for _, tableName := range []string{"config_repositories", "pipelines", "steps", "triggers", "variables", "secrets"} {
				if _, err := tx.Exec(ctx, fmt.Sprintf(`
					UPDATE %s
					SET config_repo_id = NULL,
						config_source_path = '',
						config_source_commit_sha = '',
						managed_by_config_repo = FALSE
					WHERE config_repo_id = ANY($1)
				`, tableName), prunedRepoIDs); err != nil {
					return nil, commitSHA, fmt.Errorf("failed to detach resources from pruned config repository bindings: %w", err)
				}
			}
			if _, err := tx.Exec(ctx, "DELETE FROM config_repositories WHERE id = ANY($1)", prunedRepoIDs); err != nil {
				return nil, commitSHA, fmt.Errorf("failed to prune config repository bindings: %w", err)
			}
		}
	}

	// 1. Prune Pipelines
	{
		var paths, names []string
		for _, p := range pipelines {
			paths = append(paths, p.path)
			names = append(names, p.name)
		}
		if len(paths) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM pipelines WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
				return nil, commitSHA, fmt.Errorf("failed to prune pipelines: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				DELETE FROM pipelines 
				WHERE managed_by_config_repo = TRUE
				AND config_repo_id = $3
				AND NOT EXISTS (
					SELECT 1 FROM unnest($1::text[], $2::text[]) AS t(p, n) 
					WHERE pipelines.path = t.p AND pipelines.name = t.n
				)`, paths, names, binding.ID); err != nil {
				return nil, commitSHA, fmt.Errorf("failed to prune pipelines: %w", err)
			}
		}
	}

	// 2. Prune Steps
	{
		var paths, names []string
		for _, s := range steps {
			paths = append(paths, s.path)
			names = append(names, s.name)
		}
		if len(paths) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM steps WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
				return nil, commitSHA, fmt.Errorf("failed to prune steps: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				DELETE FROM steps 
				WHERE managed_by_config_repo = TRUE
				AND config_repo_id = $3
				AND NOT EXISTS (
					SELECT 1 FROM unnest($1::text[], $2::text[]) AS t(p, n) 
					WHERE steps.path = t.p AND steps.name = t.n
				)`, paths, names, binding.ID); err != nil {
				return nil, commitSHA, fmt.Errorf("failed to prune steps: %w", err)
			}
		}
	}

	// 3. Prune Triggers
	{
		var repos []string
		for repo := range triggers {
			repos = append(repos, repo)
		}
		if len(repos) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM triggers WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
				return nil, commitSHA, fmt.Errorf("failed to prune triggers: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, "DELETE FROM triggers WHERE managed_by_config_repo = TRUE AND config_repo_id = $2 AND repository_name != ALL($1)", repos, binding.ID); err != nil {
				return nil, commitSHA, fmt.Errorf("failed to prune triggers: %w", err)
			}
		}
	}

	// 4. Prune Variables (Environment Variables)
	{
		var names []string
		var repos []*string
		var scopes []*string

		// Helper to collect all valid (name, repo, scope) tuples
		addVar := func(n string, r *string, s string) {
			names = append(names, n)
			repos = append(repos, r)
			if s == "" {
				scopes = append(scopes, nil)
			} else {
				scopes = append(scopes, &s)
			}
		}

		for key := range generalEnvs {
			addVar(key.name, nil, key.envPath)
		}
		for key := range repoEnvs {
			r := key.repo // copy loop variable
			addVar(key.name, &r, key.envPath)
		}

		if len(names) == 0 {
			if _, err := tx.Exec(ctx, "DELETE FROM variables WHERE managed_by_config_repo = TRUE AND config_repo_id = $1", binding.ID); err != nil {
				return nil, commitSHA, fmt.Errorf("failed to prune variables: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				DELETE FROM variables 
				WHERE managed_by_config_repo = TRUE
				AND config_repo_id = $4
				AND NOT EXISTS (
					SELECT 1 FROM unnest($1::text[], $2::text[], $3::text[]) AS t(n, r, s) 
					WHERE variables.name = t.n 
					AND variables.repository_name IS NOT DISTINCT FROM t.r 
					AND variables.scope IS NOT DISTINCT FROM t.s
				)`, names, repos, scopes, binding.ID); err != nil {
				return nil, commitSHA, fmt.Errorf("failed to prune variables: %w", err)
			}
		}
	}

	// Sync UI groups. Groups do not have a source column, so we do not prune them to avoid deleting user-created groups.
	if len(effectivePipelineRunStructure) > 0 {
		if err := a.syncPipelineRunGroups(ctx, tx, effectivePipelineRunStructure, details); err != nil {
			return nil, commitSHA, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, commitSHA, fmt.Errorf("failed to commit configuration synchronization transaction: %w", err)
	}

	log.Info().
		Str("repo_owner", owner).
		Str("repo_name", repo).
		Int("pipelines_synced", details["pipelines_synced"]).
		Int("steps_synced", details["steps_synced"]).
		Int("general_vars_synced", details["general_vars_synced"]).
		Int("repo_vars_synced", details["repo_vars_synced"]).
		Int("triggers_synced", details["triggers_synced"]).
		Int("config_repositories_synced", details["config_repositories_synced"]).
		Int("run_groups_created", details["run_groups_created"]).
		Int("run_groups_updated", details["run_groups_updated"]).
		Msg("Configuration synchronization from Git completed")

	return details, commitSHA, nil
}

type generalEnvKey struct {
	envPath string
	name    string
}

type repoEnvKey struct {
	repo    string
	envPath string
	name    string
}

type storedPipeline struct {
	definition string
	version    string
	path       string
	name       string
	sourcePath string
}

type storedStep struct {
	definition string
	path       string
	name       string
	sourcePath string
}

type storedConfigRepository struct {
	scopeType  string
	scopeID    string
	repoURL    string
	branch     string
	basePath   string
	enabled    bool
	sourcePath string
}

type storedEnvVar struct {
	value      string
	sourcePath string
}

type storedTrigger struct {
	definition string
	sourcePath string
}

type configRepositoryBindingFile struct {
	ScopeType string `yaml:"scope_type" json:"scope_type"`
	ScopeID   string `yaml:"scope_id" json:"scope_id"`
	RepoURL   string `yaml:"repo_url" json:"repo_url"`
	Branch    string `yaml:"branch" json:"branch"`
	BasePath  string `yaml:"base_path" json:"base_path"`
	Enabled   *bool  `yaml:"enabled" json:"enabled"`
}

func parseConfigRepositoryBindingPath(rel string) (string, string, error) {
	path, name, _, err := splitYAMLIdentifier(rel)
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", fmt.Errorf("binding path must start with groups/")
	}
	switch parts[0] {
	case "groups":
		scopeID := strings.Trim(strings.Join(append(parts[1:], name), "/"), "/")
		if scopeID == "" {
			return "", "", fmt.Errorf("group binding is missing a group path")
		}
		if _, err := cleanConfigPathSegments(scopeID, false); err != nil {
			return "", "", err
		}
		return models.ConfigRepositoryScopeFolder, scopeID, nil
	default:
		return "", "", fmt.Errorf("unsupported config repository binding scope %q", parts[0])
	}
}

func validateConfigRepositoryBindingFile(file configRepositoryBindingFile, scopeType, scopeID, sourcePath string) error {
	if declaredScopeType := strings.TrimSpace(file.ScopeType); declaredScopeType != "" && declaredScopeType != scopeType {
		return fmt.Errorf("config repository binding '%s' declares scope_type %q but path implies %q", sourcePath, declaredScopeType, scopeType)
	}
	if declaredScopeID := strings.Trim(strings.TrimSpace(file.ScopeID), "/"); declaredScopeID != "" && declaredScopeID != scopeID {
		return fmt.Errorf("config repository binding '%s' declares scope_id %q but path implies %q", sourcePath, declaredScopeID, scopeID)
	}
	if strings.TrimSpace(file.RepoURL) == "" {
		return fmt.Errorf("config repository binding '%s' is missing repo_url", sourcePath)
	}
	return nil
}

func configRepositoryOverrideScopes(ctx context.Context, tx pgx.Tx, binding models.ConfigRepository, parsed map[string]storedConfigRepository) ([]string, error) {
	scopeSet := map[string]struct{}{}
	addScope := func(scope string) {
		scope = strings.Trim(strings.TrimSpace(scope), "/")
		if scope == "" {
			return
		}
		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			boundScope := strings.Trim(strings.TrimSpace(binding.ScopeID), "/")
			if scope == boundScope || !configResourceUnderScope(scope, boundScope) {
				return
			}
		}
		scopeSet[scope] = struct{}{}
	}

	for _, repo := range parsed {
		if repo.enabled && repo.scopeType == models.ConfigRepositoryScopeFolder {
			addScope(repo.scopeID)
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT scope_id
		FROM config_repositories
		WHERE scope_type = $1
		  AND enabled = TRUE
		  AND id <> $2
	`, models.ConfigRepositoryScopeFolder, binding.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load delegated config repository scopes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var scopeID string
		if err := rows.Scan(&scopeID); err != nil {
			return nil, err
		}
		addScope(scopeID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	scopes := make([]string, 0, len(scopeSet))
	for scope := range scopeSet {
		scopes = append(scopes, scope)
	}
	return scopes, nil
}

func effectivePipelineRunStructureForConfigSync(
	binding models.ConfigRepository,
	configRepositories map[string]storedConfigRepository,
	pipelineRunStructure map[string]*pipelineRunStructureNode,
	overrideScopes []string,
) (map[string]*pipelineRunStructureNode, error) {
	effective, err := configRepositoryGroupStructure(binding, configRepositories)
	if err != nil {
		return nil, err
	}

	structure := pipelineRunStructure
	if binding.ScopeType == models.ConfigRepositoryScopeSystem && containsGroupConfigRepository(configRepositories) {
		structure = nil
	} else {
		structure = filterPipelineRunStructureByScopes(structure, configRepositoryStructureFilterScopes(binding, configRepositories, overrideScopes))
	}

	mergePipelineRunStructure(effective, structure)
	return effective, nil
}

func containsGroupConfigRepository(configRepositories map[string]storedConfigRepository) bool {
	for _, repo := range configRepositories {
		if repo.scopeType == models.ConfigRepositoryScopeFolder {
			return true
		}
	}
	return false
}

func configRepositoryStructureFilterScopes(binding models.ConfigRepository, configRepositories map[string]storedConfigRepository, overrideScopes []string) []string {
	scopeSet := map[string]struct{}{}
	addScope := func(scope string) {
		scope = strings.Trim(strings.TrimSpace(scope), "/")
		if scope == "" {
			return
		}
		if binding.ScopeType == models.ConfigRepositoryScopeFolder {
			boundScope := strings.Trim(strings.TrimSpace(binding.ScopeID), "/")
			if scope == boundScope || !configResourceUnderScope(scope, boundScope) {
				return
			}
		}
		scopeSet[scope] = struct{}{}
	}

	for _, scope := range overrideScopes {
		addScope(scope)
	}
	for _, repo := range configRepositories {
		if repo.scopeType == models.ConfigRepositoryScopeFolder {
			addScope(repo.scopeID)
		}
	}

	scopes := make([]string, 0, len(scopeSet))
	for scope := range scopeSet {
		scopes = append(scopes, scope)
	}
	return scopes
}

func configRepositoryGroupStructure(binding models.ConfigRepository, configRepositories map[string]storedConfigRepository) (map[string]*pipelineRunStructureNode, error) {
	result := map[string]*pipelineRunStructureNode{}
	addPath := func(path string) error {
		segments, err := cleanConfigPathSegments(path, false)
		if err != nil {
			return err
		}
		ensurePipelineRunStructurePath(result, segments)
		return nil
	}

	if binding.ScopeType == models.ConfigRepositoryScopeFolder {
		if err := addPath(binding.ScopeID); err != nil {
			return nil, fmt.Errorf("invalid group-scoped config repository group path %q: %w", binding.ScopeID, err)
		}
	}
	for _, repo := range configRepositories {
		if repo.scopeType != models.ConfigRepositoryScopeFolder {
			continue
		}
		if err := addPath(repo.scopeID); err != nil {
			return nil, fmt.Errorf("invalid config repository group path %q: %w", repo.scopeID, err)
		}
	}
	return result, nil
}

func ensurePipelineRunStructurePath(structure map[string]*pipelineRunStructureNode, segments []string) *pipelineRunStructureNode {
	children := structure
	var current *pipelineRunStructureNode
	for _, segment := range segments {
		if node, ok := children[segment]; ok {
			current = node
		} else {
			current = &pipelineRunStructureNode{Children: map[string]*pipelineRunStructureNode{}}
			children[segment] = current
		}
		if current.Children == nil {
			current.Children = map[string]*pipelineRunStructureNode{}
		}
		children = current.Children
	}
	return current
}

func mergePipelineRunStructure(dst map[string]*pipelineRunStructureNode, src map[string]*pipelineRunStructureNode) {
	if len(src) == 0 {
		return
	}
	var mergeNode func(target *pipelineRunStructureNode, source *pipelineRunStructureNode)
	mergeNode = func(target *pipelineRunStructureNode, source *pipelineRunStructureNode) {
		if target.Children == nil {
			target.Children = map[string]*pipelineRunStructureNode{}
		}
		if source == nil {
			return
		}
		if description := strings.TrimSpace(source.Description); description != "" {
			target.Description = description
		}
		if len(source.Repos) > 0 {
			target.Repos = append([]string{}, source.Repos...)
		}
		for childName, childSource := range source.Children {
			childTarget, ok := target.Children[childName]
			if !ok {
				childTarget = &pipelineRunStructureNode{Children: map[string]*pipelineRunStructureNode{}}
				target.Children[childName] = childTarget
			}
			mergeNode(childTarget, childSource)
		}
	}

	for name, source := range src {
		target, ok := dst[name]
		if !ok {
			target = &pipelineRunStructureNode{Children: map[string]*pipelineRunStructureNode{}}
			dst[name] = target
		}
		mergeNode(target, source)
	}
}

func filterPipelineRunStructureByScopes(structure map[string]*pipelineRunStructureNode, scopes []string) map[string]*pipelineRunStructureNode {
	if len(structure) == 0 || len(scopes) == 0 {
		return structure
	}

	var filterNode func(path []string, node *pipelineRunStructureNode) *pipelineRunStructureNode
	filterNode = func(path []string, node *pipelineRunStructureNode) *pipelineRunStructureNode {
		if configResourceUnderAnyScope(strings.Join(path, "/"), scopes) {
			return nil
		}
		filtered := &pipelineRunStructureNode{Children: map[string]*pipelineRunStructureNode{}}
		if node != nil {
			filtered.Description = node.Description
			filtered.Repos = append([]string{}, node.Repos...)
			for childName, childNode := range node.Children {
				child := filterNode(append(append([]string{}, path...), childName), childNode)
				if child != nil {
					filtered.Children[childName] = child
				}
			}
		}
		return filtered
	}

	filtered := map[string]*pipelineRunStructureNode{}
	for name, node := range structure {
		child := filterNode([]string{name}, node)
		if child != nil {
			filtered[name] = child
		}
	}
	return filtered
}

func filterDelegatedConfigResources(
	binding models.ConfigRepository,
	overrideScopes []string,
	pipelines map[string]storedPipeline,
	steps map[string]storedStep,
	generalEnvs map[generalEnvKey]storedEnvVar,
	repoEnvs map[repoEnvKey]storedEnvVar,
	triggers map[string]storedTrigger,
) {
	if len(overrideScopes) == 0 {
		return
	}

	for key := range pipelines {
		if configResourceUnderAnyScope(key, overrideScopes) {
			delete(pipelines, key)
		}
	}
	for key := range steps {
		if configResourceUnderAnyScope(key, overrideScopes) {
			delete(steps, key)
		}
	}
	for key := range generalEnvs {
		if configResourceUnderAnyScope(key.envPath, overrideScopes) {
			delete(generalEnvs, key)
		}
	}
	for key := range repoEnvs {
		if configResourceUnderAnyScope(key.repo, overrideScopes) || configResourceUnderAnyScope(key.envPath, overrideScopes) {
			delete(repoEnvs, key)
		}
	}
	for key := range triggers {
		if configResourceUnderAnyScope(key, overrideScopes) {
			delete(triggers, key)
		}
	}
}

func configResourceUnderAnyScope(resource string, scopes []string) bool {
	for _, scope := range scopes {
		if configResourceUnderScope(resource, scope) {
			return true
		}
	}
	return false
}

func configResourceUnderScope(resource, scope string) bool {
	resource = strings.Trim(strings.TrimSpace(resource), "/")
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	if resource == "" || scope == "" {
		return false
	}
	if resource == scope {
		return true
	}
	return strings.HasPrefix(resource, scope+"/")
}

func canConfigRepositoryWriteOver(current, existing models.ConfigRepository, resourceScope string) bool {
	if existing.ID == 0 || existing.ID == current.ID {
		return true
	}
	if !configResourceUnderBindingScope(resourceScope, current) {
		return false
	}
	if current.ScopeType == models.ConfigRepositoryScopeFolder {
		return existing.ScopeType == models.ConfigRepositoryScopeSystem ||
			(existing.ScopeType == models.ConfigRepositoryScopeFolder &&
				configResourceUnderScope(current.ScopeID, existing.ScopeID))
	}
	return false
}

func configRepositoryShadowsCurrent(existing, current models.ConfigRepository, resourceScope string) bool {
	if existing.ID == 0 || existing.ID == current.ID {
		return false
	}
	if !configResourceUnderBindingScope(resourceScope, existing) {
		return false
	}
	if existing.ScopeType == models.ConfigRepositoryScopeFolder {
		return current.ScopeType == models.ConfigRepositoryScopeSystem ||
			(current.ScopeType == models.ConfigRepositoryScopeFolder &&
				configResourceUnderScope(existing.ScopeID, current.ScopeID))
	}
	return false
}

func configResourceUnderBindingScope(resourceScope string, binding models.ConfigRepository) bool {
	switch binding.ScopeType {
	case models.ConfigRepositoryScopeSystem:
		return true
	case models.ConfigRepositoryScopeFolder:
		return configResourceUnderScope(resourceScope, binding.ScopeID)
	default:
		return false
	}
}

func loadConfigRepositoryByID(ctx context.Context, tx pgx.Tx, id int64) (models.ConfigRepository, error) {
	var repo models.ConfigRepository
	err := tx.QueryRow(ctx, `
		SELECT id, scope_type, scope_id
		FROM config_repositories
		WHERE id = $1
	`, id).Scan(&repo.ID, &repo.ScopeType, &repo.ScopeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ConfigRepository{}, fmt.Errorf("config repository %d not found", id)
		}
		return models.ConfigRepository{}, err
	}
	return repo, nil
}

func ensureConfigResourceWritable(ctx context.Context, tx pgx.Tx, tableName, resourceKind, resourceID string, binding models.ConfigRepository, resourceScope string, whereClause string, args ...any) (bool, error) {
	query := fmt.Sprintf("SELECT config_repo_id, managed_by_config_repo FROM %s WHERE %s LIMIT 1", tableName, whereClause)
	var existingRepoID sql.NullInt64
	var managed bool
	if err := tx.QueryRow(ctx, query, args...).Scan(&existingRepoID, &managed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		return false, err
	}
	if !managed {
		return false, fmt.Errorf("%s %s is not managed by a config repository", resourceKind, resourceID)
	}
	if !existingRepoID.Valid {
		return false, fmt.Errorf("%s %s is already managed by an unknown config repository", resourceKind, resourceID)
	}
	if existingRepoID.Int64 == binding.ID {
		return true, nil
	}

	existing, err := loadConfigRepositoryByID(ctx, tx, existingRepoID.Int64)
	if err != nil {
		return false, err
	}
	if canConfigRepositoryWriteOver(binding, existing, resourceScope) {
		return true, nil
	}
	if configRepositoryShadowsCurrent(existing, binding, resourceScope) {
		return false, nil
	}

	owner := strconv.FormatInt(existingRepoID.Int64, 10)
	return false, fmt.Errorf("%s %s is already managed by config repository %s", resourceKind, resourceID, owner)
}

func normalizeConfigRepositoryBasePathValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.Trim(value, "/")
	if value == "." {
		return ""
	}
	return value
}

func configRepoJoinPath(basePath, child string) string {
	basePath = normalizeConfigRepositoryBasePathValue(basePath)
	child = strings.Trim(strings.ReplaceAll(strings.TrimSpace(child), "\\", "/"), "/")
	if basePath == "" {
		return child
	}
	if child == "" {
		return basePath
	}
	return basePath + "/" + child
}

func relativeConfigPath(path, dir string) (string, bool) {
	path = strings.Trim(strings.ReplaceAll(filepath.ToSlash(path), "\\", "/"), "/")
	dir = strings.Trim(strings.ReplaceAll(filepath.ToSlash(dir), "\\", "/"), "/")
	if dir == "" {
		return path, true
	}
	if path == dir {
		return "", true
	}
	prefix := dir + "/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	return strings.TrimPrefix(path, prefix), true
}

func normalizeConfigPathForFolder(boundFolder string, repoRelativePath string) (string, error) {
	boundSegments, err := cleanConfigPathSegments(boundFolder, false)
	if err != nil {
		return "", fmt.Errorf("invalid bound folder: %w", err)
	}
	if len(boundSegments) == 0 {
		return "", fmt.Errorf("bound folder is required")
	}

	relative := strings.Trim(strings.ReplaceAll(filepath.ToSlash(repoRelativePath), "\\", "/"), "/")
	if relative == "" {
		return strings.Join(boundSegments, "/"), nil
	}
	relative = stripConfigResourcePrefix(relative)
	relative = strings.TrimSuffix(relative, filepath.Ext(relative))
	relSegments, err := cleanConfigPathSegments(relative, true)
	if err != nil {
		return "", err
	}
	if hasPathSegmentPrefix(relSegments, boundSegments) {
		relSegments = relSegments[len(boundSegments):]
	}

	finalSegments := append(append([]string{}, boundSegments...), relSegments...)
	return strings.Join(finalSegments, "/"), nil
}

func stripConfigResourcePrefix(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) <= 1 {
		return path
	}
	switch strings.ToLower(strings.TrimSpace(parts[0])) {
	case "pipelines", "steps", "triggers", "environments", "pipelineruns":
		return strings.Join(parts[1:], "/")
	default:
		return path
	}
}

func cleanConfigPathSegments(path string, allowEmpty bool) ([]string, error) {
	path = strings.Trim(strings.ReplaceAll(filepath.ToSlash(path), "\\", "/"), "/")
	if path == "" {
		if allowEmpty {
			return nil, nil
		}
		return nil, fmt.Errorf("path is required")
	}
	if filepath.IsAbs(path) {
		return nil, fmt.Errorf("path must be relative")
	}
	parts := strings.Split(path, "/")
	segments := make([]string, 0, len(parts))
	for _, segment := range parts {
		segment = strings.TrimSpace(segment)
		if segment == "" || segment == "." || segment == ".." {
			return nil, fmt.Errorf("path contains invalid segment %q", segment)
		}
		segments = append(segments, segment)
	}
	return segments, nil
}

func hasPathSegmentPrefix(path, prefix []string) bool {
	if len(prefix) == 0 || len(path) < len(prefix) {
		return false
	}
	for idx := range prefix {
		if path[idx] != prefix[idx] {
			return false
		}
	}
	return true
}

type pipelineRunStructureNode struct {
	Description string
	Repos       []string
	Children    map[string]*pipelineRunStructureNode
}

type groupRecord struct {
	ID          int
	ParentID    *int
	Description string
}

func parsePipelineRunStructure(content string) (map[string]*pipelineRunStructureNode, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return map[string]*pipelineRunStructureNode{}, nil
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		return nil, err
	}
	result := make(map[string]*pipelineRunStructureNode, len(raw))
	for name, value := range raw {
		normalized, err := normalizeStructureName(name)
		if err != nil {
			return nil, err
		}
		if _, exists := result[normalized]; exists {
			return nil, fmt.Errorf("duplicate folder '%s' in pipelinerun structure", normalized)
		}
		node, err := decodePipelineRunStructureNode(value)
		if err != nil {
			return nil, fmt.Errorf("folder '%s': %w", normalized, err)
		}
		result[normalized] = node
	}
	return result, nil
}

func normalizePipelineRunStructureForFolder(boundFolder string, structure map[string]*pipelineRunStructureNode) (map[string]*pipelineRunStructureNode, error) {
	if len(structure) == 0 {
		return structure, nil
	}
	boundSegments, err := cleanConfigPathSegments(boundFolder, false)
	if err != nil {
		return nil, err
	}
	result := map[string]*pipelineRunStructureNode{}

	var ensurePath func(path []string) *pipelineRunStructureNode
	ensurePath = func(path []string) *pipelineRunStructureNode {
		children := result
		var current *pipelineRunStructureNode
		for _, segment := range path {
			if node, ok := children[segment]; ok {
				current = node
			} else {
				current = &pipelineRunStructureNode{Children: map[string]*pipelineRunStructureNode{}}
				children[segment] = current
			}
			if current.Children == nil {
				current.Children = map[string]*pipelineRunStructureNode{}
			}
			children = current.Children
		}
		return current
	}

	var mergeNode func(path []string, node *pipelineRunStructureNode) error
	mergeNode = func(path []string, node *pipelineRunStructureNode) error {
		normalizedPath, err := normalizeConfigPathForFolder(boundFolder, strings.Join(path, "/"))
		if err != nil {
			return err
		}
		targetSegments, err := cleanConfigPathSegments(normalizedPath, false)
		if err != nil {
			return err
		}
		target := ensurePath(targetSegments)
		if node != nil {
			if description := strings.TrimSpace(node.Description); description != "" {
				target.Description = description
			}
			target.Repos = append(target.Repos, node.Repos...)
			for childName, childNode := range node.Children {
				childSegments, err := cleanConfigPathSegments(childName, false)
				if err != nil {
					return err
				}
				if err := mergeNode(append(append([]string{}, path...), childSegments...), childNode); err != nil {
					return err
				}
			}
		}
		return nil
	}

	for name, node := range structure {
		segments, err := cleanConfigPathSegments(name, false)
		if err != nil {
			return nil, err
		}
		if !hasPathSegmentPrefix(segments, boundSegments) {
			segments = append(append([]string{}, boundSegments...), segments...)
		}
		if err := mergeNode(segments, node); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func decodePipelineRunStructureNode(value interface{}) (*pipelineRunStructureNode, error) {
	node := &pipelineRunStructureNode{Children: map[string]*pipelineRunStructureNode{}}
	if value == nil {
		return node, nil
	}

	switch typed := value.(type) {
	case string:
		node.Description = strings.TrimSpace(typed)
		return node, nil
	case map[string]interface{}:
		return decodePipelineRunStructureMap(node, typed)
	default:
		return nil, fmt.Errorf("expected mapping or description for folder, got %T", value)
	}
}

func decodePipelineRunStructureMap(node *pipelineRunStructureNode, childMap map[string]interface{}) (*pipelineRunStructureNode, error) {

	for key, raw := range childMap {
		switch key {
		case "repos":
			repos, err := parseStructureRepoList(raw)
			if err != nil {
				return nil, err
			}
			node.Repos = repos
		case "description":
			if raw == nil {
				node.Description = ""
				continue
			}
			text, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("description must be a string, got %T", raw)
			}
			node.Description = strings.TrimSpace(text)
		default:
			childName, err := normalizeStructureName(key)
			if err != nil {
				return nil, err
			}
			if _, exists := node.Children[childName]; exists {
				return nil, fmt.Errorf("duplicate folder '%s' detected", childName)
			}
			childNode, err := decodePipelineRunStructureNode(raw)
			if err != nil {
				return nil, fmt.Errorf("folder '%s': %w", childName, err)
			}
			node.Children[childName] = childNode
		}
	}

	return node, nil
}

func parseStructureRepoList(value interface{}) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("repos must be defined as a list, got %T", value)
	}
	var repos []string
	for idx, raw := range items {
		if raw == nil {
			continue
		}
		text, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("repos entry %d must be a string, got %T", idx, raw)
		}
		repo := strings.TrimSpace(text)
		if repo == "" {
			continue
		}
		repos = append(repos, repo)
	}
	return repos, nil
}

func normalizeStructureName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("pipelinerun structure contains an empty folder or repository name")
	}
	return trimmed, nil
}

func loadExistingGroupRecords(ctx context.Context, tx pgx.Tx) (map[string]*groupRecord, error) {
	rows, err := tx.Query(ctx, "SELECT id, name, parent_id, description FROM groups")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*groupRecord)
	for rows.Next() {
		var (
			id          int
			name        string
			parentID    sql.NullInt32
			description sql.NullString
		)
		if err := rows.Scan(&id, &name, &parentID, &description); err != nil {
			return nil, err
		}
		key, err := normalizeStructureName(name)
		if err != nil {
			return nil, err
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate group name '%s' detected in database", key)
		}
		result[key] = &groupRecord{
			ID:          id,
			ParentID:    pointerFromNullInt(parentID),
			Description: strings.TrimSpace(description.String),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func pointerFromNullInt(value sql.NullInt32) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int32)
	return &v
}

func copyIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}

func parentPointersEqual(a, b *int) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a != nil && b != nil:
		return *a == *b
	default:
		return false
	}
}

func (a *App) syncPipelineRunGroups(ctx context.Context, tx pgx.Tx, structure map[string]*pipelineRunStructureNode, details map[string]int) error {
	if len(structure) == 0 {
		return nil
	}

	existingGroups, err := loadExistingGroupRecords(ctx, tx)
	if err != nil {
		return fmt.Errorf("failed to load existing pipeline run folders: %w", err)
	}

	var ensureGroup func(name string, parentID *int, description string) (int, error)
	ensureGroup = func(name string, parentID *int, description string) (int, error) {
		normalized, err := normalizeStructureName(name)
		if err != nil {
			return 0, err
		}
		description = strings.TrimSpace(description)
		if record, ok := existingGroups[normalized]; ok {
			parentChanged := !parentPointersEqual(record.ParentID, parentID)
			descChanged := strings.TrimSpace(record.Description) != description
			if parentChanged || descChanged {
				if _, err := tx.Exec(ctx, "UPDATE groups SET parent_id = $1, description = $2, updated_at = NOW() WHERE id = $3", parentID, description, record.ID); err != nil {
					return 0, fmt.Errorf("failed to update folder '%s': %w", normalized, err)
				}
				record.ParentID = copyIntPointer(parentID)
				record.Description = description
				details["run_groups_updated"]++
			}
			return record.ID, nil
		}

		var newID int
		if err := tx.QueryRow(ctx, "INSERT INTO groups (name, parent_id, description) VALUES ($1, $2, $3) RETURNING id", normalized, parentID, description).Scan(&newID); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				refreshed, loadErr := loadExistingGroupRecords(ctx, tx)
				if loadErr != nil {
					return 0, fmt.Errorf("failed to reload folders after conflict: %w", loadErr)
				}
				existingGroups = refreshed
				if _, ok := existingGroups[normalized]; ok {
					return ensureGroup(normalized, parentID, description)
				}
			}
			return 0, fmt.Errorf("failed to create folder '%s': %w", normalized, err)
		}
		existingGroups[normalized] = &groupRecord{ID: newID, ParentID: copyIntPointer(parentID), Description: description}
		details["run_groups_created"]++
		return newID, nil
	}

	var applyNode func(name string, node *pipelineRunStructureNode, parentID *int) error
	applyNode = func(name string, node *pipelineRunStructureNode, parentID *int) error {
		groupID, err := ensureGroup(name, parentID, node.Description)
		if err != nil {
			return err
		}
		for _, repoName := range node.Repos {
			if _, err := ensureGroup(repoName, &groupID, ""); err != nil {
				return err
			}
		}
		for childName, childNode := range node.Children {
			if err := applyNode(childName, childNode, &groupID); err != nil {
				return err
			}
		}
		return nil
	}

	for name, node := range structure {
		if node == nil {
			node = &pipelineRunStructureNode{Children: map[string]*pipelineRunStructureNode{}}
		}
		if err := applyNode(name, node, nil); err != nil {
			return fmt.Errorf("failed to sync folder '%s': %w", name, err)
		}
	}

	return nil
}

func normalizeVariableSourceKey(value string) string {
	key := strings.TrimSpace(strings.ToLower(value))
	switch {
	case strings.Contains(key, "git"):
		return "git"
	case strings.Contains(key, "draft"):
		return "draft"
	case strings.Contains(key, "local"):
		return "local"
	default:
		return "database"
	}
}

func isYAMLFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}

func sanitizeInput(name string) string {
	sanitized := strings.ReplaceAll(name, " ", "-")
	return nonAlphanumericRegex.ReplaceAllString(sanitized, "")
}

func isTerminalRunStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "failure", "cancelled", "timed_out", "failure (ignored)":
		return true
	default:
		return false
	}
}

func isCompletedRunStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "failure", "timed_out":
		return true
	default:
		return false
	}
}

func normalizeFinalizeRunStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success":
		return "success"
	case "cancelled":
		return "cancelled"
	default:
		return "failure"
	}
}

func buildAgentContainerName(pipelineName, repoName, triggerEventID, runID string) string {
	sanitizedPipelineName := sanitizeInput(pipelineName)
	sanitizedTriggerID := sanitizeInput(strings.TrimSpace(triggerEventID))
	if sanitizedTriggerID == "" {
		sanitizedTriggerID = "no-trigger"
	} else if len(sanitizedTriggerID) > 8 {
		sanitizedTriggerID = sanitizedTriggerID[:8]
	}

	shortRunID := runID
	if len(shortRunID) > 8 {
		shortRunID = shortRunID[:8]
	}

	if strings.TrimSpace(repoName) != "" {
		sanitizedRepoName := sanitizeInput(repoName)
		return fmt.Sprintf("agent-%s-%s-%s-%s", sanitizedRepoName, sanitizedPipelineName, sanitizedTriggerID, shortRunID)
	}

	return fmt.Sprintf("agent-%s-%s-%s", sanitizedPipelineName, sanitizedTriggerID, shortRunID)
}

func normalizePipelineVersion(version string) string {
	sanitized := sanitizeInput(version)
	if sanitized == "" {
		return "latest"
	}
	return sanitized
}

func splitYAMLIdentifier(identifier string) (string, string, string, error) {
	trimmed := strings.TrimSpace(identifier)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "", "", "", fmt.Errorf("identifier cannot be empty")
	}

	normalized := filepath.ToSlash(trimmed)
	lower := strings.ToLower(normalized)
	var ext string
	switch {
	case strings.HasSuffix(lower, ".yaml"):
		ext = normalized[len(normalized)-len(".yaml"):]
		normalized = normalized[:len(normalized)-len(".yaml")]
	case strings.HasSuffix(lower, ".yml"):
		ext = normalized[len(normalized)-len(".yml"):]
		normalized = normalized[:len(normalized)-len(".yml")]
	}

	parts := strings.Split(normalized, "/")
	name := parts[len(parts)-1]
	if name == "" {
		return "", "", "", fmt.Errorf("identifier missing name")
	}

	var path string
	if len(parts) > 1 {
		path = strings.Join(parts[:len(parts)-1], "/")
	}
	if strings.Contains(path, "..") {
		return "", "", "", fmt.Errorf("identifier contains invalid path segments")
	}

	return path, name, ext, nil
}

func splitPipelineIdentifier(identifier string) (string, string, string, error) {
	return splitYAMLIdentifier(identifier)
}

func buildPipelineIdentifier(path, name string) string {
	if path == "" {
		return name
	}
	return path + "/" + name
}

func buildPipelineFilePath(path, name, ext string) string {
	if ext == "" {
		ext = ".yaml"
	}
	if path == "" {
		return name + ext
	}
	return path + "/" + name + ext
}

func splitStepIdentifier(identifier string) (string, string, string, error) {
	return splitYAMLIdentifier(identifier)
}

func buildStepIdentifier(path, name string) string {
	return buildPipelineIdentifier(path, name)
}

func parseEnvironmentFilePath(rel string) (string, bool, error) {
	lower := strings.ToLower(rel)
	if !strings.HasSuffix(lower, "env.yaml") && !strings.HasSuffix(lower, "env.yml") {
		return "", false, nil
	}

	base := filepath.Base(rel)
	if !strings.EqualFold(base, "env.yaml") && !strings.EqualFold(base, "env.yml") {
		return "", false, nil
	}

	envPath := strings.TrimSuffix(rel[:len(rel)-len(base)], "/")
	envPath = strings.Trim(envPath, "/")
	if envPath != "" {
		if strings.Contains(envPath, "..") {
			return "", false, fmt.Errorf("environment path contains invalid segments")
		}
		segments := strings.Split(envPath, "/")
		for _, segment := range segments {
			if segment == "" {
				return "", false, fmt.Errorf("environment path contains empty segments")
			}
		}
		envPath = filepath.ToSlash(envPath)
	}

	return envPath, true, nil
}

func parseGitHubRepoURL(raw string) (string, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", fmt.Errorf("config repository URL is empty")
	}

	trimmed = strings.TrimSuffix(trimmed, ".git")

	if strings.HasPrefix(trimmed, "git@") {
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid config repository URL: %s", raw)
		}
		trimmed = parts[1]
	}

	if strings.Contains(trimmed, "://") {
		u, err := url.Parse(trimmed)
		if err != nil {
			return "", "", fmt.Errorf("invalid config repository URL: %w", err)
		}
		path := strings.Trim(u.Path, "/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			return "", "", fmt.Errorf("invalid config repository URL: %s", raw)
		}
		return parts[len(parts)-2], parts[len(parts)-1], nil
	}

	trimmed = strings.Trim(trimmed, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid config repository URL: %s", raw)
	}

	return parts[len(parts)-2], parts[len(parts)-1], nil
}

func (a *App) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var group Group
	if err := httpapi.DecodeJSON(r, &group); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	group.Description = strings.TrimSpace(group.Description)

	if group.ParentID != nil {
		parentResource, err := a.folderGrantResourceByGroupID(r.Context(), *group.ParentID)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		if !a.requireAAADecision(w, r, "folder.create", model.ResourceRef{Type: grantResourceFolder, ID: parentResource.ID}) {
			return
		}
	} else if !a.requireAAADecision(w, r, "folder.create", model.ResourceRef{Type: grantResourceFolder, ID: "*"}) {
		return
	}

	query := `INSERT INTO groups (name, parent_id, description) VALUES ($1, $2, $3) RETURNING id`
	err := a.db.QueryRow(context.Background(), query, group.Name, group.ParentID, group.Description).Scan(&group.ID)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") {
			http.Error(w, "A folder or repository with this name already exists.", http.StatusConflict)
			return
		}
		log.Error().Err(err).Msg("Failed to create group")
		http.Error(w, "Failed to create group", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(group)
}

func (a *App) handleGetGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(context.Background(), "SELECT id, name, parent_id, description FROM groups")
	if err != nil {
		log.Error().Err(err).Msg("Failed to query groups from database")
		http.Error(w, "Failed to retrieve groups", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var allGroups []Group
	groupMap := make(map[int]*Group)

	for rows.Next() {
		var g Group
		var parentID sql.NullInt32
		var description sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &parentID, &description); err != nil {
			log.Error().Err(err).Msg("Failed to scan group row")
			http.Error(w, "Error processing groups", http.StatusInternalServerError)
			return
		}
		if parentID.Valid {
			pid := int(parentID.Int32)
			g.ParentID = &pid
		}
		if description.Valid {
			g.Description = description.String
		}
		allGroups = append(allGroups, g)
	}
	if rows.Err() != nil {
		log.Error().Err(rows.Err()).Msg("Error iterating over group rows")
		http.Error(w, "Error retrieving groups", http.StatusInternalServerError)
		return
	}

	for i := range allGroups {
		groupMap[allGroups[i].ID] = &allGroups[i]
	}

	pathRecords, err := a.folderPathRecords(r.Context())
	if err != nil {
		http.Error(w, "Failed to resolve folder paths", http.StatusInternalServerError)
		return
	}

	resources := make([]model.ResourceRef, 0, len(allGroups))
	resourceByGroupID := make(map[int]model.ResourceRef, len(allGroups))
	for _, group := range allGroups {
		record, ok := pathRecords[group.ID]
		if !ok || strings.TrimSpace(record.Path) == "" {
			continue
		}
		resource := model.ResourceRef{Type: grantResourceFolder, ID: record.Path}
		resources = append(resources, resource)
		resourceByGroupID[group.ID] = resource
	}

	allowedSet, err := a.allowedResourceSet(r, "folder.list", resources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	query := `
        SELECT g.id, MAX(r.started_at)
        FROM groups g
        JOIN pipeline_runs r ON g.id = r.group_id
        GROUP BY g.id
    `
	runRows, err := a.db.Query(context.Background(), query)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query last run times for groups")
	} else {
		defer runRows.Close()
		for runRows.Next() {
			var groupID int
			var lastRunAt sql.NullTime
			if err := runRows.Scan(&groupID, &lastRunAt); err == nil {
				if lastRunAt.Valid {
					if group, ok := groupMap[groupID]; ok {
						group.LastRunAt = &lastRunAt.Time
					}
				}
			}
		}
	}

	filtered := make([]Group, 0, len(allGroups))
	for _, group := range allGroups {
		resource, ok := resourceByGroupID[group.ID]
		if !ok {
			continue
		}
		if _, ok := allowedSet[resourceKey(resource)]; !ok {
			continue
		}
		filtered = append(filtered, group)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filtered)
}

func (a *App) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.Atoi(r.PathValue("groupID"))
	if err != nil {
		http.Error(w, "Invalid group ID", http.StatusBadRequest)
		return
	}

	var group Group
	if err := httpapi.DecodeJSON(r, &group); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	group.Description = strings.TrimSpace(group.Description)

	resource, err := a.folderGrantResourceByGroupID(r.Context(), groupID)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	if !a.requireAAADecision(w, r, "folder.update", model.ResourceRef{Type: grantResourceFolder, ID: resource.ID}) {
		return
	}

	query := `UPDATE groups SET name = $1, parent_id = $2, description = $3, updated_at = NOW() WHERE id = $4`
	_, err = a.db.Exec(context.Background(), query, group.Name, group.ParentID, group.Description, groupID)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") {
			http.Error(w, "A folder or repository with this name already exists.", http.StatusConflict)
			return
		}
		log.Error().Err(err).Msg("Failed to update group")
		http.Error(w, "Failed to update group", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a *App) handleMoveGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.Atoi(r.PathValue("groupID"))
	if err != nil {
		http.Error(w, "Invalid group ID", http.StatusBadRequest)
		return
	}

	var payload struct {
		ParentID *int `json:"parent_id"`
	}
	if err := httpapi.DecodeJSON(r, &payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	resource, err := a.folderGrantResourceByGroupID(r.Context(), groupID)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	if !a.requireAAADecision(w, r, "folder.move", model.ResourceRef{Type: grantResourceFolder, ID: resource.ID}) {
		return
	}
	if payload.ParentID != nil {
		parentResource, err := a.folderGrantResourceByGroupID(r.Context(), *payload.ParentID)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not found") {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		if !a.requireAAADecision(w, r, "folder.create", model.ResourceRef{Type: grantResourceFolder, ID: parentResource.ID}) {
			return
		}
	} else if !a.requireAAADecision(w, r, "folder.create", model.ResourceRef{Type: grantResourceFolder, ID: "*"}) {
		return
	}

	// Validation: Prevent moving a group into itself or its own children.
	if payload.ParentID != nil {
		if groupID == *payload.ParentID {
			http.Error(w, "Cannot move a folder into itself.", http.StatusBadRequest)
			return
		}

		var isChild bool
		query := `
			WITH RECURSIVE Descendants AS (
				SELECT id, parent_id FROM groups WHERE id = $1
				UNION ALL
				SELECT g.id, g.parent_id FROM groups g
				INNER JOIN Descendants d ON g.id = d.parent_id
			)
			SELECT EXISTS (SELECT 1 FROM Descendants WHERE id = $2)
		`
		err := a.db.QueryRow(context.Background(), query, *payload.ParentID, groupID).Scan(&isChild)
		if err != nil {
			log.Error().Err(err).Msg("Failed during ancestry check for group move")
			http.Error(w, "Server error during validation.", http.StatusInternalServerError)
			return
		}
		if isChild {
			http.Error(w, "Cannot move a folder into one of its own subfolders.", http.StatusBadRequest)
			return
		}
	}

	_, err = a.db.Exec(context.Background(), "UPDATE groups SET parent_id = $1, updated_at = NOW() WHERE id = $2", payload.ParentID, groupID)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") {
			http.Error(w, "A folder or repository with the same name already exists in the target location.", http.StatusConflict)
			return
		}
		log.Error().Err(err).Msg("Failed to update group parent")
		http.Error(w, "Failed to move group", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.Atoi(r.PathValue("groupID"))
	if err != nil {
		http.Error(w, "Invalid group ID", http.StatusBadRequest)
		return
	}

	action, resource, err := a.groupDeleteAuthorizationTarget(r.Context(), groupID)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	if !a.requireAAADecision(w, r, action, resource) {
		return
	}

	_, err = a.db.Exec(context.Background(), "DELETE FROM groups WHERE id = $1", groupID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete group")
		http.Error(w, "Failed to delete group", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) groupDeleteAuthorizationTarget(ctx context.Context, groupID int) (string, model.ResourceRef, error) {
	if a == nil || a.db == nil {
		return "", model.ResourceRef{}, fmt.Errorf("database unavailable")
	}

	var groupName string
	if err := a.db.QueryRow(ctx, `SELECT name FROM groups WHERE id = $1`, groupID).Scan(&groupName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return "", model.ResourceRef{}, fmt.Errorf("resource not found")
		}
		return "", model.ResourceRef{}, err
	}

	resource, err := a.folderGrantResourceByGroupID(ctx, groupID)
	if err != nil {
		return "", model.ResourceRef{}, err
	}
	action, resourceRef := groupDeleteAuthorizationTargetFromName(groupName, resource)
	return action, resourceRef, nil
}

func groupDeleteAuthorizationTargetFromName(groupName string, folderResource accessGrantResource) (string, model.ResourceRef) {
	repositoryID := strings.Trim(strings.TrimSpace(groupName), "/")
	if strings.Contains(repositoryID, "/") {
		return "repository.delete", model.ResourceRef{Type: grantResourceRepo, ID: repositoryID}
	}
	return "folder.delete", model.ResourceRef{Type: grantResourceFolder, ID: folderResource.ID}
}

func (a *App) handleListRepoBranches(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)

	query := `
		SELECT DISTINCT git_ref
		FROM pipeline_runs
		WHERE git_repo_name = $1 AND git_ref IS NOT NULL
		ORDER BY git_ref ASC
	`

	rows, err := a.db.Query(context.Background(), query, fullName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query branches from database")
		http.Error(w, "Failed to retrieve branches", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var branches []string
	for rows.Next() {
		var branch sql.NullString
		if err := rows.Scan(&branch); err != nil {
			log.Error().Err(err).Msg("Failed to scan branch name")
			http.Error(w, "Failed to process branches", http.StatusInternalServerError)
			return
		}
		if branch.Valid {
			branches = append(branches, strings.TrimPrefix(branch.String, "refs/heads/"))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(branches)
}

func main() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yml"
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to load config from %s", configPath)
	}

	if cfg.MasterKey == "" {
		log.Fatal().Msg("NOPSAI_MASTER_KEY environment variable is not set. This is required for secret encryption.")
	}
	key := sha256.Sum256([]byte(cfg.MasterKey))

	if cfg.JWTExpiryMinutes == 0 {
		cfg.JWTExpiryMinutes = 60
	}
	if cfg.IdleTimeoutMinutes == 0 {
		cfg.IdleTimeoutMinutes = 30
	}
	if cfg.RefreshTokenTTLMinutes == 0 {
		cfg.RefreshTokenTTLMinutes = 60 * 24 * 30
	}
	if cfg.RateLimitLoginPerMinute == 0 {
		cfg.RateLimitLoginPerMinute = 10
	}
	if cfg.LoginLockoutThreshold == 0 {
		cfg.LoginLockoutThreshold = 5
	}
	if cfg.LoginLockoutWindowMin == 0 {
		cfg.LoginLockoutWindowMin = 15
	}
	if !cfg.AuthProviderLocalEnabled {
		cfg.AuthProviderLocalEnabled = true
	}

	logLevel, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.Warn().Msgf("Invalid log level '%s', defaulting to 'info'", cfg.LogLevel)
		logLevel = zerolog.InfoLevel
	}
	if cfg.LogFormat == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})
	}
	zerolog.SetGlobalLevel(logLevel)

	envFilePath := os.Getenv("ENV_FILE_PATH")
	if envFilePath == "" {
		envFilePath = filepath.Join(filepath.Dir(configPath), ".env")
	}

	if strings.TrimSpace(cfg.NopsaiListenAddress) == "" {
		cfg.NopsaiListenAddress = "0.0.0.0:8080"
	}

	var dbpool *pgxpool.Pool
	for i := 0; i < 5; i++ {
		dbpool, err = pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err == nil {
			if err = dbpool.Ping(context.Background()); err == nil {
				log.Info().Msg("Successfully connected to the database.")
				break
			}
		}
		log.Warn().Err(err).Msgf("Unable to connect to database. Retrying in 3 seconds...")
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database after multiple retries")
	}
	defer dbpool.Close()

	if err := ensureDefaultAdmin(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure default admin")
	}
	if err := ensureProductAccessBootstrap(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure product access roles")
	}
	if err := ensureConfigRepositorySchema(context.Background(), dbpool); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure config repository schema")
	}

	dispatcherAddr := strings.TrimSpace(cfg.DispatcherAddress)
	if dispatcherAddr == "" {
		dispatcherAddr = "localhost:9090"
	}
	dispatcherConn, err := grpc.Dial(dispatcherAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal().Err(err).Str("addr", dispatcherAddr).Msg("Failed to connect to dispatcher")
	}
	defer dispatcherConn.Close()

	authCfg := auth.Config{
		LocalEnabled:       cfg.AuthProviderLocalEnabled,
		SigningKey:         cfg.JWTSigningKey,
		JWTIssuer:          cfg.JWTIssuer,
		JWTAudience:        cfg.JWTAudience,
		AccessTTL:          time.Duration(cfg.JWTExpiryMinutes) * time.Minute,
		RefreshTTL:         time.Duration(cfg.RefreshTokenTTLMinutes) * time.Minute,
		LoginRateLimit:     cfg.RateLimitLoginPerMinute,
		LoginLockoutThresh: cfg.LoginLockoutThreshold,
		LoginLockoutWindow: time.Duration(cfg.LoginLockoutWindowMin) * time.Minute,
	}

	authService, err := auth.NewService(context.Background(), dbpool, authCfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize authentication service")
	}
	authzEnforcer, err := authz.NewEnforcer(context.Background(), dbpool)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load RBAC policies")
	}
	aaaClient := aaaclient.New(cfg.AAAAPIURL, cfg.AAASharedToken)
	aaaLocal := aaaauthz.NewEvaluator(aaastore.NewPGStore(dbpool))
	auditLogger := audit.NewLogger(dbpool)

	app := &App{
		db:          dbpool,
		cfg:         cfg,
		dispatcher:  proto.NewDispatcherServiceClient(dispatcherConn),
		encKey:      key[:],
		httpClient:  proxyhttp.NewInternalAwareClient(10 * time.Second),
		store:       store.NewPGStore(dbpool),
		configPath:  configPath,
		envFilePath: envFilePath,
		authService: authService,
		aaaClient:   aaaClient,
		aaaLocal:    aaaLocal,
		authz:       authzEnforcer,
		auditLogger: auditLogger,
		configSyncStatus: ConfigSyncStatus{
			Status:  "idle",
			Message: "No configuration sync has been requested yet.",
		},
		idleTimeout: time.Duration(cfg.IdleTimeoutMinutes) * time.Minute,
	}

	handler := app.buildHTTPHandler()

	server := &http.Server{
		Addr:    cfg.NopsaiListenAddress,
		Handler: handler,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Info().Msgf("Nopsai API server listening on %s", cfg.NopsaiListenAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	<-stop

	log.Info().Msg("Shutting down the server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	app.db.Close()
	log.Info().Msg("Server exiting")
}
