package agent

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nopsai/pkg/models"
	llmruntime "nopsai/services/agent/internal/llm"
)

type aiUsageReporter interface {
	ReportAIUsage(context.Context, models.AIUsageReport) error
}

type nopsaiAIUsageReporter struct {
	runID string
}

func newNopsaiAIUsageReporter(runID string) nopsaiAIUsageReporter {
	return nopsaiAIUsageReporter{runID: strings.TrimSpace(runID)}
}

func (r nopsaiAIUsageReporter) ReportAIUsage(ctx context.Context, report models.AIUsageReport) error {
	if r.runID == "" {
		return fmt.Errorf("run ID is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return nopsaiAgentRequest(reqCtx, http.MethodPost, fmt.Sprintf("/v1/internal/runs/%s/ai-usage", r.runID), report, nil)
}

func reportCollectedAIUsage(ctx context.Context, reporter aiUsageReporter, feature, stepName, taskName, agentProfile string, usages []llmruntime.Usage) {
	if reporter == nil || len(usages) == 0 {
		return
	}
	for _, usage := range usages {
		report := aiUsageReportFromLLMUsage(feature, stepName, taskName, agentProfile, usage)
		if !aiUsageReportHasValue(report) {
			continue
		}
		if err := reporter.ReportAIUsage(ctx, report); err != nil {
			stepLog("", "", stepName, taskName).Warn().Err(err).Msg("Failed to report AI usage")
		}
	}
}

func aiUsageReportFromLLMUsage(feature, stepName, taskName, agentProfile string, usage llmruntime.Usage) models.AIUsageReport {
	metadata := map[string]any{}
	if agentProfile = strings.TrimSpace(agentProfile); agentProfile != "" {
		metadata["agent_profile"] = agentProfile
	}
	if usage.Estimated {
		metadata["estimated_tokens"] = true
	}
	return models.AIUsageReport{
		StepName:         strings.TrimSpace(stepName),
		TaskName:         strings.TrimSpace(taskName),
		Feature:          strings.TrimSpace(feature),
		Provider:         strings.TrimSpace(usage.Provider),
		Model:            strings.TrimSpace(usage.Model),
		LLMProfile:       strings.TrimSpace(usage.Profile),
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		InputCostUSD:     usage.InputCostUSD,
		OutputCostUSD:    usage.OutputCostUSD,
		TotalCostUSD:     usage.TotalCostUSD,
		Metadata:         metadata,
	}
}

func aiUsageReportHasValue(report models.AIUsageReport) bool {
	return report.PromptTokens > 0 || report.CompletionTokens > 0 || report.TotalTokens > 0 ||
		report.InputCostUSD > 0 || report.OutputCostUSD > 0 || report.TotalCostUSD > 0
}
