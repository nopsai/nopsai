package configsync

import (
	"strings"
	"testing"
)

func TestParseBindingPath(t *testing.T) {
	tests := []struct {
		name          string
		relPath       string
		wantScopeType string
		wantScopeID   string
		wantErr       bool
	}{
		{name: "group binding", relPath: "groups/data-team.yaml", wantScopeType: "folder", wantScopeID: "data-team"},
		{name: "nested group binding", relPath: "groups/data-team/platform.yml", wantScopeType: "folder", wantScopeID: "data-team/platform"},
		{name: "unsupported scope", relPath: "system/global.yaml", wantErr: true},
		{name: "escaping path", relPath: "groups/data-team/../prod.yaml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScopeType, gotScopeID, err := ParseBindingPath(tt.relPath)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ParseBindingPath() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseBindingPath() error = %v", err)
			}
			if gotScopeType != tt.wantScopeType || gotScopeID != tt.wantScopeID {
				t.Fatalf("ParseBindingPath() = (%q, %q), want (%q, %q)", gotScopeType, gotScopeID, tt.wantScopeType, tt.wantScopeID)
			}
		})
	}
}

func TestValidateBindingFile(t *testing.T) {
	valid := BindingFile{
		RepoURL:     "git@github.com:acme/config.git",
		WriteBranch: "nopsai/data-team-ui",
	}
	if err := ValidateBindingFile(valid, "folder", "data-team", "groups/data-team.yaml"); err != nil {
		t.Fatalf("ValidateBindingFile() error = %v", err)
	}

	if err := ValidateBindingFile(BindingFile{RepoURL: "git@github.com:acme/config.git", ScopeID: "other"}, "folder", "data-team", "groups/data-team.yaml"); err == nil {
		t.Fatal("ValidateBindingFile() accepted mismatched scope_id")
	}
	if err := ValidateBindingFile(BindingFile{RepoURL: "git@github.com:acme/config.git", WriteBranch: "refs/heads/main"}, "folder", "data-team", "groups/data-team.yaml"); err == nil {
		t.Fatal("ValidateBindingFile() accepted ref path write_branch")
	}
	if err := ValidateBindingFile(BindingFile{}, "folder", "data-team", "groups/data-team.yaml"); err == nil {
		t.Fatal("ValidateBindingFile() accepted missing repo_url")
	}
}

func TestParseConfigRepositoryGroupPipelineRunStructureApps(t *testing.T) {
	structure, ok, err := ParseConfigRepositoryGroupPipelineRunStructure("groups/team-1/structure.yaml", `
description: Team 1 apps
apps:
  - name: api
    repo_url: https://github.com/acme/service-api.git
  - git@github.com:acme/worker.git
dev:
  apps:
    - name: dev-api
      repo_url: https://github.com/acme/dev-api
`)
	if err != nil {
		t.Fatalf("ParseConfigRepositoryGroupPipelineRunStructure() error = %v", err)
	}
	if !ok {
		t.Fatal("expected groups/team-1/structure.yaml to be treated as a group structure file")
	}

	team := structure["team-1"]
	if team == nil {
		t.Fatal("expected team-1 structure")
	}
	if team.Description != "Team 1 apps" {
		t.Fatalf("description = %q, want Team 1 apps", team.Description)
	}
	if len(team.Apps) != 2 {
		t.Fatalf("apps = %#v, want 2 entries", team.Apps)
	}
	if team.Apps[0].Name != "api" || team.Apps[0].RepositoryFullName != "acme/service-api" {
		t.Fatalf("first app = %#v, want api -> acme/service-api", team.Apps[0])
	}
	if team.Apps[1].Name != "worker" || team.Apps[1].RepositoryFullName != "acme/worker" {
		t.Fatalf("second app = %#v, want worker -> acme/worker", team.Apps[1])
	}
	dev := team.Children["dev"]
	if dev == nil || len(dev.Apps) != 1 || dev.Apps[0].RepositoryFullName != "acme/dev-api" {
		t.Fatalf("dev apps = %#v, want acme/dev-api", dev)
	}
}

func TestParseConfigRepositoryGroupPipelineRunStructureRejectsReposShortcut(t *testing.T) {
	_, _, err := ParseConfigRepositoryGroupPipelineRunStructure("groups/team-1/structure.yaml", `
repos:
  - acme/service-api
`)
	if err == nil || !strings.Contains(err.Error(), "repos is not supported") {
		t.Fatalf("error = %v, want unsupported repos message", err)
	}
}

func TestParseConfigRepositoryGroupPipelineRunStructureRejectsAggregateFile(t *testing.T) {
	_, ok, err := ParseConfigRepositoryGroupPipelineRunStructure("groups/structure.yaml", `
team-1:
  description: Team 1
`)
	if !ok {
		t.Fatal("expected aggregate structure path to be recognized and rejected")
	}
	if err == nil || !strings.Contains(err.Error(), "aggregate group structure file is not supported") {
		t.Fatalf("error = %v, want aggregate structure rejection", err)
	}
}

func TestNormalizePipelineRunStructureForFolder(t *testing.T) {
	structure := map[string]*PipelineRunStructureNode{
		"dev": {
			Description: "Development",
			Repos:       []string{"acme/app"},
			Children: map[string]*PipelineRunStructureNode{
				"services": {
					Description: "Service workloads",
					Children:    map[string]*PipelineRunStructureNode{},
				},
			},
		},
		"team-1": {
			Description: "Already scoped",
			Children: map[string]*PipelineRunStructureNode{
				"platform": {
					Description: "Platform",
					Children:    map[string]*PipelineRunStructureNode{},
				},
			},
		},
	}

	got, err := NormalizePipelineRunStructureForFolder("team-1", structure)
	if err != nil {
		t.Fatalf("NormalizePipelineRunStructureForFolder() error = %v", err)
	}
	team, ok := got["team-1"]
	if !ok {
		t.Fatal("expected team-1 root group")
	}
	if _, ok := team.Children["dev"]; !ok {
		t.Fatal("expected dev to be nested under team-1")
	}
	if gotRepos := team.Children["dev"].Repos; len(gotRepos) != 1 || gotRepos[0] != "acme/app" {
		t.Fatalf("dev repos = %#v, want acme/app", gotRepos)
	}
	if _, ok := team.Children["platform"]; !ok {
		t.Fatal("expected already scoped platform group to remain under team-1")
	}
	if _, duplicated := team.Children["team-1"]; duplicated {
		t.Fatal("did not expect duplicated team-1 child")
	}
}

func TestRepositoryFullNameFromURLUsesRepositoryRoot(t *testing.T) {
	got, err := RepositoryFullNameFromURL("https://github.com/acme/service-api/tree/main")
	if err != nil {
		t.Fatalf("RepositoryFullNameFromURL() error = %v", err)
	}
	if got != "acme/service-api" {
		t.Fatalf("RepositoryFullNameFromURL() = %q, want acme/service-api", got)
	}
}

func TestNormalizeStructureNameRejectsReservedRootGroupName(t *testing.T) {
	tests := []string{"root", " Root ", "/root/", "__general__"}
	for _, name := range tests {
		if _, err := NormalizeStructureName(name); err == nil {
			t.Fatalf("NormalizeStructureName(%q) error = nil, want reserved root error", name)
		}
	}
}
