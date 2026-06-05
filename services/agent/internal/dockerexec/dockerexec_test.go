package dockerexec

import "testing"

func TestBuildStepContainerNameIncludesRepository(t *testing.T) {
	got := BuildStepContainerName("payments api", "deploy/prod", "ship now", "1234567890abcdef")
	want := "payments-api-deployprod-ship-now-12345678"
	if got != want {
		t.Fatalf("BuildStepContainerName() = %q, want %q", got, want)
	}
}

func TestBuildStepContainerNameWithoutRepository(t *testing.T) {
	got := BuildStepContainerName("", "deploy prod", "ship_now", "1234567890abcdef")
	want := "deploy-prod-ship_now-12345678"
	if got != want {
		t.Fatalf("BuildStepContainerName() = %q, want %q", got, want)
	}
}

func TestBuildStepContainerNameKeepsShortRunID(t *testing.T) {
	got := BuildStepContainerName("", "pipeline", "step", "abc")
	want := "pipeline-step-abc"
	if got != want {
		t.Fatalf("BuildStepContainerName() = %q, want %q", got, want)
	}
}
