package main

import "testing"

func TestEscapePrometheusLabel(t *testing.T) {
	got := escapePrometheusLabel("repo\\name\"\nnext")
	want := `repo\\name\"\nnext`
	if got != want {
		t.Fatalf("escapePrometheusLabel() = %q, want %q", got, want)
	}
}
