package nopsai

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	"nopsai/pkg/models"
	"nopsai/pkg/serviceauth"
)

func (a *App) handleGetRunPolicyRevision(w http.ResponseWriter, r *http.Request) {
	if !requireInternalServiceRole(w, r, serviceauth.RoleAgent) {
		return
	}
	runID := strings.TrimSpace(r.PathValue("runID"))
	if _, err := uuid.Parse(runID); err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}
	resp, err := a.currentRunPolicyRevision(r.Context(), runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to calculate run policy revision")
		http.Error(w, "Failed to calculate policy revision", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) currentRunPolicyRevision(ctx context.Context, runID string) (models.PolicyRevisionResponse, error) {
	runStartSnapshots, err := a.loadRunKnowledgeContextSnapshots(ctx, runID)
	if err != nil {
		return models.PolicyRevisionResponse{}, err
	}
	currentSnapshots := make([]models.KnowledgeContextSnapshot, 0, len(runStartSnapshots))
	for _, snapshot := range runStartSnapshots {
		if !models.KnowledgeContextKindIsBlocking(snapshot.Kind) {
			continue
		}
		currentSnapshot := snapshot
		if strings.TrimSpace(snapshot.KnowledgeContextID) != "" {
			refreshed, err := a.loadCurrentBlockingKnowledgeContextSnapshot(ctx, snapshot)
			if err != nil {
				return models.PolicyRevisionResponse{}, err
			}
			currentSnapshot = refreshed
		}
		currentSnapshots = append(currentSnapshots, currentSnapshot)
	}
	return models.PolicyRevisionResponse{
		RunStartPolicyRevision: models.KnowledgeContextRevision(runStartSnapshots, true),
		CurrentPolicyRevision:  models.KnowledgeContextRevision(currentSnapshots, true),
		BlockingContextCount:   len(currentSnapshots),
		KnowledgeContexts:      currentSnapshots,
		CheckedAt:              time.Now().UTC(),
	}, nil
}

func (a *App) loadCurrentBlockingKnowledgeContextSnapshot(ctx context.Context, runSnapshot models.KnowledgeContextSnapshot) (models.KnowledgeContextSnapshot, error) {
	current := runSnapshot
	var resolvedAt time.Time
	err := a.db.QueryRow(ctx, `
		SELECT id::text, kind, team_path, name, description, content,
		       source, config_source_path, config_source_commit_sha, updated_at
		FROM knowledge_contexts
		WHERE id = $1::uuid
	`, strings.TrimSpace(runSnapshot.KnowledgeContextID)).Scan(
		&current.KnowledgeContextID, &current.Kind, &current.Team, &current.Name,
		&current.Description, &current.Content, &current.Source,
		&current.ConfigSourcePath, &current.ConfigSourceCommitSHA, &resolvedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return models.KnowledgeContextSnapshot{}, errors.New("blocking knowledge context was removed after run start")
		}
		return models.KnowledgeContextSnapshot{}, err
	}
	current.ID = runSnapshot.ID
	current.Ref = runSnapshot.Ref
	current.Path = runSnapshot.Path
	current.Required = runSnapshot.Required
	current.ResolvedAt = resolvedAt
	return current, nil
}
