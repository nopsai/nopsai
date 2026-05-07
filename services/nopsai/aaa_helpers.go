package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/auth"
)

type aaaContextKey string

const ctxKeyAAASubject aaaContextKey = "aaa-subject"

func isAuthenticatedOnlyPath(path string) bool {
	switch strings.TrimSpace(path) {
	case "/v1/auth/me", "/v1/auth/password", "/v1/auth/email":
		return true
	default:
		return false
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
	return model.Subject{
		Type:  model.SubjectTypeUser,
		Sub:   strings.TrimSpace(claims.Sub),
		Email: strings.TrimSpace(claims.Email),
	}
}

func withAAASubject(ctx context.Context, subject model.Subject) context.Context {
	return context.WithValue(ctx, ctxKeyAAASubject, subject)
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

func (a *App) requireAAADecision(w http.ResponseWriter, r *http.Request, action string, resource model.ResourceRef) bool {
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "missing authorization subject", http.StatusUnauthorized)
		return false
	}
	if a.aaaClient == nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return false
	}

	decision, err := a.aaaClient.Check(r.Context(), subject, action, resource, a.aaaRequestContext(r))
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
	if a.aaaClient == nil {
		return nil, fmt.Errorf("authorization unavailable")
	}

	allowed, err := a.aaaClient.Filter(r.Context(), subject, action, resources, a.aaaRequestContext(r))
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
