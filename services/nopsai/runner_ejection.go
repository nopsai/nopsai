package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"nopsai/config"
	"nopsai/services/nopsai/internal/systemconfig"
)

func (a *App) recordRunnerEjection(ctx context.Context, runnerID string) error {
	runnerID = strings.TrimSpace(runnerID)
	if runnerID == "" {
		return fmt.Errorf("runner_id is required")
	}
	if a == nil {
		return nil
	}

	a.cfgMu.Lock()
	if a.cfg == nil {
		a.cfg = &config.Config{}
	}
	a.cfg.EjectedRunnerIDs = config.NormalizeRunnerIDs(append(a.cfg.EjectedRunnerIDs, runnerID))
	a.cfg.DispatcherRouting, _ = systemconfig.RemoveRunnersFromDispatcherRouting(a.cfg.DispatcherRouting, a.cfg.EjectedRunnerIDs)
	cfg := *a.cfg
	cfg.EjectedRunnerIDs = append([]string(nil), a.cfg.EjectedRunnerIDs...)
	cfg.DispatcherRouting = systemconfig.CloneDispatcherRouting(a.cfg.DispatcherRouting)
	ids := append([]string(nil), cfg.EjectedRunnerIDs...)
	routing := systemconfig.CloneDispatcherRouting(cfg.DispatcherRouting)
	a.cfgMu.Unlock()

	if a.db == nil {
		return nil
	}
	rawIDs, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("marshal ejected runner ids: %w", err)
	}
	rawRouting, err := json.Marshal(routing)
	if err != nil {
		return fmt.Errorf("marshal dispatcher routing: %w", err)
	}
	tag, err := a.db.Exec(ctx, `
		UPDATE runtime_settings
		SET payload = jsonb_set(
				jsonb_set(payload, '{ejected_runner_ids}', $1::jsonb, true),
				'{dispatcher_routing}', $2::jsonb, true
			),
			version = version + 1,
			updated_at = NOW()
		WHERE id = TRUE
	`, string(rawIDs), string(rawRouting))
	if err != nil {
		return fmt.Errorf("persist runner ejection: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return persistRuntimeSettingsSnapshotToDB(ctx, a.db, cfg, "database", nil, "", "", false)
	}
	return nil
}
