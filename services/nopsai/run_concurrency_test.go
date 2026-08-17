package nopsai

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestBuildRunConcurrencyGroupKeysOnRepositoryRefAndPipeline(t *testing.T) {
	gitContext := map[string]string{
		"repo_owner": "acme",
		"repo_name":  "api",
		"ref":        "refs/heads/main",
	}
	group := buildRunConcurrencyGroup(gitContext, "release")
	if group != "acme/api@refs/heads/main#release" {
		t.Fatalf("group = %q", group)
	}

	// A different branch or a different pipeline is unrelated work and must not
	// wait behind this one.
	otherBranch := buildRunConcurrencyGroup(map[string]string{
		"repo_owner": "acme",
		"repo_name":  "api",
		"ref":        "refs/heads/feature",
	}, "release")
	otherPipeline := buildRunConcurrencyGroup(gitContext, "lint")
	if otherBranch == group || otherPipeline == group {
		t.Fatalf("unrelated runs share a group: %q / %q / %q", group, otherBranch, otherPipeline)
	}
}

// Runs without a repository and ref — manual runs, schedules, external triggers
// — are not serialized, so an empty group means "dispatch immediately".
func TestBuildRunConcurrencyGroupIsUnsetWithoutGitContext(t *testing.T) {
	for name, gitContext := range map[string]map[string]string{
		"empty":        {},
		"no ref":       {"repo_owner": "acme", "repo_name": "api"},
		"no repo name": {"repo_owner": "acme", "ref": "refs/heads/main"},
	} {
		t.Run(name, func(t *testing.T) {
			if group := buildRunConcurrencyGroup(gitContext, "release"); group != runConcurrencyGroupUnset {
				t.Fatalf("group = %q, want unset", group)
			}
		})
	}
	if group := buildRunConcurrencyGroup(map[string]string{
		"repo_owner": "acme",
		"repo_name":  "api",
		"ref":        "refs/heads/main",
	}, "  "); group != runConcurrencyGroupUnset {
		t.Fatalf("group without a pipeline name = %q, want unset", group)
	}
}

// A child run must never join its parent's queue: the parent holds the group
// while it waits for the child, so queueing the child would deadlock the run.
func TestRunConcurrencyGroupForRequestExcludesChildRuns(t *testing.T) {
	gitContext := map[string]string{
		"repo_owner": "acme",
		"repo_name":  "api",
		"ref":        "refs/heads/main",
	}
	parent := createPendingRunRequest{RunID: uuid.New(), GitContext: gitContext}
	parent.Pipeline.Name = "release"
	if group := runConcurrencyGroupForRequest(parent); group == runConcurrencyGroupUnset {
		t.Fatal("top-level run did not receive a concurrency group")
	}

	child := parent
	child.ParentRunID = uuid.New().String()
	if group := runConcurrencyGroupForRequest(child); group != runConcurrencyGroupUnset {
		t.Fatalf("child run group = %q, want unset", group)
	}
}

// Without a group there is nothing to wait for, and a run with no database is
// never blocked by a queue it cannot read.
func TestRunHoldsConcurrencyGroupAllowsUngroupedRuns(t *testing.T) {
	app := App{}
	if _, mayStart := app.runHoldsConcurrencyGroup(contextBackground(), "run-1", runConcurrencyGroupUnset); !mayStart {
		t.Fatal("ungrouped run was blocked")
	}
}

func TestQueuedBehindRunMessageNamesTheHolder(t *testing.T) {
	message := queuedBehindRunMessage("11111111-1111-1111-1111-111111111111", "acme/api@refs/heads/main#release")
	for _, want := range []string{"11111111-1111-1111-1111-111111111111", "acme/api@refs/heads/main#release"} {
		if !strings.Contains(message, want) {
			t.Fatalf("message %q does not mention %q", message, want)
		}
	}
}
