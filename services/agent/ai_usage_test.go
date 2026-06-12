package agent

import (
	"testing"

	llmruntime "nopsai/services/agent/internal/llm"
)

func TestAIUsageReportFromLLMUsageMarksEstimatedTokens(t *testing.T) {
	report := aiUsageReportFromLLMUsage("goal_resolution", "build", "lint", "sre", llmruntime.Usage{
		Provider:     "lmstudio",
		Model:        "model-a",
		Profile:      "local",
		PromptTokens: 12,
		TotalTokens:  12,
		Estimated:    true,
	})
	if report.Metadata["agent_profile"] != "sre" {
		t.Fatalf("agent profile metadata = %#v", report.Metadata)
	}
	if report.Metadata["estimated_tokens"] != true {
		t.Fatalf("estimated metadata = %#v", report.Metadata)
	}
	if report.Feature != "goal_resolution" || report.StepName != "build" || report.TaskName != "lint" {
		t.Fatalf("report context = %#v", report)
	}
}
