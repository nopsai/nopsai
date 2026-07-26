package nopsai

import "testing"

func TestMergeReusableStepVariablesKeepsDefaultsAndAppliesOverrides(t *testing.T) {
	merged := mergeReusableStepVariables(
		map[string]string{
			"CHANNEL": "stable",
			"REGION":  "eu-west-1",
		},
		map[string]string{
			"CHANNEL": "nightly",
			"IMAGE":   "api",
		},
	)

	if merged["CHANNEL"] != "nightly" {
		t.Fatalf("CHANNEL = %q, want override value", merged["CHANNEL"])
	}
	if merged["REGION"] != "eu-west-1" {
		t.Fatalf("REGION = %q, want reusable default", merged["REGION"])
	}
	if merged["IMAGE"] != "api" {
		t.Fatalf("IMAGE = %q, want include value", merged["IMAGE"])
	}
}
