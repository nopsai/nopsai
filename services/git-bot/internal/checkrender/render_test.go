package checkrender

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestMarkdownListOrdersByTaskIndex(t *testing.T) {
	state := &State{
		StepOrder: []string{"deploy"},
		Steps: map[string]map[string]TaskStatusUpdate{
			"deploy": {
				"second": {StepName: "deploy", TaskName: "second", TaskStatus: "pending", TaskIndex: 2},
				"first":  {StepName: "deploy", TaskName: "first", TaskStatus: "success", TaskIndex: 1},
			},
		},
	}

	got := MarkdownList(state)
	first := strings.Index(got, "`first`")
	second := strings.Index(got, "`second`")
	if first < 0 || second < 0 {
		t.Fatalf("list missing tasks:\n%s", got)
	}
	if first > second {
		t.Fatalf("list did not sort by task index:\n%s", got)
	}
}

func TestMarkdownListRendersWarningIcon(t *testing.T) {
	state := &State{
		StepOrder: []string{"lint"},
		Steps: map[string]map[string]TaskStatusUpdate{
			"lint": {
				"lint": {StepName: "lint", TaskName: "lint", TaskStatus: "warning", TaskIndex: 1},
			},
		},
	}

	got := MarkdownList(state)
	if !strings.Contains(got, "⚠️ **lint**: `lint` - warning") {
		t.Fatalf("list did not render warning icon:\n%s", got)
	}
}

func TestRenderUsesGraphForGraphOption(t *testing.T) {
	state := &State{
		DisplayOption:      models.DisplayOptionGraph,
		PipelineDefinition: "name: ci\nsteps: []\n",
		Steps:              map[string]map[string]TaskStatusUpdate{},
	}

	got := Render(state)
	if !strings.Contains(got, "```mermaid") {
		t.Fatalf("Render() = %q, want Mermaid output", got)
	}
}

func TestRenderUsesListForListOption(t *testing.T) {
	state := &State{
		DisplayOption: models.DisplayOptionList,
		StepOrder:     []string{"deploy"},
		Steps: map[string]map[string]TaskStatusUpdate{
			"deploy": {
				"first": {StepName: "deploy", TaskName: "first", TaskStatus: "success", TaskIndex: 1},
			},
		},
	}

	got := Render(state)
	if strings.Contains(got, "```mermaid") {
		t.Fatalf("Render() = %q, want Markdown list output", got)
	}
	if !strings.Contains(got, "**deploy**: `first`") {
		t.Fatalf("Render() = %q, want the task rendered as a list item", got)
	}
}

// An unset display option falls back to the graph, matching
// models.DefaultDisplayOption.
func TestRenderDefaultsToGraph(t *testing.T) {
	state := &State{
		DisplayOption:      "",
		PipelineDefinition: "name: ci\nsteps: []\n",
		Steps:              map[string]map[string]TaskStatusUpdate{},
	}

	got := Render(state)
	if !strings.Contains(got, "```mermaid") {
		t.Fatalf("Render() = %q, want Mermaid output by default", got)
	}
}
