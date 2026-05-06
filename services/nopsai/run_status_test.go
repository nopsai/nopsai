package main

import "testing"

func TestNormalizeFinalizeRunStatus(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "success preserved", raw: "success", want: "success"},
		{name: "cancelled preserved", raw: "cancelled", want: "cancelled"},
		{name: "failure normalized", raw: "failure", want: "failure"},
		{name: "unknown treated as failure", raw: "timed_out", want: "failure"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeFinalizeRunStatus(tt.raw); got != tt.want {
				t.Fatalf("normalizeFinalizeRunStatus(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestIsCompletedRunStatus(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{status: "success", want: true},
		{status: "failure", want: true},
		{status: "timed_out", want: true},
		{status: "cancelled", want: false},
		{status: "running", want: false},
	}

	for _, tt := range tests {
		if got := isCompletedRunStatus(tt.status); got != tt.want {
			t.Fatalf("isCompletedRunStatus(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
}
