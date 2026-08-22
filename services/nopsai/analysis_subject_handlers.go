package nopsai

import (
	"net/http"
	"strings"

	"nopsai/pkg/httpapi"
)

// The subject analysis route is the same engine the assistant calls through
// nopsai.analyze_team and nopsai.analyze_pipeline. One implementation keeps the
// modal, the chat, and any future report from disagreeing about a finding or a
// score, which is the whole point of moving analysis to the server.
//
// It needs no LLM and is not gated by assistant feature flags: every piece of
// evidence is read through the permission-checked API bridge as the caller, so
// authorization is exactly what the caller could read themselves.
type analysisSubjectRequest struct {
	SubjectID string `json:"subject_id"`
	Days      int    `json:"days"`
}

func analysisSubjectTypeFromPath(path string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(path), "/")
	if index := strings.LastIndex(trimmed, "/"); index >= 0 {
		return strings.ToLower(trimmed[index+1:])
	}
	return ""
}

func (a *App) handleAnalyzeSubject(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "authentication is required", http.StatusUnauthorized)
		return
	}

	var req analysisSubjectRequest
	if err := httpapi.DecodeOptionalJSON(r, &req); err != nil {
		http.Error(w, "invalid analysis payload", http.StatusBadRequest)
		return
	}
	subjectID := strings.TrimSpace(req.SubjectID)
	if subjectID == "" {
		subjectID = strings.TrimSpace(r.URL.Query().Get("subject_id"))
	}
	args := map[string]any{}
	if req.Days > 0 {
		args["days"] = float64(req.Days)
	}

	subjectType := analysisSubjectTypeFromPath(r.URL.Path)
	var (
		result map[string]any
		err    error
	)
	switch subjectType {
	case "team":
		args["team"] = subjectID
		result, err = a.hostedMCPAnalyzeTeam(r.Context(), subject, args)
	case "pipeline":
		if subjectID == "" {
			http.Error(w, "subject_id is required", http.StatusBadRequest)
			return
		}
		args["pipeline"] = subjectID
		result, err = a.hostedMCPAnalyzePipeline(r.Context(), subject, args)
	case "run":
		if subjectID == "" {
			http.Error(w, "subject_id is required", http.StatusBadRequest)
			return
		}
		args["run_id"] = subjectID
		result, err = a.hostedMCPAnalyzeRun(r.Context(), subject, args)
	default:
		http.Error(w, "subject type must be team, pipeline, or run", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "analysis failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, result)
}
