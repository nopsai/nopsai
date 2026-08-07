package checkrender

import (
	"strings"
	"testing"
)

func TestMarkdownFlatListOrdersByTaskIndex(t *testing.T) {
	state := &State{
		StepOrder: []string{"deploy"},
		Steps: map[string]map[string]TaskStatusUpdate{
			"deploy": {
				"second": {StepName: "deploy", TaskName: "second", TaskStatus: "pending", TaskIndex: 2},
				"first":  {StepName: "deploy", TaskName: "first", TaskStatus: "success", TaskIndex: 1},
			},
		},
	}

	got := MarkdownFlatList(state)
	first := strings.Index(got, "`first`")
	second := strings.Index(got, "`second`")
	if first < 0 || second < 0 {
		t.Fatalf("flat list missing tasks:\n%s", got)
	}
	if first > second {
		t.Fatalf("flat list did not sort by task index:\n%s", got)
	}
}

func TestMarkdownFlatListRendersWarningIcon(t *testing.T) {
	state := &State{
		StepOrder: []string{"lint"},
		Steps: map[string]map[string]TaskStatusUpdate{
			"lint": {
				"lint": {StepName: "lint", TaskName: "lint", TaskStatus: "warning", TaskIndex: 1},
			},
		},
	}

	got := MarkdownFlatList(state)
	if !strings.Contains(got, "⚠️ **lint**: `lint` - warning") {
		t.Fatalf("flat list did not render warning icon:\n%s", got)
	}
}

func TestMarkdownTreeRendersDependencyChild(t *testing.T) {
	state := &State{
		Steps: map[string]map[string]TaskStatusUpdate{
			"build": {
				"compile": {StepName: "build", TaskName: "compile", TaskStatus: "success", TaskIndex: 1},
				"test":    {StepName: "build", TaskName: "test", TaskStatus: "pending", TaskIndex: 2, DependsOn: []string{"compile"}},
			},
		},
	}

	got := MarkdownTree(state)
	if !strings.Contains(got, "**build**: `compile`") || !strings.Contains(got, "  - ⏳ **build**: `test`") {
		t.Fatalf("tree did not render dependency child:\n%s", got)
	}
}

func TestRenderUsesGitHubView(t *testing.T) {
	state := &State{
		GitHubView:         "mermaid",
		PipelineDefinition: "name: ci\nsteps: []\n",
		Steps:              map[string]map[string]TaskStatusUpdate{},
	}

	got := Render(state)
	if !strings.Contains(got, "```mermaid") {
		t.Fatalf("Render() = %q, want Mermaid output", got)
	}
}
