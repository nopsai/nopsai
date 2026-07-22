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

	"nopsai/pkg/correlation"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/systemlogs"
	"nopsai/services/nopsai/pkg/audit"
	"nopsai/services/nopsai/pkg/auth"
)

const systemLogRedactionWarning = "Secret redaction is best effort. Avoid emitting sensitive values and restrict access to operational logs."

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
	entries, err := a.systemLogs.Tail(r.Context(), r.PathValue("sourceID"), lines)
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
