package nopsai

import (
	"testing"
	"time"
)

func TestTeamResponseFromTeamUsesCompatibilityFields(t *testing.T) {
	parent := 7
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	team := Team{
		ID:          8,
		Name:        "platform",
		Kind:        "team",
		ParentID:    &parent,
		Description: "Platform team",
		LastRunAt:   &now,
	}
	records := map[int]teamPathRecord{
		8: {ID: 8, Name: "platform", Path: "engineering/platform"},
	}

	got := teamResponseFromTeam(team, records, []int{10, 11})
	if got.Kind != "team" || got.Name != "platform" || got.DisplayName != "platform" {
		t.Fatalf("team response identity = %#v", got)
	}
	if got.ParentTeamID == nil || *got.ParentTeamID != parent || got.ParentID == nil || *got.ParentID != parent {
		t.Fatalf("parent fields = parent_team_id:%v parent_id:%v", got.ParentTeamID, got.ParentID)
	}
	if got.Path != "engineering/platform" {
		t.Fatalf("path = %q, want engineering/platform", got.Path)
	}
	if got.LastRunAt == nil || !got.LastRunAt.Equal(now) {
		t.Fatalf("last run = %#v, want %s", got.LastRunAt, now)
	}
	if len(got.Applications) != 2 || got.Applications[0] != 10 || got.Applications[1] != 11 {
		t.Fatalf("applications = %#v", got.Applications)
	}
}

func TestApplicationResponseFromTeamUsesRepositoryDisplayName(t *testing.T) {
	parent := 8
	team := Team{
		ID:                 12,
		Name:               "acme/payments-api",
		Kind:               "app",
		ParentID:           &parent,
		RepoURL:            "https://github.com/acme/payments-api",
		RepositoryFullName: "acme/payments-api",
	}
	records := map[int]teamPathRecord{
		8:  {ID: 8, Name: "platform", Path: "platform"},
		12: {ID: 12, Name: "payments-api", Path: "platform/payments-api"},
	}

	got := applicationResponseFromTeam(team, records)
	if got.Kind != "application" || got.DisplayName != "payments-api" {
		t.Fatalf("application identity = %#v", got)
	}
	if got.TeamID == nil || *got.TeamID != parent || got.ParentID == nil || *got.ParentID != parent {
		t.Fatalf("parent fields = team_id:%v parent_id:%v", got.TeamID, got.ParentID)
	}
	if got.Path != "platform/payments-api" || got.TeamPath != "platform" {
		t.Fatalf("paths = %q / %q", got.Path, got.TeamPath)
	}
	if got.RepositoryFullName != "acme/payments-api" || got.RepoURL == "" {
		t.Fatalf("repository fields = %#v", got)
	}
}
