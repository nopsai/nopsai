package nopsai

import (
	"context"
	"net/http"

	"nopsai/services/nopsai/pkg/audit"
)

func (a *App) auditDashboardAction(ctx context.Context, r *http.Request, action string, dashboard dashboardRecord, result string, metadata map[string]any) {
	if a == nil || a.auditLogger == nil {
		return
	}
	entry := audit.Entry{
		Action:   action,
		Resource: "dashboard:" + dashboard.ref(),
		Result:   result,
		Metadata: metadata,
	}
	if r != nil {
		entry.ActorSub = actorIDFromRequest(r)
	}
	_ = a.auditLogger.Write(ctx, entry)
}
