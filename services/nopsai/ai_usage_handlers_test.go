package nopsai

import (
	"testing"

	"nopsai/pkg/models"
)

func TestNormalizeAIUsageReportComputesTotalsAndDefaults(t *testing.T) {
	report, ok := normalizeAIUsageReport(models.AIUsageReport{
		StepName:         " build ",
		TaskName:         " test ",
		Provider:         " lmstudio ",
		ProviderModel:    " model-a ",
		LLMProfile:       " local ",
		PromptTokens:     10,
		CompletionTokens: 4,
		InputCostUSD:     -1,
		OutputCostUSD:    0.25,
	})
	if !ok {
		t.Fatal("normalizeAIUsageReport() ok = false, want true")
	}
	if report.StepName != "build" || report.TaskName != "test" || report.Feature != "unknown" {
		t.Fatalf("normalized labels = %#v", report)
	}
	if report.Provider != "lmstudio" || report.ProviderModel != "model-a" || report.LLMProfile != "local" {
		t.Fatalf("normalized identity = %#v", report)
	}
	if report.TotalTokens != 14 {
		t.Fatalf("TotalTokens = %d, want 14", report.TotalTokens)
	}
	if report.InputCostUSD != 0 || report.TotalCostUSD != 0.25 {
		t.Fatalf("costs = (%f,%f), want (0,0.25)", report.InputCostUSD, report.TotalCostUSD)
	}
	if report.Metadata == nil {
		t.Fatal("Metadata is nil, want empty map")
	}
}

func TestNormalizeAIUsageReportDropsEmptyReports(t *testing.T) {
	report, ok := normalizeAIUsageReport(models.AIUsageReport{
		PromptTokens:     -5,
		CompletionTokens: -1,
		TotalTokens:      -9,
		TotalCostUSD:     -3,
	})
	if ok {
		t.Fatalf("normalizeAIUsageReport() ok = true for empty report: %#v", report)
	}
}
