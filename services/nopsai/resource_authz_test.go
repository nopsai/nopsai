package main

import (
	"testing"

	"nopsai/pkg/models"
)

func TestIsSameGroupBoundaryExamples(t *testing.T) {
	tests := []struct {
		name     string
		caller   string
		resource string
		want     bool
	}{
		{name: "same group", caller: "team-1", resource: "team-1", want: true},
		{name: "child resource", caller: "team-1", resource: "team-1/shared", want: true},
		{name: "sibling under team", caller: "team-1/app", resource: "team-1/shared", want: true},
		{name: "other team", caller: "team-1", resource: "team-2", want: false},
		{name: "platform boundary", caller: "team-1", resource: "platform/shared", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSameGroupBoundary(tt.caller, tt.resource)
			if got != tt.want {
				t.Fatalf("IsSameGroupBoundary(%q, %q) = %t, want %t", tt.caller, tt.resource, got, tt.want)
			}
		})
	}
}

func TestGroupGrantIncludesCallerGroup(t *testing.T) {
	tests := []struct {
		name        string
		grantGroup  string
		callerGroup string
		want        bool
	}{
		{name: "same group", grantGroup: "team-1", callerGroup: "team-1", want: true},
		{name: "child caller", grantGroup: "team-1", callerGroup: "team-1/app", want: true},
		{name: "sibling caller excluded", grantGroup: "team-1/shared", callerGroup: "team-1/app", want: false},
		{name: "other team", grantGroup: "team-1", callerGroup: "team-2/app", want: false},
		{name: "general excluded", grantGroup: generalGrantID, callerGroup: "team-1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupGrantIncludesCallerGroup(tt.grantGroup, tt.callerGroup)
			if got != tt.want {
				t.Fatalf("groupGrantIncludesCallerGroup(%q, %q) = %t, want %t", tt.grantGroup, tt.callerGroup, got, tt.want)
			}
		})
	}
}

func TestCollectReferencedPipelineIdentifiers(t *testing.T) {
	pipeline := &models.Pipeline{
		Steps: []models.PipelineStep{
			{Step: &models.IncludeStep{BaseStep: models.BaseStep{Name: "build"}, Include: "pipeline:platform/shared/build"}},
			{Step: &models.IncludeStep{BaseStep: models.BaseStep{Name: "notify"}, Include: "step:team-1/notify"}},
			{Step: &models.IncludeStep{BaseStep: models.BaseStep{Name: "deploy"}, Include: "pipeline:/platform/shared/deploy/"}},
			{Step: &models.IncludeStep{BaseStep: models.BaseStep{Name: "dupe"}, Include: "pipeline:platform/shared/build"}},
		},
	}

	got := collectReferencedPipelineIdentifiers(pipeline)
	want := []string{"platform/shared/build", "platform/shared/deploy"}
	if len(got) != len(want) {
		t.Fatalf("collectReferencedPipelineIdentifiers() = %#v, want %#v", got, want)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("collectReferencedPipelineIdentifiers()[%d] = %q, want %q", idx, got[idx], want[idx])
		}
	}
}
