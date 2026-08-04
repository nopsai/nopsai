package nopsai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"nopsai/config"
	"nopsai/pkg/correlation"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/systemlogs"
	"nopsai/services/nopsai/pkg/audit"
	"nopsai/services/nopsai/pkg/auth"

	"github.com/rs/zerolog/log"
)

const systemLogRedactionWarning = "Secret redaction is best effort. Avoid emitting sensitive values and restrict access to operational logs."

const runnerRecentDisconnectWindow = 15 * time.Minute

type systemLogRateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string][]time.Time
}

func newSystemLogRateLimiter(limit int, window time.Duration) *systemLogRateLimiter {
	return &systemLogRateLimiter{limit: limit, window: window, entries: make(map[string][]time.Time)}
}

func (l *systemLogRateLimiter) Allow(key string, now time.Time) bool {
	if l == nil || l.limit <= 0 || l.window <= 0 {
		return true
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = "anonymous"
	}
	cutoff := now.Add(-l.window)
	l.mu.Lock()
	defer l.mu.Unlock()
	entries := l.entries[key]
	first := 0
	for first < len(entries) && entries[first].Before(cutoff) {
		first++
	}
	entries = append(entries[first:], now)
	l.entries[key] = entries
	return len(entries) <= l.limit
}

func (a *App) handleListSystemLogSources(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	if a == nil || a.systemLogs == nil {
		http.Error(w, "system logs unavailable", http.StatusServiceUnavailable)
		return
	}
	sources, err := a.systemLogs.ListSources(r.Context())
	if err != nil {
		http.Error(w, "failed to list system log sources", http.StatusBadGateway)
		return
	}
	sources = a.mergeRegisteredRunnerSystemLogSources(r.Context(), sources)
	sources = a.filterVisibleSystemLogSources(r.Context(), sources)
	resources := make([]model.ResourceRef, 0, len(sources))
	for _, source := range sources {
		resources = append(resources, model.ResourceRef{Type: "system_log", ID: source.ID})
	}
	allowed, err := a.allowedResourceSet(r, "system_log.read", resources)
	if err != nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	visible := sources[:0]
	for _, source := range sources {
		if _, ok := allowed[resourceKey(model.ResourceRef{Type: "system_log", ID: source.ID})]; ok {
			visible = append(visible, source)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sources": visible, "redaction_warning": systemLogRedactionWarning,
		"max_tail_lines": a.systemLogs.MaxTailLines(),
	})
}

func (a *App) handleTailSystemLogs(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	if a == nil || a.systemLogs == nil {
		http.Error(w, "system logs unavailable", http.StatusServiceUnavailable)
		return
	}
	lines, err := systemLogTailLines(r, 500)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sourceID := r.PathValue("sourceID")
	if err := a.ensureSystemLogSourceVisible(r.Context(), sourceID); err != nil {
		writeSystemLogError(w, err)
		return
	}
	entries, err := a.systemLogs.Tail(r.Context(), sourceID, lines)
	if err != nil {
		writeSystemLogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "redaction_warning": systemLogRedactionWarning})
}

func (a *App) handleStreamSystemLogs(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.systemLogs == nil {
		http.Error(w, "system logs unavailable", http.StatusServiceUnavailable)
		return
	}
	lines, err := systemLogTailLines(r, 500)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	claims, _ := auth.ClaimsFromContext(r.Context())
	actorKey := r.RemoteAddr
	if claims != nil && strings.TrimSpace(claims.Sub) != "" {
		actorKey = claims.Sub
	}
	if !a.systemLogLimiter.Allow(actorKey, time.Now()) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, systemlogs.ErrReconnectRateLimit.Error(), http.StatusTooManyRequests)
		return
	}

	sourceID := r.PathValue("sourceID")
	if err := a.ensureSystemLogSourceVisible(r.Context(), sourceID); err != nil {
		writeSystemLogError(w, err)
		return
	}
	subscription, err := a.systemLogs.Subscribe(r.Context(), sourceID, r.URL.Query().Get("cursor"), lines)
	if err != nil {
		writeSystemLogError(w, err)
		return
	}
	defer subscription.Close()
	a.auditSystemLogStream(context.WithoutCancel(r.Context()), claims, sourceID, "system_log.stream.open", "success")
	defer a.auditSystemLogStream(context.WithoutCancel(r.Context()), claims, sourceID, "system_log.stream.close", "success")

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	if err := writeSystemLogSSE(w, "status", "", map[string]string{"state": "connected", "source_id": sourceID}); err != nil {
		return
	}
	if subscription.Reset {
		if err := writeSystemLogSSE(w, "reset", "", map[string]string{"reason": "cursor_expired"}); err != nil {
			return
		}
		// A reset marks the gap first; a fresh bounded tail is then delivered on
		// the existing subscription channel before live following continues.
		_, _ = a.systemLogs.Tail(r.Context(), sourceID, lines)
	}
	for _, entry := range subscription.Replay {
		if err := writeSystemLogSSE(w, "log", entry.ID, entry); err != nil {
			return
		}
	}
	if err := controller.Flush(); err != nil {
		return
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case entry, ok := <-subscription.Entries:
			if !ok {
				return
			}
			if err := writeSystemLogSSE(w, "log", entry.ID, entry); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}
}

func (a *App) filterVisibleSystemLogSources(ctx context.Context, sources []systemlogs.SourceStatus) []systemlogs.SourceStatus {
	needsRunnerVisibility := false
	for _, source := range sources {
		if _, ok := systemlogs.ParseRunnerSourceID(source.ID); ok {
			needsRunnerVisibility = true
			break
		}
	}
	if !needsRunnerVisibility {
		return sources
	}
	visible, err := a.visibleRunnerLogSourceIDs(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to resolve dispatcher runner visibility for system logs")
	}
	out := sources[:0]
	for _, source := range sources {
		if _, ok := systemlogs.ParseRunnerSourceID(source.ID); !ok {
			out = append(out, source)
			continue
		}
		if _, ok := visible[source.ID]; ok {
			out = append(out, source)
		}
	}
	return out
}

func (a *App) mergeRegisteredRunnerSystemLogSources(ctx context.Context, sources []systemlogs.SourceStatus) []systemlogs.SourceStatus {
	if a == nil {
		return sources
	}
	status, err := a.fetchDispatcherStatus(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to resolve dispatcher runner sources for system logs")
		return sources
	}
	revoked := a.revokedRunnerIDSet()
	runnerMetadataBySourceID := make(map[string]map[string]string, len(status.GetRunners()))
	for _, runner := range status.GetRunners() {
		runnerID := strings.TrimSpace(runner.GetRunnerId())
		if runnerID == "" {
			continue
		}
		if _, blocked := revoked[runnerID]; blocked {
			continue
		}
		sourceID := runnerLogSourceID(runnerID, runner.GetMetadata())
		if sourceID == "" {
			continue
		}
		runnerMetadataBySourceID[sourceID] = runner.GetMetadata()
	}

	seen := make(map[string]struct{}, len(sources)+len(status.GetRunners()))
	out := make([]systemlogs.SourceStatus, 0, len(sources)+len(status.GetRunners()))
	now := time.Now()
	for _, source := range sources {
		if source.ID == "" {
			continue
		}
		if metadata, ok := runnerMetadataBySourceID[source.ID]; ok {
			source = annotateRegisteredRunnerSystemLogSource(source, metadata, now)
		}
		seen[source.ID] = struct{}{}
		out = append(out, source)
	}
	for _, runner := range status.GetRunners() {
		runnerID := strings.TrimSpace(runner.GetRunnerId())
		if runnerID == "" {
			continue
		}
		if _, blocked := revoked[runnerID]; blocked {
			continue
		}
		sourceID := runnerLogSourceID(runnerID, runner.GetMetadata())
		if sourceID == "" {
			continue
		}
		if _, exists := seen[sourceID]; exists {
			continue
		}
		source, ok := systemlogs.NewRunnerSource(runnerID, "runner")
		if !ok {
			continue
		}
		seen[sourceID] = struct{}{}
		sourceStatus := systemlogs.SourceStatus{
			ID:            source.ID,
			DisplayName:   source.DisplayName,
			ContainerName: source.ContainerName,
			Available:     false,
			State:         "unavailable",
			Status:        registeredRunnerLogStatus(runner.GetMetadata()),
		}
		out = append(out, annotateRegisteredRunnerSystemLogSource(sourceStatus, runner.GetMetadata(), now))
	}
	return out
}

func (a *App) ensureSystemLogSourceVisible(ctx context.Context, sourceID string) error {
	if _, ok := systemlogs.ParseRunnerSourceID(sourceID); !ok {
		return nil
	}
	visible, err := a.visibleRunnerLogSourceIDs(ctx)
	if err != nil {
		return fmt.Errorf("dispatcher runner visibility unavailable: %w", err)
	}
	if _, ok := visible[strings.TrimSpace(sourceID)]; !ok {
		return systemlogs.ErrSourceNotFound
	}
	return nil
}

func (a *App) visibleRunnerLogSourceIDs(ctx context.Context) (map[string]struct{}, error) {
	status, err := a.fetchDispatcherStatus(ctx)
	if err != nil {
		return nil, err
	}
	revoked := a.revokedRunnerIDSet()
	visible := make(map[string]struct{}, len(status.GetRunners()))
	for _, runner := range status.GetRunners() {
		runnerID := strings.TrimSpace(runner.GetRunnerId())
		if runnerID == "" {
			continue
		}
		if _, blocked := revoked[runnerID]; blocked {
			continue
		}
		sourceID := runnerLogSourceID(runnerID, runner.GetMetadata())
		if sourceID != "" {
			visible[sourceID] = struct{}{}
		}
	}
	return visible, nil
}

func (a *App) revokedRunnerIDSet() map[string]struct{} {
	if a == nil {
		return nil
	}
	a.cfgMu.RLock()
	var ids []string
	if a.cfg != nil {
		ids = append([]string(nil), a.cfg.EjectedRunnerIDs...)
	}
	a.cfgMu.RUnlock()
	normalized := config.NormalizeRunnerIDs(ids)
	if len(normalized) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(normalized))
	for _, id := range normalized {
		out[id] = struct{}{}
	}
	return out
}

func runnerLogSourceID(runnerID string, metadata map[string]string) string {
	runnerID = strings.TrimSpace(runnerID)
	if runnerID == "" {
		return ""
	}
	sourceID := strings.TrimSpace(metadata["log_source_id"])
	if sourceID == "" {
		return systemlogs.RunnerSourceID(runnerID)
	}
	if sourceRunnerID, ok := systemlogs.ParseRunnerSourceID(sourceID); !ok || sourceRunnerID != runnerID {
		return systemlogs.RunnerSourceID(runnerID)
	}
	return sourceID
}

func registeredRunnerLogStatus(metadata map[string]string) string {
	switch strings.ToLower(strings.TrimSpace(metadata["runtime"])) {
	case "kubernetes":
		return "registered runner; Kubernetes pod not discovered by System Logs provider"
	case "docker":
		return "registered runner; Docker container not discovered by System Logs provider"
	default:
		return "registered runner; log source not discovered by System Logs provider"
	}
}

func annotateRegisteredRunnerSystemLogSource(source systemlogs.SourceStatus, metadata map[string]string, now time.Time) systemlogs.SourceStatus {
	if _, ok := systemlogs.ParseRunnerSourceID(source.ID); !ok {
		return source
	}
	if !runnerReachable(metadata) {
		source.Health = "dispatcher unreachable"
		source.Status = appendSystemLogSourceStatus(source.Status, runnerDispatcherLogStatusMessage("dispatcher connection unreachable", metadata))
		return source
	}
	if runnerRecentlyDisconnected(metadata, now) {
		source.Health = "recently reconnected"
		source.Status = appendSystemLogSourceStatus(source.Status, runnerDispatcherLogStatusMessage("dispatcher stream reconnected after disconnect", metadata))
	}
	return source
}

func runnerRecentlyDisconnected(metadata map[string]string, now time.Time) bool {
	disconnectedAt := strings.TrimSpace(metadata["last_disconnected_at"])
	if disconnectedAt == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, disconnectedAt)
	if err != nil {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	age := now.Sub(parsed)
	return age >= 0 && age <= runnerRecentDisconnectWindow
}

func runnerDispatcherLogStatusMessage(prefix string, metadata map[string]string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "dispatcher connection status changed"
	}
	if disconnectedAt := strings.TrimSpace(metadata["last_disconnected_at"]); disconnectedAt != "" {
		return prefix + " at " + disconnectedAt
	}
	return prefix
}

func appendSystemLogSourceStatus(current, addition string) string {
	current = strings.TrimSpace(current)
	addition = strings.TrimSpace(addition)
	if addition == "" {
		return current
	}
	if current == "" {
		return addition
	}
	if strings.Contains(current, addition) {
		return current
	}
	return current + "; " + addition
}

func systemLogTailLines(r *http.Request, fallback int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("lines"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("tail"))
	}
	if raw == "" {
		return fallback, nil
	}
	lines, err := strconv.Atoi(raw)
	if err != nil || lines < 0 {
		return 0, errors.New("lines must be a non-negative integer")
	}
	return lines, nil
}

func writeSystemLogSSE(w http.ResponseWriter, event, id string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
	return err
}

func writeSystemLogError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, systemlogs.ErrSourceNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, systemlogs.ErrCursorInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, systemlogs.ErrStreamLimit):
		http.Error(w, err.Error(), http.StatusTooManyRequests)
	default:
		http.Error(w, "system log provider unavailable", http.StatusBadGateway)
	}
}

func (a *App) auditSystemLogStream(ctx context.Context, claims *auth.Claims, sourceID, action, result string) {
	if a == nil || a.auditLogger == nil {
		return
	}
	metadata := map[string]any{"source_id": sourceID}
	if requestID := requestIDFromContext(ctx); requestID != "" {
		metadata["request_id"] = requestID
	}
	if traceparent := correlation.TraceparentFromContext(ctx); traceparent != "" {
		metadata["traceparent"] = traceparent
	}
	entry := audit.Entry{Action: action, Resource: "system_log:" + sourceID, Result: result, Metadata: metadata}
	if claims != nil {
		entry.ActorSub, entry.ActorEmail, entry.Provider = claims.Sub, claims.Email, claims.Provider
	}
	_ = a.auditLogger.Write(ctx, entry)
}
