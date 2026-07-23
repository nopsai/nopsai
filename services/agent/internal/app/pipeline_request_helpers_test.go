package app

import (
	"testing"

	"nopsai/pkg/models"
)

func TestEffectiveIgnoreFailureUsesTaskOrStepFlag(t *testing.T) {
	step := &models.PipelineStep{Step: &models.ScriptStep{
		BaseStep: models.BaseStep{Name: "lint", IgnoreFailure: true},
		Script:   "npm run lint",
	}}

	if !effectiveIgnoreFailure(step, &models.Task{Name: "lint"}) {
		t.Fatal("effectiveIgnoreFailure() = false, want true from step")
	}
	if !effectiveIgnoreFailure(nil, &models.Task{Name: "lint", IgnoreFailure: true}) {
		t.Fatal("effectiveIgnoreFailure() = false, want true from task")
	}
	if effectiveIgnoreFailure(nil, &models.Task{Name: "lint"}) {
		t.Fatal("effectiveIgnoreFailure() = true, want false without task or step flag")
	}
}

func TestFailureStatusWithToleranceOnlyRewritesFailures(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{status: "failure", want: "failure (ignored)"},
		{status: "not_found", want: "failure (ignored)"},
		{status: "timed_out", want: "failure (ignored)"},
		{status: "cancelled", want: "cancelled"},
		{status: "skipped", want: "skipped"},
		{status: "failure (ignored)", want: "failure (ignored)"},
	}

	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			if got := failureStatusWithTolerance(tc.status, true); got != tc.want {
				t.Fatalf("failureStatusWithTolerance(%q, true) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}
