package agent

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"nopsai/pkg/models"
)

func checkRunPolicyRevision(ctx context.Context, runID string) (models.PolicyRevisionResponse, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return models.PolicyRevisionResponse{}, fmt.Errorf("run ID is required")
	}
	var resp models.PolicyRevisionResponse
	if err := nopsaiAgentRequest(ctx, http.MethodGet, fmt.Sprintf("/v1/internal/runs/%s/policy-revision", runID), nil, &resp); err != nil {
		return models.PolicyRevisionResponse{}, err
	}
	return resp, nil
}
