package models

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Manifest represents the structure of the .nopsai.yaml manifest file.
type Manifest struct {
	Triggers []Trigger `yaml:"triggers" json:"triggers"`
}

// Trigger defines a rule for when a pipeline should be run.
type Trigger struct {
	On          string           `yaml:"on" json:"on"`
	Branches    []string         `yaml:"branches,omitempty" json:"branches,omitempty"`
	Tags        []string         `yaml:"tags,omitempty" json:"tags,omitempty"`
	Pipelines   []PipelineSource `yaml:"pipelines" json:"pipelines"`
	Environment string           `yaml:"environment,omitempty" json:"environment,omitempty"`
}

// PipelineSource defines a single pipeline to be run from a local path or stored definition.
type PipelineSource struct {
	Path string `yaml:"path" json:"path"`
}

// UnmarshalYAML allows pipeline sources to be declared as simple strings (path).
func (p *PipelineSource) UnmarshalYAML(value *yaml.Node) error {
	*p = PipelineSource{}
	if value.Kind == yaml.ScalarNode {
		var path string
		if err := value.Decode(&path); err != nil {
			return err
		}
		if path == "" {
			return fmt.Errorf("pipeline path cannot be empty")
		}
		p.Path = path
		return nil
	}
	return fmt.Errorf("invalid pipeline source definition; only scalar paths are supported")
}

// Pipeline represents the structure of a pipeline definition file.
type Pipeline struct {
	Name              string         `yaml:"name" json:"name"`
	Description       string         `yaml:"description" json:"description"`
	ContainerImage    string         `yaml:"container_image" json:"container_image"`
	DisplayOptions    DisplayOptions `yaml:"display_options" json:"display_options"`
	WorkingDirectory  string         `yaml:"working_directory,omitempty" json:"working_directory,omitempty"`
	Environment       []string       `yaml:"environment" json:"environment"`
	Steps             []PipelineStep `yaml:"steps" json:"steps"`
	Timeout           string         `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	LlmContentSharing *bool          `yaml:"llm_content_sharing,omitempty" json:"llm_content_sharing,omitempty"`
	LlmOutputSharing  *bool          `yaml:"llm_output_sharing,omitempty" json:"llm_output_sharing,omitempty"`
}

// DisplayOptions defines how the pipeline progress is displayed in integrations like GitHub.
type DisplayOptions struct {
	GitHubView string `yaml:"github_view,omitempty" json:"github_view,omitempty"` // "mermaid", "tree" or "flat"
}

// Task is an individual command or goal within a PipelineStep.
type Task struct {
	Name             string   `yaml:"name" json:"name"`
	Goal             string   `yaml:"goal" json:"goal"`
	Script           string   `yaml:"script,omitempty" json:"script,omitempty"`
	DependsOn        []string `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	IgnoreFailure    bool     `yaml:"ignore_failure,omitempty" json:"ignore_failure,omitempty"`
	LlmOutputSharing *bool    `yaml:"llm_output_sharing,omitempty" json:"llm_output_sharing,omitempty"`
}

// PipelineStep is a single logical step within a pipeline definition.
type PipelineStep struct {
	Name        string            `yaml:"name" json:"name"`
	Include     string            `yaml:"include,omitempty" json:"include,omitempty"`
	Sync        bool              `yaml:"sync,omitempty" json:"sync,omitempty"`
	Image       string            `yaml:"image,omitempty" json:"image,omitempty"`
	Secrets     []string          `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty" json:"environment,omitempty"`
	Tasks       []Task            `yaml:"tasks" json:"tasks"`

	// Legacy fields
	Goal             string   `yaml:"goal,omitempty" json:"goal,omitempty"`
	Script           string   `yaml:"script,omitempty" json:"script,omitempty"`
	DependsOn        []string `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	IgnoreFailure    bool     `yaml:"ignore_failure,omitempty" json:"ignore_failure,omitempty"`
	LlmOutputSharing *bool    `yaml:"llm_output_sharing,omitempty" json:"llm_output_sharing,omitempty"`
}

// CommandAction defines a command to be executed in the shell.
type CommandAction struct {
	Command string `json:"command"`
}

// FileAction defines a file to be created or replaced.
type FileAction struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// AnswerAction defines a simple text response from the LLM.
type AnswerAction struct {
	Answer string `json:"answer"`
}

// Action is the structured command returned by the LLM Agent to the Agent.
type Action struct {
	Type          string         `json:"type"`
	CommandAction *CommandAction `json:"command_action,omitempty"`
	FileAction    *FileAction    `json:"file_action,omitempty"`
	AnswerAction  *AnswerAction  `json:"answer_action,omitempty"`
}

const (
	ActionTypeExecuteCommand string = "EXECUTE_COMMAND"
	ActionTypeReplaceFile    string = "REPLACE_FILE"
	ActionTypeReturnAnswer   string = "RETURN_ANSWER"
)

// ActionResult is sent from the Agent back to the LLM Agent after an action is performed.
type ActionResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// Gemini API specific structures
type GeminiRequest struct {
	Contents []Content `json:"contents"`
}
type Content struct {
	Parts []Part `json:"parts"`
}
type Part struct {
	Text string `json:"text"`
}
type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []Part `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}
