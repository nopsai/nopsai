package checkrender

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
)

type TaskStatusUpdate struct {
	RunID         string    `json:"run_id"`
	RepoOwner     string    `json:"repo_owner"`
	RepoName      string    `json:"repo_name"`
	CheckRunID    int64     `json:"check_run_id"`
	StepName      string    `json:"step_name"`
	TaskName      string    `json:"task_name"`
	TaskStatus    string    `json:"task_status"`
	TaskIndex     int       `json:"task_index"`
	TotalTasks    int       `json:"total_tasks"`
	DependsOn     []string  `json:"depends_on"`
	DisplayOption string    `json:"display_option"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
}

// State stores check-run tasks nested by step.
type State struct {
	RunID              string
	Steps              map[string]map[string]TaskStatusUpdate
	StepOrder          []string
	DisplayOption      string
	PipelineName       string
	PipelineDefinition string
}

func Render(state *State) string {
	if state == nil {
		return ""
	}
	if state.DisplayOption == models.DisplayOptionList {
		return MarkdownList(state)
	}
	return MermaidGraph(state)
}

func MermaidGraph(state *State) string {
	var builder strings.Builder
	builder.WriteString("```mermaid\n")
	builder.WriteString("graph TD\n")

	builder.WriteString("\n    %% --- Style Definitions ---\n")
	builder.WriteString("    classDef success fill:#1a3021,stroke:#3fb950,color:#c9d1d9\n")
	builder.WriteString("    classDef failure fill:#38191c,stroke:#f85149,color:#c9d1d9\n")
	builder.WriteString("    classDef ignored fill:#34291a,stroke:#d29922,color:#c9d1d9\n")
	builder.WriteString("    classDef running fill:#132a3d,stroke:#58a6ff,color:#c9d1d9\n")
	builder.WriteString("    classDef pending fill:#242930,stroke:#6e7681,color:#c9d1d9\n")
	builder.WriteString("    classDef cancelled fill:#242930,stroke:#8b949e,color:#8b949e\n")
	builder.WriteString("    classDef skipped fill:#242930,stroke:#6e7681,color:#c9d1d9\n")
	builder.WriteString("    linkStyle default stroke:#6e7681,stroke-width:1px\n")

	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(state.PipelineDefinition), &pipeline); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal pipeline definition for Mermaid graph")
		return "Error: Could not render dependency graph."
	}

	taskToNodeID := make(map[string]string)
	stepStartNodes := make(map[string]string)
	stepEndNodes := make(map[string]string)

	builder.WriteString("\n    %% --- Node Definitions ---\n")
	nodeCounter := 0
	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		tasksInStep := state.Steps[stepName]

		stepStartNodes[stepName] = fmt.Sprintf("S%d_start", nodeCounter)
		stepEndNodes[stepName] = fmt.Sprintf("S%d_end", nodeCounter)
		builder.WriteString(fmt.Sprintf("    %s(( )); style %s fill:none,stroke:none,width:0,height:0\n", stepStartNodes[stepName], stepStartNodes[stepName]))
		builder.WriteString(fmt.Sprintf("    %s(( )); style %s fill:none,stroke:none,width:0,height:0\n", stepEndNodes[stepName], stepEndNodes[stepName]))

		for taskName, task := range tasksInStep {
			nodeID := fmt.Sprintf("T%d", nodeCounter)
			taskToNodeID[taskName] = nodeID
			nodeCounter++

			statusIcon, styleClass := mermaidTaskStyle(task.TaskStatus)
			duration := ""
			if !task.StartedAt.IsZero() && !task.FinishedAt.IsZero() {
				duration = fmt.Sprintf("<br/>%s", task.FinishedAt.Sub(task.StartedAt).Round(time.Second))
			}

			nodeText := fmt.Sprintf("%s %s:<br/>%s%s", statusIcon, task.StepName, task.TaskName, duration)
			builder.WriteString(fmt.Sprintf("    %s(\"`%s`\"):::%s\n", nodeID, nodeText, styleClass))
		}
	}

	builder.WriteString("\n    %% --- Link Definitions ---\n")
	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		tasksInStep := state.Steps[stepName]
		internalDependencies := make(map[string]bool)

		for taskName, task := range tasksInStep {
			toNode := taskToNodeID[taskName]
			if len(task.DependsOn) > 0 {
				for _, depName := range task.DependsOn {
					fromNode := taskToNodeID[depName]
					builder.WriteString(fmt.Sprintf("    %s --> %s\n", fromNode, toNode))
					internalDependencies[taskName] = true
				}
			}
		}

		for taskName := range tasksInStep {
			if !internalDependencies[taskName] {
				builder.WriteString(fmt.Sprintf("    %s --> %s\n", stepStartNodes[stepName], taskToNodeID[taskName]))
			}
		}

		allTaskDeps := make(map[string]bool)
		for _, task := range tasksInStep {
			for _, dep := range task.DependsOn {
				allTaskDeps[dep] = true
			}
		}
		for taskName := range tasksInStep {
			if !allTaskDeps[taskName] {
				builder.WriteString(fmt.Sprintf("    %s --> %s\n", taskToNodeID[taskName], stepEndNodes[stepName]))
			}
		}

		if len(step.GetDependsOn()) > 0 {
			for _, depStepName := range step.GetDependsOn() {
				builder.WriteString(fmt.Sprintf("    %s --> %s\n", stepEndNodes[depStepName], stepStartNodes[stepName]))
			}
		}
	}

	builder.WriteString("```\n")
	return builder.String()
}

func MarkdownList(state *State) string {
	var builder strings.Builder
	allTasks := []TaskStatusUpdate{}

	for _, stepName := range state.StepOrder {
		tasks := state.Steps[stepName]
		for _, task := range tasks {
			allTasks = append(allTasks, task)
		}
	}

	sort.SliceStable(allTasks, func(i, j int) bool {
		return allTasks[i].TaskIndex < allTasks[j].TaskIndex
	})

	for _, task := range allTasks {
		icon := taskIcon(task.TaskStatus)
		duration := taskDuration(task)
		builder.WriteString(fmt.Sprintf("- %s **%s**: `%s` - %s%s\n", icon, task.StepName, task.TaskName, task.TaskStatus, duration))
	}
	return builder.String()
}

// Work that is under way must not look like work that has not started: an
// operator watching a check run needs to see progress without reading each
// status word.
func taskIcon(status string) string {
	switch {
	case status == "success":
		return "✅"
	case strings.EqualFold(status, "warning") || strings.Contains(strings.ToLower(status), "failure (ignored)"):
		return "⚠️"
	case strings.Contains(strings.ToLower(status), "fail"):
		return "❌"
	case isRunningTaskStatus(status):
		return "🔄"
	case isCancelledTaskStatus(status):
		return "🚫"
	case status == "skipped":
		return "⚪️"
	case status == "not_found":
		return "❓"
	default:
		return "⏳"
	}
}

func isRunningTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "in_progress", "in progress", "started":
		return true
	default:
		return false
	}
}

func isCancelledTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func taskDuration(task TaskStatusUpdate) string {
	if task.StartedAt.IsZero() || task.FinishedAt.IsZero() {
		return ""
	}
	return fmt.Sprintf(" (took %s)", task.FinishedAt.Sub(task.StartedAt).Round(time.Second))
}

func mermaidTaskStyle(status string) (string, string) {
	switch {
	case status == "success":
		return "✅", "success"
	case strings.EqualFold(status, "warning") || strings.Contains(status, "failure (ignored)"):
		return "⚠️", "ignored"
	case strings.Contains(status, "fail"):
		return "❌", "failure"
	case isRunningTaskStatus(status):
		return "🔄", "running"
	case isCancelledTaskStatus(status):
		return "🚫", "cancelled"
	case status == "skipped", status == "not_found":
		return "⚪️", "skipped"
	default:
		return "⏳", "pending"
	}
}
