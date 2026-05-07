package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"nopsai/services/aaa/pkg/authz"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/aaa/pkg/store"

	"github.com/google/uuid"
)

const internalTokenHeader = "X-Internal-Token"

type Server struct {
	evaluator   *authz.Evaluator
	sharedToken string
}

func New(sharedToken string, evaluator *authz.Evaluator) *Server {
	return &Server{
		evaluator:   evaluator,
		sharedToken: strings.TrimSpace(sharedToken),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.Handle("/v1/authn/introspect", s.requireInternalToken(http.HandlerFunc(s.handleIntrospect)))
	mux.Handle("/v1/authz/check", s.requireInternalToken(http.HandlerFunc(s.handleCheck)))
	mux.Handle("/v1/authz/batch-check", s.requireInternalToken(http.HandlerFunc(s.handleBatchCheck)))
	mux.Handle("/v1/authz/filter", s.requireInternalToken(http.HandlerFunc(s.handleFilter)))
	mux.Handle("/v1/audit/record", s.requireInternalToken(http.HandlerFunc(s.handleRecordAudit)))
	return s.requestIDMiddleware(mux)
}

func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) requireInternalToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(r.URL.Path) == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if strings.TrimSpace(r.Header.Get(internalTokenHeader)) != s.sharedToken {
			http.Error(w, "invalid internal token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleIntrospect(w http.ResponseWriter, r *http.Request) {
	var req model.IntrospectRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	resp, err := s.evaluator.Introspect(r.Context(), req.Subject)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrSubjectInactive):
			http.Error(w, "subject inactive", http.StatusForbidden)
		case errors.Is(err, store.ErrSubjectNotFound):
			http.Error(w, "subject not found", http.StatusNotFound)
		default:
			http.Error(w, "failed to resolve subject", http.StatusServiceUnavailable)
		}
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCheck(w http.ResponseWriter, r *http.Request) {
	var req model.CheckRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	decision, err := s.evaluator.Check(r.Context(), req.Subject, req.Action, req.Resource, requestContext(r, req.Context))
	if err != nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, decision)
}

func (s *Server) handleBatchCheck(w http.ResponseWriter, r *http.Request) {
	var req model.BatchCheckRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	decisions, err := s.evaluator.BatchCheck(r.Context(), req.Subject, req.Checks, requestContext(r, req.Context))
	if err != nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, model.BatchCheckResponse{Decisions: decisions})
}

func (s *Server) handleFilter(w http.ResponseWriter, r *http.Request) {
	var req model.FilterRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	resources, err := s.evaluator.Filter(r.Context(), req.Subject, req.Action, req.Resources, requestContext(r, req.Context))
	if err != nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, model.FilterResponse{Resources: resources})
}

func (s *Server) handleRecordAudit(w http.ResponseWriter, r *http.Request) {
	var req model.AuditRecordRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.RequestID == "" {
		req.RequestID = requestIDFromRequest(r)
	}
	if req.Context == nil {
		req.Context = map[string]any{}
	}
	if req.Context["request_id"] == nil {
		req.Context["request_id"] = req.RequestID
	}
	if req.Context["path"] == nil {
		req.Context["path"] = r.URL.Path
	}
	if req.Context["method"] == nil {
		req.Context["method"] = r.Method
	}

	if err := s.evaluator.RecordAudit(r.Context(), req); err != nil {
		http.Error(w, "failed to record audit event", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

type requestIDContextKey struct{}

func requestIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if requestID, _ := r.Context().Value(requestIDContextKey{}).(string); requestID != "" {
		return requestID
	}
	return strings.TrimSpace(r.Header.Get("X-Request-ID"))
}

func requestContext(r *http.Request, values map[string]any) map[string]any {
	if values == nil {
		values = make(map[string]any)
	}
	if values["request_id"] == nil {
		values["request_id"] = requestIDFromRequest(r)
	}
	if values["path"] == nil {
		values["path"] = r.URL.Path
	}
	if values["method"] == nil {
		values["method"] = r.Method
	}
	return values
}
