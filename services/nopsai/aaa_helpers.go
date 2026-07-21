package nopsai

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/auth"
)

type aaaContextKey string

const ctxKeyAAASubject aaaContextKey = "aaa-subject"

const aaaRetryBackoff = 10 * time.Second

type AAASubjectResolver interface {
	Introspect(ctx context.Context, subject model.Subject) (*model.IntrospectResponse, error)
}

type AAAAuthorizationClient interface {
	Check(ctx context.Context, subject model.Subject, action string, resource model.ResourceRef, requestContext map[string]any) (model.Decision, error)
	BatchCheck(ctx context.Context, subject model.Subject, checks []model.BatchCheckItem, requestContext map[string]any) ([]model.Decision, error)
	Filter(ctx context.Context, subject model.Subject, action string, resources []model.ResourceRef, requestContext map[string]any) ([]model.ResourceRef, error)
}

type AAAAuditRecorder interface {
	RecordAudit(ctx context.Context, req model.AuditRecordRequest) error
}

type AAAClient interface {
	AAASubjectResolver
	AAAAuthorizationClient
	AAAAuditRecorder
}

func isAuthenticatedOnlyPath(path string) bool {
	path = strings.TrimSpace(path)
	switch path {
	case "/v1/auth/me", "/v1/auth/password", "/v1/auth/email", "/v1/git/events":
		return true
	default:
		return path == "/v1/auth/personal-tokens" ||
			path == "/v1/monitoring/dispatcher" ||
			path == "/v1/mcp" ||
			path == "/v1/assistant/config" ||
			path == "/v1/assistant/llm-profiles" ||
			path == "/v1/assistant/conversations" ||
			path == "/v1/internal/dispatcher/routing" ||
			strings.HasPrefix(path, "/internal/v1/runtime-config/") ||
			strings.HasPrefix(path, "/v1/internal/runs/") ||
			strings.HasPrefix(path, "/v1/assistant/conversations/") ||
			strings.HasPrefix(path, "/v1/auth/personal-tokens/")
	}
}

func (a *App) buildAAASubject(claims *auth.Claims) model.Subject {
	if isDispatcherInternalClaims(claims) {
		return model.Subject{
			Type: model.SubjectTypeInternalService,
			ID:   "dispatcher",
			Sub:  strings.TrimSpace(claims.Sub),
		}
	}
	if strings.EqualFold(strings.TrimSpace(claims.Provider), auth.ProviderServiceAccountToken) ||
		strings.EqualFold(strings.TrimSpace(claims.Provider), auth.ProviderServiceAccount) {
		return model.Subject{
			Type: model.SubjectTypeServiceAccount,
			ID:   strings.Trim(strings.TrimSpace(claims.Sub), "/"),
			Sub:  strings.TrimSpace(claims.Sub),
		}
	}
	return model.Subject{
		Type:  model.SubjectTypeUser,
		Sub:   strings.TrimSpace(claims.Sub),
		Email: strings.TrimSpace(claims.Email),
	}
}

func withAAASubject(ctx context.Context, subject model.Subject) context.Context {
	return context.WithValue(ctx, ctxKeyAAASubject, subject)
}

func (a *App) withDispatcherInternalSubject(r *http.Request) *http.Request {
	claims := &auth.Claims{
		Sub:      "dispatcher",
		Provider: "internal-service",
	}
	ctx := auth.WithClaims(r.Context(), claims)
	ctx = withAAASubject(ctx, a.buildAAASubject(claims))
	return r.WithContext(ctx)
}

func aaaSubjectFromContext(ctx context.Context) (model.Subject, bool) {
	if ctx == nil {
		return model.Subject{}, false
	}
	value := ctx.Value(ctxKeyAAASubject)
	if value == nil {
		return model.Subject{}, false
	}
	subject, ok := value.(model.Subject)
	return subject, ok
}

func (a *App) currentAAASubject(r *http.Request) (model.Subject, bool) {
	if r == nil {
		return model.Subject{}, false
	}
	if subject, ok := aaaSubjectFromContext(r.Context()); ok {
		return subject, true
	}
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		return model.Subject{}, false
	}
	return a.buildAAASubject(claims), true
}

func (a *App) aaaRequestContext(r *http.Request) map[string]any {
	requestID, _ := r.Context().Value(ctxKeyRequestID).(string)
	return map[string]any{
		"request_id": requestID,
		"path":       r.URL.Path,
		"method":     r.Method,
	}
}

func (a *App) aaaAvailable() bool {
	return a != nil && (a.aaaClient != nil || a.aaaLocal != nil)
}

func (a *App) shouldTryRemoteAAA() bool {
	if a == nil || a.aaaClient == nil {
		return false
	}
	if a.aaaLocal == nil {
		return true
	}
	a.aaaRemoteMu.Lock()
	defer a.aaaRemoteMu.Unlock()
	return a.aaaRetryAfter.IsZero() || !time.Now().Before(a.aaaRetryAfter)
}

func (a *App) markAAARemoteHealthy() {
	if a == nil || a.aaaLocal == nil {
		return
	}
	a.aaaRemoteMu.Lock()
	defer a.aaaRemoteMu.Unlock()
	a.aaaRetryAfter = time.Time{}
}

func (a *App) markAAARemoteUnavailable() {
	if a == nil || a.aaaLocal == nil {
		return
	}
	a.aaaRemoteMu.Lock()
	defer a.aaaRemoteMu.Unlock()
	a.aaaRetryAfter = time.Now().Add(aaaRetryBackoff)
}

func (a *App) aaaFallback(op string, err error) (AAAClient, bool) {
	if a == nil || a.aaaLocal == nil {
		return nil, false
	}
	a.markAAARemoteUnavailable()
	log.Warn().Err(err).Str("operation", op).Msg("AAA service unavailable; falling back to in-process evaluator")
	return a.aaaLocal, true
}

func (a *App) aaaIntrospect(ctx context.Context, subject model.Subject) (*model.IntrospectResponse, error) {
	if a == nil {
		return nil, fmt.Errorf("authorization unavailable")
	}
	if a.shouldTryRemoteAAA() {
		resp, err := a.aaaClient.Introspect(ctx, subject)
		if err == nil {
			a.markAAARemoteHealthy()
			return resp, nil
		}
		if fallback, ok := a.aaaFallback("introspect", err); ok {
			return fallback.Introspect(ctx, subject)
		}
		return nil, err
	}
	if a.aaaLocal != nil {
		return a.aaaLocal.Introspect(ctx, subject)
	}
	return nil, fmt.Errorf("authorization unavailable")
}

func (a *App) aaaCheck(ctx context.Context, subject model.Subject, action string, resource model.ResourceRef, requestContext map[string]any) (model.Decision, error) {
	if a == nil {
		return model.Decision{}, fmt.Errorf("authorization unavailable")
	}
	if a.shouldTryRemoteAAA() {
		decision, err := a.aaaClient.Check(ctx, subject, action, resource, requestContext)
		if err == nil {
			a.markAAARemoteHealthy()
			return decision, nil
		}
		if fallback, ok := a.aaaFallback("check", err); ok {
			return fallback.Check(ctx, subject, action, resource, requestContext)
		}
		return model.Decision{}, err
	}
	if a.aaaLocal != nil {
		return a.aaaLocal.Check(ctx, subject, action, resource, requestContext)
	}
	return model.Decision{}, fmt.Errorf("authorization unavailable")
}

func (a *App) aaaBatchCheck(ctx context.Context, subject model.Subject, checks []model.BatchCheckItem, requestContext map[string]any) ([]model.Decision, error) {
	if a == nil {
		return nil, fmt.Errorf("authorization unavailable")
	}
	if len(checks) == 0 {
		return nil, nil
	}
	if a.shouldTryRemoteAAA() {
		decisions, err := a.aaaClient.BatchCheck(ctx, subject, checks, requestContext)
		if err == nil {
			a.markAAARemoteHealthy()
			return decisions, nil
		}
		if fallback, ok := a.aaaFallback("batch_check", err); ok {
			return fallback.BatchCheck(ctx, subject, checks, requestContext)
		}
		return nil, err
	}
	if a.aaaLocal != nil {
		return a.aaaLocal.BatchCheck(ctx, subject, checks, requestContext)
	}
	return nil, fmt.Errorf("authorization unavailable")
}

func (a *App) aaaFilter(ctx context.Context, subject model.Subject, action string, resources []model.ResourceRef, requestContext map[string]any) ([]model.ResourceRef, error) {
	if a == nil {
		return nil, fmt.Errorf("authorization unavailable")
	}
	if a.shouldTryRemoteAAA() {
		allowed, err := a.aaaClient.Filter(ctx, subject, action, resources, requestContext)
		if err == nil {
			a.markAAARemoteHealthy()
			return allowed, nil
		}
		if fallback, ok := a.aaaFallback("filter", err); ok {
			return fallback.Filter(ctx, subject, action, resources, requestContext)
		}
		return nil, err
	}
	if a.aaaLocal != nil {
		return a.aaaLocal.Filter(ctx, subject, action, resources, requestContext)
	}
	return nil, fmt.Errorf("authorization unavailable")
}

func (a *App) aaaRecordAudit(ctx context.Context, req model.AuditRecordRequest) error {
	if a == nil {
		return fmt.Errorf("authorization unavailable")
	}
	if a.shouldTryRemoteAAA() {
		if err := a.aaaClient.RecordAudit(ctx, req); err == nil {
			a.markAAARemoteHealthy()
			return nil
		} else if fallback, ok := a.aaaFallback("record_audit", err); ok {
			return fallback.RecordAudit(ctx, req)
		} else {
			return err
		}
	}
	if a.aaaLocal != nil {
		return a.aaaLocal.RecordAudit(ctx, req)
	}
	return fmt.Errorf("authorization unavailable")
}

func (a *App) requireAAADecision(w http.ResponseWriter, r *http.Request, action string, resource model.ResourceRef) bool {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "missing authorization subject", http.StatusUnauthorized)
		return false
	}
	if a.aaaClient == nil {
		if a.aaaLocal == nil {
			http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
			return false
		}
	}

	decision, err := a.aaaCheck(r.Context(), subject, action, resource, a.aaaRequestContext(r))
	if err != nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return false
	}
	if !decision.Allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func (a *App) allowedResourceSet(r *http.Request, action string, resources []model.ResourceRef) (map[string]struct{}, error) {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		return nil, fmt.Errorf("missing authorization subject")
	}
	if !a.aaaAvailable() {
		return nil, fmt.Errorf("authorization unavailable")
	}

	allowed, err := a.aaaFilter(r.Context(), subject, action, resources, a.aaaRequestContext(r))
	if err != nil {
		return nil, err
	}

	allowedSet := make(map[string]struct{}, len(allowed))
	for _, resource := range allowed {
		allowedSet[resourceKey(resource)] = struct{}{}
	}
	return allowedSet, nil
}

func resourceKey(resource model.ResourceRef) string {
	return strings.TrimSpace(resource.Type) + "|" + strings.TrimSpace(resource.ID)
}
