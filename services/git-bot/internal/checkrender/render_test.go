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

// Running work must be distinguishable from work that has not started, in both
// renderings, without reading the status word.
func TestTaskStatusIconsSeparateRunningFromPending(t *testing.T) {
	state := &State{
		StepOrder: []string{"release"},
		Steps: map[string]map[string]TaskStatusUpdate{
			"release": {
				"quality-gates":     {StepName: "release", TaskName: "quality-gates", TaskStatus: "running", TaskIndex: 1},
				"publish-images":    {StepName: "release", TaskName: "publish-images", TaskStatus: "pending", TaskIndex: 2},
				"publish-artifacts": {StepName: "release", TaskName: "publish-artifacts", TaskStatus: "cancelled", TaskIndex: 3},
			},
		},
	}

	got := MarkdownList(state)
	for _, want := range []string{
		"🔄 **release**: `quality-gates` - running",
		"⏳ **release**: `publish-images` - pending",
		"🚫 **release**: `publish-artifacts` - cancelled",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("list missing %q:\n%s", want, got)
		}
	}

	if icon, style := mermaidTaskStyle("running"); icon != "🔄" || style != "running" {
		t.Fatalf("mermaidTaskStyle(running) = %q/%q", icon, style)
	}
	if icon, style := mermaidTaskStyle("pending"); icon != "⏳" || style != "pending" {
		t.Fatalf("mermaidTaskStyle(pending) = %q/%q", icon, style)
	}
	if icon, style := mermaidTaskStyle("cancelled"); icon != "🚫" || style != "cancelled" {
		t.Fatalf("mermaidTaskStyle(cancelled) = %q/%q", icon, style)
	}
}

func TestMermaidGraphDefinesEveryStyleClassItUses(t *testing.T) {
	graph := MermaidGraph(&State{
		PipelineDefinition: "name: release\nsteps:\n  - name: release\n    tasks:\n      - name: gate\n        script: make gate\n",
		StepOrder:          []string{"release"},
		Steps: map[string]map[string]TaskStatusUpdate{
			"release": {"gate": {StepName: "release", TaskName: "gate", TaskStatus: "running", TaskIndex: 1}},
		},
	})
	for _, class := range []string{"running", "pending", "cancelled", "success", "failure", "ignored", "skipped"} {
		if !strings.Contains(graph, "classDef "+class+" ") {
			t.Fatalf("graph does not define the %q style class:\n%s", class, graph)
		}
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
