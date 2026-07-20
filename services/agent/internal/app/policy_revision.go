package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"nopsai/pkg/models"

	"github.com/rs/zerolog"
)

type policyRevisionState struct {
	mu             sync.Mutex
	runStart       string
	lastEffective  string
	blockingPolicy bool
}

func newPolicyRevisionState(snapshots []models.KnowledgeContextSnapshot) *policyRevisionState {
	runStart := models.KnowledgeContextRevision(snapshots, true)
	return &policyRevisionState{
		runStart:       runStart,
		lastEffective:  runStart,
		blockingPolicy: runStart != "",
	}
}

func (s *policyRevisionState) EnsureCurrent(ctx context.Context, runID string, checker PolicyRevisionChecker, logger *zerolog.Logger, stage string) error {
	if s == nil || !s.blockingPolicy {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if checker == nil {
		return fmt.Errorf("blocking policy revision cannot be checked before %s; policy revision checker is not configured", strings.TrimSpace(stage))
	}
	status, err := checker(ctx, runID)
	if err != nil {
		return fmt.Errorf("blocking policy revision check failed before %s: %w", strings.TrimSpace(stage), err)
	}
	current := strings.TrimSpace(status.CurrentPolicyRevision)
	if current == "" && status.BlockingContextCount > 0 {
		return fmt.Errorf("blocking policy revision check returned an empty current revision before %s", strings.TrimSpace(stage))
	}
	if current != s.lastEffective && logger != nil {
		logger.Warn().
			Str("run_start_policy_revision", s.runStart).
			Str("previous_effective_policy_revision", s.lastEffective).
			Str("current_policy_revision", current).
			Str("policy_check_stage", strings.TrimSpace(stage)).
			Msg("Blocking policy revision changed during run; invalidating static context cache")
	}
	s.lastEffective = current
	if current != s.runStart {
		return fmt.Errorf(
			"blocking policy revision changed from %s to %s before %s; rerun the pipeline so the action is evaluated with the current policy",
			s.runStart,
			current,
			strings.TrimSpace(stage),
		)
	}
	return nil
}
