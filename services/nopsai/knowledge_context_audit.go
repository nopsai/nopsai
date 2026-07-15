package nopsai

import (
	"context"
	"net/http"
	"strings"

	"nopsai/services/nopsai/pkg/audit"
	"nopsai/services/nopsai/pkg/auth"
)

func (a *App) auditKnowledgeContextSync(ctx context.Context, r *http.Request, detail knowledgeContextDetail, connection knowledgeConnectionRecord, mode string, syncErr error) {
	if a == nil || a.auditLogger == nil {
		return
	}
	result := "success"
	metadata := map[string]any{
		"mode":                 strings.TrimSpace(mode),
		"provider":             strings.TrimSpace(connection.Provider),
		"knowledge_connection": strings.TrimSpace(connection.ID),
	}
	if syncErr != nil {
		result = "failure"
		metadata["error"] = syncErr.Error()
		metadata["sync_status"] = externalKnowledgeSyncStatus(syncErr)
	}
	entry := audit.Entry{
		ActorSub: "system",
		Provider: "system",
		Action:   "knowledge_context.sync",
		Resource: grantResourceKnowledgeContext + ":" + detail.ID,
		Result:   result,
		Metadata: metadata,
	}
	if r != nil {
		if claims, _ := auth.ClaimsFromContext(r.Context()); claims != nil {
			entry.ActorSub = claims.Sub
			entry.ActorEmail = claims.Email
			entry.Provider = claims.Provider
		}
		if requestID, _ := r.Context().Value(ctxKeyRequestID).(string); strings.TrimSpace(requestID) != "" {
			entry.Metadata["request_id"] = requestID
		}
	}
	_ = a.auditLogger.Write(ctx, entry)
}

func (a *App) auditKnowledgeConnectionAction(ctx context.Context, r *http.Request, action string, record knowledgeConnectionRecord, result string, metadata map[string]any) {
	if a == nil || a.auditLogger == nil {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["provider"] = strings.TrimSpace(record.Provider)
	metadata["team"] = strings.TrimSpace(record.Team)
	metadata["disabled"] = record.Disabled
	entry := audit.Entry{
		ActorSub: "system",
		Provider: "system",
		Action:   action,
		Resource: grantResourceKnowledgeConnection + ":" + record.ID,
		Result:   strings.TrimSpace(result),
		Metadata: metadata,
	}
	if entry.Result == "" {
		entry.Result = "success"
	}
	if r != nil {
		if claims, _ := auth.ClaimsFromContext(r.Context()); claims != nil {
			entry.ActorSub = claims.Sub
			entry.ActorEmail = claims.Email
			entry.Provider = claims.Provider
		}
		if requestID, _ := r.Context().Value(ctxKeyRequestID).(string); strings.TrimSpace(requestID) != "" {
			entry.Metadata["request_id"] = requestID
		}
	}
	_ = a.auditLogger.Write(ctx, entry)
}
