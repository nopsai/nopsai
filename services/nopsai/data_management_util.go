package nopsai

import (
	"context"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"nopsai/services/nopsai/pkg/audit"
	"nopsai/services/nopsai/pkg/auth"
)

func queryLimit(r *http.Request, defaultLimit, maxLimit int) int {
	if r == nil {
		return defaultLimit
	}
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultLimit
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return defaultLimit
	}
	if parsed > maxLimit {
		return maxLimit
	}
	return parsed
}

func normalizeCleanupTrigger(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), dataCleanupTriggerScheduled) {
		return dataCleanupTriggerScheduled
	}
	return dataCleanupTriggerManual
}

func quoteSQLIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func sanitizeDownloadFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "nopsai-backup.jsonl.gz"
	}
	return strings.ReplaceAll(name, `"`, "")
}

func (a *App) backupFilePathAllowed(filePath string) bool {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return false
	}
	baseDir, err := filepath.Abs(a.dataBackupDirectory())
	if err != nil {
		return false
	}
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(baseDir, absFile)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

func (a *App) auditDataManagementAction(ctx context.Context, r *http.Request, action, resource, result string, metadata map[string]any) {
	if a == nil || a.auditLogger == nil {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	entry := audit.Entry{
		ActorSub:   "system",
		ActorEmail: "",
		Provider:   "system",
		Action:     action,
		Resource:   resource,
		Result:     result,
		Metadata:   metadata,
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
