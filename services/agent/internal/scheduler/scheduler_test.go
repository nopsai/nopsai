package scheduler

import (
	"reflect"
	"testing"

	"nopsai/pkg/models"
)

func TestNextRunnableTasksHonorsStepAndTaskDependencies(t *testing.T) {
	pipeline := schedulerTestPipeline()

	runnable := NextRunnableTasks(&pipeline, map[string]bool{})
	assertRunnableKeys(t, runnable, []string{"build/compile"})

	runnable = NextRunnableTasks(&pipeline, map[string]bool{
		"build/compile": true,
	})
	assertRunnableKeys(t, runnable, []string{"build/unit"})

	runnable = NextRunnableTasks(&pipeline, map[string]bool{
		"build/compile": true,
		"build/unit":    true,
	})
	assertRunnableKeys(t, runnable, []string{"approve/approve"})

	runnable = NextRunnableTasks(&pipeline, map[string]bool{
		"build/compile":   true,
		"build/unit":      true,
		"approve/approve": true,
	})
	assertRunnableKeys(t, runnable, []string{"deploy/deploy"})
}

func TestFirstApprovalRunnable(t *testing.T) {
	pipeline := schedulerTestPipeline()
	runnable := NextRunnableTasks(&pipeline, map[string]bool{
		"build/compile": true,
		"build/unit":    true,
	})

	approval := FirstApprovalRunnable(runnable)
	if approval == nil {
		t.Fatal("FirstApprovalRunnable() = nil, want approval task")
	}
	if approval.GlobalKey != "approve/approve" {
		t.Fatalf("FirstApprovalRunnable() key = %q, want approve/approve", approval.GlobalKey)
	}
}

func TestNextRunnableTasksCopiesSingleModeStepIgnoreFailure(t *testing.T) {
	pipeline := models.Pipeline{
		Name: "release",
		Steps: []models.PipelineStep{
			{
				Step: &models.ScriptStep{
					BaseStep: models.BaseStep{
						Name:          "lint",
						IgnoreFailure: true,
					},
					Script: "npm run lint",
				},
			},
		},
	}

	runnable := NextRunnableTasks(&pipeline, map[string]bool{})
	if len(runnable) != 1 {
		t.Fatalf("runnable count = %d, want 1", len(runnable))
	}
	if !runnable[0].Task.IgnoreFailure {
		t.Fatal("synthetic task IgnoreFailure = false, want true from step")
	}
}

func TestImagePullQueueSkipsApprovalSteps(t *testing.T) {
	pipeline := schedulerTestPipeline()

	queue := ImagePullQueue(&pipeline, CountPipelineTasks(&pipeline))
	want := []string{"golang:1.22", "alpine:3.20"}
	if !reflect.DeepEqual(queue, want) {
		t.Fatalf("ImagePullQueue() = %#v, want %#v", queue, want)
	}
}

func TestCompletedTaskKeysSnapshotOnlyIncludesDoneTasks(t *testing.T) {
	got := CompletedTaskKeysSnapshot(map[string]bool{
		"deploy/deploy": false,
		"build/unit":    true,
		"build/compile": true,
	})
	want := []string{"build/compile", "build/unit"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CompletedTaskKeysSnapshot() = %#v, want %#v", got, want)
	}
}

func assertRunnableKeys(t *testing.T, runnable []*RunnableTask, want []string) {
	t.Helper()
	got := make([]string, 0, len(runnable))
	for _, task := range runnable {
		got = append(got, task.GlobalKey)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runnable keys = %#v, want %#v", got, want)
	}
}

func schedulerTestPipeline() models.Pipeline {
	return models.Pipeline{
		Name:           "release",
		ContainerImage: "alpine:3.20",
		Steps: []models.PipelineStep{
			{
				Step: &models.TaskStep{
					BaseStep: models.BaseStep{
						Name:  "build",
						Image: "golang:1.22",
					},
					Tasks: []models.Task{
						{Name: "compile", Script: "go build ./..."},
						{Name: "unit", Script: "go test ./...", DependsOn: []string{"compile"}},
					},
				},
			},
			{
				Step: &models.ApprovalStep{
					BaseStep: models.BaseStep{
						Name:      "approve",
						DependsOn: []string{"build"},
					},
				},
			},
			{
				Step: &models.ScriptStep{
					BaseStep: models.BaseStep{
						Name:      "deploy",
						DependsOn: []string{"approve"},
					},
					Script: "./deploy.sh",
				},
			},
		},
	}
}
