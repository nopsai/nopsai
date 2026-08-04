package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"nopsai/config"

	"github.com/rs/zerolog/log"
)

func (a *App) removeRunnerRevocation(ctx context.Context, runnerID string) (bool, error) {
	runnerID = strings.TrimSpace(runnerID)
	if runnerID == "" || a == nil {
		return false, nil
	}

	a.cfgMu.Lock()
	if a.cfg == nil {
		a.cfg = &config.Config{}
	}
	current := config.NormalizeRunnerIDs(a.cfg.EjectedRunnerIDs)
	next := make([]string, 0, len(current))
	changed := false
	for _, id := range current {
		if id == runnerID {
			changed = true
			continue
		}
		next = append(next, id)
	}
	a.cfg.EjectedRunnerIDs = next
	cfg := *a.cfg
	cfg.EjectedRunnerIDs = append([]string(nil), next...)
	a.cfgMu.Unlock()

	if !changed || a.db == nil {
		return changed, nil
	}
	rawIDs, err := json.Marshal(next)
	if err != nil {
		return false, fmt.Errorf("marshal revoked runner ids: %w", err)
	}
	tag, err := a.db.Exec(ctx, `
		UPDATE runtime_settings
		SET payload = jsonb_set(payload, '{ejected_runner_ids}', $1::jsonb, true),
			version = version + 1,
			updated_at = NOW()
		WHERE id = TRUE
	`, string(rawIDs))
	if err != nil {
		return false, fmt.Errorf("persist runner revocation removal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return true, persistRuntimeSettingsSnapshotToDB(ctx, a.db, cfg, "database", nil, "", "", false)
	}
	return true, nil
}

func (a *App) allowRunnerIDReuse(ctx context.Context, runnerID string) error {
	removed, err := a.removeRunnerRevocation(ctx, runnerID)
	if err != nil {
		return err
	}
	if removed {
		log.Info().Str("runner_id", runnerID).Msg("cleared runner ID revocation for reuse")
	}
	return nil
}

func (a *App) allowRunnerInstallIdentityReuse(ctx context.Context, runnerID, runnerName string) error {
	seen := map[string]struct{}{}
	for _, id := range []string{runnerID, runnerName} {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		if err := a.allowRunnerIDReuse(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
