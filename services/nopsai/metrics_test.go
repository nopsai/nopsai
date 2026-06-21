package nopsai

import (
	"strings"
	"testing"
)

func TestEscapePrometheusLabel(t *testing.T) {
	got := escapePrometheusLabel("repo\\name\"\nnext")
	want := `repo\\name\"\nnext`
	if got != want {
		t.Fatalf("escapePrometheusLabel() = %q, want %q", got, want)
	}
}

func TestBuildInfoMetric(t *testing.T) {
	var output strings.Builder
	appendBuildInfoMetric(&output)
	if !strings.Contains(output.String(), "# TYPE nopsai_build_info gauge") || !strings.Contains(output.String(), `api_version="v1"`) {
		t.Fatalf("metric = %q", output.String())
	}
}

func TestPipelineFinalOutputGenerationValueExpression(t *testing.T) {
	tests := map[string]string{
		"attempts":            "pro.generation_attempts",
		"contract_violations": "pro.contract_violations",
		"retries":             "GREATEST(pro.generation_attempts - 1, 0)",
		"render_attempts":     "pro.render_attempts",
		"render_failures":     "pro.render_failures",
		"unknown":             "pro.generation_attempts",
	}
	for valueKind, want := range tests {
		if got := pipelineFinalOutputGenerationValueExpression(valueKind); got != want {
			t.Fatalf("pipelineFinalOutputGenerationValueExpression(%q) = %q, want %q", valueKind, got, want)
		}
	}
}
